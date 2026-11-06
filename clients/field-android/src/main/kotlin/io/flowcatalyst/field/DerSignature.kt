package io.flowcatalyst.field

import java.util.Base64

/** Pure helpers (no Android types) so they are unit-testable on the JVM. */
internal object DerSignature {
    /** ASN.1 DER ECDSA signature (SEQUENCE { INTEGER r, INTEGER s }) -> raw r‖s, each left-padded to [size] bytes. */
    fun derToRaw(der: ByteArray, size: Int = 32): ByteArray {
        var i = 0
        fun need(n: Int) { if (i + n > der.size) throw IllegalArgumentException("malformed DER signature") }
        fun readLen(): Int {
            need(1)
            val b = der[i++].toInt() and 0xff
            if (b < 0x80) return b
            val n = b and 0x7f
            if (n == 0 || n > 2) throw IllegalArgumentException("malformed DER signature")
            need(n)
            var len = 0
            repeat(n) { len = (len shl 8) or (der[i++].toInt() and 0xff) }
            return len
        }
        need(1)
        if (der[i++].toInt() != 0x30) throw IllegalArgumentException("malformed DER signature")
        val seqLen = readLen()
        if (i + seqLen != der.size) throw IllegalArgumentException("malformed DER signature")
        fun readInt(): ByteArray {
            need(1)
            if (der[i++].toInt() != 0x02) throw IllegalArgumentException("malformed DER signature")
            val len = readLen()
            need(len)
            var start = i
            var end = i + len
            i = end
            while (start < end - 1 && der[start].toInt() == 0) start++ // strip sign padding
            val n = end - start
            if (n > size) throw IllegalArgumentException("malformed DER signature")
            val out = ByteArray(size)
            System.arraycopy(der, start, out, size - n, n)
            return out
        }
        val r = readInt()
        val s = readInt()
        if (i != der.size) throw IllegalArgumentException("malformed DER signature")
        return r + s
    }

    /** Raw r‖s -> DER (used by tests to round-trip). */
    fun rawToDer(raw: ByteArray): ByteArray {
        require(raw.size % 2 == 0 && raw.isNotEmpty()) { "bad raw signature" }
        val half = raw.size / 2
        fun enc(v: ByteArray): ByteArray {
            var start = 0
            while (start < v.size - 1 && v[start].toInt() == 0) start++
            var body = v.copyOfRange(start, v.size)
            if (body[0].toInt() and 0x80 != 0) body = byteArrayOf(0) + body
            return byteArrayOf(0x02, body.size.toByte()) + body
        }
        val seq = enc(raw.copyOfRange(0, half)) + enc(raw.copyOfRange(half, raw.size))
        val hdr = if (seq.size < 0x80) byteArrayOf(0x30, seq.size.toByte()) else byteArrayOf(0x30, 0x81.toByte(), seq.size.toByte())
        return hdr + seq
    }

    /** Left-pad / strip sign byte to exactly [size] bytes (for JWK x / y). */
    fun fixedLength(v: ByteArray, size: Int = 32): ByteArray {
        var start = 0
        while (v.size - start > size && v[start].toInt() == 0) start++
        val n = v.size - start
        require(n <= size) { "integer too large" }
        val out = ByteArray(size)
        System.arraycopy(v, start, out, size - n, n)
        return out
    }

    private val b64u = Base64.getUrlEncoder().withoutPadding()
    private val b64uDec = Base64.getUrlDecoder()
    fun toBase64Url(b: ByteArray): String = b64u.encodeToString(b)
    fun fromBase64Url(s: String): ByteArray = b64uDec.decode(s)
}
