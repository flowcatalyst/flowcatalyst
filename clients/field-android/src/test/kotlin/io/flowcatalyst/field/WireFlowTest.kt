package io.flowcatalyst.field

import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import org.json.JSONObject
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Before
import org.junit.Test

class WireFlowTest {
    private lateinit var h: Harness
    private val p get() = h.platform

    @Before fun setUp() { h = Harness() }
    @After fun tearDown() { h.close() }

    private fun ok(claims: JSONObject = JSONObject()) = { _: Rec -> p.token(claims = claims) }

    private fun failsWith(code: String, block: suspend () -> Unit): FieldAuthException {
        try {
            runBlocking { block() }
        } catch (e: FieldAuthException) {
            assertEquals(code, e.code)
            return e
        }
        fail("expected FieldAuthException $code")
        throw IllegalStateException()
    }

    // ── enrolment ────────────────────────────────────────────────────────

    @Test fun enrolByQrSetsPlatformUrlSendsKeyAndPersistsDevice() = runBlocking {
        p.on("/field/devices/enrol") { MockResponse().setResponseCode(201).setBody("""{"deviceId":"dev1","mode":"PERSONAL","clientId":"c1","appCodes":["app1"]}""") }
        val s = h.session(h.config(platformUrl = "")) // no configured URL: the QR is adopted
        val rec = s.enrol("fcfield:1:${p.url}:ABCDE12345", DeviceInfo("android", "Pixel 9", "van 3"))
        assertEquals(DeviceRecord("dev1", "PERSONAL", "c1", listOf("app1"), p.url), rec)
        // the nonce route is primed first, then the single-use nonce goes into the enrol proof
        assertEquals(listOf("/field/auth/nonce", "/field/devices/enrol"), p.requests.map { it.path })
        val body = p.all("/field/devices/enrol").single().body!!
        assertEquals("ABCDE12345", body.getString("code"))
        assertEquals("android", body.getString("platform"))
        assertEquals("Pixel 9", body.getString("model"))
        assertEquals("van 3", body.getString("label"))
        assertEquals(h.keys.key.x, body.getJSONObject("publicKeyJwk").getString("x"))
        assertEquals(h.keys.key.y, body.getJSONObject("publicKeyJwk").getString("y"))
        assertNotNull(h.storage.get(FieldSession.STORE_DEVICE))
    }

    @Test fun qrForAnotherOriginThanConfiguredIsRefusedAndNothingPersisted() {
        val s = h.session(h.config(platformUrl = "https://configured.example.com"))
        failsWith("PLATFORM_MISMATCH") { s.enrol("fcfield:1:${p.url}:ABCDE12345", DeviceInfo()) }
        assertTrue(p.requests.isEmpty())
        assertNull(h.storage.get(FieldSession.STORE_DEVICE))
    }

    @Test fun qrForTheConfiguredOriginIsAccepted() = runBlocking {
        p.on("/field/devices/enrol") { MockResponse().setResponseCode(201).setBody("""{"deviceId":"dev1","mode":"PERSONAL","clientId":"c1","appCodes":[]}""") }
        val rec = h.session().enrol("fcfield:1:${p.url}:ABCDE12345", DeviceInfo())
        assertEquals(p.url, rec.platformUrl)
    }

    @Test fun qrForAnAllowedOriginIsAccepted() = runBlocking {
        p.on("/field/devices/enrol") { MockResponse().setResponseCode(201).setBody("""{"deviceId":"dev1","mode":"PERSONAL","clientId":"c1","appCodes":[]}""") }
        val s = h.session(h.config(platformUrl = "https://configured.example.com").copy(allowedPlatformOrigins = listOf(p.url)))
        assertEquals(p.url, s.enrol("fcfield:1:${p.url}:ABCDE12345", DeviceInfo()).platformUrl)
    }

    @Test fun httpQrIsRefusedUnlessLoopback() = runBlocking {
        val s = h.session(h.config(platformUrl = ""))
        failsWith("INVALID_ENROLMENT_CODE") { s.enrol("fcfield:1:http://example.com:ABCDE12345", DeviceInfo()) }
        p.on("/field/devices/enrol") { MockResponse().setResponseCode(201).setBody("""{"deviceId":"dev1","mode":"PERSONAL","clientId":"c1","appCodes":[]}""") }
        // the mock platform is on a loopback host over http: accepted
        assertEquals(p.url, h.session(h.config(platformUrl = "")).enrol("fcfield:1:${p.url}:ABCDE12345", DeviceInfo()).platformUrl)
    }

