package io.flowcatalyst.field

import java.security.PublicKey
import java.security.SecureRandom
import java.security.spec.MGF1ParameterSpec
import java.util.Base64
import javax.crypto.Cipher
import javax.crypto.spec.GCMParameterSpec
import javax.crypto.spec.OAEPParameterSpec
import javax.crypto.spec.PSource
import javax.crypto.spec.SecretKeySpec

/**
 * Hybrid envelope for biometric store entries: a random AES-256-GCM data key encrypts the value; the data key is
 * wrapped with an RSA public key (OAEP SHA-256 / MGF1-SHA-1, the combination Android Keystore supports). Sealing
 * needs only the public key, so it never prompts; opening needs the Keystore private key (biometric-gated).
 * Format: "v2:b:<wrappedKey>:<iv>:<ciphertext>" (standard base64). Pure JVM; never logs anything.
 */
internal object HybridEnvelope {
    const val RSA_TRANSFORM = "RSA/ECB/OAEPWithSHA-256AndMGF1Padding"
    private const val AES_TRANSFORM = "AES/GCM/NoPadding"

    val oaep: OAEPParameterSpec
        get() = OAEPParameterSpec("SHA-256", "MGF1", MGF1ParameterSpec.SHA1, PSource.PSpecified.DEFAULT)

    class Parsed(val wrapped: ByteArray, val iv: ByteArray, val ct: ByteArray)

    fun seal(value: String, wrapKey: PublicKey): String {
        val dataKey = ByteArray(32).also { SecureRandom().nextBytes(it) }
        val aes = Cipher.getInstance(AES_TRANSFORM)
        aes.init(Cipher.ENCRYPT_MODE, SecretKeySpec(dataKey, "AES"))
        val ct = aes.doFinal(value.toByteArray(Charsets.UTF_8))
        val rsa = Cipher.getInstance(RSA_TRANSFORM)
        rsa.init(Cipher.ENCRYPT_MODE, wrapKey, oaep)
        val wrapped = rsa.doFinal(dataKey)
        java.util.Arrays.fill(dataKey, 0)
        val e = Base64.getEncoder()
        return "v2:b:${e.encodeToString(wrapped)}:${e.encodeToString(aes.iv)}:${e.encodeToString(ct)}"
    }

    fun parse(s: String?): Parsed? {
        if (s == null) return null
        val p = s.split(":")
        if (p.size != 5 || p[0] != "v2" || p[1] != "b") return null
        return try {
            val d = Base64.getDecoder()
            Parsed(d.decode(p[2]), d.decode(p[3]), d.decode(p[4]))
        } catch (e: IllegalArgumentException) {
            null
        }
    }

    /** [unwrap] turns the wrapped data key into the raw key (the biometric-authenticated RSA decrypt). */
    fun open(p: Parsed, unwrap: (ByteArray) -> ByteArray): String {
        val dataKey = unwrap(p.wrapped)
        try {
            val aes = Cipher.getInstance(AES_TRANSFORM)
            aes.init(Cipher.DECRYPT_MODE, SecretKeySpec(dataKey, "AES"), GCMParameterSpec(128, p.iv))
            return String(aes.doFinal(p.ct), Charsets.UTF_8)
        } finally {
            java.util.Arrays.fill(dataKey, 0)
        }
    }
}
