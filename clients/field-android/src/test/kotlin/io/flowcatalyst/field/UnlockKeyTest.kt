package io.flowcatalyst.field

import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import org.json.JSONArray
import org.json.JSONObject
import org.junit.After
import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Before
import org.junit.Test
import java.security.MessageDigest
import java.util.concurrent.atomic.AtomicInteger

/**
 * Unlock keys (docs/field-unlock-keys-spec.md section 12) against a fake platform that verifies the key proof and the
 * assertion as the real one does, with software keys behind the Keystore's interface and a fake prompt.
 */
class UnlockKeyTest {
    private lateinit var h: Harness
    private lateinit var keys: SoftUnlockKeys
    private val p get() = h.platform
    private val prompts = AtomicInteger()
    private val prompt = UnlockKeyPrompt("Sign in", "Use PIN")

    /** Signs after "a biometric match". */
    private val finger = UnlockKeyPrompter { _, sig, data -> prompts.incrementAndGet(); sig.sign(data) }

    private fun refusing(code: String) = UnlockKeyPrompter { _, _, _ -> prompts.incrementAndGet(); throw FieldAuthException(code) }

    // What the fake platform holds.
    private var registeredJwk: JSONObject? = null
    private var registerAnswer: ((Rec) -> MockResponse)? = null
    private var probeAnswer: (() -> MockResponse)? = null
    private var challengeAnswer: (() -> MockResponse)? = null
    private var unlockAnswer: ((Rec) -> MockResponse)? = null
    private val issued = mutableListOf<String>()
    private val revoked = mutableListOf<String>()
    private var revokeAnswer: (() -> MockResponse)? = null

    private val pinClaims get() = claims(listOf("pin"), "fam1")

    private fun claims(amr: List<String>, sid: String, worker: String = "w1") = p.clientClaims(p.deviceClaims("PERSONAL", "dev1"))
        .put("sub", worker).put("sid", sid).put("amr", JSONArray(amr))

    @Before fun setUp() {
        keys = SoftUnlockKeys()
        h = Harness(keys = SoftKeyProvider("TEE"), unlockKeys = keys)
        install()
    }

    private fun install() {
        p.on("/field/devices/enrol") {
            MockResponse().setResponseCode(201).setBody("""{"deviceId":"dev1","mode":"PERSONAL","clientId":"clt_1","appCodes":["app1"]}""")
        }
        p.on("/field/auth/login") { p.token(claims = pinClaims) }
        p.on("/field/auth/refresh") { p.token(claims = pinClaims) }
        p.on("/field/auth/logout") { MockResponse().setResponseCode(204) }
        p.on("/field/auth/unlock-keys") { r ->
            if (r.body == null || r.body.length() == 0) {
                probeAnswer?.invoke() ?: p.error(400, "INVALID_REQUEST", "publicKeyJwk and keyProof are required")
            } else {
                registerAnswer?.invoke(r) ?: register(r)
            }
        }
        p.on("/field/auth/unlock/challenge") {
            challengeAnswer?.invoke() ?: run {
                val c = "challenge-" + (issued.size + 1)
                issued.add(c)
                MockResponse().setBody(JSONObject().put("challenge", c).put("expiresAt", "2030-01-01T00:00:00Z").toString())
            }
        }
        p.on("/field/auth/unlock") { r -> unlockAnswer?.invoke(r) ?: unlock(r) }
        p.on("/field/auth/unlock-keys/revoke") { r ->
            revokeAnswer?.invoke() ?: run {
                revoked.add(r.body!!.getString("unlockKeyId"))
                MockResponse().setResponseCode(204)
            }
        }
    }

    @After fun tearDown() { h.close() }

    /** The platform's registration: the key proof verifies with publicKeyJwk and signs this request's DPoP nonce. */
    private fun register(r: Rec): MockResponse {
        val body = r.body!!
        val jwk = body.getJSONObject("publicKeyJwk")
        val proof = body.getString("keyProof")
        val h0 = jwsHeader(proof)
        val c = jwsClaims(proof)
        val ok = verifyEs256(proof, jwk.getString("x"), jwk.getString("y")) &&
            h0.keySet() == setOf("typ", "alg") && h0.getString("typ") == "fc-unlock-key+jwt" &&
            c.getString("aud") == p.url + "/field/auth/unlock-keys" && c.getString("nonce") == r.proof!!.nonce &&
            c.getString("sub") == "w1" && c.getString("device_id") == "dev1" && r.bearer != null
        if (!ok) return p.error(401, "INVALID_UNLOCK_PROOF")
        registeredJwk = jwk
        return MockResponse().setResponseCode(201).setBody(
            JSONObject().put("unlockKeyId", "fwk_1").put("deviceId", "dev1").put("workerId", "w1")
                .put("createdAt", "2027-01-15T08:00:00Z").put("pinRequiredBy", "2027-02-14T08:00:00Z")
                .put("attestation", JSONObject().put("verdict", "TRUSTED")).toString(),
        )
    }

