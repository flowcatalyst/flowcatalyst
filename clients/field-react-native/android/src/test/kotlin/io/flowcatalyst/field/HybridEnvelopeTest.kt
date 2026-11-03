package io.flowcatalyst.field

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test
import java.security.KeyPair
import java.security.KeyPairGenerator
import javax.crypto.Cipher

class HybridEnvelopeTest {
    private fun rsa(): KeyPair = KeyPairGenerator.getInstance("RSA").apply { initialize(2048) }.generateKeyPair()

    private fun unwrapWith(kp: KeyPair): (ByteArray) -> ByteArray = { w ->
        Cipher.getInstance(HybridEnvelope.RSA_TRANSFORM).run {
            init(Cipher.DECRYPT_MODE, kp.private, HybridEnvelope.oaep)
            doFinal(w)
        }
    }

    @Test fun roundTrip() {
        val kp = rsa()
        for (v in listOf("", "refresh-token", "unicode é中\"'</script>\n", "x".repeat(10_000))) {
            val sealed = HybridEnvelope.seal(v, kp.public)
            assertEquals(v, HybridEnvelope.open(HybridEnvelope.parse(sealed)!!, unwrapWith(kp)))
        }
    }

    @Test fun formatAndFreshKeyPerSeal() {
        val kp = rsa()
        val a = HybridEnvelope.seal("same", kp.public)
        val b = HybridEnvelope.seal("same", kp.public)
        assertNotEquals(a, b)
        val parts = a.split(":")
        assertEquals(5, parts.size)
        assertEquals("v2", parts[0]); assertEquals("b", parts[1])
        val p = HybridEnvelope.parse(a)!!
        assertEquals(256, p.wrapped.size) // RSA-2048 block
        assertEquals(12, p.iv.size)
        assertEquals(4 + 16, p.ct.size)   // GCM tag included
        assertTrue(!a.contains("same"))
    }

    @Test fun wrongKeyFails() {
        val sealed = HybridEnvelope.seal("v", rsa().public)
        try { HybridEnvelope.open(HybridEnvelope.parse(sealed)!!, unwrapWith(rsa())); fail() } catch (e: java.security.GeneralSecurityException) {}
    }

    @Test fun tamperedCiphertextFails() {
        val kp = rsa()
        val p = HybridEnvelope.parse(HybridEnvelope.seal("v", kp.public))!!
        p.ct[0] = (p.ct[0].toInt() xor 1).toByte()
        try { HybridEnvelope.open(p, unwrapWith(kp)); fail() } catch (e: java.security.GeneralSecurityException) {}
    }

    @Test fun parseRejectsOtherFormats() {
        assertNull(HybridEnvelope.parse(null))
        assertNull(HybridEnvelope.parse("v1:b:AA==:AA==:AA=="))
        assertNull(HybridEnvelope.parse("v2:n:AA==:AA==:AA=="))
        assertNull(HybridEnvelope.parse("v2:b:AA==:AA=="))
        assertNull(HybridEnvelope.parse("v2:b:!!:!!:!!"))
    }
}
