package io.flowcatalyst.field

import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import kotlin.time.Duration.Companion.minutes
import kotlin.time.Duration.Companion.seconds

@OptIn(ExperimentalCoroutinesApi::class)
class TimersTest {
    private lateinit var h: Harness
    private val p get() = h.platform

    @Before fun setUp() {
        h = Harness()
        p.on("/field/auth/login") { p.token() }
        p.on("/field/auth/refresh") { p.token() }
    }

    @After fun tearDown() { h.close() }

    /** A session whose clock and timers run on the test scheduler (virtual time). */
    private fun TestScope.vsession(config: FieldConfig = h.config()): FieldSession {
        val clockFn = { BASE_TIME + testScheduler.currentTime }
        h.platform.clock = clockFn
        return h.session(config, clockFn, backgroundScope, StandardTestDispatcher(testScheduler))
    }

    private suspend fun TestScope.advance(ms: Long) { advanceTimeBy(ms); runCurrent() }

    @Test fun keepAliveRefreshesAtTheIdleMidpoint() = runTest {
        p.idleWindowMs = 30 * MIN // midpoint (15 min) is earlier than idle - 2 min
        p.absoluteWindowMs = 600 * MIN
        val s = vsession()
        s.login(LoginCredentials(workerCode = "W001", pin = "1234"))
        s.keepAlive(true)
        advance(15 * MIN - 1_000)
        assertEquals(0, p.count("/field/auth/refresh"))
        advance(1_000)
        assertEquals(1, p.count("/field/auth/refresh"))
        // re-planned from the new idle deadline: next at +15 min again
        advance(15 * MIN - 2_000)
        assertEquals(1, p.count("/field/auth/refresh"))
        advance(2_000)
        assertEquals(2, p.count("/field/auth/refresh"))
    }

    @Test fun keepAliveUsesTheLeadWhenTheIdleWindowIsShort() = runTest {
        p.idleWindowMs = 3 * MIN // idle - 2 min = 1 min, midpoint = 1.5 min
        p.absoluteWindowMs = 600 * MIN
        val s = vsession()
        s.login(LoginCredentials(workerCode = "W001", pin = "1234"))
        s.keepAlive(true)
        advance(MIN - 1_000)
        assertEquals(0, p.count("/field/auth/refresh"))
        advance(1_000)
        assertEquals(1, p.count("/field/auth/refresh"))
    }

    @Test fun keepAliveCustomLead() = runTest {
        p.idleWindowMs = 100 * MIN // lead 5 min => idle-5 = 95 min, midpoint 50 => midpoint wins
        p.absoluteWindowMs = 600 * MIN
        val s = vsession(h.config().copy(keepAliveLead = 5.minutes))
        s.login(LoginCredentials(workerCode = "W001", pin = "1234"))
        s.keepAlive(true)
        advance(50 * MIN - 1_000)
        assertEquals(0, p.count("/field/auth/refresh"))
        advance(1_000)
        assertEquals(1, p.count("/field/auth/refresh"))
    }

    @Test fun keepAliveOffStopsTheTimer() = runTest {
        p.absoluteWindowMs = 600 * MIN
        val s = vsession()
        s.login(LoginCredentials(workerCode = "W001", pin = "1234"))
        s.keepAlive(true)
        advance(5 * MIN)
        s.keepAlive(false)
        advance(120 * MIN)
        assertEquals(0, p.count("/field/auth/refresh"))
    }

    @Test fun keepAliveStopsOnSessionEndAndResumesOnNextLogin() = runTest {
        p.absoluteWindowMs = 600 * MIN
        val s = vsession()
        s.login(LoginCredentials(workerCode = "W001", pin = "1234"))
        s.keepAlive(true)
        p.on("/field/auth/refresh") { p.error(401, "INVALID_GRANT") }
        advance(15 * MIN)
        assertEquals(1, p.count("/field/auth/refresh"))
        assertEquals(FieldState.SignedOut, s.state.value)
        advance(120 * MIN)
        assertEquals(1, p.count("/field/auth/refresh")) // no more attempts
        p.on("/field/auth/refresh") { p.token() }
        s.login(LoginCredentials(workerCode = "W001", pin = "1234"))
        advance(15 * MIN)
        assertEquals(2, p.count("/field/auth/refresh")) // the flag stayed on
    }

