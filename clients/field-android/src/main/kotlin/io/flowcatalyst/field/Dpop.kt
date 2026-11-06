package io.flowcatalyst.field

import okhttp3.HttpUrl
import org.json.JSONObject
import java.security.MessageDigest
import java.security.SecureRandom
import java.util.Base64

internal object Dpop {
    private val enc = Base64.getUrlEncoder().withoutPadding()
    private val dec = Base64.getUrlDecoder()
    private val rng = SecureRandom()

    fun b64u(b: ByteArray): String = enc.encodeToString(b)
    fun b64uDecode(s: String): ByteArray = dec.decode(s)

    /** base64url(SHA-256(token)), the proof's `ath`. */
    fun accessTokenHash(token: String): String = b64u(MessageDigest.getInstance("SHA-256").digest(token.toByteArray(Charsets.UTF_8)))

    /** The request URL without query or fragment. */
    fun htu(url: HttpUrl): String = url.newBuilder().query(null).fragment(null).build().toString()

    fun randomId(bytes: Int = 16): String = ByteArray(bytes).also { rng.nextBytes(it) }.let(::b64u)

    /** Builds an ES256 DPoP proof (RFC 9449 shape) signed through the key provider. */
    fun build(
        keys: DeviceKeyProvider,
        key: DeviceKey,
        htm: String,
        htu: String,
        nonce: String?,
        accessToken: String?,
        iatSeconds: Long,
    ): String {
        val header = JSONObject()
            .put("typ", "dpop+jwt")
            .put("alg", "ES256")
            .put("jwk", JSONObject().put("kty", "EC").put("crv", "P-256").put("x", key.x).put("y", key.y))
        val claims = JSONObject()
            .put("htm", htm)
            .put("htu", htu)
            .put("iat", iatSeconds)
            .put("jti", randomId(16))
        if (!nonce.isNullOrEmpty()) claims.put("nonce", nonce)
        if (!accessToken.isNullOrEmpty()) claims.put("ath", accessTokenHash(accessToken))
        val input = b64u(header.toString().toByteArray(Charsets.UTF_8)) + "." + b64u(claims.toString().toByteArray(Charsets.UTF_8))
        val sig = keys.sign(input.toByteArray(Charsets.UTF_8))
        if (sig.size != 64) throw FieldAuthException("INTERNAL", "the key provider returned a malformed signature")
        return input + "." + b64u(sig)
    }

    /** RFC 7638 thumbprint of an EC P-256 public key. */
    fun thumbprint(x: String, y: String): String =
        b64u(MessageDigest.getInstance("SHA-256").digest("""{"crv":"P-256","kty":"EC","x":"$x","y":"$y"}""".toByteArray(Charsets.UTF_8)))
}