    // A name under .invalid never resolves (RFC 2606): the request fails at DNS, immediately, and reaches no host.
    private val lan = "http://dev-platform.invalid:8080"

    @Test fun httpQrForTheConfiguredHttpOriginPassesThePolicy() {
        // getting past the policy means the request is attempted (and fails with a network error)
        val e = try { runBlocking { h.session(h.config(platformUrl = lan)).enrol("fcfield:1:$lan:ABCDE12345", DeviceInfo()) }; null } catch (e: FieldAuthException) { e }
        assertTrue(e == null || (e.code != "INVALID_ENROLMENT_CODE" && e.code != "PLATFORM_MISMATCH"))
    }

    @Test fun httpQrForAnAllowedOriginPassesThePolicy() {
        val s = h.session(h.config(platformUrl = "https://configured.example.com").copy(allowedPlatformOrigins = listOf(lan)))
        val e = try { runBlocking { s.enrol("fcfield:1:$lan:ABCDE12345", DeviceInfo()) }; null } catch (e: FieldAuthException) { e }
        assertTrue(e == null || (e.code != "INVALID_ENROLMENT_CODE" && e.code != "PLATFORM_MISMATCH"))
    }

    @Test fun lanHttpQrWithNoConfiguredUrlIsRefused() {
        failsWith("INVALID_ENROLMENT_CODE") { h.session(h.config(platformUrl = "")).enrol("fcfield:1:$lan:ABCDE12345", DeviceInfo()) }
        assertTrue(p.requests.isEmpty())
    }

    @Test fun configuredHttpsOriginWithHttpQrForTheSameHostIsAMismatch() {
        failsWith("PLATFORM_MISMATCH") { h.session(h.config(platformUrl = "https://dev-platform.invalid:8080")).enrol("fcfield:1:$lan:ABCDE12345", DeviceInfo()) }
    }

    @Test fun enrolWithPlainCodeUsesConfiguredPlatform() = runBlocking {
        p.on("/field/devices/enrol") { MockResponse().setResponseCode(201).setBody("""{"deviceId":"dev1","mode":"SHARED","clientId":"c1","appCodes":[]}""") }
        val rec = h.session().enrol("  ABCDE12345 ", DeviceInfo())
        assertEquals("SHARED", rec.mode)
        assertEquals("ABCDE12345", p.all("/field/devices/enrol").single().body!!.getString("code"))
    }

    @Test fun badQrIsInvalidEnrolmentCode() {
        for (bad in listOf("fcfield:1:nocode", "fcfield:1:ftp://x.example:CODE", "fcfield:1:${p.url}:", "fcfield:1:${p.url}:CO DE")) {
            failsWith("INVALID_ENROLMENT_CODE") { h.session().enrol(bad, DeviceInfo()) }
        }
        assertTrue(p.requests.isEmpty())
    }

    @Test fun enrolErrorCodePassesThrough() {
        p.on("/field/devices/enrol") { p.error(401, "INVALID_ENROLMENT_CODE") }
        val e = failsWith("INVALID_ENROLMENT_CODE") { h.session().enrol("ABCDE12345", DeviceInfo()) }
        assertEquals(401, e.status)
    }

    // ── login ────────────────────────────────────────────────────────────

    @Test fun unenrolledLoginByWorkerCodeNeedsClientIdentifier() {
        val s = h.session(h.config(clientIdentifier = null))
        failsWith("INVALID_REQUEST") { s.login(LoginCredentials(workerCode = "W001", pin = "1234")) }
        assertTrue(p.requests.isEmpty())
    }