    /** The platform's unlock: the assertion verifies with the registered key and signs the last challenge. */
    private fun unlock(r: Rec): MockResponse {
        val body = r.body!!
        val jwk = registeredJwk ?: return p.error(401, "UNLOCK_KEY_INVALID")
        val a = body.getString("assertion")
        val hd = jwsHeader(a)
        val c = jwsClaims(a)
        val ok = verifyEs256(a, jwk.getString("x"), jwk.getString("y")) &&
            hd.getString("typ") == "fc-unlock+jwt" && hd.getString("kid") == body.getString("unlockKeyId") &&
            c.getString("aud") == p.url + "/field/auth/unlock" && c.getString("nonce") == issued.lastOrNull() &&
            c.getString("sub") == "w1" && c.getString("device_id") == "dev1" && c.getString("app") == "app1" &&
            body.getString("appCode") == "app1" && r.bearer == null
        if (!ok) return p.error(401, "INVALID_UNLOCK_PROOF")
        return p.token(claims = claims(listOf("hwk", "uv"), "fam2"))
    }

    private fun signedIn(): FieldSession = h.session().also {
        runBlocking {
            it.enrol("ABCDE12345", DeviceInfo())
            it.login(LoginCredentials(workerCode = "W001", pin = "1234"))
        }
    }

    private fun enabled(): FieldSession = signedIn().also { runBlocking { it.enableUnlockKeyWith(finger, prompt) } }

    private suspend fun refused(code: String, block: suspend () -> Unit): FieldAuthException {
        try {
            block()
        } catch (e: FieldAuthException) {
            assertEquals(code, e.code)
            return e
        }
        fail("expected $code")
        throw AssertionError()
    }

    // ── availability ─────────────────────────────────────────────────────

    @Test fun availableRightAfterAPinSignInOnAnEnrolledPersonalPhone() = runBlocking<Unit> {
        assertEquals(UnlockKeyAvailability.AVAILABLE, signedIn().unlockKeyAvailability())
    }

    @Test fun notOfferedOnAnUnenrolledOrSharedPhoneOrWithNobodySignedIn() = runBlocking<Unit> {
        val s = h.session()
        assertEquals(UnlockKeyAvailability.NOT_ENROLLED, s.unlockKeyAvailability())
        s.enrol("ABCDE12345", DeviceInfo())
        assertEquals(UnlockKeyAvailability.NOT_SIGNED_IN, s.unlockKeyAvailability())

        val h2 = Harness(keys = SoftKeyProvider("TEE"), unlockKeys = SoftUnlockKeys())
        try {
            h2.platform.on("/field/devices/enrol") {
                MockResponse().setResponseCode(201).setBody("""{"deviceId":"dev1","mode":"SHARED","clientId":"c","appCodes":[]}""")
            }
            h2.platform.on("/field/auth/login") { h2.platform.token(claims = h2.platform.deviceClaims("SHARED", "dev1").put("amr", JSONArray(listOf("pin"))).put("sid", "f")) }
            val shared = h2.session()
            shared.enrol("ABCDE12345", DeviceInfo())
            shared.login(LoginCredentials(workerCode = "W001", pin = "1234"))
            assertEquals(UnlockKeyAvailability.SHARED_DEVICE, shared.unlockKeyAvailability())
            refused("SHARED_DEVICE") { shared.enableUnlockKeyWith(finger, prompt) }
        } finally {
            h2.close()
        }
    }

    @Test fun onlyAFreshPinSignInOfTheWorkerThemselves() = runBlocking<Unit> {
        val s = signedIn()
        h.clock.advance(10 * MIN)
        assertEquals(UnlockKeyAvailability.PIN_SIGN_IN_REQUIRED, s.unlockKeyAvailability())
        refused("PIN_SIGN_IN_REQUIRED") { s.enableUnlockKeyWith(finger, prompt) }
        assertEquals(0, prompts.get())

        p.on("/field/auth/login") { p.token(claims = claims(listOf("badge"), "fam3")) }
        s.login(LoginCredentials(badgeToken = "B1"))
        assertEquals(UnlockKeyAvailability.PIN_SIGN_IN_REQUIRED, s.unlockKeyAvailability())

        p.on("/field/auth/login") { p.token(claims = claims(listOf("badge", "pin"), "fam4")) }
        s.login(LoginCredentials(badgeToken = "B1", pin = "1234"))
        assertEquals(UnlockKeyAvailability.AVAILABLE, s.unlockKeyAvailability())

        p.on("/field/auth/login") { p.token(claims = claims(listOf("pin"), "fam5").put("pcr", true), pinChangeRequired = true) }
        s.login(LoginCredentials(workerCode = "W001", pin = "1234"))
        assertEquals(UnlockKeyAvailability.PIN_SIGN_IN_REQUIRED, s.unlockKeyAvailability())
    }

