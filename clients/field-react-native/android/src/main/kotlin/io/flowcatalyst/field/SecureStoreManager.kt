package io.flowcatalyst.field

import android.app.KeyguardManager
import android.content.Context
import android.os.Build
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyPermanentlyInvalidatedException
import android.security.keystore.KeyProperties
import androidx.biometric.BiometricManager
import androidx.biometric.BiometricPrompt
import androidx.core.content.ContextCompat
import androidx.fragment.app.FragmentActivity
import java.security.KeyPairGenerator
import java.security.KeyStore
import java.security.UnrecoverableKeyException
import java.util.Base64
import java.util.concurrent.CompletableFuture
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

/**
 * AES-256-GCM secure store. Ciphertext + IV live in SharedPreferences (base64); keys live in the Keystore.
 * Plain entries: "v1:n:<iv b64>:<ciphertext b64>"; biometric entries use HybridEnvelope ("v2:b:..."). Never logs keys, values or plaintext.
 */
internal class SecureStoreManager(private val context: Context) {
    private val prefs = context.getSharedPreferences(PREFS, Context.MODE_PRIVATE)

    private fun keyStore(): KeyStore = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }

    // ---- availability -------------------------------------------------------------------------

    fun biometricAvailable(): Boolean = try {
        deviceSecure() && BiometricManager.from(context).canAuthenticate(BiometricManager.Authenticators.BIOMETRIC_STRONG) ==
            BiometricManager.BIOMETRIC_SUCCESS
    } catch (e: Exception) {
        false
    }

    private fun deviceSecure(): Boolean =
        (context.getSystemService(Context.KEYGUARD_SERVICE) as? KeyguardManager)?.isDeviceSecure == true

    // ---- plain (no user auth) -----------------------------------------------------------------

    @Synchronized
    fun getPlain(name: String): String? {
        val entry = parse(prefs.getString(name, null)) ?: return null
        if (entry.bio) throw FcFieldException("INTERNAL", "entry requires biometric")
        try {
            val cipher = Cipher.getInstance(TRANSFORM)
            cipher.init(Cipher.DECRYPT_MODE, key(KEY_PLAIN), GCMParameterSpec(128, entry.iv))
            return String(cipher.doFinal(entry.ct), Charsets.UTF_8)
        } catch (e: Exception) {
            return lost(name, KEY_PLAIN, e)
        }
    }

    @Synchronized
    fun setPlain(name: String, value: String) {
        try {
            val cipher = Cipher.getInstance(TRANSFORM)
            try {
                cipher.init(Cipher.ENCRYPT_MODE, key(KEY_PLAIN))
            } catch (e: Exception) {
                runCatching { keyStore().deleteEntry(KEY_PLAIN) }
                cipher.init(Cipher.ENCRYPT_MODE, key(KEY_PLAIN))
            }
            val ct = cipher.doFinal(value.toByteArray(Charsets.UTF_8))
            prefs.edit().putString(name, format(false, cipher.iv, ct)).apply()
        } catch (e: FcFieldException) {
            throw e
        } catch (e: Exception) {
            throw FcFieldException("INTERNAL", "could not store value", e)
        }
    }

    fun delete(name: String) {
        prefs.edit().remove(name).apply()
    }

    // ---- biometric ----------------------------------------------------------------------------
    // Hybrid scheme: the RSA key pair `fc_field_store_bio` is biometric-gated for DECRYPT only. `set` wraps a fresh
    // AES data key with the PUBLIC key (no auth, never prompts); only `get` prompts, to unwrap the data key.

    /** Seal a biometric entry. Never prompts. */
    @Synchronized
    fun biometricSet(name: String, value: String) {
        requireBiometricReady()
        try {
            val sealed = HybridEnvelope.seal(value, bioPublicKey())
            prefs.edit().putString(name, sealed).apply()
        } catch (e: FcFieldException) {
            throw e
        } catch (e: Exception) {
            throw FcFieldException("INTERNAL", "could not store value", e)
        }
    }

    /** Public half of the biometric key pair, regenerating the pair if enrolment changed or it is unusable. */
    private fun bioPublicKey(): java.security.PublicKey {
        val ks = keyStore()
        if (ks.containsAlias(KEY_BIO)) {
            try {
                // Init (without using) the private key: throws if it was invalidated by an enrolment change.
                val probe = Cipher.getInstance(HybridEnvelope.RSA_TRANSFORM)
                probe.init(Cipher.DECRYPT_MODE, ks.getKey(KEY_BIO, null) as java.security.PrivateKey, HybridEnvelope.oaep)
                return ks.getCertificate(KEY_BIO).publicKey
            } catch (e: Exception) {
                // Entries under the old key are unrecoverable; the stale ones report KEY_INVALIDATED on get.
                runCatching { ks.deleteEntry(KEY_BIO) }
            }
        }
        generateBioKeyPair()
        return keyStore().getCertificate(KEY_BIO).publicKey
    }

    private fun generateBioKeyPair() {
        val b = KeyGenParameterSpec.Builder(KEY_BIO, KeyProperties.PURPOSE_DECRYPT)
            .setKeySize(2048)
            .setDigests(KeyProperties.DIGEST_SHA256, KeyProperties.DIGEST_SHA1)
            .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_RSA_OAEP)
            .setUserAuthenticationRequired(true)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
            b.setUserAuthenticationParameters(0, KeyProperties.AUTH_BIOMETRIC_STRONG)
        } else {
            @Suppress("DEPRECATION")
            b.setUserAuthenticationValidityDurationSeconds(-1)
        }
        b.setInvalidatedByBiometricEnrollment(true)
        val kpg = KeyPairGenerator.getInstance(KeyProperties.KEY_ALGORITHM_RSA, "AndroidKeyStore")
        kpg.initialize(b.build())
        kpg.generateKeyPair()
    }

    fun biometricEntryExists(name: String): Boolean = prefs.getString(name, null)?.startsWith("v2:b:") == true

    /** Prepare the unwrap cipher for a biometric entry; null when the entry does not exist. */
    @Synchronized
    fun biometricGetCipher(name: String): Pair<Cipher, HybridEnvelope.Parsed>? {
        requireBiometricReady()
        val env = HybridEnvelope.parse(prefs.getString(name, null)) ?: return null
        try {
            val ks = keyStore()
            val priv = ks.getKey(KEY_BIO, null) as? java.security.PrivateKey
                ?: throw UnrecoverableKeyException("no biometric key")
            val cipher = Cipher.getInstance(HybridEnvelope.RSA_TRANSFORM)
            cipher.init(Cipher.DECRYPT_MODE, priv, HybridEnvelope.oaep)
            return cipher to env
        } catch (e: Exception) {
            lost<Unit>(name, KEY_BIO, e)
            throw IllegalStateException() // unreachable: lost() always throws
        }
    }

    /** [authed] is the cipher returned by the BiometricPrompt CryptoObject. */
    fun biometricFinishGet(name: String, authed: Cipher, env: HybridEnvelope.Parsed): String =
        try {
            HybridEnvelope.open(env) { authed.doFinal(it) }
        } catch (e: KeyPermanentlyInvalidatedException) {
            lost(name, KEY_BIO, e)
        } catch (e: javax.crypto.BadPaddingException) {
            // Entry was sealed under an earlier key pair (regenerated after an enrolment change): drop just the entry.
            prefs.edit().remove(name).apply()
            throw FcFieldException("KEY_INVALIDATED", "stored value is no longer recoverable", e)
        } catch (e: javax.crypto.IllegalBlockSizeException) {
            prefs.edit().remove(name).apply()
            throw FcFieldException("KEY_INVALIDATED", "stored value is no longer recoverable", e)
        } catch (e: Exception) {
            throw FcFieldException("INTERNAL", "could not decrypt value", e)
        }

    private fun requireBiometricReady() {
        if (!deviceSecure()) throw FcFieldException("NO_SECURE_LOCK_SCREEN", "device has no secure lock screen")
        when (BiometricManager.from(context).canAuthenticate(BiometricManager.Authenticators.BIOMETRIC_STRONG)) {
            BiometricManager.BIOMETRIC_SUCCESS -> {}
            else -> throw FcFieldException("BIOMETRIC_UNAVAILABLE", "no strong biometric enrolled or available")
        }
    }

    /** Show the prompt for [cipher]; blocks the calling (worker) thread until the user finishes. */
    fun authenticate(activity: FragmentActivity, cipher: Cipher, prompt: String?): Cipher {
        val future = CompletableFuture<Cipher>()
        val executor = ContextCompat.getMainExecutor(activity)
        activity.runOnUiThread {
            try {
                val callback = object : BiometricPrompt.AuthenticationCallback() {
                    override fun onAuthenticationSucceeded(result: BiometricPrompt.AuthenticationResult) {
                        val c = result.cryptoObject?.cipher
                        if (c != null) future.complete(c)
                        else future.completeExceptionally(FcFieldException("INTERNAL", "no crypto object"))
                    }

                    override fun onAuthenticationError(errorCode: Int, errString: CharSequence) {
                        future.completeExceptionally(mapPromptError(errorCode))
                    }
                }
                val info = BiometricPrompt.PromptInfo.Builder()
                    .setTitle(prompt ?: "Unlock")
                    .setNegativeButtonText("Cancel")
                    .setAllowedAuthenticators(BiometricManager.Authenticators.BIOMETRIC_STRONG)
                    .build()
                BiometricPrompt(activity, executor, callback).authenticate(info, BiometricPrompt.CryptoObject(cipher))
            } catch (e: Exception) {
                future.completeExceptionally(FcFieldException("INTERNAL", "could not show biometric prompt", e))
            }
        }
        try {
            return future.get()
        } catch (e: java.util.concurrent.ExecutionException) {
            val c = e.cause
            throw (c as? FcFieldException) ?: FcFieldException("INTERNAL", "biometric prompt failed", c)
        }
    }

    private fun mapPromptError(code: Int): FcFieldException = when (code) {
        BiometricPrompt.ERROR_USER_CANCELED, BiometricPrompt.ERROR_NEGATIVE_BUTTON, BiometricPrompt.ERROR_CANCELED ->
            FcFieldException("BIOMETRIC_CANCELLED", "biometric prompt cancelled")
        BiometricPrompt.ERROR_NO_DEVICE_CREDENTIAL -> FcFieldException("NO_SECURE_LOCK_SCREEN", "device has no secure lock screen")
        BiometricPrompt.ERROR_NO_BIOMETRICS, BiometricPrompt.ERROR_HW_NOT_PRESENT, BiometricPrompt.ERROR_HW_UNAVAILABLE,
        BiometricPrompt.ERROR_LOCKOUT, BiometricPrompt.ERROR_LOCKOUT_PERMANENT ->
            FcFieldException("BIOMETRIC_UNAVAILABLE", "biometric unavailable")
        else -> FcFieldException("INTERNAL", "biometric error $code")
    }

    // ---- keys & entries -----------------------------------------------------------------------

    private fun key(alias: String): SecretKey {
        val ks = keyStore()
        (ks.getKey(alias, null) as? SecretKey)?.let { return it }
        val b = KeyGenParameterSpec.Builder(alias, KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
            .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
            .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
            .setKeySize(256)
        val kg = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore")
        kg.init(b.build())
        return kg.generateKey()
    }

    /** A key/ciphertext that can no longer be used: drop the entry and the key. Biometric path -> KEY_INVALIDATED. */
    private fun <T> lost(name: String, keyAlias: String, e: Exception): T {
        val invalidated = e is KeyPermanentlyInvalidatedException || e is UnrecoverableKeyException ||
            e is java.security.GeneralSecurityException
        prefs.edit().remove(name).apply()
        runCatching { keyStore().deleteEntry(keyAlias) }
        if (invalidated) throw FcFieldException("KEY_INVALIDATED", "stored value is no longer recoverable", e)
        throw FcFieldException("INTERNAL", "could not read value", e)
    }

    private class Entry(val bio: Boolean, val iv: ByteArray, val ct: ByteArray)

    private fun format(bio: Boolean, iv: ByteArray, ct: ByteArray): String {
        val e = Base64.getEncoder()
        return "v1:${if (bio) "b" else "n"}:${e.encodeToString(iv)}:${e.encodeToString(ct)}"
    }

    private fun parse(s: String?): Entry? {
        if (s == null) return null
        val p = s.split(":")
        if (p.size != 4 || p[0] != "v1") return null
        return try {
            val d = Base64.getDecoder()
            Entry(p[1] == "b", d.decode(p[2]), d.decode(p[3]))
        } catch (e: IllegalArgumentException) {
            null
        }
    }

    companion object {
        const val PREFS = "fc_field_secure_store"
        const val KEY_PLAIN = "fc_field_store"
        const val KEY_BIO = "fc_field_store_bio"
        private const val TRANSFORM = "AES/GCM/NoPadding"
    }
}
