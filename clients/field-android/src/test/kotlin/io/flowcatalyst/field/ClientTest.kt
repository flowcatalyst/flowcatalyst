package io.flowcatalyst.field

import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import org.json.JSONObject
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Before
import org.junit.Test

/** The client the worker belongs to, read from the access token's claims. */
class ClientTest {
    private lateinit var h: Harness
    private val p get() = h.platform

    @Before fun setUp() { h = Harness() }
    @After fun tearDown() { h.close() }

    private fun claims() = p.clientClaims(p.deviceClaims("PERSONAL", "dev1"))

    private val adminActor get() = JSONObject().put("act", JSONObject().put("sub", "adm1").put("typ", "principal").put("name", "Ada Admin"))

    private fun login(s: FieldSession) = runBlocking { s.login(LoginCredentials(workerCode = "W001", pin = "1234")) }

    @Test fun signInTakesTheClientFromTheClaims() {
        p.on("/field/auth/login") { p.token(claims = claims()) }
        val ses = login(h.session())
        assertEquals("clt_1", ses.clientId)
        assertEquals("acme", ses.clientIdentifier)
    }

    @Test fun refreshKeepsIt() = runBlocking {
        p.on("/field/auth/login") { p.token(claims = claims()) }
        p.on("/field/auth/refresh") { p.token(claims = p.clientClaims(p.deviceClaims("PERSONAL", "dev1"), id = "clt_2", identifier = "globex")) }
        val s = h.session()
        login(s)
        val ses = s.refresh()
        assertEquals("clt_2", ses.clientId)
        assertEquals("globex", ses.clientIdentifier)
    }

    @Test fun aRestoredSessionHasIt() = runBlocking {
        p.on("/field/auth/login") { p.token(claims = claims()) }
        login(h.session())
        val restored = h.session().restore() as FieldState.SignedIn
        assertEquals("clt_1", restored.session.clientId)
        assertEquals("acme", restored.session.clientIdentifier)
    }

    @Test fun anImpersonatedSessionHasTheWorkersClient() = runBlocking {
        p.on("/field/auth/sign-in-code") { p.token(claims = p.clientClaims(adminActor)) }
        val ses = h.session().redeemSignInCode("ABCD-1234")
        assertNotNull(ses.actor)
        assertEquals("clt_1", ses.clientId)
        assertEquals("acme", ses.clientIdentifier)

        p.on("/field/auth/login") { p.token("sup", "S001", p.clientClaims()) }
        p.on("/field/auth/act-as") { p.token("w2", "W002", p.clientClaims(adminActor)) }
        val s = h.session()
        s.login(LoginCredentials(workerCode = "S001", pin = "9999"))
        val worker = s.actAs(ActAsTarget(workerCode = "W002", pin = "9999"))
        assertEquals("acme", worker.clientIdentifier)
    }

    @Test fun anAbsentClaimIsNull() {
        p.on("/field/auth/login") { p.token(claims = p.deviceClaims("PERSONAL", "dev1")) }
        val ses = login(h.session())
        assertNull(ses.clientId)
        assertNull(ses.clientIdentifier)
    }

    @Test fun anEmptyClaimIsNull() {
        p.on("/field/auth/login") { p.token(claims = p.clientClaims(identifier = "")) }
        val ses = login(h.session())
        assertEquals("clt_1", ses.clientId)
        assertNull(ses.clientIdentifier)
    }

    @Test fun aSessionStoredBeforeTheFieldsExistedReadsAsNullAndTheNextRefreshFillsThem() = runBlocking {
        p.on("/field/auth/login") { p.token(claims = claims()) }
        p.on("/field/auth/refresh") { p.token(claims = claims()) }
        login(h.session())
        val stored = JSONObject(h.storage.get(FieldSession.STORE_SESSION)!!)
        stored.remove("clientId")
        stored.remove("clientIdentifier")
        h.storage.put(FieldSession.STORE_SESSION, stored.toString())
        val s2 = h.session()
        val restored = (s2.restore() as FieldState.SignedIn).session
        assertNull(restored.clientId)
        assertNull(restored.clientIdentifier)
        assertEquals("acme", s2.refresh().clientIdentifier)
    }
}