    @Test fun aRefreshKeepsTheFamilyAndItsPinSignIn() = runBlocking<Unit> {
        val s = signedIn()
        h.clock.advance(5 * MIN)
        s.refresh()
        assertEquals(UnlockKeyAvailability.AVAILABLE, s.unlockKeyAvailability())
        // and the record survives a restart
        assertEquals(UnlockKeyAvailability.AVAILABLE, h.session().unlockKeyAvailability())
    }

    @Test fun neverForAnImpersonatedSession() = runBlocking<Unit> {
        val s = h.session()
        s.enrol("ABCDE12345", DeviceInfo())
        p.on("/field/auth/sign-in-code") {
            p.token(claims = claims(listOf("code", "imp"), "fam9").put("act", JSONObject().put("sub", "adm").put("typ", "principal").put("name", "A")))
        }
        s.redeemSignInCode("CODE1234")
        assertEquals(UnlockKeyAvailability.IMPERSONATING, s.unlockKeyAvailability())
        refused("IMPERSONATING") { s.enableUnlockKeyWith(finger, prompt) }
    }

    @Test fun thePhonesOwnLimits() = runBlocking<Unit> {
        val s = signedIn()
        keys.biometric = UnlockKeyAvailability.BIOMETRIC_NOT_ENROLLED
        assertEquals(UnlockKeyAvailability.BIOMETRIC_NOT_ENROLLED, s.unlockKeyAvailability())
        refused("BIOMETRIC_NOT_ENROLLED") { s.enableUnlockKeyWith(finger, prompt) }
        keys.biometric = UnlockKeyAvailability.BIOMETRIC_UNAVAILABLE
        assertEquals(UnlockKeyAvailability.BIOMETRIC_UNAVAILABLE, s.unlockKeyAvailability())
        keys.biometric = UnlockKeyAvailability.OS_TOO_OLD
        assertEquals(UnlockKeyAvailability.OS_TOO_OLD, s.unlockKeyAvailability())
        refused("UNLOCK_KEYS_UNSUPPORTED") { s.enableUnlockKeyWith(finger, prompt) }
        assertEquals(0, prompts.get())
        assertTrue(keys.aliases.isEmpty())
    }

    @Test fun aSoftwareKeystoreIsNotOffered() = runBlocking<Unit> {
        h.close()
        h = Harness(keys = SoftKeyProvider("SOFTWARE"), unlockKeys = keys)
        setUpAgain()
        assertEquals(UnlockKeyAvailability.NO_SECURE_HARDWARE, signedIn().unlockKeyAvailability())
    }

    @Test fun aKeyMadeOutsideSecureHardwareIsDeletedBeforeAnyPrompt() = runBlocking<Unit> {
        val s = signedIn()
        keys.level = "SOFTWARE"
        refused("UNLOCK_KEYS_UNSUPPORTED") { s.enableUnlockKeyWith(finger, prompt) }
        assertEquals(0, prompts.get())
        assertTrue(keys.aliases.isEmpty())
        assertEquals(0, p.all("/field/auth/unlock-keys").count { it.body?.has("keyProof") == true })
    }

    @Test fun withoutTheNativeLibraryThereAreNoUnlockKeys() = runBlocking<Unit> {
        h.close()
        h = Harness(keys = SoftKeyProvider("TEE"))
        setUpAgain()
        val s = signedIn()
        assertEquals(UnlockKeyAvailability.UNSUPPORTED, s.unlockKeyAvailability())
        refused("UNLOCK_KEYS_UNSUPPORTED") { s.enableUnlockKeyWith(finger, prompt) }
    }

    @Test fun askingThePlatformLearnsThePolicyWithoutAPromptAndRemembersItForADay() = runBlocking<Unit> {
        val s = signedIn()
        assertEquals(UnlockKeyAvailability.AVAILABLE, s.unlockKeyAvailability(askPlatform = true))
        val probe = p.all("/field/auth/unlock-keys").last()
        assertEquals(0, probe.body!!.length())
        assertNotNull(probe.bearer)

        probeAnswer = { p.error(403, "UNLOCK_KEYS_DISABLED") }
        assertEquals(UnlockKeyAvailability.POLICY_OFF, s.unlockKeyAvailability(askPlatform = true))
        val asked = p.count("/field/auth/unlock-keys")
        assertEquals(UnlockKeyAvailability.POLICY_OFF, s.unlockKeyAvailability(askPlatform = true))
        assertEquals(UnlockKeyAvailability.POLICY_OFF, s.unlockKeyAvailability())
        assertEquals(asked, p.count("/field/auth/unlock-keys")) // remembered: not asked again
        assertEquals(0, prompts.get())

        // enabling asks again (it never prompts first) and is refused the same way
        refused("UNLOCK_KEYS_DISABLED") { s.enableUnlockKeyWith(finger, prompt) }
        assertEquals(0, prompts.get())
        assertTrue(keys.aliases.isEmpty())

        probeAnswer = null
        h.clock.advance(24 * 60 * MIN)
        p.on("/field/auth/login") { p.token(claims = claims(listOf("pin"), "fam7")) }
        s.login(LoginCredentials(workerCode = "W001", pin = "1234"))
        assertEquals(UnlockKeyAvailability.AVAILABLE, s.unlockKeyAvailability(askPlatform = true))
    }