    @Test fun unenrolledLoginWorksAndIsUnenrolled() = runBlocking {
        p.on("/field/auth/login", ok(JSONObject().put("device_mode", "UNENROLLED")))
        val s = h.session()
        val ses = s.login(LoginCredentials(workerCode = "W001", pin = "1234"))
        assertEquals(DeviceMode.UNENROLLED, ses.deviceMode)
        assertNull(ses.deviceId)
        assertNull(ses.actor)
        assertEquals("W001", ses.worker.workerCode)
        assertEquals("app1", ses.appCode)
        assertEquals(h.clock.now() + 600_000, ses.expiresAt)
        val body = p.all("/field/auth/login").single().body!!
        assertEquals("app1", body.getString("appCode"))
        assertEquals("W001", body.getString("workerCode"))
        assertEquals("1234", body.getString("pin"))
        assertEquals("acme", body.getString("clientIdentifier"))
        assertFalse(body.has("badgeToken"))
        assertEquals(FieldState.SignedIn(ses), s.state.value)
        assertTrue(h.events.contains(FieldEvent.SessionChanged(ses)))
    }

    @Test fun perCallClientIdentifierOverridesConfig() = runBlocking {
        p.on("/field/auth/login", ok())
        h.session().login(LoginCredentials(workerCode = "W001", pin = "1234", clientIdentifier = "other"))
        assertEquals("other", p.all("/field/auth/login").single().body!!.getString("clientIdentifier"))
    }

    @Test fun enrolledLoginDoesNotNeedClientIdentifierAndTakesDeviceFromClaims() = runBlocking {
        p.on("/field/devices/enrol") { MockResponse().setResponseCode(201).setBody("""{"deviceId":"dev1","mode":"PERSONAL","clientId":"c1","appCodes":["app1"]}""") }
        p.on("/field/auth/login", ok(p.deviceClaims("PERSONAL", "dev1")))
        val s = h.session(h.config(clientIdentifier = null))
        s.enrol("ABCDE12345", DeviceInfo())
        val ses = s.login(LoginCredentials(workerCode = "W001", pin = "1234"))
        assertEquals(DeviceMode.PERSONAL, ses.deviceMode)
        assertEquals("dev1", ses.deviceId)
        assertFalse(p.all("/field/auth/login").single().body!!.has("clientIdentifier"))
    }

    @Test fun badgeLoginNormalisesAndSendsBadgeToken() = runBlocking {
        p.on("/field/auth/login", ok())
        h.session().login(LoginCredentials(badgeToken = "  ab-12\r\n", pin = "1234"))
        val body = p.all("/field/auth/login").single().body!!
        assertEquals("AB-12", body.getString("badgeToken"))
        assertFalse(body.has("workerCode"))
    }

    @Test fun badgeOnlyLoginOmitsPin() = runBlocking {
        p.on("/field/auth/login", ok())
        h.session().login(LoginCredentials(badgeToken = "AB12"))
        assertFalse(p.all("/field/auth/login").single().body!!.has("pin"))
    }

    @Test fun loginNeedsExactlyOneOfCodeAndBadge() {
        val s = h.session()
        failsWith("INVALID_REQUEST") { s.login(LoginCredentials(pin = "1234")) }
        failsWith("INVALID_REQUEST") { s.login(LoginCredentials(workerCode = "W", badgeToken = "B", pin = "1234")) }
    }

    @Test fun wrongPinKeepsCodeAndNeverEchoesPin() {
        p.on("/field/auth/login") { p.error(401, "INVALID_CREDENTIALS", "wrong pin 987654") }
        val e = failsWith("INVALID_CREDENTIALS") { h.session().login(LoginCredentials(workerCode = "W001", pin = "987654")) }
        assertFalse(e.message!!.contains("987654"))
        assertEquals(401, e.status)
    }

    @Test fun lockoutsRaiseWorkerLockedEvents() {
        val s = h.session()
        p.on("/field/auth/login") { p.error(429, "WORKER_LOCKED_TEMPORARILY", retryAfter = 30) }
        val e = failsWith("WORKER_LOCKED_TEMPORARILY") { s.login(LoginCredentials(workerCode = "W", pin = "1234")) }
        assertEquals(30, e.retryAfterSeconds)
        assertTrue(h.events.contains(FieldEvent.WorkerLocked(false, 30)))
        p.on("/field/auth/login") { p.error(423, "WORKER_LOCKED") }
        failsWith("WORKER_LOCKED") { s.login(LoginCredentials(workerCode = "W", pin = "1234")) }
        assertTrue(h.events.contains(FieldEvent.WorkerLocked(true, null)))
        p.on("/field/auth/login") { p.error(429, "TOO_MANY_ATTEMPTS", retryAfter = 60) }
        failsWith("TOO_MANY_ATTEMPTS") { s.login(LoginCredentials(workerCode = "W", pin = "1234")) }
        assertTrue(h.events.contains(FieldEvent.WorkerLocked(false, 60)))
    }

