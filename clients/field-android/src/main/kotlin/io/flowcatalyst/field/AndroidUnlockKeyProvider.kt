package io.flowcatalyst.field

import android.content.Context
import android.os.Build
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyInfo
import android.security.keystore.KeyPermanentlyInvalidatedException
import android.security.keystore.KeyProperties
import android.security.keystore.StrongBoxUnavailableException
import androidx.annotation.RequiresApi
import androidx.biometric.BiometricManager
import androidx.biometric.BiometricManager.Authenticators.BIOMETRIC_STRONG
import java.security.KeyFactory
import java.security.KeyPairGenerator
import java.security.KeyStore
import java.security.PrivateKey
import java.security.Signature
import java.security.UnrecoverableKeyException
import java.security.interfaces.ECPublicKey
import java.security.spec.ECGenParameterSpec
import java.util.Base64

/**
 * Unlock keys in the Android Keystore (docs/field-unlock-keys-spec.md section 12.1): EC P-256, `PURPOSE_SIGN`,
 * `DIGEST_SHA256`, user authentication required for every use by a strong biometric
 * (`setUserAuthenticationParameters(0, AUTH_BIOMETRIC_STRONG)`), invalidated by a new biometric enrolment, StrongBox
 * first then the TEE, with the attestation challenge of section 3. Android 11 (API 30) and later only. Never logs key
 * material or signatures.
 */
