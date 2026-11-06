package io.flowcatalyst.field

import android.os.Build
import androidx.biometric.BiometricManager
import androidx.biometric.BiometricManager.Authenticators.BIOMETRIC_STRONG
import androidx.biometric.BiometricManager.Authenticators.BIOMETRIC_WEAK
import androidx.biometric.BiometricManager.Authenticators.DEVICE_CREDENTIAL
import androidx.biometric.BiometricPrompt
import androidx.core.content.ContextCompat
import androidx.fragment.app.FragmentActivity
import kotlinx.coroutines.CompletableDeferred

/** App unlock: BiometricPrompt (BIOMETRIC_STRONG or DEVICE_CREDENTIAL), no CryptoObject. Main-thread work is posted. */
internal class BiometricUnlocker(private val activity: FragmentActivity) : UnlockPrompter {
    override suspend fun prompt(text: String) {
        // BIOMETRIC_STRONG | DEVICE_CREDENTIAL is unsupported by the library on API 28-29.
        val authenticators = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
            BIOMETRIC_STRONG or DEVICE_CREDENTIAL
        } else {
            BIOMETRIC_WEAK or DEVICE_CREDENTIAL
        }
        when (BiometricManager.from(activity).canAuthenticate(authenticators)) {
            BiometricManager.BIOMETRIC_SUCCESS -> {}
            else -> throw FieldAuthException("BIOMETRIC_UNAVAILABLE", "no biometric or screen lock is available")
        }
        val done = CompletableDeferred<Unit>()
        activity.runOnUiThread {
            try {
                val callback = object : BiometricPrompt.AuthenticationCallback() {
                    override fun onAuthenticationSucceeded(result: BiometricPrompt.AuthenticationResult) {
                        done.complete(Unit)
                    }

                    override fun onAuthenticationError(errorCode: Int, errString: CharSequence) {
                        done.completeExceptionally(mapError(errorCode))
                    }
                }
                val info = BiometricPrompt.PromptInfo.Builder()
                    .setTitle(text)
                    .setAllowedAuthenticators(authenticators)
                    .build()
                BiometricPrompt(activity, ContextCompat.getMainExecutor(activity), callback).authenticate(info)
            } catch (e: Exception) {
                done.completeExceptionally(FieldAuthException("INTERNAL", "could not show the unlock prompt"))
            }
        }
        done.await()
    }

    private fun mapError(code: Int): FieldAuthException = when (code) {
        BiometricPrompt.ERROR_USER_CANCELED, BiometricPrompt.ERROR_NEGATIVE_BUTTON, BiometricPrompt.ERROR_CANCELED ->
            FieldAuthException("BIOMETRIC_CANCELLED", "unlock cancelled")
        BiometricPrompt.ERROR_NO_DEVICE_CREDENTIAL ->
            FieldAuthException("NO_SECURE_LOCK_SCREEN", "device has no secure lock screen")
        BiometricPrompt.ERROR_NO_BIOMETRICS, BiometricPrompt.ERROR_HW_NOT_PRESENT, BiometricPrompt.ERROR_HW_UNAVAILABLE,
        BiometricPrompt.ERROR_LOCKOUT, BiometricPrompt.ERROR_LOCKOUT_PERMANENT ->
            FieldAuthException("BIOMETRIC_UNAVAILABLE", "biometric unavailable")
        else -> FieldAuthException("INTERNAL", "unlock error $code")
    }
}