    @Test fun theProbeMapsTheSessionRefusals() = runBlocking<Unit> {
        val s = signedIn()
        for ((code, want) in listOf(
            "SHARED_DEVICE" to UnlockKeyAvailability.SHARED_DEVICE,
            "PIN_SIGN_IN_REQUIRED" to UnlockKeyAvailability.PIN_SIGN_IN_REQUIRED,
            "IMPERSONATING" to UnlockKeyAvailability.IMPERSONATING,
            "UNKNOWN_DEVICE" to UnlockKeyAvailability.NOT_ENROLLED,
        )) {
            probeAnswer = { p.error(if (code == "UNKNOWN_DEVICE") 401 else 403, code) }
            assertEquals(code, want, s.unlockKeyAvailability(askPlatform = true))
        }
        probeAnswer = null
        p.offline = true
        refused("NETWORK") { s.unlockKeyAvailability(askPlatform = true) }
        p.offline = false
    }

    // ── enabling ─────────────────────────────────────────────────────────

    @Test fun enablingMakesAnAttestedKeyAndRegistersItWithOnePrompt() = runBlocking<Unit> {
        val s = signedIn()
        val events = h.events.size
        val info = s.enableUnlockKeyWith(finger, prompt)
        assertEquals(1, prompts.get())
        assertEquals("fwk_1", info.unlockKeyId)
        assertEquals("w1", info.workerId)
        assertEquals("W001", info.workerCode)
        assertEquals("dev1", info.deviceId)
        assertEquals("TRUSTED", info.attestationVerdict)
        assertEquals(java.time.Instant.parse("2027-02-14T08:00:00Z").toEpochMilli(), info.pinRequiredBy)

        val reg = p.all("/field/auth/unlock-keys").last { it.body?.has("keyProof") == true }
        assertEquals(JSONArray(listOf("bGVhZg==", "cm9vdA==")).toString(), reg.body!!.getJSONArray("attestationChain").toString())
        assertEquals(setOf("kty", "crv", "x", "y"), reg.body.getJSONObject("publicKeyJwk").keySet())
        // The key is not the device key.
        assertTrue(reg.body.getJSONObject("publicKeyJwk").getString("x") != reg.proof!!.jwk.getString("x"))

        val want = MessageDigest.getInstance("SHA-256").digest("fc-unlock-key-attestation:v1\nfam1\nw1\ndev1".toByteArray())
        assertArrayEquals(want, keys.attestationChallenges.single())

        assertEquals(UnlockKeyAvailability.ENABLED, s.unlockKeyAvailability())
        assertEquals(info, s.unlockKey())
        assertEquals(info, h.session().unlockKey()) // kept across a restart
        assertTrue(h.events.drop(events).any { it is FieldEvent.UnlockKeyChanged && it.key == info })
    }

    @Test fun nothingIsKeptWhenThePromptIsCancelled() = runBlocking<Unit> {
        val s = signedIn()
        refused("BIOMETRIC_CANCELLED") { s.enableUnlockKeyWith(refusing("BIOMETRIC_CANCELLED"), prompt) }
        assertTrue(keys.aliases.isEmpty())
        assertNull(s.unlockKey())
        assertEquals(0, p.all("/field/auth/unlock-keys").count { it.body?.has("keyProof") == true })
    }

    @Test fun theRegistrationRefusals() = runBlocking<Unit> {
        val s = signedIn()
        for ((status, code) in listOf(
            409 to "UNLOCK_KEY_OTHER_WORKER", 403 to "PIN_SIGN_IN_REQUIRED", 403 to "WORKER_UNAVAILABLE",
            401 to "INVALID_UNLOCK_PROOF", 503 to "ATTESTATION_STATUS_UNAVAILABLE", 400 to "INVALID_PUBLIC_KEY",
        )) {
            registerAnswer = { p.error(status, code, retryAfter = if (status == 503) 300 else null) }
            val e = refused(code) { s.enableUnlockKeyWith(finger, prompt) }
            if (status == 503) assertEquals(300, e.retryAfterSeconds)
            assertTrue(code, keys.aliases.isEmpty())
            assertNull(s.unlockKey())
        }
    }

