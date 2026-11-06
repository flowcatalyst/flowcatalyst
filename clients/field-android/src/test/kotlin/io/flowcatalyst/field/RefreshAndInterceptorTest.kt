package io.flowcatalyst.field

import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.runBlocking
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.mockwebserver.MockResponse
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Before
import org.junit.Test
import java.util.concurrent.CountDownLatch
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit
import kotlin.concurrent.thread

class RefreshAndInterceptorTest {
    private lateinit var h: Harness
    private lateinit var app: FakeApp
    private val p get() = h.platform

    @Before fun setUp() {
        h = Harness()
        app = FakeApp()
        p.on("/field/auth/login") { p.token() }
    }

    @After fun tearDown() { h.close(); app.stop() }

    private fun loggedIn(): FieldSession = h.session().also {
        runBlocking { it.login(LoginCredentials(workerCode = "W001", pin = "1234")) }
    }

    private fun client(s: FieldSession) = OkHttpClient.Builder().addInterceptor(s.interceptor()).build()
    private fun get(c: OkHttpClient, path: String = "/api/x") = c.newCall(Request.Builder().url(app.url + path).build()).execute()

    // ── single flight ────────────────────────────────────────────────────

    @Test fun concurrentRefreshesShareOneRequest() = runBlocking {
        val s = loggedIn()
        p.on("/field/auth/refresh") { p.token().setBodyDelay(400, TimeUnit.MILLISECONDS) }
        val all = (1..20).map { async(kotlinx.coroutines.Dispatchers.IO) { s.refresh() } }.awaitAll()
        assertEquals(1, p.count("/field/auth/refresh"))
        assertEquals(1, all.map { it.expiresAt }.distinct().size) // everyone got the same session
    }

    @Test fun twoCoroutinesAndTheInterceptorTriggerExactlyOneRefresh() = runBlocking {
        val s = loggedIn()
        app.requireNonce = false
        h.clock.advance(11 * MIN) // the access token has lapsed: every path wants a refresh
        p.on("/field/auth/refresh") { p.token().setBodyDelay(500, TimeUnit.MILLISECONDS) }
        val c = client(s)
        val start = CountDownLatch(1)
        var interceptorStatus = 0
        val t = thread { start.await(); interceptorStatus = get(c).use { it.code } }
        val a = async(kotlinx.coroutines.Dispatchers.IO) { start.await(); s.headersFor("GET", "${app.url}/a") }
        val b = async(kotlinx.coroutines.Dispatchers.IO) { start.await(); s.headersFor("POST", "${app.url}/b") }
        start.countDown()
        val ha = a.await()
        val hb = b.await()
        t.join(10_000)
        assertEquals(200, interceptorStatus)
        assertEquals(1, p.count("/field/auth/refresh"))
        // everyone signed with the same, refreshed access token
        assertEquals(ha["Authorization"], hb["Authorization"])
        assertEquals(ha["Authorization"], "DPoP " + app.requests.single().bearer)
    }

    @Test fun freshTokenMeansNoRefresh() = runBlocking {
        val s = loggedIn()
        s.headersFor("GET", "${app.url}/a")
        s.headersFor("GET", "${app.url}/a")
        assertEquals(0, p.count("/field/auth/refresh"))
    }

    @Test fun tokenExpiringWithin30sRefreshesFirst() = runBlocking {
        val s = loggedIn()
        p.on("/field/auth/refresh") { p.token() }
        h.clock.advance(10 * MIN - 31_000)
        s.headersFor("GET", "${app.url}/a")
        assertEquals(0, p.count("/field/auth/refresh"))
        h.clock.advance(2_000)
        s.headersFor("GET", "${app.url}/a")
        assertEquals(1, p.count("/field/auth/refresh"))
    }

    // ── session-ended mapping ────────────────────────────────────────────