    @Test fun pinResetRequiredIsTypedAndNotRetriedOrLocked() {
        val s = h.session()
        p.on("/field/auth/login") { p.error(403, "PIN_RESET_REQUIRED", "pin reset required") }
        val e = failsWith("PIN_RESET_REQUIRED") { s.login(LoginCredentials(workerCode = "W", pin = "1234")) }
        assertTrue(e.isPinResetRequired)
        assertEquals(403, e.status)
        assertEquals(1, p.count("/field/auth/login"))
        assertTrue(h.events.none { it is FieldEvent.WorkerLocked })
        assertFalse(h.events.any { it is FieldEvent.SessionEnded })
        assertEquals(FieldState.SignedOut, s.state.value)
        assertFalse(FieldAuthException("INVALID_CREDENTIALS").isPinResetRequired)
    }

    @Test fun actAsPinResetRequiredLeavesTheSupervisorSession() = runBlocking {
        val s = h.session()
        val sup = supervisorLogin(s)
        p.on("/field/auth/act-as") { p.error(403, "PIN_RESET_REQUIRED", "pin reset required") }
        val e = failsWith("PIN_RESET_REQUIRED") { s.actAs(ActAsTarget(workerCode = "W002", pin = "9999")) }
        assertTrue(e.isPinResetRequired)
        assertEquals(1, p.count("/field/auth/act-as"))
        assertEquals(FieldState.SignedIn(sup), s.state.value)
    }

    @Test fun pinPolicyViolationsAreReturned() {
        p.on("/field/auth/login", ok())
        p.on("/field/auth/pin") {
            MockResponse().setResponseCode(422).setBody("""{"error":"PIN_POLICY","message":"no","violations":["TOO_SHORT","SEQUENTIAL"]}""")
        }
        val s = h.session()
        runBlocking { s.login(LoginCredentials(workerCode = "W001", pin = "1234")) }
        val e = failsWith("PIN_POLICY") { s.changePin("1234", "1111") }
        assertEquals(listOf("TOO_SHORT", "SEQUENTIAL"), e.violations)
    }

    @Test fun pinChangeRequiredEmitsEvent() = runBlocking {
        p.on("/field/auth/login") { p.token(pinChangeRequired = true) }
        val ses = h.session().login(LoginCredentials(workerCode = "W001", pin = "1234"))
        assertTrue(ses.pinChangeRequired)
        assertTrue(h.events.contains(FieldEvent.PinChangeRequired(ses)))
    }

    @Test fun networkFailureIsNetworkError() {
        val s = h.session(h.config(platformUrl = "http://127.0.0.1:1"))
        failsWith("NETWORK") { s.login(LoginCredentials(workerCode = "W001", pin = "1234")) }
    }

    @Test fun malformedTokenResponseIsInternal() {
        p.on("/field/auth/login") { MockResponse().setBody("""{"accessToken":"x"}""") }
        failsWith("INTERNAL") { h.session().login(LoginCredentials(workerCode = "W001", pin = "1234")) }
    }

    // ── sign-in code ─────────────────────────────────────────────────────

    private val adminActor get() = JSONObject().put("act", JSONObject().put("sub", "adm1").put("typ", "principal").put("name", "Ada Admin"))

    @Test fun signInCodeRedeemsWithActorAndNoSupervisor() = runBlocking {
        p.on("/field/auth/sign-in-code", ok(adminActor))
        val s = h.session()
        val ses = s.redeemSignInCode(" abcd-1234\n")
        assertEquals("ABCD-1234", p.all("/field/auth/sign-in-code").single().body!!.getString("code"))
        assertEquals(Actor("adm1", "principal", "Ada Admin"), ses.actor)
        assertNull(h.storage.get(FieldSession.STORE_SUPERVISOR))
        // nothing to return to: ending the impersonation just ends the session
        p.on("/field/auth/logout") { MockResponse().setResponseCode(204) }
        assertNull(s.endImpersonation())
        assertEquals(FieldState.SignedOut, s.state.value)
        assertEquals("session", p.all("/field/auth/logout").single().body!!.getString("scope"))
    }