    @Test fun attestationRequiredIsRememberedForAWeek() = runBlocking<Unit> {
        val s = signedIn()
        registerAnswer = { p.error(422, "ATTESTATION_REQUIRED") }
        refused("ATTESTATION_REQUIRED") { s.enableUnlockKeyWith(finger, prompt) }
        assertTrue(keys.aliases.isEmpty())
        assertEquals(UnlockKeyAvailability.ATTESTATION_REQUIRED, s.unlockKeyAvailability(askPlatform = true))
        h.clock.advance(6 * 24 * 60 * MIN)
        p.on("/field/auth/login") { p.token(claims = claims(listOf("pin"), "fam8")) }
        s.login(LoginCredentials(workerCode = "W001", pin = "1234"))
        assertEquals(UnlockKeyAvailability.ATTESTATION_REQUIRED, s.unlockKeyAvailability())
        h.clock.advance(2 * 24 * 60 * MIN)
        p.on("/field/auth/login") { p.token(claims = claims(listOf("pin"), "fam9")) }
        s.login(LoginCredentials(workerCode = "W001", pin = "1234"))
        assertEquals(UnlockKeyAvailability.AVAILABLE, s.unlockKeyAvailability())
    }

    @Test fun aWorkerLockReportsTheEvent() = runBlocking<Unit> {
        val s = signedIn()
        registerAnswer = { p.error(429, "WORKER_LOCKED_TEMPORARILY", retryAfter = 60) }
        refused("WORKER_LOCKED_TEMPORARILY") { s.enableUnlockKeyWith(finger, prompt) }
        assertTrue(h.events.any { it is FieldEvent.WorkerLocked && !it.hard && it.retryAfterSeconds == 60 })
    }

    @Test fun aKeyThatExistsAlreadyIsMadeAgainOnce() = runBlocking<Unit> {
        val s = signedIn()
        var n = 0
        registerAnswer = { r -> if (n++ == 0) p.error(409, "UNLOCK_KEY_EXISTS") else register(r) }
        val info = s.enableUnlockKeyWith(finger, prompt)
        assertEquals("fwk_1", info.unlockKeyId)
        assertEquals(2, prompts.get())
        assertEquals(1, keys.aliases.size)
        assertEquals(1, keys.deleted.size)
    }

    @Test fun aNonceThatExpiredDuringTheFirstPromptCostsOneMorePrompt() = runBlocking<Unit> {
        val s = signedIn()
        var n = 0
        registerAnswer = { r ->
            if (n++ == 0) MockResponse().setResponseCode(401).setHeader("WWW-Authenticate", "DPoP error=\"use_dpop_nonce\"")
                .setBody("""{"error":"use_dpop_nonce"}""")
            else register(r)
        }
        assertEquals("fwk_1", s.enableUnlockKeyWith(finger, prompt).unlockKeyId)
        assertEquals(2, prompts.get())
    }

    @Test fun enablingAgainReplacesTheWorkersOwnKey() = runBlocking<Unit> {
        val s = enabled()
        val first = keys.aliases.single()
        s.enableUnlockKeyWith(finger, prompt)
        assertEquals(1, keys.aliases.size)
        assertTrue(first in keys.deleted)
        assertTrue(first !in keys.aliases)
    }

    @Test fun anotherWorkersKeyIsNotReplacedSilently() = runBlocking<Unit> {
        val s = enabled()
        p.on("/field/auth/login") { p.token("w2", "W002", claims(listOf("pin"), "famB", worker = "w2")) }
        s.login(LoginCredentials(workerCode = "W002", pin = "5678"))
        assertEquals(UnlockKeyAvailability.OTHER_WORKER, s.unlockKeyAvailability())
        refused("UNLOCK_KEY_OTHER_WORKER") { s.enableUnlockKeyWith(finger, prompt) }
        assertEquals(1, prompts.get())
        // the hand-over: the phone's holder disables the key, then registers their own
        s.disableUnlockKey()
        assertEquals(listOf("fwk_1"), revoked)
        assertEquals(UnlockKeyAvailability.AVAILABLE, s.unlockKeyAvailability())
    }

    // ── signing in with the key ──────────────────────────────────────────

    @Test fun signingInAsksForTheChallengeBeforeThePromptAndGivesASessionAsALoginDoes() = runBlocking<Unit> {
        enabled().logout()
        prompts.set(0)
        val s = h.session()
        val before = p.requests.size
        var challengedBeforePrompt = false
        val ses = s.loginWithUnlockKeyWith(
            { pr, sig, data ->
                challengedBeforePrompt = p.count("/field/auth/unlock/challenge") == 1 && p.count("/field/auth/unlock") == 0
                assertEquals(prompt, pr)
                prompts.incrementAndGet()
                sig.sign(data)
            },
            prompt,
        )
        assertTrue(challengedBeforePrompt)
        assertEquals(1, prompts.get())
        assertEquals(listOf("hwk", "uv"), ses.amr)
        assertEquals("w1", ses.worker.id)
        assertEquals(FieldState.SignedIn(ses), s.state.value)
        assertNotNull(s.headersFor("GET", "https://api.example.com/x")["Authorization"])
        val paths = p.requests.drop(before).map { it.path }.filter { it != "/field/auth/nonce" }
        assertEquals(listOf("/field/auth/unlock/challenge", "/field/auth/unlock"), paths)
        // a session from an unlock never registers the next key
        assertEquals(UnlockKeyAvailability.ENABLED, s.unlockKeyAvailability())
        assertTrue(h.events.any { it is FieldEvent.SessionChanged && it.session == ses })
    }

