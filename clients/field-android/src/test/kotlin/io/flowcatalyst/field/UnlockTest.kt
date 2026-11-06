package io.flowcatalyst.field

import kotlinx.coroutines.runBlocking
import org.json.JSONObject
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Before
import org.junit.Test
import kotlin.time.Duration.Companion.minutes

class UnlockTest {
    private lateinit var h: Harness
    private val p get() = h.platform
    private val policy = UnlockPolicy(afterColdStart = true, afterBackground = 5.minutes)

    @Before fun setUp() { h = Harness(); p.on("/field/auth/refresh") { p.token(claims = p.deviceClaims()) } }
    @After fun tearDown() { h.close() }

    private fun enrolledLogin(claims: JSONObject = p.deviceClaims("PERSONAL", "dev1"), cfg: FieldConfig = h.config(requireUnlock = policy)): FieldSession {
        p.on("/field/devices/enrol") { okhttp3.mockwebserver.MockResponse().setResponseCode(201).setBody("""{"deviceId":"dev1","mode":"PERSONAL","clientId":"c1","appCodes":["app1"]}""") }
        p.on("/field/auth/login") { p.token(claims = claims) }
        return h.session(cfg).also {
            runBlocking {
                it.enrol("ABCDE12345", DeviceInfo())
                it.login(LoginCredentials(workerCode = "W001", pin = "1234"))
            }
        }
    }

    private val ok = UnlockPrompter { }

    @Test fun freshLoginIsNotLockedButColdStartIs() = runBlocking {
        val s = enrolledLogin()
        assertTrue(s.state.value is FieldState.SignedIn)
        val s2 = h.session(h.config(requireUnlock = policy))
        assertTrue(s2.restore() is FieldState.Locked)
        assertTrue(h.events.any { it is FieldEvent.Locked })
    }

    @Test fun lockedDoesNotBlockHeadersOrRefresh() = runBlocking {
        enrolledLogin()
        val s2 = h.session(h.config(requireUnlock = policy))
        assertTrue(s2.restore() is FieldState.Locked)
        assertNotNull(s2.headersFor("GET", "https://api.example.com/x")["DPoP"]) // refreshes in the background
        assertEquals(1, p.count("/field/auth/refresh"))
        assertTrue(s2.state.value is FieldState.Locked) // a refresh does not unlock
    }

    @Test fun unlockClearsTheLock() = runBlocking {
        enrolledLogin()
        val s2 = h.session(h.config(requireUnlock = policy))
        s2.restore()
        var shown: String? = null
        val ses = s2.unlockWith({ shown = it }, "Unlock fulfil")
        assertEquals("Unlock fulfil", shown)
        assertEquals(FieldState.SignedIn(ses), s2.state.value)
    }

    @Test fun cancelledPromptStaysLocked() = runBlocking {
        enrolledLogin()
        val s2 = h.session(h.config(requireUnlock = policy))
        s2.restore()
        try {
            s2.unlockWith({ throw FieldAuthException("BIOMETRIC_CANCELLED") }, "x")
            fail()
        } catch (e: FieldAuthException) { assertEquals("BIOMETRIC_CANCELLED", e.code) }
        assertTrue(s2.state.value is FieldState.Locked)
    }

    @Test fun backgroundTimeLocksOnlyAfterTheLimit() = runBlocking {
        val s = enrolledLogin()
        s.noteBackgrounded()
        h.clock.advance(4 * MIN)
        s.noteForegrounded()
        assertTrue(s.state.value is FieldState.SignedIn)
        s.noteBackgrounded()
        h.clock.advance(5 * MIN)
        s.noteForegrounded()
        assertTrue(s.state.value is FieldState.Locked)
    }

    @Test fun unenrolledNeverLocks() = runBlocking {
        p.on("/field/auth/login") { p.token(claims = p.deviceClaims("UNENROLLED").also { it.remove("device_id") }) }
        h.session(h.config(requireUnlock = policy)).login(LoginCredentials(workerCode = "W001", pin = "1234"))
        val s2 = h.session(h.config(requireUnlock = policy))
        assertTrue(s2.restore() is FieldState.SignedIn)
        try { s2.unlockWith(ok, "x"); fail() } catch (e: FieldAuthException) { assertEquals("BIOMETRIC_UNAVAILABLE", e.code) }
    }

    @Test fun sharedDeviceNeverLocks() = runBlocking {
        enrolledLogin(claims = p.deviceClaims("SHARED", "dev1"))
        assertTrue(h.session(h.config(requireUnlock = policy)).restore() is FieldState.SignedIn)
    }

    @Test fun impersonatedSessionNeverLocks() = runBlocking {
        val act = JSONObject().put("act", JSONObject().put("sub", "adm").put("typ", "principal").put("name", "A")).put("device_mode", "PERSONAL").put("device_id", "dev1")
        p.on("/field/auth/sign-in-code") { p.token(claims = act) }
        val s = h.session(h.config(requireUnlock = policy))
        s.redeemSignInCode("CODE1234")
        assertTrue(h.session(h.config(requireUnlock = policy)).restore() is FieldState.SignedIn)
        s.noteBackgrounded(); h.clock.advance(10 * MIN); s.noteForegrounded()
        assertTrue(s.state.value is FieldState.SignedIn)
    }

    @Test fun unlockWhenNotLockedJustReturnsTheSession() = runBlocking {
        val s = enrolledLogin()
        var prompted = false
        s.unlockWith({ prompted = true }, "x")
        assertTrue(!prompted)
    }

    @Test fun neverPolicyNeverLocks() = runBlocking {
        enrolledLogin(cfg = h.config())
        assertTrue(h.session(h.config()).restore() is FieldState.SignedIn)
    }

    @Test fun sameConfigIsRequiredByGet() {
        assertEquals(h.config(), h.config())
    }
}
