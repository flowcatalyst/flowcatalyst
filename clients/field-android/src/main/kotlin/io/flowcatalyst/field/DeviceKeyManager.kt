package io.flowcatalyst.field

import android.os.Build
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyInfo
import android.security.keystore.KeyPermanentlyInvalidatedException
import android.security.keystore.KeyProperties
import android.security.keystore.StrongBoxUnavailableException
import java.security.KeyFactory
import java.security.KeyPairGenerator
import java.security.KeyStore
import java.security.PrivateKey
import java.security.SecureRandom
import java.security.Signature
import java.security.UnrecoverableKeyException
import java.security.interfaces.ECPublicKey
import java.security.spec.ECGenParameterSpec
import java.util.Base64

/** Keystore EC P-256 device key. Never logs key material or signatures. */
internal class DeviceKeyManager {
    private fun keyStore(): KeyStore = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }

    @Synchronized
    fun getOrCreate(): Map<String, Any?> {
        val ks = try { keyStore() } catch (e: Exception) { throw FcFieldException("INTERNAL", "Keystore unavailable", e) }
        if (ks.containsAlias(ALIAS)) {
            try {
                return describe(ks)
            } catch (e: Exception) {
                // Lost / unreadable key (OEM Keystore churn, restore from backup): start over with a fresh key.
                // The new public key differs, so the server will reject the old identity and the app re-enrols.
                runCatching { ks.deleteEntry(ALIAS) }
            }
        }
        generate()
        try {
            return describe(keyStore())
        } catch (e: Exception) {
            throw FcFieldException("INTERNAL", "could not read generated key", e)
        }
    }

    @Synchronized
    fun sign(data: ByteArray): ByteArray {
        val ks = try { keyStore() } catch (e: Exception) { throw FcFieldException("INTERNAL", "Keystore unavailable", e) }
        val key = try {
            ks.getKey(ALIAS, null) as? PrivateKey
        } catch (e: UnrecoverableKeyException) {
            invalidate(ks); throw FcFieldException("KEY_INVALIDATED", "device key is no longer usable", e)
        } catch (e: KeyPermanentlyInvalidatedException) {
            invalidate(ks); throw FcFieldException("KEY_INVALIDATED", "device key is no longer usable", e)
        } catch (e: Exception) {
            throw FcFieldException("INTERNAL", "could not load device key", e)
        } ?: throw FcFieldException("KEY_INVALIDATED", "no device key", null)
        try {
            val sig = Signature.getInstance("SHA256withECDSA")
            sig.initSign(key)
            sig.update(data)
            return DerSignature.derToRaw(sig.sign())
        } catch (e: KeyPermanentlyInvalidatedException) {
            invalidate(ks); throw FcFieldException("KEY_INVALIDATED", "device key is no longer usable", e)
        } catch (e: UnrecoverableKeyException) {
            invalidate(ks); throw FcFieldException("KEY_INVALIDATED", "device key is no longer usable", e)
        } catch (e: Exception) {
            throw FcFieldException("INTERNAL", "signing failed", e)
        }
    }

    @Synchronized
    fun delete() {
        try {
            val ks = keyStore()
            if (ks.containsAlias(ALIAS)) ks.deleteEntry(ALIAS)
        } catch (e: Exception) {
            throw FcFieldException("INTERNAL", "could not delete device key", e)
        }
    }

    private fun invalidate(ks: KeyStore) {
        runCatching { ks.deleteEntry(ALIAS) }
    }

    private fun generate() {
        val challenge = ByteArray(32).also { SecureRandom().nextBytes(it) }
        // Order: StrongBox + attestation, TEE + attestation, TEE without attestation (some emulators / OEMs reject it).
        val attempts = ArrayList<Pair<Boolean, Boolean>>() // (strongBox, attest)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) attempts.add(true to true)
        attempts.add(false to true)
        attempts.add(false to false)
        var last: Exception? = null
        for ((strongBox, attest) in attempts) {
            try {
                val b = KeyGenParameterSpec.Builder(ALIAS, KeyProperties.PURPOSE_SIGN)
                    .setAlgorithmParameterSpec(ECGenParameterSpec("secp256r1"))
                    .setDigests(KeyProperties.DIGEST_SHA256)
                if (attest) b.setAttestationChallenge(challenge)
                if (strongBox && Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) b.setIsStrongBoxBacked(true)
                val kpg = KeyPairGenerator.getInstance(KeyProperties.KEY_ALGORITHM_EC, "AndroidKeyStore")
                kpg.initialize(b.build())
                kpg.generateKeyPair()
                return
            } catch (e: StrongBoxUnavailableException) {
                last = e
            } catch (e: Exception) {
                runCatching { keyStore().deleteEntry(ALIAS) }
                last = e
            }
        }
        throw FcFieldException("INTERNAL", "could not generate device key", last)
    }

    private fun describe(ks: KeyStore): Map<String, Any?> {
        val cert = ks.getCertificate(ALIAS) ?: throw IllegalStateException("no certificate")
        val pub = cert.publicKey as ECPublicKey
        val x = DerSignature.toBase64Url(DerSignature.fixedLength(pub.w.affineX.toByteArray()))
        val y = DerSignature.toBase64Url(DerSignature.fixedLength(pub.w.affineY.toByteArray()))
        val priv = ks.getKey(ALIAS, null) as PrivateKey
        val level = securityLevel(priv)
        val chain = ks.getCertificateChain(ALIAS)?.map { Base64.getEncoder().encodeToString(it.encoded) }
        val attestation = HashMap<String, Any?>()
        attestation["level"] = level
        // A software-generated or attestation-less key still has a self-signed cert; only send a chain when
        // it is more than that (leaf + at least one issuer) so the server never sees a bogus chain.
        if (chain != null && chain.size > 1) attestation["chain"] = chain
        return mapOf(
            "jwk" to mapOf("kty" to "EC", "crv" to "P-256", "x" to x, "y" to y),
            "attestation" to attestation,
        )
    }

    private fun securityLevel(priv: PrivateKey): String {
        return try {
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

    companion object {
        const val ALIAS = "fc_field_device"
    }
}