    @Test fun emptySignInCodeIsRejected() {
        failsWith("INVALID_REQUEST") { h.session().redeemSignInCode("  \n") }
    }

    @Test fun endImpersonationWithoutImpersonationFails() {
        p.on("/field/auth/login", ok())
        val s = h.session()
        runBlocking { s.login(LoginCredentials(workerCode = "W001", pin = "1234")) }
        failsWith("NOT_IMPERSONATING") { s.endImpersonation() }
    }

    // ── act-as, supervisor kept aside ────────────────────────────────────

    private fun supervisorLogin(s: FieldSession): Session = runBlocking {
        var n = 0
        p.on("/field/auth/login") { p.token("sup", "S001") }
        p.on("/field/auth/act-as") {
            n++
            p.token("w2", "W002", JSONObject().put("act", JSONObject().put("sub", "sup").put("typ", "field_worker").put("name", "Name S001")))
        }
        p.on("/field/auth/logout") { MockResponse().setResponseCode(204) }
        s.login(LoginCredentials(workerCode = "S001", pin = "9999"))
    }

    @Test fun actAsKeepsSupervisorAsideAndEndImpersonationRestoresIt() = runBlocking {
        val s = h.session()
        val sup = supervisorLogin(s)
        val supToken = p.all("/field/auth/login").single().let { "" }
        val worker = s.actAs(ActAsTarget(workerCode = "W002", pin = "9999"))
        assertEquals("W002", worker.worker.workerCode)
        assertEquals(Actor("sup", "field_worker", "Name S001"), worker.actor)
        // act-as is a bearer call by the supervisor, bound to its access token
        val req = p.all("/field/auth/act-as").single()
        assertNotNull(req.bearer)
        assertEquals(sha256b64u(req.bearer!!), req.proof!!.claims.getString("ath"))
        assertEquals("W002", req.body!!.getString("workerCode"))
        assertFalse(req.body!!.has("workerId"))
        // the supervisor is kept aside in storage, the active refresh token is the worker's
        assertNotNull(h.storage.get(FieldSession.STORE_SUPERVISOR))
        assertEquals("fwr_1", h.storage.get(FieldSession.STORE_SUPERVISOR_REFRESH))
        assertEquals("fwr_2", h.storage.get(FieldSession.STORE_REFRESH))
        // a second act-as is refused
        try { s.actAs(ActAsTarget(workerId = "w3", pin = "9999")); fail() } catch (e: FieldAuthException) { assertEquals("IMPERSONATING", e.code) }
        try { s.changePin("1", "2"); fail() } catch (e: FieldAuthException) { assertEquals("IMPERSONATING", e.code) }

        val back = s.endImpersonation()
        assertEquals(sup, back)
        // the worker's family was logged out with the worker's token
        assertEquals("fwr_2", p.all("/field/auth/logout").single().body!!.getString("refreshToken"))
        // the supervisor's access token was still valid: no refresh
        assertEquals(0, p.count("/field/auth/refresh"))
        assertEquals("fwr_1", h.storage.get(FieldSession.STORE_REFRESH))
        assertNull(h.storage.get(FieldSession.STORE_SUPERVISOR))
        assertNull(h.storage.get(FieldSession.STORE_SUPERVISOR_REFRESH))
        assertEquals(FieldState.SignedIn(sup), s.state.value)
    }

    @Test fun actAsByWorkerIdAndValidation() = runBlocking {
        val s = h.session()
        supervisorLogin(s)
        failsWith("INVALID_REQUEST") { s.actAs(ActAsTarget(pin = "9999")) }
        failsWith("INVALID_REQUEST") { s.actAs(ActAsTarget(workerCode = "a", workerId = "b", pin = "9999")) }
        s.actAs(ActAsTarget(workerId = "w2", pin = "9999"))
        val body = p.all("/field/auth/act-as").single().body!!
        assertEquals("w2", body.getString("workerId"))
        assertFalse(body.has("workerCode"))
    }

