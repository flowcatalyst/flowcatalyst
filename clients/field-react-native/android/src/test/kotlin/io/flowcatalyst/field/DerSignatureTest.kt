package io.flowcatalyst.field

import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.fail
import org.junit.Test
import java.security.KeyPairGenerator
import java.security.Signature
import java.security.spec.ECGenParameterSpec

class DerSignatureTest {
    private fun hex(s: String) = s.chunked(2).map { it.toInt(16).toByte() }.toByteArray()

    @Test fun shortRAndSArePadded() {
        // r = 0x01, s = 0x02
        val raw = DerSignature.derToRaw(hex("3006020101020102"))
        assertEquals(64, raw.size)
        assertEquals(1, raw[31].toInt()); assertEquals(2, raw[63].toInt())
        for (i in 0 until 31) assertEquals(0, raw[i].toInt())
    }

    @Test fun signPaddingByteIsStripped() {
        // r has leading 0x00 because its high bit is set
        val r = "00" + "ff".repeat(32)
        val s = "7f" + "00".repeat(31)
        val der = hex("30" + "45" + "0221" + r + "0220" + s)
        val raw = DerSignature.derToRaw(der)
        assertArrayEquals(hex("ff".repeat(32)), raw.copyOfRange(0, 32))
        assertArrayEquals(hex(s), raw.copyOfRange(32, 64))
    }

    @Test fun longFormSequenceLength() {
        val r = "00" + "ff".repeat(32)
        val s = "00" + "ff".repeat(32)
        val der = hex("3081" + "46" + "0221" + r + "0221" + s)
        assertEquals(64, DerSignature.derToRaw(der).size)
    }

    @Test fun roundTripWithRealJcaSignatures() {
        val kpg = KeyPairGenerator.getInstance("EC"); kpg.initialize(ECGenParameterSpec("secp256r1"))
        repeat(200) {
            val kp = kpg.generateKeyPair()
            val sig = Signature.getInstance("SHA256withECDSA"); sig.initSign(kp.private); sig.update(byteArrayOf(it.toByte()))
            val der = sig.sign()
            val raw = DerSignature.derToRaw(der)
            assertEquals(64, raw.size)
            assertArrayEquals(der, DerSignature.rawToDer(raw))
            // verify using the DER re-encoded from raw
            val v = Signature.getInstance("SHA256withECDSA"); v.initVerify(kp.public); v.update(byteArrayOf(it.toByte()))
            assertEquals(true, v.verify(DerSignature.rawToDer(raw)))
        }
    }

    @Test fun malformedIsRejected() {
        for (bad in listOf("", "30", "3006020101020202", "3106020101020102", "300602010102", "3004020100ff")) {
            try { DerSignature.derToRaw(hex(bad)); fail("accepted $bad") } catch (e: IllegalArgumentException) {}
        }
    }

    @Test fun fixedLengthStripsAndPads() {
        assertEquals(32, DerSignature.fixedLength(byteArrayOf(0, 1) + ByteArray(31)).size) // 33 bytes with sign byte
        assertEquals(32, DerSignature.fixedLength(byteArrayOf(5)).size)
        assertEquals(5, DerSignature.fixedLength(byteArrayOf(5))[31].toInt())
    }

    @Test fun base64UrlHasNoPadding() {
        assertEquals("AQID", DerSignature.toBase64Url(byteArrayOf(1, 2, 3)))
        assertEquals("AQI", DerSignature.toBase64Url(byteArrayOf(1, 2)))
        assertArrayEquals(byteArrayOf(1, 2), DerSignature.fromBase64Url("AQI"))
    }
}