    @Test fun keepAliveRetriesAfterATransientFailure() = runTest {
        p.absoluteWindowMs = 600 * MIN
        val s = vsession()
        s.login(LoginCredentials(workerCode = "W001", pin = "1234"))
        s.keepAlive(true)
        p.on("/field/auth/refresh") { p.error(503, "UNAVAILABLE") }
        advance(15 * MIN)
        assertEquals(1, p.count("/field/auth/refresh"))
        p.on("/field/auth/refresh") { p.token() }
        advance(31 * 1000L)
        assertEquals(2, p.count("/field/auth/refresh"))
        assertTrue(s.state.value is FieldState.SignedIn)
    }

    @Test fun keepAliveRefreshOnARestoredSessionHappensImmediatelyWhenDue() = runTest {
        p.absoluteWindowMs = 600 * MIN
        val s = vsession()
        s.login(LoginCredentials(workerCode = "W001", pin = "1234"))
        advance(29 * MIN) // idle deadline (30 min) is 1 min away: inside the 2 min lead
        val s2 = vsession()
        s2.restore()
        s2.keepAlive(true)
        advance(1)
        assertEquals(1, p.count("/field/auth/refresh"))
    }

    // ── expiry warnings ──────────────────────────────────────────────────

    private fun warnings() = h.events.filterIsInstance<FieldEvent.ExpiryWarning>()

    @Test fun warningsFireOncePerThresholdPerFamily() = runTest {
        p.absoluteWindowMs = 60 * MIN
        val s = vsession()
        s.login(LoginCredentials(workerCode = "W001", pin = "1234"))
        advance(30 * MIN - 1_000)
        assertEquals(0, warnings().size)
        advance(1_000)
        assertEquals(listOf(FieldEvent.ExpiryWarning(30.minutes)), warnings())
        advance(20 * MIN - 1_000)
        assertEquals(1, warnings().size)
        advance(1_000)
        assertEquals(listOf(FieldEvent.ExpiryWarning(30.minutes), FieldEvent.ExpiryWarning(10.minutes)), warnings())
        // a refresh stays in the same family: nothing fires again
        p.on("/field/auth/refresh") { p.token(refreshExpiresAt = BASE_TIME + 60 * MIN) }
        s.refresh()
        advance(30 * MIN)
        assertEquals(2, warnings().size)
    }

    @Test fun aNewFamilyWarnsAgain() = runTest {
        p.absoluteWindowMs = 60 * MIN
        val s = vsession()
        s.login(LoginCredentials(workerCode = "W001", pin = "1234"))
        advance(55 * MIN)
        assertEquals(2, warnings().size)
        s.login(LoginCredentials(workerCode = "W001", pin = "1234")) // new absolute deadline
        advance(30 * MIN)
        assertEquals(3, warnings().size)
        advance(20 * MIN)
        assertEquals(4, warnings().size)
    }

    @Test fun customThresholds() = runTest {
        p.absoluteWindowMs = 60 * MIN
        val s = vsession(h.config().copy(expiryWarnings = listOf(5.minutes)))
        s.login(LoginCredentials(workerCode = "W001", pin = "1234"))
        advance(55 * MIN - 1_000)
        assertEquals(0, warnings().size)
        advance(1_000)
        assertEquals(listOf(FieldEvent.ExpiryWarning(5.minutes)), warnings())
    }

    @Test fun restoredSessionAlreadyInsideBothThresholdsWarnsOnce() = runTest {
        p.absoluteWindowMs = 60 * MIN
        val s = vsession()
        s.login(LoginCredentials(workerCode = "W001", pin = "1234"))
        advance(55 * MIN)
        h.events.clear()
        val s2 = vsession()
        s2.restore()
        runCurrent()
        assertEquals(listOf(FieldEvent.ExpiryWarning(5.minutes)), warnings())
        advance(10 * MIN)
        assertEquals(1, warnings().size)
    }

    @Test fun noWarningsAfterLogout() = runTest {
        p.absoluteWindowMs = 60 * MIN
        p.on("/field/auth/logout") { okhttp3.mockwebserver.MockResponse().setResponseCode(204) }
        val s = vsession()
        s.login(LoginCredentials(workerCode = "W001", pin = "1234"))
        s.logout()
        advance(120 * MIN)
        assertEquals(0, warnings().size)
    }
}