    @Test fun endImpersonationRefreshesALapsedSupervisorToken() = runBlocking {
        val s = h.session()
        val sup = supervisorLogin(s)
        p.on("/field/auth/refresh") { p.token("sup", "S001") }
        s.actAs(ActAsTarget(workerCode = "W002", pin = "9999"))
        h.clock.advance(11 * MIN) // the supervisor's access token (10 min) has lapsed
        val back = s.endImpersonation()!!
        assertEquals("S001", back.worker.workerCode)
        assertEquals("fwr_1", p.all("/field/auth/refresh").single().body!!.getString("refreshToken"))
        assertTrue(back.expiresAt > h.clock.now())
        assertNotNull(sup)
    }

    @Test fun supervisorSurvivesProcessRestart() = runBlocking {
        val s = h.session()
        val sup = supervisorLogin(s)
        s.actAs(ActAsTarget(workerCode = "W002", pin = "9999"))
        // a "new process": fresh session over the same storage
        val s2 = h.session()
        val st = s2.restore()
        assertTrue(st is FieldState.SignedIn)
        assertEquals("W002", (st as FieldState.SignedIn).session.worker.workerCode)
        assertNotNull(st.session.actor)
        try { s2.actAs(ActAsTarget(workerCode = "W003", pin = "9999")); fail() } catch (e: FieldAuthException) { assertEquals("IMPERSONATING", e.code) }
        p.on("/field/auth/refresh") { p.token("sup", "S001") }
        val back = s2.endImpersonation()!!
        assertEquals(sup.worker, back.worker)
        // the worker was logged out with the restored worker token, the supervisor refreshed with its stored one
        assertEquals("fwr_2", p.all("/field/auth/logout").single().body!!.getString("refreshToken"))
        assertEquals("fwr_1", p.all("/field/auth/refresh").single().body!!.getString("refreshToken"))
        assertNull(h.storage.get(FieldSession.STORE_SUPERVISOR))
    }

    @Test fun lostSupervisorTokenFailsBeforeTearingDown() = runBlocking {
        val s = h.session()
        supervisorLogin(s)
        s.actAs(ActAsTarget(workerCode = "W002", pin = "9999"))
        val s2 = h.session()
        s2.restore()
        h.storage.remove(FieldSession.STORE_SUPERVISOR_REFRESH)
        failsWith("SESSION_EXPIRED") { s2.endImpersonation() }
        assertEquals(0, p.count("/field/auth/logout"))
        assertTrue(s2.state.value is FieldState.SignedIn)
    }

    @Test fun impersonationEndingOnItsOwnHandsBackToSupervisor() = runBlocking {
        val s = h.session()
        val sup = supervisorLogin(s)
        s.actAs(ActAsTarget(workerCode = "W002", pin = "9999"))
        p.on("/field/auth/refresh") { p.error(401, "SESSION_EXPIRED") }
        h.clock.advance(11 * MIN)
        try { s.refresh(); fail() } catch (e: FieldAuthException) { assertEquals("SESSION_EXPIRED", e.code) }
        assertEquals(FieldState.SignedIn(sup), s.state.value)
        assertTrue(h.events.contains(FieldEvent.SessionEnded("SESSION_EXPIRED")))
        assertEquals("fwr_1", h.storage.get(FieldSession.STORE_REFRESH))
    }

    // ── PIN change, logout ───────────────────────────────────────────────

    @Test fun changePinIsABearerCallThenRefreshes() = runBlocking {
        p.on("/field/auth/login", ok())
        p.on("/field/auth/pin") { MockResponse().setResponseCode(204) }
        p.on("/field/auth/refresh") { p.token() }
        val s = h.session()
        s.login(LoginCredentials(workerCode = "W001", pin = "1234"))
        s.changePin("1234", "8642")
        val req = p.all("/field/auth/pin").single()
        assertEquals("1234", req.body!!.getString("currentPin"))
        assertEquals("8642", req.body!!.getString("newPin"))
        assertNotNull(req.proof!!.claims.getString("ath"))
        assertEquals(1, p.count("/field/auth/refresh"))
    }

    @Test fun logoutSendsScopeAndClearsEverything() = runBlocking {
        p.on("/field/auth/login", ok())
        p.on("/field/auth/logout") { MockResponse().setResponseCode(204) }
        val s = h.session()
        s.login(LoginCredentials(workerCode = "W001", pin = "1234"))
        s.logout()
        val body = p.all("/field/auth/logout").single().body!!
        assertEquals("fwr_1", body.getString("refreshToken"))
        assertEquals("session", body.getString("scope"))
        assertEquals(FieldState.SignedOut, s.state.value)
        assertNull(h.storage.get(FieldSession.STORE_REFRESH))
        assertNull(h.storage.get(FieldSession.STORE_SESSION))
        assertTrue(h.events.contains(FieldEvent.SessionEnded("logout")))
        assertTrue(h.events.contains(FieldEvent.SessionChanged(null)))
    }

