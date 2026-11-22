package io.flowcatalyst.field

import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okio.Buffer
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

class PhotoTest {
    private lateinit var h: Harness
    private val p get() = h.platform
    private val jpeg = byteArrayOf(0xFF.toByte(), 0xD8.toByte(), 0xFF.toByte(), 0xE0.toByte(), 0, 0x10, 'J'.code.toByte(), 'F'.code.toByte(), 'I'.code.toByte(), 'F'.code.toByte(), 0, 0xFF.toByte(), 0xD9.toByte())
    private val version = "0123456789abcdef"

    @Before fun setUp() { h = Harness() }
    @After fun tearDown() { h.close() }

    private fun signedIn(photoVersion: String? = null, canSet: Boolean = false, claims: JSONObject = JSONObject().put("device_mode", "PERSONAL")): FieldSession = runBlocking {
        p.on("/field/auth/login") { p.token(claims = claims, photoVersion = photoVersion, canSetOwnPhoto = canSet) }
        p.on("/field/auth/refresh") { p.token(claims = claims, photoVersion = photoVersion, canSetOwnPhoto = canSet) }
        val s = h.session()
        s.login(LoginCredentials(workerCode = "W001", pin = "1234"))
        s
    }

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

    private fun image(etag: String) = MockResponse().setHeader("Content-Type", "image/jpeg").setHeader("ETag", etag).setBody(Buffer().write(jpeg))

    @Test fun theSessionsWorkerCarriesThePhotoFieldsAndKeepsThemAcrossARestart() = runBlocking {
        val s = signedIn(version, canSet = true)
        val w = (s.restore() as FieldState.SignedIn).session.worker
        assertEquals(version, w.photoVersion)
        assertTrue(w.canSetOwnPhoto)
        val restarted = h.session()
        val again = (restarted.restore() as FieldState.SignedIn).session.worker
        assertEquals(version, again.photoVersion)
        assertTrue(again.canSetOwnPhoto)
        val bare = signedIn()
        val b = (bare.restore() as FieldState.SignedIn).session.worker
        assertNull(b.photoVersion)
        assertFalse(b.canSetOwnPhoto)
    }

    @Test fun aSessionStoredByAnOlderVersionGetsNoPhotoFields() = runBlocking {
        signedIn(version, true)
        val old = JSONObject(h.storage.get(FieldSession.STORE_SESSION)!!)
        old.getJSONObject("worker").remove("photoVersion")
        old.getJSONObject("worker").remove("canSetOwnPhoto")
        h.storage.put(FieldSession.STORE_SESSION, old.toString())
        val w = (h.session().restore() as FieldState.SignedIn).session.worker
        assertNull(w.photoVersion)
        assertFalse(w.canSetOwnPhoto)
    }

    @Test fun aChangedPhotoShowsAtTheNextRefreshAsSessionChanged() = runBlocking {
        val s = signedIn(version)
        h.events.clear()
        p.on("/field/auth/refresh") { p.token(claims = JSONObject().put("device_mode", "PERSONAL"), photoVersion = "fedcba9876543210") }
        val after = s.refresh()
        assertEquals("fedcba9876543210", after.worker.photoVersion)
        val changed = h.events.filterIsInstance<FieldEvent.SessionChanged>().last().session!!
        assertEquals("fedcba9876543210", changed.worker.photoVersion)
    }

    @Test fun getPhotoReadsTheBytesSignedLikeOtherCalls() = runBlocking {
        val s = signedIn(version)
        p.on("/field/worker/photo") { image("\"$version-thumb\"") }
        val photo = s.getPhoto()!!
        assertArrayEquals(jpeg, photo.bytes)
        assertEquals("image/jpeg", photo.contentType)
        assertEquals(version, photo.version)
        assertFalse(photo.notModified)
        val rec = p.all("/field/worker/photo").single()
        assertEquals("thumb", rec.body!!.getString("size"))
        assertFalse(rec.body!!.has("version"))
        assertTrue(rec.proof!!.valid())
        assertTrue(rec.bearer!!.isNotEmpty())
        assertEquals(p.url + "/field/worker/photo", rec.proof!!.claims.getString("htu"))
        s.getPhoto(PhotoSize.FULL)
        assertEquals("full", p.all("/field/worker/photo").last().body!!.getString("size"))
    }