    @Test fun sessionEndingCodesEndTheSessionAndFailTheRequest() {
        for ((status, code) in listOf(401 to "INVALID_GRANT", 401 to "SESSION_EXPIRED", 403 to "DEVICE_REVOKED", 403 to "DEVICE_SUSPENDED")) {
            val hh = Harness()
            try {
                hh.platform.on("/field/auth/login") { hh.platform.token() }
                hh.platform.on("/field/auth/refresh") { hh.platform.error(status, code) }
                val s = hh.session()
                runBlocking { s.login(LoginCredentials(workerCode = "W001", pin = "1234")) }
                val a = FakeApp().also { it.requireNonce = false }
                try {
                    hh.clock.advance(11 * MIN)
                    try {
                        client(s).newCall(Request.Builder().url(a.url + "/x").build()).execute()
                        fail("expected $code")
                    } catch (e: FieldAuthException) {
                        assertEquals(code, e.code)
                        assertTrue(e is java.io.IOException)
                    }
                    assertEquals(0, a.requests.size) // never reached the app
                } finally { a.stop() }
                assertEquals(FieldState.SignedOut, s.state.value)
                assertTrue("$code: ${hh.events}", hh.events.contains(FieldEvent.SessionEnded(code)))
                assertNull(hh.storage.get(FieldSession.STORE_REFRESH))
                assertNull(hh.storage.get(FieldSession.STORE_SESSION))
            } finally { hh.close() }
        }
    }

    @Test fun otherRefreshFailuresKeepTheSession() = runBlocking {
        val s = loggedIn()
        p.on("/field/auth/refresh") { p.error(503, "UNAVAILABLE") }
        try { s.refresh(); fail() } catch (e: FieldAuthException) { assertEquals("UNAVAILABLE", e.code) }
        assertTrue(s.state.value is FieldState.SignedIn)
        assertNotNull(h.storage.get(FieldSession.STORE_REFRESH))
        assertTrue(h.events.none { it is FieldEvent.SessionEnded })
    }

    @Test fun unknownDeviceAndNoAppAccessAlsoEndTheSession() = runBlocking {
        for (code in listOf("UNKNOWN_DEVICE", "NO_APP_ACCESS")) {
            val s = loggedIn()
            p.on("/field/auth/refresh") { p.error(401, code) }
            try { s.refresh(); fail() } catch (e: FieldAuthException) { assertEquals(code, e.code) }
            assertEquals(FieldState.SignedOut, s.state.value)
        }
    }

    // ── interceptor: nonce and invalid_token ─────────────────────────────

    @Test fun interceptorRetriesOnceOnUseDpopNonce() = runBlocking {
        val s = loggedIn()
        val r = get(client(s))
        assertEquals(200, r.code)
        assertEquals(2, app.requests.size)
        assertNull(app.requests[0].proof!!.nonce)
        assertEquals("app-nonce-1", app.requests[1].proof!!.nonce)
        // the cached application nonce is reused by the next request: no retry
        get(client(s)).close()
        assertEquals(3, app.requests.size)
        assertEquals("app-nonce-1", app.requests[2].proof!!.nonce)
        assertEquals(0, p.count("/field/auth/refresh"))
    }

    @Test fun nonceRetryHappensOnlyOnce() = runBlocking {
        val s = loggedIn()
        app.requireNonce = true
        // an app that always demands a different nonce: first retry still fails, second 401 is returned as is
        val always = object : okhttp3.mockwebserver.Dispatcher() {
            var n = 0
            override fun dispatch(request: okhttp3.mockwebserver.RecordedRequest) =
                MockResponse().setResponseCode(401).setHeader("WWW-Authenticate", "DPoP error=\"use_dpop_nonce\"").setHeader("DPoP-Nonce", "n${n++}")
        }
        val srv = okhttp3.mockwebserver.MockWebServer().also { it.dispatcher = always; it.start() }
        try {
            val r = client(s).newCall(Request.Builder().url(srv.url("/x")).build()).execute()
            assertEquals(401, r.code)
            assertEquals(2, srv.requestCount)
        } finally { srv.shutdown() }
    }

    @Test fun invalidTokenRefreshesOnceAndRetriesOnce() = runBlocking {
        val s = loggedIn()
        app.requireNonce = false
        val first = s.headersFor("GET", "${app.url}/x")["Authorization"]!!.removePrefix("DPoP ")
        app.accept = { it != first }
        p.on("/field/auth/refresh") { p.token() }
        val r = get(client(s))
        assertEquals(200, r.code)
        assertEquals(1, p.count("/field/auth/refresh"))
        assertEquals(2, app.requests.size)
        assertTrue(app.requests[0].bearer == first && app.requests[1].bearer != first)
    }

