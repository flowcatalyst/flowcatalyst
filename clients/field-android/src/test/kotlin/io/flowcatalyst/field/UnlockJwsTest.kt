package io.flowcatalyst.field

import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Test
import java.io.File
import java.time.Instant

/** The unlock-key signer against the platform's byte vectors (api/field-fixtures/unlock.json). */
class UnlockJwsTest {
    private val fixture: JSONObject by lazy {
        val f = File("../../api/field-fixtures/unlock.json")
        assumeTrue("api/field-fixtures/unlock.json is in the repository", f.isFile)
        JSONObject(f.readText())
    }

    private fun case(name: String): JSONObject {
        val cases = fixture.getJSONArray("cases")
        for (i in 0 until cases.length()) if (cases.getJSONObject(i).getString("name") == name) return cases.getJSONObject(i)
        throw AssertionError("no case $name")
    }

    private val base get() = fixture.getString("baseUrl")
    private val key get() = fixture.getJSONObject("unlockKey")
    private fun iat(c: JSONObject) = c.getJSONObject("claims").getLong("iat")

    /** What the library builds from the values the platform expects (verifyWith). */
    private fun ours(c: JSONObject): String {
        val v = c.getJSONObject("verifyWith")
        return if (c.getString("kind") == "keyProof") {
            UnlockJws.keyProofSigningInput(base, v.getString("nonce"), v.getString("sub"), v.getString("deviceId"), iat(c))
        } else {
            UnlockJws.assertionSigningInput(
                base, v.getString("keyId"), v.getString("nonce"), v.getString("sub"), v.getString("deviceId"), v.getString("app"), iat(c),
            )
        }
    }

    @Test fun keyProofSigningInputIsTheVectorsByteForByte() {
        val c = case("key_proof_valid")
        assertEquals(c.getString("signingInput"), ours(c))
    }

    @Test fun assertionSigningInputIsTheVectorsByteForByte() {
        val c = case("assertion_valid")
        assertEquals(c.getString("signingInput"), ours(c))
    }

    @Test fun theValidVectorsVerifyWithTheUnlockKeyAndTheOtherKeysDoesNot() {
        for (name in listOf("key_proof_valid", "assertion_valid")) {
            val c = case(name)
            assertTrue(name, verifyEs256(c.getString("jws"), key.getString("x"), key.getString("y")))
            // The compact form is the signing input and the raw signature.
            val sig = Dpop.b64uDecode(c.getString("jws").substringAfterLast('.'))
            assertEquals(c.getString("jws"), UnlockJws.compact(ours(c), sig))
        }
        assertFalse(verifyEs256(case("assertion_signed_by_another_key").getString("jws"), key.getString("x"), key.getString("y")))
    }

    @Test fun theLibraryCannotBuildAnyRefusedVariant() {
        val refused = listOf(
            "assertion_with_key_proof_typ", "assertion_with_dpop_typ", "assertion_other_kid", "assertion_other_app",
            "assertion_key_proof_audience", "key_proof_with_kid", "key_proof_other_nonce",
        )
        for (name in refused) {
            val c = case(name)
            assertNotEquals(c.getString("expect"), "valid")
            assertNotEquals(name, c.getString("signingInput"), ours(c))
        }
    }

    @Test fun headersHoldExactlyTheMembersTheSpecNames() {
        val proof = JSONObject(String(Dpop.b64uDecode(ours(case("key_proof_valid")).substringBefore('.'))))
        assertEquals(setOf("typ", "alg"), proof.keySet())
        val assertion = JSONObject(String(Dpop.b64uDecode(ours(case("assertion_valid")).substringBefore('.'))))
        assertEquals(setOf("typ", "alg", "kid"), assertion.keySet())
    }

    @Test fun attestationChallengeIsTheVectors() {
        val a = fixture.getJSONObject("attestationChallenge")
        val parts = a.getString("input").removePrefix("fc-unlock-key-attestation:v1\n").split("\n")
        val got = UnlockJws.attestationChallenge(parts[0], parts[1], parts[2])
        assertEquals(32, got.size)
        assertEquals(a.getString("hex"), got.joinToString("") { "%02x".format(it) })
    }

    @Test fun aTrailingSlashOnTheBaseUrlChangesNothing() {
        val c = case("key_proof_valid")
        val v = c.getJSONObject("verifyWith")
        assertEquals(
            c.getString("signingInput"),
            UnlockJws.keyProofSigningInput("$base/", v.getString("nonce"), v.getString("sub"), v.getString("deviceId"), iat(c)),
        )
    }

    @Test fun theFixtureTimeIsTheClaim() {
        val c = case("assertion_valid")
        assertEquals(Instant.parse(c.getString("now")).epochSecond, iat(c))
    }

    @Test fun jsonEscapesOnlyWhatJsonNeeds() {
        assertEquals("""{"a":"x/y\"z\\\n\u0001","n":5}""", UnlockJws.json(listOf("a" to "x/y\"z\\\n\u0001", "n" to 5L)))
    }

    @Test fun aSignatureOfTheWrongLengthIsRefused() {
        try {
            UnlockJws.compact("a.b", ByteArray(70))
            throw AssertionError("expected a refusal")
        } catch (e: FieldAuthException) {
            assertEquals("INTERNAL", e.code)
        }
    }
}