    @Test fun getPhotoNotModifiedAndNone() = runBlocking {
        val s = signedIn(version)
        p.on("/field/worker/photo") { MockResponse().setResponseCode(304).setHeader("ETag", "\"$version-thumb\"") }
        val same = s.getPhoto(haveVersion = version)!!
        assertTrue(same.notModified)
        assertNull(same.bytes)
        assertEquals(version, same.version)
        assertEquals(version, p.all("/field/worker/photo").last().body!!.getString("version"))
        p.on("/field/worker/photo") { p.error(404, "PHOTO_NOT_FOUND") }
        assertNull(s.getPhoto())
        p.on("/field/worker/photo") { p.error(404, "PHOTOS_DISABLED") }
        failsWith("PHOTOS_DISABLED") { s.getPhoto() }
        p.on("/field/worker/photo") { p.error(429, "TOO_MANY_REQUESTS", retryAfter = 90) }
        assertEquals(90, failsWith("TOO_MANY_REQUESTS") { s.getPhoto() }.retryAfterSeconds)
    }

    @Test fun setPhotoSendsTheBytesAsTheyAreAndUpdatesTheSession() = runBlocking {
        val s = signedIn(canSet = true)
        p.on("/field/worker/photo/set") { MockResponse().setBody("""{"photoVersion":"$version"}""") }
        h.events.clear()
        val after = s.setPhoto(jpeg, "image/jpeg")
        val rec = p.all("/field/worker/photo/set").single()
        assertArrayEquals(jpeg, rec.raw)
        assertEquals("image/jpeg", rec.headers["Content-Type"])
        assertTrue(rec.proof!!.valid())
        assertEquals(version, after.worker.photoVersion)
        assertEquals(version, (s.restore() as FieldState.SignedIn).session.worker.photoVersion)
        assertEquals(1, h.events.filterIsInstance<FieldEvent.SessionChanged>().size)
        // a restart keeps it
        assertEquals(version, (h.session().restore() as FieldState.SignedIn).session.worker.photoVersion)
        s.setPhoto(jpeg)
        assertEquals("application/octet-stream", p.all("/field/worker/photo/set").last().headers["Content-Type"]!!.substringBefore(";"))
    }

    @Test fun removePhotoClearsTheVersion() = runBlocking {
        val s = signedIn(version, canSet = true)
        p.on("/field/worker/photo/remove") { MockResponse().setBody("""{"photoVersion":null}""") }
        val after = s.removePhoto()
        assertNull(after.worker.photoVersion)
        assertEquals(1, p.count("/field/worker/photo/remove"))
    }

    @Test fun theRefusalsComeByTheirCodes() = runBlocking {
        val s = signedIn(canSet = true)
        for ((status, code) in listOf(
            403 to "OWN_PHOTO_NOT_ALLOWED", 404 to "PHOTOS_DISABLED", 409 to "PHOTOS_DISABLED_FOR_CLIENT", 413 to "PHOTO_TOO_LARGE",
            415 to "PHOTO_UNSUPPORTED_TYPE", 400 to "PHOTO_TYPE_MISMATCH", 400 to "PHOTO_DIMENSIONS_TOO_LARGE", 400 to "PHOTO_UNREADABLE",
            400 to "PHOTO_EMPTY", 503 to "PHOTO_BUSY",
        )) {
            p.on("/field/worker/photo/set") { p.error(status, code) }
            val e = failsWith(code) { s.setPhoto(jpeg, "image/jpeg") }
            assertEquals(status, e.status)
        }
        p.on("/field/worker/photo/set") { p.error(429, "TOO_MANY_REQUESTS", retryAfter = 30) }
        assertEquals(30, failsWith("TOO_MANY_REQUESTS") { s.setPhoto(jpeg) }.retryAfterSeconds)
        assertNull(s.restoreSync().worker.photoVersion)
    }

    @Test fun anImpersonatingSessionMayNotChangeThePhotoButMayRead() = runBlocking {
        val s = signedIn(canSet = true, claims = JSONObject().put("device_mode", "PERSONAL"))
        p.on("/field/auth/act-as") {
            p.token(workerId = "w2", workerCode = "W002", claims = JSONObject().put("device_mode", "PERSONAL").put("act", JSONObject().put("sub", "w1").put("typ", "field_worker").put("name", "Sup")), photoVersion = version, canSetOwnPhoto = false)
        }
        val imp = s.actAs(ActAsTarget(workerCode = "W002", pin = "1234"))
        assertNotNull(imp.actor)
        assertFalse(imp.worker.canSetOwnPhoto)
        val before = p.requests.size
        failsWith("IMPERSONATING") { s.setPhoto(jpeg, "image/jpeg") }
        failsWith("IMPERSONATING") { s.removePhoto() }
        assertEquals("refused before anything was sent", before, p.requests.size)
        p.on("/field/worker/photo") { image("\"$version-thumb\"") }
        assertNotNull(s.getPhoto())
    }

    private suspend fun FieldSession.restoreSync(): Session = (restore() as FieldState.SignedIn).session
}
