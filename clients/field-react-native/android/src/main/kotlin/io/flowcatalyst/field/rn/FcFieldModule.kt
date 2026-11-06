package io.flowcatalyst.field.rn

import io.flowcatalyst.field.FieldAuthException
import io.flowcatalyst.field.FcFieldException
import io.flowcatalyst.field.AndroidKeyProvider
import io.flowcatalyst.field.FieldSession
import io.flowcatalyst.field.FieldEvent
import androidx.fragment.app.FragmentActivity
import com.facebook.fbreact.specs.NativeFcFieldSpec
import com.facebook.react.bridge.Arguments
import com.facebook.react.bridge.Promise
import com.facebook.react.bridge.ReactApplicationContext
import com.facebook.react.bridge.ReadableMap
import com.facebook.react.bridge.WritableMap
import com.facebook.react.modules.core.DeviceEventManagerModule
import java.io.Closeable
import java.util.Base64
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.launch

/**
 * JS name "FcField" (New Architecture TurboModule; the spec is src/NativeFcField.ts).
 *
 * The session methods are a thin delegation to the process-wide [FieldSession]. The key/store methods serve the
 * WebView bridge (protocol v1): key operations use the same [AndroidKeyProvider] key as the session, so there is one
 * device key per app. Rejections carry `code`; nothing here logs values, PINs, tokens, data or signatures.
 */
class FcFieldModule(private val reactContext: ReactApplicationContext) : NativeFcFieldSpec(reactContext) {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val keys = AndroidKeyProvider()
    private val store by lazy { SecureStoreManager(reactContext.applicationContext) }

    @Volatile private var session: FieldSession? = null
    private var subscription: Closeable? = null

    /** Rejects with a stable code; cancellation is not an error. */
    private fun launch(promise: Promise, block: suspend () -> Any?) {
        scope.launch {
            try {
                promise.resolve(block())
            } catch (t: Throwable) {
                if (NativeErrors.isCancellation(t)) throw t
                reject(promise, t)
            }
        }
    }

    private fun reject(promise: Promise, t: Throwable) {
        val r = NativeErrors.classify(t)
        val info: WritableMap = Arguments.createMap()
        r.status?.let { info.putInt("status", it) }
        r.retryAfterSeconds?.let { info.putInt("retryAfterSeconds", it) }
        if (r.violations.isNotEmpty()) info.putArray("violations", SessionJson.array(r.violations))
        promise.reject(r.code, r.message, info)
    }

    private fun owner(): FieldSession =
        session ?: throw FieldAuthException("NOT_CONFIGURED", "configure() must be called first")

    private fun emit(e: FieldEvent) {
        if (!reactContext.hasActiveReactInstance()) return
        val (name, payload) = SessionJson.event(e)
        reactContext.getJSModule(DeviceEventManagerModule.RCTDeviceEventEmitter::class.java).emit(name, payload)
    }

    // ── session owner ────────────────────────────────────────────────────

    override fun configure(config: ReadableMap, promise: Promise) = launch(promise) {
        val s = FieldSession.get(reactContext.applicationContext, SessionJson.config(config))
        synchronized(this@FcFieldModule) {
            if (session !== s || subscription == null) {
                subscription?.close()
                subscription = s.addListener { emit(it) }
            }
            session = s
        }
        null
    }

    override fun restore(promise: Promise) = launch(promise) {
        val s = owner()
        SessionJson.state(s.restore(), null)
    }

    override fun getState(promise: Promise) = launch(promise) {
        SessionJson.state(owner().state.value, null)
    }

    override fun enrol(qrOrCode: String, info: ReadableMap, promise: Promise) = launch(promise) {
        SessionJson.device(owner().enrol(qrOrCode, SessionJson.deviceInfo(info)))
    }

    override fun login(creds: ReadableMap, promise: Promise) = launch(promise) {
        SessionJson.session(owner().login(SessionJson.credentials(creds)))
    }

    override fun redeemSignInCode(code: String, promise: Promise) = launch(promise) {
        SessionJson.session(owner().redeemSignInCode(code))
    }

    override fun actAs(target: ReadableMap, promise: Promise) = launch(promise) {
        SessionJson.session(owner().actAs(SessionJson.actAsTarget(target)))
    }