    @Test fun aSessionFromAnUnlockCannotMakeANewKey() = runBlocking<Unit> {
        enabled()
        val s = h.session()
        s.loginWithUnlockKeyWith(finger, prompt)
        s.disableUnlockKey()
        assertEquals(UnlockKeyAvailability.PIN_SIGN_IN_REQUIRED, s.unlockKeyAvailability())
    }

    @Test fun aRefusedChallengeNeverShowsAPrompt() = runBlocking<Unit> {
        enabled()
        prompts.set(0)
        val kept = listOf(
            Triple(401, "PIN_REAUTH_REQUIRED", true), Triple(403, "PIN_RESET_REQUIRED", true),
            Triple(503, "ATTESTATION_STATUS_UNAVAILABLE", true), Triple(423, "WORKER_LOCKED", true),
            Triple(429, "WORKER_LOCKED_TEMPORARILY", true), Triple(429, "TOO_MANY_ATTEMPTS", true),
            Triple(403, "DEVICE_SUSPENDED", true),
        )
        val s = h.session()
        for ((status, code, _) in kept) {
            challengeAnswer = { p.error(status, code, retryAfter = if (status == 503 || status == 429) 30 else null) }
            val e = refused(code) { s.loginWithUnlockKeyWith(finger, prompt) }
            if (status == 503) assertEquals(30, e.retryAfterSeconds)
            assertNotNull(code, s.unlockKey())
        }
        assertEquals(0, prompts.get())
        assertEquals(0, p.count("/field/auth/unlock"))
        assertTrue(h.events.any { it is FieldEvent.WorkerLocked && it.hard })
        assertTrue(revoked.isEmpty())
    }

    @Test fun anUnknownOrRevokedKeyIsDeletedAndRevoked() = runBlocking<Unit> {
        enabled()
        val s = h.session()
        challengeAnswer = { p.error(401, "UNLOCK_KEY_INVALID") }
        refused("UNLOCK_KEY_INVALID") { s.loginWithUnlockKeyWith(finger, prompt) }
        assertNull(s.unlockKey())
        assertTrue(keys.aliases.isEmpty())
        assertEquals(listOf("fwk_1"), revoked)
        assertTrue(h.events.any { it is FieldEvent.UnlockKeyChanged && it.key == null })
        refused(UnlockKeyCodes.NO_UNLOCK_KEY) { s.loginWithUnlockKeyWith(finger, prompt) }
    }

    @Test fun policyOffDeletesTheKey() = runBlocking<Unit> {
        enabled()
        val s = h.session()
        challengeAnswer = { p.error(403, "UNLOCK_KEYS_DISABLED") }
        refused("UNLOCK_KEYS_DISABLED") { s.loginWithUnlockKeyWith(finger, prompt) }
        assertNull(s.unlockKey())
        assertTrue(keys.aliases.isEmpty())
        assertTrue(revoked.isEmpty()) // the policy revoked it already
    }

    @Test fun aRevokedOrUnknownDeviceDeletesTheKey() = runBlocking<Unit> {
        for ((status, code) in listOf(403 to "DEVICE_REVOKED", 401 to "UNKNOWN_DEVICE", 403 to "SHARED_DEVICE")) {
            challengeAnswer = null
            enabled()
            val s = h.session()
            challengeAnswer = { p.error(status, code) }
            refused(code) { s.loginWithUnlockKeyWith(finger, prompt) }
            assertNull(code, s.unlockKey())
        }
    }

    @Test fun aKeyThePhoneInvalidatedIsRevokedAndDeletedBeforeAnyChallengeOrPrompt() = runBlocking<Unit> {
        enabled()
        prompts.set(0)
        keys.invalidated.addAll(keys.aliases)
        val s = h.session()
        refused(UnlockKeyCodes.UNLOCK_KEY_INVALIDATED) { s.loginWithUnlockKeyWith(finger, prompt) }
        assertEquals(0, prompts.get())
        assertEquals(0, p.count("/field/auth/unlock/challenge"))
        assertEquals(listOf("fwk_1"), revoked)
        assertNull(s.unlockKey())
        assertTrue(keys.aliases.isEmpty())
    }

    @Test fun anInvalidationReportedByThePromptIsHandledTheSameWay() = runBlocking<Unit> {
        enabled()
        val s = h.session()
        refused(UnlockKeyCodes.UNLOCK_KEY_INVALIDATED) { s.loginWithUnlockKeyWith(refusing("KEY_INVALIDATED"), prompt) }
        assertEquals(listOf("fwk_1"), revoked)
        assertNull(s.unlockKey())
        assertEquals(0, p.count("/field/auth/unlock"))
    }