    @Test fun invalidTokenThatSurvivesARefreshEmitsSessionEnded() = runBlocking {
        val s = loggedIn()
        app.requireNonce = false
        app.accept = { false }
        p.on("/field/auth/refresh") { p.token() }
        val r = get(client(s))
        assertEquals(401, r.code)
        assertEquals(1, p.count("/field/auth/refresh"))
        assertEquals(2, app.requests.size)
        assertTrue(h.events.contains(FieldEvent.SessionEnded("invalid_token")))
    }

    @Test fun invalidTokenAfterAnotherPathAlreadyRefreshedDoesNotRefreshAgain() = runBlocking {
        val s = loggedIn()
        app.requireNonce = false
        val first = s.headersFor("GET", "${app.url}/x")["Authorization"]!!.removePrefix("DPoP ")
        p.on("/field/auth/refresh") { p.token() }
        s.refresh() // someone else replaced the token
        val before = p.count("/field/auth/refresh")
        app.accept = { it != first }
        // a stale in-flight request: simulate by refreshIfStale with the old token
        s.refreshIfStale(first)
        assertEquals(before, p.count("/field/auth/refresh"))
    }

    @Test fun nonceSeenInAResponseFeedsLaterProofs() = runBlocking {
        val s = loggedIn()
        s.noteNonce("${app.url}/api/x?y=1", "seen-elsewhere")
        val h1 = s.headersFor("GET", "${app.url}/api/z")
        assertEquals("seen-elsewhere", Proof(h1["DPoP"]!!).nonce)
        // application nonces are reusable
        assertEquals("seen-elsewhere", Proof(s.headersFor("GET", "${app.url}/api/z")["DPoP"]!!).nonce)
        // a nonce is per origin
        assertNull(Proof(s.headersFor("GET", "http://127.0.0.1:9/api")["DPoP"]!!).nonce)
    }

    @Test fun headersForRetryUsesTheGivenNonce() = runBlocking {
        val s = loggedIn()
        val hdr = s.headersForRetry("GET", "${app.url}/api/x", "fresh-1")
        assertEquals("fresh-1", Proof(hdr["DPoP"]!!).nonce)
    }

    @Test fun platformRouteNoncesAreSingleUse() = runBlocking {
        val s = loggedIn()
        s.noteNonce("${p.url}/field/auth/pin", "single")
        assertEquals("single", Proof(s.headersFor("POST", "${p.url}/field/auth/pin")["DPoP"]!!).nonce)
        assertNull(Proof(s.headersFor("POST", "${p.url}/field/auth/pin")["DPoP"]!!).nonce)
    }

    @Test fun headersForBlockingWorksOffTheMainThread() {
        val s = loggedIn()
        val exec = Executors.newSingleThreadExecutor()
        try {
            val m = exec.submit<Map<String, String>> { s.headersForBlocking("GET", "${app.url}/x") }.get(10, TimeUnit.SECONDS)
            assertTrue(m["Authorization"]!!.startsWith("DPoP "))
        } finally { exec.shutdown() }
    }

    // ── platform nonce handling ──────────────────────────────────────────

    @Test fun platformCallsPrimeTheNonceThenUseTheResponseNonce() = runBlocking {
        p.on("/field/auth/refresh") { p.token() }
        val s = loggedIn()
        s.refresh()
        s.refresh()
        // login primes once; refreshes reuse the nonce each response handed over
        assertEquals(1, p.count("/field/auth/nonce"))
        assertEquals(listOf("/field/auth/nonce", "/field/auth/login", "/field/auth/refresh", "/field/auth/refresh"), p.requests.map { it.path })
        val nonces = p.requests.filter { it.path != "/field/auth/nonce" }.map { it.proof!!.nonce }
        assertEquals(nonces.size, nonces.toSet().size) // never shared
    }

    @Test fun platformRetriesOnceOnUseDpopNonce() = runBlocking {
        p.failNextNonceChecks = 1
        val s = h.session()
        s.login(LoginCredentials(workerCode = "W001", pin = "1234"))
        assertEquals(2, p.count("/field/auth/login"))
    }

    @Test fun platformDoesNotRetryTwice() {
        p.failNextNonceChecks = 2
        val s = h.session()
        try {
            runBlocking { s.login(LoginCredentials(workerCode = "W001", pin = "1234")) }
            fail()
        } catch (e: FieldAuthException) {
            assertEquals("use_dpop_nonce", e.code)
        }
        assertEquals(2, p.count("/field/auth/login"))
    }
}
