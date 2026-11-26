package io.flowcatalyst.field

import androidx.biometric.BiometricManager.Authenticators.BIOMETRIC_STRONG
import androidx.biometric.BiometricPrompt
import androidx.core.content.ContextCompat
import androidx.fragment.app.FragmentActivity
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CompletableDeferred

/**
 * The unlock key's prompt: `BiometricPrompt` with a `CryptoObject` over the readied signature, strong biometrics only
 * (no device credential: the key accepts only a biometric), so the signature can be made only after the match. The
 * prompt's button is the prompt's `cancel` text ("Use PIN"). Main-thread work is posted.
 */
internal class BiometricUnlockKeyPrompter(private val activity: FragmentActivity) : UnlockKeyPrompter {
    override suspend fun authorise(prompt: UnlockKeyPrompt, signature: PreparedSignature, data: ByteArray): ByteArray {
        val sig = (signature as? KeystoreSignature)?.signature
            ?: throw FieldAuthException("INTERNAL", "not a Keystore signature")
        val done = CompletableDeferred<ByteArray>()
        var shown: BiometricPrompt? = null
        activity.runOnUiThread {
            try {
                val callback = object : BiometricPrompt.AuthenticationCallback() {
                    override fun onAuthenticationSucceeded(result: BiometricPrompt.AuthenticationResult) {
                        val authorised = result.cryptoObject?.signature
                        if (authorised == null) {
                            done.completeExceptionally(FieldAuthException("INTERNAL", "the prompt returned no signature"))
                            return
                        }
                        try {
                            done.complete(KeystoreSignature(authorised).sign(data))
                        } catch (e: FcFieldException) {
                            done.completeExceptionally(FieldAuthException(e.code, "unlock signing failed: ${e.code}"))
                        }
                    }

                    override fun onAuthenticationError(errorCode: Int, errString: CharSequence) {
                        done.completeExceptionally(BiometricErrors.forUnlockKey(errorCode))
                    }
                    // onAuthenticationFailed (a finger not recognised): the prompt stays up and counts it itself.
                }
                val b = BiometricPrompt.PromptInfo.Builder()
                    .setTitle(prompt.title)
                    .setNegativeButtonText(prompt.cancel)
                    .setAllowedAuthenticators(BIOMETRIC_STRONG)
                    .setConfirmationRequired(false)
                prompt.subtitle?.let { b.setSubtitle(it) }
                val p = BiometricPrompt(activity, ContextCompat.getMainExecutor(activity), callback)
                shown = p
                p.authenticate(b.build(), BiometricPrompt.CryptoObject(sig))
            } catch (e: Exception) {
                done.completeExceptionally(FieldAuthException("INTERNAL", "could not show the biometric prompt"))
            }
        }
        try {
            return done.await()
        } catch (e: CancellationException) {
            activity.runOnUiThread { runCatching { shown?.cancelAuthentication() } }
            throw e
        }
    }
}

/** BiometricPrompt error codes to the library's codes (plain integers, so tested on the JVM). */
internal object BiometricErrors {
    fun forUnlockKey(code: Int): FieldAuthException = when (code) {
        BiometricPrompt.ERROR_USER_CANCELED, BiometricPrompt.ERROR_NEGATIVE_BUTTON, BiometricPrompt.ERROR_CANCELED,
        BiometricPrompt.ERROR_TIMEOUT ->
            FieldAuthException("BIOMETRIC_CANCELLED", "the biometric prompt was cancelled")
        BiometricPrompt.ERROR_LOCKOUT, BiometricPrompt.ERROR_LOCKOUT_PERMANENT ->
            FieldAuthException(UnlockKeyCodes.BIOMETRIC_LOCKOUT, "too many failed biometric attempts")
        BiometricPrompt.ERROR_NO_BIOMETRICS ->
            FieldAuthException(UnlockKeyCodes.BIOMETRIC_NOT_ENROLLED, "no biometric is enrolled")
        BiometricPrompt.ERROR_HW_NOT_PRESENT, BiometricPrompt.ERROR_HW_UNAVAILABLE, BiometricPrompt.ERROR_SECURITY_UPDATE_REQUIRED ->
            FieldAuthException("BIOMETRIC_UNAVAILABLE", "biometric unavailable")
        BiometricPrompt.ERROR_NO_DEVICE_CREDENTIAL ->
            FieldAuthException("NO_SECURE_LOCK_SCREEN", "device has no secure lock screen")
        else -> FieldAuthException("INTERNAL", "biometric error $code")
    }
}