    @Test fun deviceAllLogoutNeedsEnrolmentAndUsesWireScope() = runBlocking {
        p.on("/field/auth/login", ok())
        p.on("/field/devices/enrol") { MockResponse().setResponseCode(201).setBody("""{"deviceId":"dev1","mode":"SHARED","clientId":"c1","appCodes":[]}""") }
        p.on("/field/auth/logout") { MockResponse().setResponseCode(204) }
        val s = h.session()
        s.login(LoginCredentials(workerCode = "W001", pin = "1234"))
        failsWith("INVALID_REQUEST") { s.logout(LogoutScope.DEVICE_ALL) }
        s.enrol("ABCDE12345", DeviceInfo())
        s.logout(LogoutScope.DEVICE_ALL)
        assertEquals("device-all", p.all("/field/auth/logout").single().body!!.getString("scope"))
    }

    @Test fun logoutClearsLocalStateEvenWhenThePlatformIsDown() = runBlocking {
        p.on("/field/auth/login", ok())
        val s = h.session()
        s.login(LoginCredentials(workerCode = "W001", pin = "1234"))
        p.on("/field/auth/logout") { p.error(500, "INTERNAL") }
        failsWith("INTERNAL") { s.logout() }
        assertEquals(FieldState.SignedOut, s.state.value)
        assertNull(h.storage.get(FieldSession.STORE_REFRESH))
    }

    // ── refresh, storage ─────────────────────────────────────────────────

    @Test fun refreshRotatesAndPersistsTheToken() = runBlocking {
        p.on("/field/auth/login", ok())
        p.on("/field/auth/refresh") { p.token() }
        val s = h.session()
        s.login(LoginCredentials(workerCode = "W001", pin = "1234"))
        h.clock.advance(5 * MIN)
        s.refresh()
        s.refresh()
        assertEquals("fwr_1", p.all("/field/auth/refresh")[0].body!!.getString("refreshToken"))
        assertEquals("fwr_2", p.all("/field/auth/refresh")[1].body!!.getString("refreshToken"))
        assertEquals("fwr_3", h.storage.get(FieldSession.STORE_REFRESH))
    }

    @Test fun storageRoundTripRestoresSessionAndRefreshesFromStoredToken() = runBlocking {
        p.on("/field/auth/login", ok(p.deviceClaims("PERSONAL", "dev1").put("test", true)))
        p.on("/field/auth/refresh") { p.token(claims = p.deviceClaims("PERSONAL", "dev1").put("test", true)) }
        val s = h.session()
        val ses = s.login(LoginCredentials(workerCode = "W001", pin = "1234"))
        assertTrue(ses.test)
        val s2 = h.session()
        assertEquals(FieldState.SignedOut, s2.state.value) // not loaded yet
        assertEquals(FieldState.SignedIn(ses), s2.restore())
        // the access token is not persisted: the first use refreshes with the stored token
        val headers = s2.headersFor("GET", "${p.url}/x")
        assertNotNull(headers["DPoP"])
        assertEquals("fwr_1", p.all("/field/auth/refresh").single().body!!.getString("refreshToken"))
        assertEquals("fwr_2", h.storage.get(FieldSession.STORE_REFRESH))
    }

    @Test fun nothingSensitiveInPlainSessionJson() = runBlocking {
        p.on("/field/auth/login", ok())
        h.session().login(LoginCredentials(workerCode = "W001", pin = "1234"))
        val all = h.storage.keys.joinToString { h.storage.get(it)!! }
        assertFalse(all.contains("1234"))
        assertFalse(h.storage.get(FieldSession.STORE_SESSION)!!.contains("fwr_"))
    }

    @Test fun refreshWithoutAnySessionFails() {
        failsWith("SESSION_EXPIRED") { h.session().refresh() }
        failsWith("SESSION_EXPIRED") { h.session().headersFor("GET", "${p.url}/x") }
        assertTrue(h.events.isEmpty()) // nothing to end, nothing to announce
    }
}