internal class AndroidUnlockKeyProvider(private val context: Context) : UnlockKeyProvider {
    private fun keyStore(): KeyStore = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }

    override val attests = true

    override fun biometricStatus(): UnlockKeyAvailability {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.R) return UnlockKeyAvailability.OS_TOO_OLD
        return when (BiometricManager.from(context).canAuthenticate(BIOMETRIC_STRONG)) {
            BiometricManager.BIOMETRIC_SUCCESS -> UnlockKeyAvailability.AVAILABLE
            BiometricManager.BIOMETRIC_ERROR_NONE_ENROLLED -> UnlockKeyAvailability.BIOMETRIC_NOT_ENROLLED
            else -> UnlockKeyAvailability.BIOMETRIC_UNAVAILABLE
        }
    }

    @Synchronized
    override fun create(alias: String, attestationChallenge: ByteArray): UnlockKeyMaterial {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.R) {
            throw FcFieldException(UnlockKeyCodes.UNLOCK_KEYS_UNSUPPORTED, "unlock keys need Android 11 or later")
        }
        generate(alias, attestationChallenge)
        try {
            return describe(keyStore(), alias)
        } catch (e: Exception) {
            runCatching { keyStore().deleteEntry(alias) }
            throw FcFieldException("INTERNAL", "could not read the generated unlock key", e)
        }
    }

    @Synchronized
    override fun prepare(alias: String): PreparedSignature {
        val ks = try { keyStore() } catch (e: Exception) { throw FcFieldException("INTERNAL", "Keystore unavailable", e) }
        val key = try {
            ks.getKey(alias, null) as? PrivateKey
        } catch (e: UnrecoverableKeyException) {
            throw FcFieldException("KEY_INVALIDATED", "the unlock key is no longer usable", e)
        } catch (e: Exception) {
            throw FcFieldException("INTERNAL", "could not load the unlock key", e)
        } ?: throw FcFieldException("KEY_INVALIDATED", "the unlock key is gone")
        try {
            val sig = Signature.getInstance("SHA256withECDSA")
            sig.initSign(key) // a key invalidated by a new enrolment fails here, before any prompt
            return KeystoreSignature(sig)
        } catch (e: KeyPermanentlyInvalidatedException) {
            throw FcFieldException("KEY_INVALIDATED", "the unlock key was invalidated", e)
        } catch (e: UnrecoverableKeyException) {
            throw FcFieldException("KEY_INVALIDATED", "the unlock key is no longer usable", e)
        } catch (e: Exception) {
            throw FcFieldException("INTERNAL", "could not ready the unlock key", e)
        }
    }

    @Synchronized
    override fun delete(alias: String) {
        try {
            val ks = keyStore()
            if (ks.containsAlias(alias)) ks.deleteEntry(alias)
        } catch (e: Exception) {
            throw FcFieldException("INTERNAL", "could not delete the unlock key", e)
        }
    }

    @RequiresApi(Build.VERSION_CODES.R)
    private fun generate(alias: String, challenge: ByteArray) {
        // StrongBox with attestation, the TEE with attestation, the TEE without (some OEMs refuse attestation; the
        // platform then records NONE, refused only under REQUIRED).
        val attempts = listOf(true to true, false to true, false to false) // (strongBox, attest)
        var last: Exception? = null
        for ((strongBox, attest) in attempts) {
            try {
                val b = KeyGenParameterSpec.Builder(alias, KeyProperties.PURPOSE_SIGN)
                    .setAlgorithmParameterSpec(ECGenParameterSpec("secp256r1"))
                    .setDigests(KeyProperties.DIGEST_SHA256)
                    .setUserAuthenticationRequired(true)
                    .setUserAuthenticationParameters(0, KeyProperties.AUTH_BIOMETRIC_STRONG)
                    .setInvalidatedByBiometricEnrollment(true)
                if (attest) b.setAttestationChallenge(challenge)
                if (strongBox) b.setIsStrongBoxBacked(true)
                val kpg = KeyPairGenerator.getInstance(KeyProperties.KEY_ALGORITHM_EC, "AndroidKeyStore")
                kpg.initialize(b.build())
                kpg.generateKeyPair()
                return
            } catch (e: StrongBoxUnavailableException) {
                last = e
            } catch (e: Exception) {
                runCatching { keyStore().deleteEntry(alias) }
                last = e
            }
        }
        throw FcFieldException("INTERNAL", "could not make an unlock key", last)
    }

    private fun describe(ks: KeyStore, alias: String): UnlockKeyMaterial {
        val cert = ks.getCertificate(alias) ?: throw IllegalStateException("no certificate")
        val pub = cert.publicKey as ECPublicKey
        val x = DerSignature.toBase64Url(DerSignature.fixedLength(pub.w.affineX.toByteArray()))
        val y = DerSignature.toBase64Url(DerSignature.fixedLength(pub.w.affineY.toByteArray()))
        val chain = ks.getCertificateChain(alias)?.map { Base64.getEncoder().encodeToString(it.encoded) }
        val priv = ks.getKey(alias, null) as PrivateKey
        // Only a real chain (leaf + issuers) is sent; a lone self-signed certificate is no attestation.
        return UnlockKeyMaterial(x, y, chain?.takeIf { it.size > 1 }, securityLevel(priv))
    }

    private fun securityLevel(priv: PrivateKey): String = try {
        val info = KeyFactory.getInstance(priv.algorithm, "AndroidKeyStore").getKeySpec(priv, KeyInfo::class.java)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
            when (info.securityLevel) {
                KeyProperties.SECURITY_LEVEL_STRONGBOX -> "STRONGBOX"
                KeyProperties.SECURITY_LEVEL_TRUSTED_ENVIRONMENT -> "TEE"
                else -> "SOFTWARE"
            }
        } else {
            @Suppress("DEPRECATION")
            if (info.isInsideSecureHardware) "TEE" else "SOFTWARE"
        }
    } catch (e: Exception) {
        "NONE"
    }
}

/** A Keystore signature readied with [Signature.initSign]; the prompt's CryptoObject authorises it. */
internal class KeystoreSignature(val signature: Signature) : PreparedSignature {
    override fun sign(data: ByteArray): ByteArray = try {
        signature.update(data)
        DerSignature.derToRaw(signature.sign())
    } catch (e: KeyPermanentlyInvalidatedException) {
        throw FcFieldException("KEY_INVALIDATED", "the unlock key was invalidated", e)
    } catch (e: Exception) {
        throw FcFieldException("INTERNAL", "unlock signing failed", e)
    }
}
