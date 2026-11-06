package io.flowcatalyst.field

import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.security.MessageDigest
import java.util.Base64

/** Verifies proofs the way a server would: signature against the embedded jwk, claims, thumbprint. */
class DpopProofTest {
    private lateinit var h: Harness

    @Before fun setUp() { h = Harness() }
    @After fun tearDown() { h.close() }

    private fun thumbprintIndependently(x: String, y: String): String {
        val canonical = "{\"crv\":\"P-256\",\"kty\":\"EC\",\"x\":\"$x\",\"y\":\"$y\"}"
        return Base64.getUrlEncoder().withoutPadding().encodeToString(MessageDigest.getInstance("SHA-256").digest(canonical.toByteArray()))
    }

    @Test fun loginProofVerifies() = runBlocking {
        h.platform.on("/field/auth/login") { h.platform.token() }
        h.session().login(LoginCredentials(workerCode = "W001", pin = "1234"))
        val login = h.platform.all("/field/auth/login").single()
        val proof = login.proof!!
        assertEquals("dpop+jwt", proof.header.getString("typ"))
        assertEquals("ES256", proof.header.getString("alg"))
        assertTrue("signature verifies", proof.valid())
        assertEquals("POST", proof.claims.getString("htm"))
        assertEquals("${h.platform.url}/field/auth/login", proof.claims.getString("htu"))
        assertEquals(h.clock.now() / 1000, proof.claims.getLong("iat"))
        assertTrue(proof.claims.getString("jti").length >= 16)
        assertTrue(proof.nonce!!.startsWith("nonce-"))
        assertTrue(!proof.claims.has("ath")) // no token yet
        // the jwk is exactly the public key; its thumbprint is the RFC 7638 jkt
        assertEquals(h.keys.key.x, proof.jwk.getString("x"))
        assertEquals(h.keys.key.y, proof.jwk.getString("y"))
        assertEquals("EC", proof.jwk.getString("kty"))
        assertEquals("P-256", proof.jwk.getString("crv"))
        assertTrue(!proof.jwk.has("d"))
        assertEquals(thumbprintIndependently(h.keys.key.x, h.keys.key.y), Dpop.thumbprint(h.keys.key.x, h.keys.key.y))
        // the nonce proof (no nonce yet) verifies as well
        val prime = h.platform.all("/field/auth/nonce").single().proof!!
        assertTrue(prime.valid())
        assertNull(prime.nonce)
    }

    @Test fun headersForProofBindsMethodUrlTokenAndNonce() = runBlocking {
        h.platform.on("/field/auth/login") { h.platform.token() }
        val s = h.session()
        s.login(LoginCredentials(workerCode = "W001", pin = "1234"))
        s.noteNonce("https://api.example.com/x", "srv-nonce")
        val hdr = s.headersFor("post", "https://api.example.com/v1/items?limit=5#frag")
        val token = hdr.getValue("Authorization").removePrefix("DPoP ")
        val proof = Proof(hdr.getValue("DPoP"))
        assertTrue(proof.valid())
        assertEquals("POST", proof.claims.getString("htm"))
        assertEquals("https://api.example.com/v1/items", proof.claims.getString("htu")) // no query, no fragment
        assertEquals("srv-nonce", proof.nonce)
        assertEquals(sha256b64u(token), proof.claims.getString("ath"))
        assertEquals(h.clock.now() / 1000, proof.claims.getLong("iat"))
        // jti differs per proof
        val again = Proof(s.headersFor("post", "https://api.example.com/v1/items")["DPoP"]!!)
        assertNotEquals(proof.claims.getString("jti"), again.claims.getString("jti"))
        assertTrue(again.valid())
    }

    @Test fun aTamperedProofDoesNotVerify() = runBlocking {
        h.platform.on("/field/auth/login") { h.platform.token() }
        val s = h.session()
        s.login(LoginCredentials(workerCode = "W001", pin = "1234"))
        val jwt = s.headersFor("GET", "https://api.example.com/x")["DPoP"]!!
        val parts = jwt.split(".")
        val claims = String(Base64.getUrlDecoder().decode(parts[1])).replace("GET", "PUT")
        val forged = parts[0] + "." + Base64.getUrlEncoder().withoutPadding().encodeToString(claims.toByteArray()) + "." + parts[2]
        assertTrue(!Proof(forged).valid())
    }
}