    override fun endImpersonation(promise: Promise) = launch(promise) {
        val s = owner().endImpersonation()
        Arguments.createMap().also {
            val m = SessionJson.sessionOrNull(s)
            if (m == null) it.putNull("session") else it.putMap("session", m)
        }
    }

    override fun changePin(currentPin: String, newPin: String, promise: Promise) = launch(promise) {
        owner().changePin(currentPin, newPin)
        null
    }

    override fun logout(scope: String, promise: Promise) = launch(promise) {
        val sc = SessionJson.logoutScope(scope)
        owner().logout(sc)
        null
    }

    override fun unlock(prompt: String, promise: Promise) = launch(promise) {
        val activity = reactContext.currentActivity as? FragmentActivity
            ?: throw FieldAuthException("BIOMETRIC_UNAVAILABLE", "no foreground activity for the unlock prompt")
        SessionJson.session(owner().unlock(activity, prompt))
    }

    override fun refresh(promise: Promise) = launch(promise) {
        SessionJson.session(owner().refresh())
    }

    override fun headersFor(method: String, url: String, promise: Promise) = launch(promise) {
        SessionJson.headers(owner().headersFor(method, url))
    }

    override fun headersForRetry(method: String, url: String, nonce: String, promise: Promise) = launch(promise) {
        SessionJson.headers(owner().headersForRetry(method, url, nonce))
    }

    override fun noteNonce(url: String, nonce: String) {
        session?.noteNonce(url, nonce)
    }

    override fun keepAlive(on: Boolean) {
        session?.keepAlive(on)
    }

    // NativeEventEmitter plumbing: events are always emitted once configured; nothing to count.
    override fun addListener(eventName: String) {}

    override fun removeListeners(count: Double) {}

    override fun invalidate() {
        synchronized(this) {
            subscription?.close()
            subscription = null
        }
        scope.cancel()
        super.invalidate()
    }

    // ── key / store (WebView bridge protocol v1, one device key per app) ─

    override fun getOrCreateDeviceKey(promise: Promise) = launch(promise) {
        val k = keys.getOrCreate()
        Arguments.createMap().apply {
            putMap("jwk", Arguments.createMap().apply {
                putString("kty", "EC")
                putString("crv", "P-256")
                putString("x", k.x)
                putString("y", k.y)
            })
            putMap("attestation", Arguments.createMap().apply {
                putString("level", k.level)
                k.attestationChain?.let { putArray("chain", SessionJson.array(it)) }
            })
        }
    }

    override fun sign(dataB64u: String, promise: Promise) = launch(promise) {
        val data = try {
            B64U_DEC.decode(dataB64u)
        } catch (e: IllegalArgumentException) {
            throw FcFieldException("INTERNAL", "bad base64url")
        }
        B64U_ENC.encodeToString(keys.sign(data))
    }

    override fun deleteDeviceKey(promise: Promise) = launch(promise) {
        keys.delete()
        null
    }

    override fun storeGet(key: String, prompt: String?, promise: Promise) = launch(promise) {
        val bioEntry = store.biometricEntryExists(key)
        if (!bioEntry) return@launch store.getPlain(key)
        val (cipher, env) = store.biometricGetCipher(key) ?: return@launch null
        val authed = store.authenticate(activity(), cipher, prompt)
        store.biometricFinishGet(key, authed, env)
    }

    override fun storeSet(key: String, value: String, requireBiometric: Boolean, promise: Promise) = launch(promise) {
        if (!requireBiometric) {
            store.setPlain(key, value)
        } else {
            store.biometricSet(key, value) // never prompts; only get of a biometric entry does
        }
        null
    }

    override fun storeDelete(key: String, promise: Promise) = launch(promise) {
        store.delete(key)
        null
    }

    override fun biometricAvailable(promise: Promise) = launch(promise) { store.biometricAvailable() }

    private fun activity(): FragmentActivity =
        (reactContext.currentActivity as? FragmentActivity)
            ?: throw FcFieldException("BIOMETRIC_UNAVAILABLE", "no foreground activity for the biometric prompt")

    private companion object {
        val B64U_ENC: Base64.Encoder = Base64.getUrlEncoder().withoutPadding()
        val B64U_DEC: Base64.Decoder = Base64.getUrlDecoder()
    }
}