    @Test fun aCancelledPromptKeepsTheKey() = runBlocking<Unit> {
        enabled()
        val s = h.session()
        for (code in listOf("BIOMETRIC_CANCELLED", UnlockKeyCodes.BIOMETRIC_LOCKOUT)) {
            refused(code) { s.loginWithUnlockKeyWith(refusing(code), prompt) }
            assertNotNull(s.unlockKey())
        }
        assertEquals(0, p.count("/field/auth/unlock"))
        assertTrue(s.state.value is FieldState.SignedOut || s.state.value is FieldState.SignedIn)
    }

    @Test fun noBiometricEnrolledAsksNothing() = runBlocking<Unit> {
        enabled()
        keys.biometric = UnlockKeyAvailability.BIOMETRIC_NOT_ENROLLED
        val s = h.session()
        val before = p.requests.size
        refused("BIOMETRIC_NOT_ENROLLED") { s.loginWithUnlockKeyWith(finger, prompt) }
        assertEquals(before, p.requests.size)
        assertNotNull(s.unlockKey())
    }

    @Test fun anExpiredChallengeIsAskedForAgainOnce() = runBlocking<Unit> {
        enabled()
        prompts.set(0)
        var n = 0
        unlockAnswer = { r -> if (n++ == 0) p.error(401, "CHALLENGE_EXPIRED") else unlock(r) }
        val ses = h.session().loginWithUnlockKeyWith(finger, prompt)
        assertEquals(listOf("hwk", "uv"), ses.amr)
        assertEquals(2, prompts.get())
        assertEquals(2, p.count("/field/auth/unlock/challenge"))
        unlockAnswer = { p.error(401, "CHALLENGE_EXPIRED") }
        refused("CHALLENGE_EXPIRED") { h.session().loginWithUnlockKeyWith(finger, prompt) }
    }

    @Test fun aBadSignatureTwiceDeletesTheKey() = runBlocking<Unit> {
        enabled()
        val s = h.session()
        unlockAnswer = { p.error(401, "INVALID_UNLOCK_PROOF") }
        refused("INVALID_UNLOCK_PROOF") { s.loginWithUnlockKeyWith(finger, prompt) }
        assertNotNull(s.unlockKey())
        refused("INVALID_UNLOCK_PROOF") { h.session().loginWithUnlockKeyWith(finger, prompt) }
        assertNull(s.unlockKey())
        assertEquals(listOf("fwk_1"), revoked)
    }

    @Test fun aSuccessResetsTheBadSignatureCount() = runBlocking<Unit> {
        enabled()
        val s = h.session()
        var n = 0
        unlockAnswer = { r -> if (n++ % 2 == 0) p.error(401, "INVALID_UNLOCK_PROOF") else unlock(r) }
        refused("INVALID_UNLOCK_PROOF") { s.loginWithUnlockKeyWith(finger, prompt) }
        s.loginWithUnlockKeyWith(finger, prompt)
        refused("INVALID_UNLOCK_PROOF") { s.loginWithUnlockKeyWith(finger, prompt) }
        assertNotNull(s.unlockKey())
    }

    @Test fun theOtherUnlockRefusalsKeepTheKey() = runBlocking<Unit> {
        enabled()
        val s = h.session()
        for ((status, code) in listOf(401 to "PIN_REAUTH_REQUIRED", 403 to "NO_APP_ACCESS", 423 to "WORKER_LOCKED")) {
            unlockAnswer = { p.error(status, code) }
            refused(code) { s.loginWithUnlockKeyWith(finger, prompt) }
            assertNotNull(code, s.unlockKey())
        }
        assertTrue(s.state.value is FieldState.SignedOut || s.state.value is FieldState.SignedIn)
    }

    @Test fun thePinDeadlineFollowsTheChallenge() = runBlocking<Unit> {
        enabled()
        challengeAnswer = {
            issued.add("c-x")
            MockResponse().setBody(JSONObject().put("challenge", "c-x").put("expiresAt", "2030-01-01T00:00:00Z").toString())
        }
        val s = h.session()
        s.loginWithUnlockKeyWith(finger, prompt)
        assertNull(s.unlockKey()!!.pinRequiredBy) // no periodic PIN: absent on the wire
    }

    @Test fun noKeyOrUnenrolled() = runBlocking<Unit> {
        val s = h.session()
        refused("NOT_ENROLLED") { s.loginWithUnlockKeyWith(finger, prompt) }
        s.enrol("ABCDE12345", DeviceInfo())
        refused(UnlockKeyCodes.NO_UNLOCK_KEY) { s.loginWithUnlockKeyWith(finger, prompt) }
    }

