package io.flowcatalyst.field

import androidx.fragment.app.FragmentActivity
import com.facebook.react.bridge.Arguments
import com.facebook.react.bridge.Promise
import com.facebook.react.bridge.ReactApplicationContext
import com.facebook.react.bridge.ReactContextBaseJavaModule
import com.facebook.react.bridge.ReactMethod
import com.facebook.react.bridge.WritableArray
import com.facebook.react.bridge.WritableMap
import java.util.concurrent.Executors

/**
 * JS name "FcField". Rejections carry `code` = FieldErrorCode. Nothing here logs values, data or signatures.
 */
class FcFieldModule(private val reactContext: ReactApplicationContext) : ReactContextBaseJavaModule(reactContext) {
    private val keys = DeviceKeyManager()
    private val store by lazy { SecureStoreManager(reactContext.applicationContext) }
    private val worker = Executors.newSingleThreadExecutor()

    override fun getName(): String = "FcField"

    private fun run(promise: Promise, block: () -> Any?) {
        worker.execute {
            try {
                promise.resolve(toJs(block()))
            } catch (e: FcFieldException) {
                promise.reject(e.code, e.message ?: e.code)
            } catch (e: Throwable) {
                // Lost keys / OEM Keystore faults must never crash the app.
                promise.reject("INTERNAL", "unexpected native error")
            }
        }
    }

    @ReactMethod
    fun getOrCreateDeviceKey(promise: Promise) = run(promise) { keys.getOrCreate() }

    @ReactMethod
    fun sign(dataB64u: String, promise: Promise) = run(promise) {
        val data = try { DerSignature.fromBase64Url(dataB64u) } catch (e: IllegalArgumentException) {
            throw FcFieldException("INTERNAL", "bad base64url")
        }
        DerSignature.toBase64Url(keys.sign(data))
    }

    @ReactMethod
    fun deleteDeviceKey(promise: Promise) = run(promise) { keys.delete(); null }

    @ReactMethod
    fun storeGet(key: String, prompt: String?, promise: Promise) = run(promise) {
        val bioEntry = store.biometricEntryExists(key)
        if (!bioEntry) return@run store.getPlain(key)
        val (cipher, env) = store.biometricGetCipher(key) ?: return@run null
        val authed = store.authenticate(activity(), cipher, prompt)
        store.biometricFinishGet(key, authed, env)
    }

    @ReactMethod
    fun storeSet(key: String, value: String, requireBiometric: Boolean, promise: Promise) = run(promise) {
        if (!requireBiometric) {
            store.setPlain(key, value)
        } else {
            store.biometricSet(key, value) // never prompts; only get of a biometric entry does
        }
        null
    }

    @ReactMethod
    fun storeDelete(key: String, promise: Promise) = run(promise) { store.delete(key); null }

    @ReactMethod
    fun biometricAvailable(promise: Promise) = run(promise) { store.biometricAvailable() }

    private fun activity(): FragmentActivity =
        (reactContext.currentActivity as? FragmentActivity)
            ?: throw FcFieldException("BIOMETRIC_UNAVAILABLE", "no foreground activity for the biometric prompt")

    private fun toJs(v: Any?): Any? = when (v) {
        null -> null
        is Map<*, *> -> Arguments.createMap().also { m -> v.forEach { (k, x) -> put(m, k as String, x) } }
        else -> v
    }

    private fun put(m: WritableMap, k: String, x: Any?) {
        when (x) {
            null -> m.putNull(k)
            is String -> m.putString(k, x)
            is Boolean -> m.putBoolean(k, x)
            is Int -> m.putInt(k, x)
            is Map<*, *> -> m.putMap(k, toJs(x) as WritableMap)
            is List<*> -> m.putArray(k, list(x))
            else -> m.putString(k, x.toString())
        }
    }

    private fun list(l: List<*>): WritableArray = Arguments.createArray().also { a ->
        l.forEach { e ->
            when (e) {
                is String -> a.pushString(e)
                else -> a.pushString(e.toString())
            }
        }
    }
}