    @Test fun theKeySurvivesASignOutAndARestart() = runBlocking<Unit> {
        val s = enabled()
        s.logout()
        assertNotNull(s.unlockKey())
        assertNotNull(h.session().unlockKey())
        assertEquals("w1", h.session().loginWithUnlockKeyWith(finger, prompt).worker.id)
    }

    @Test fun aNewEnrolmentForgetsTheKeyOfTheOldDeviceRow() = runBlocking<Unit> {
        val s = enabled()
        p.on("/field/devices/enrol") {
            MockResponse().setResponseCode(201).setBody("""{"deviceId":"dev2","mode":"PERSONAL","clientId":"clt_1","appCodes":["app1"]}""")
        }
        s.enrol("ABCDE99999", DeviceInfo())
        assertNull(s.unlockKey())
        assertTrue(keys.aliases.isEmpty())
    }

    // ── disabling ────────────────────────────────────────────────────────

    @Test fun disablingRevokesWithTheDeviceAloneAndDeletesTheKey() = runBlocking<Unit> {
        val s = enabled()
        s.logout()
        s.disableUnlockKey()
        assertEquals(listOf("fwk_1"), revoked)
        val r = p.all("/field/auth/unlock-keys/revoke").single()
        assertNull(r.bearer)
        assertTrue(r.proof!!.valid())
        assertNull(s.unlockKey())
        assertTrue(keys.aliases.isEmpty())
        s.disableUnlockKey() // nothing left: no request
        assertEquals(1, p.count("/field/auth/unlock-keys/revoke"))
    }

    @Test fun disablingOfflineDeletesAtOnceAndRevokesLater() = runBlocking<Unit> {
        val s = enabled()
        p.offline = true
        s.disableUnlockKey()
        assertNull(s.unlockKey())
        assertTrue(keys.aliases.isEmpty())
        assertTrue(revoked.isEmpty())
        p.offline = false
        // the next platform call sends it (here: the next sign-in, also from a new process)
        h.session().login(LoginCredentials(workerCode = "W001", pin = "1234"))
        assertEquals(listOf("fwk_1"), revoked)
        h.session().login(LoginCredentials(workerCode = "W001", pin = "1234"))
        assertEquals(listOf("fwk_1"), revoked) // sent once
    }

    @Test fun aRevocationTheDeviceCanNoLongerSendIsDropped() = runBlocking<Unit> {
        val s = enabled()
        revokeAnswer = { p.error(403, "DEVICE_REVOKED") }
        s.disableUnlockKey()
        revokeAnswer = null
        s.login(LoginCredentials(workerCode = "W001", pin = "1234"))
        assertTrue(revoked.isEmpty())
        assertEquals(1, p.count("/field/auth/unlock-keys/revoke"))
    }

    @Test fun theAppUnlockIsSeparateAndUnchanged() = runBlocking<Unit> {
        val s = enabled()
        var shown = false
        s.unlockWith({ shown = true }, "x") // not locked: returns at once, no prompt, no key used
        assertFalse(shown)
        assertEquals(0, p.count("/field/auth/unlock/challenge"))
    }

    @Test fun noPinOrSecretInAnyUnlockRequest() = runBlocking<Unit> {
        val s = enabled()
        s.logout()
        h.session().loginWithUnlockKeyWith(finger, prompt)
        for (r in p.requests.filter { it.path.contains("unlock") }) {
            val text = r.body?.toString() ?: ""
            assertFalse(text.contains("1234"))
            assertFalse(text.contains("fwr_"))
        }
    }

    @Test fun biometricErrorsMap() {
        assertEquals("BIOMETRIC_CANCELLED", BiometricErrors.forUnlockKey(androidx.biometric.BiometricPrompt.ERROR_NEGATIVE_BUTTON).code)
        assertEquals("BIOMETRIC_CANCELLED", BiometricErrors.forUnlockKey(androidx.biometric.BiometricPrompt.ERROR_USER_CANCELED).code)
        assertEquals(UnlockKeyCodes.BIOMETRIC_LOCKOUT, BiometricErrors.forUnlockKey(androidx.biometric.BiometricPrompt.ERROR_LOCKOUT).code)
        assertEquals(UnlockKeyCodes.BIOMETRIC_LOCKOUT, BiometricErrors.forUnlockKey(androidx.biometric.BiometricPrompt.ERROR_LOCKOUT_PERMANENT).code)
        assertEquals(UnlockKeyCodes.BIOMETRIC_NOT_ENROLLED, BiometricErrors.forUnlockKey(androidx.biometric.BiometricPrompt.ERROR_NO_BIOMETRICS).code)
        assertEquals("BIOMETRIC_UNAVAILABLE", BiometricErrors.forUnlockKey(androidx.biometric.BiometricPrompt.ERROR_HW_UNAVAILABLE).code)
        assertEquals("INTERNAL", BiometricErrors.forUnlockKey(999).code)
    }

    /** setUp's handlers on a fresh harness (tests that change the harness's keys). */
    private fun setUpAgain() = install()
}
