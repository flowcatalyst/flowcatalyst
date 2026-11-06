package io.flowcatalyst.field

import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import okhttp3.mockwebserver.Dispatcher
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okhttp3.mockwebserver.RecordedRequest
import org.json.JSONObject
import java.math.BigInteger
import java.security.KeyPairGenerator
import java.security.KeyFactory
import java.security.MessageDigest
import java.security.Signature
import java.security.interfaces.ECPublicKey
import java.security.spec.ECGenParameterSpec
import java.security.spec.ECParameterSpec
import java.security.spec.ECPoint
import java.security.spec.ECPublicKeySpec
import java.time.Instant
import java.util.Base64
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.CopyOnWriteArrayList
import java.util.concurrent.atomic.AtomicInteger
import java.util.concurrent.atomic.AtomicLong

const val BASE_TIME = 1_800_000_000_000L
const val MIN = 60_000L

/** A software P-256 key behind the same interface the Keystore key uses. */
class SoftKeyProvider : DeviceKeyProvider {
    private val pair = KeyPairGenerator.getInstance("EC").apply { initialize(ECGenParameterSpec("secp256r1")) }.generateKeyPair()
    val publicKey = pair.public as ECPublicKey
    val key = DeviceKey(
        DerSignature.toBase64Url(DerSignature.fixedLength(publicKey.w.affineX.toByteArray())),
        DerSignature.toBase64Url(DerSignature.fixedLength(publicKey.w.affineY.toByteArray())),
        null,
    )
    override fun getOrCreate() = key
    override fun sign(data: ByteArray): ByteArray {
        val s = Signature.getInstance("SHA256withECDSA")
        s.initSign(pair.private)
        s.update(data)
        return DerSignature.derToRaw(s.sign())
    }
    override fun delete() {}
}

/** A parsed DPoP proof; [valid] is true when the ES256 signature verifies against the embedded jwk. */
class Proof(val jwt: String) {
    val parts = jwt.split(".")
    val header = JSONObject(String(Base64.getUrlDecoder().decode(parts[0])))
    val claims = JSONObject(String(Base64.getUrlDecoder().decode(parts[1])))
    val nonce: String? get() = if (claims.has("nonce")) claims.getString("nonce") else null
    val jwk: JSONObject get() = header.getJSONObject("jwk")

    fun valid(): Boolean {
        val ap = java.security.AlgorithmParameters.getInstance("EC").apply { init(ECGenParameterSpec("secp256r1")) }
        val params = ap.getParameterSpec(ECParameterSpec::class.java)
        val pt = ECPoint(BigInteger(1, Base64.getUrlDecoder().decode(jwk.getString("x"))), BigInteger(1, Base64.getUrlDecoder().decode(jwk.getString("y"))))
        val pub = KeyFactory.getInstance("EC").generatePublic(ECPublicKeySpec(pt, params))
        val sig = Base64.getUrlDecoder().decode(parts[2])
        if (sig.size != 64) return false
        val v = Signature.getInstance("SHA256withECDSA")
        v.initVerify(pub)
        v.update((parts[0] + "." + parts[1]).toByteArray())
        return v.verify(DerSignature.rawToDer(sig))
    }
}

class Rec(val method: String, val path: String, val headers: okhttp3.Headers, val body: JSONObject?) {
    val proof: Proof? = headers["DPoP"]?.let { Proof(it) }
    val bearer: String? = headers["Authorization"]?.removePrefix("DPoP ")
}

/** The test clock; tests move it by hand. */
class TestClock(start: Long = BASE_TIME) {
    private val t = AtomicLong(start)
    fun now() = t.get()
    fun advance(ms: Long) { t.addAndGet(ms) }
}

/**
 * A fake platform: enforces single-use nonces on every /field/ route except the nonce route, puts a fresh
 * DPoP-Nonce on every response, records every request.
 */
class FakePlatform(initialClock: () -> Long) : Dispatcher() {
    @Volatile var clock: () -> Long = initialClock
    val server = MockWebServer()
    val requests = CopyOnWriteArrayList<Rec>()
    private val handlers = ConcurrentHashMap<String, (Rec) -> MockResponse>()
    private val nonces: MutableSet<String> = ConcurrentHashMap.newKeySet()
    private val seq = AtomicInteger()
    @Volatile var failNextNonceChecks = 0
    var idleWindowMs = 30 * MIN
    var absoluteWindowMs = 60 * MIN
    var accessLifetimeSec = 600

    init { server.dispatcher = this; server.start() }

    val url: String get() = server.url("/").toString().trimEnd('/')
    fun on(path: String, h: (Rec) -> MockResponse) { handlers[path] = h }
    fun count(path: String) = requests.count { it.path == path }
    fun all(path: String) = requests.filter { it.path == path }
    fun stop() = server.shutdown()

    private fun freshNonce(): String {
        val n = "nonce-" + seq.incrementAndGet()
        nonces.add(n)
        return n
    }

    override fun dispatch(request: RecordedRequest): MockResponse {
        val text = request.body.readUtf8()
        val rec = Rec(request.method ?: "", request.path ?: "", request.headers, runCatching { JSONObject(text) }.getOrNull())
        requests.add(rec)
        val resp: MockResponse
        if (rec.path.startsWith("/field/") && rec.path != "/field/auth/nonce") {
            val n = rec.proof?.nonce
            val forced = failNextNonceChecks > 0
            if (forced) failNextNonceChecks--
            if (forced || n == null || !nonces.remove(n)) {
                resp = MockResponse().setResponseCode(401)
                    .setHeader("WWW-Authenticate", "DPoP error=\"use_dpop_nonce\"")
                    .setBody("""{"error":"use_dpop_nonce"}""")
                resp.setHeader("DPoP-Nonce", freshNonce())
                return resp
            }
        }
        val h = handlers[rec.path]
        resp = (h?.invoke(rec) ?: MockResponse().setResponseCode(404).setBody("""{"error":"NOT_FOUND"}"""))
        resp.setHeader("DPoP-Nonce", freshNonce())
        return resp
    }

    init {
        on("/field/auth/nonce") { MockResponse().setBody("""{"nonce":"x","expiresAt":"2030-01-01T00:00:00Z"}""") }
    }

    private val tokenSeq = AtomicInteger()
    private val refreshSeq = AtomicInteger()

    /** A FieldTokenResponse. The access token is a (unsigned) JWT with [claims]; `at` is unique per call. */
    fun token(
        workerId: String = "w1",
        workerCode: String = "W001",
        claims: JSONObject = JSONObject(),
        pinChangeRequired: Boolean = false,
        refreshExpiresAt: Long? = null,
    ): MockResponse {
        val n = tokenSeq.incrementAndGet()
        claims.put("jti", "at-$n")
        val b64 = Base64.getUrlEncoder().withoutPadding()
        val jwt = b64.encodeToString("""{"alg":"none"}""".toByteArray()) + "." + b64.encodeToString(claims.toString().toByteArray()) + ".sig"
        val now = clock()
        val body = JSONObject()
            .put("accessToken", jwt)
            .put("tokenType", "DPoP")
            .put("expiresIn", accessLifetimeSec)
            .put("refreshToken", "fwr_" + refreshSeq.incrementAndGet())
            .put("refreshExpiresAt", Instant.ofEpochMilli(refreshExpiresAt ?: (now + absoluteWindowMs)).toString())
            .put("idleExpiresAt", Instant.ofEpochMilli(now + idleWindowMs).toString())
            .put("worker", JSONObject().put("id", workerId).put("workerCode", workerCode).put("name", "Name $workerCode"))
            .put("pinChangeRequired", pinChangeRequired)
        return MockResponse().setBody(body.toString())
    }

    fun error(status: Int, code: String, message: String = code, retryAfter: Int? = null): MockResponse {
        val r = MockResponse().setResponseCode(status).setBody(JSONObject().put("error", code).put("message", message).toString())
        if (retryAfter != null) r.setHeader("Retry-After", retryAfter.toString())
        return r
    }

    fun deviceClaims(mode: String = "PERSONAL", id: String = "dev1") = JSONObject().put("device_mode", mode).put("device_id", id)
}

/** An application backend: reusable nonce, accepts tokens per [accept]. */
class FakeApp : Dispatcher() {
    val server = MockWebServer()
    val requests = CopyOnWriteArrayList<Rec>()
    @Volatile var requireNonce = true
    @Volatile var accept: (String) -> Boolean = { true }
    private val current = "app-nonce-1"
    init { server.dispatcher = this; server.start() }
    val url: String get() = server.url("/").toString().trimEnd('/')
    fun stop() = server.shutdown()
    override fun dispatch(request: RecordedRequest): MockResponse {
        val rec = Rec(request.method ?: "", request.path ?: "", request.headers, null)
        requests.add(rec)
        if (requireNonce && rec.proof?.nonce != current) {
            return MockResponse().setResponseCode(401).setHeader("WWW-Authenticate", "DPoP error=\"use_dpop_nonce\"").setHeader("DPoP-Nonce", current)
        }
        if (rec.bearer == null || !accept(rec.bearer)) {
            return MockResponse().setResponseCode(401).setHeader("WWW-Authenticate", "DPoP error=\"invalid_token\"").setHeader("DPoP-Nonce", current)
        }
        return MockResponse().setBody("""{"ok":true}""").setHeader("DPoP-Nonce", current)
    }
}

class Harness(
    val clock: TestClock = TestClock(),
    val storage: InMemoryStorage = InMemoryStorage(),
    val keys: SoftKeyProvider = SoftKeyProvider(),
) {
    val platform = FakePlatform(clock::now)
    val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    val events = CopyOnWriteArrayList<FieldEvent>()

    fun config(
        clientIdentifier: String? = "acme",
        requireUnlock: UnlockPolicy = UnlockPolicy.NEVER,
        platformUrl: String = platform.url,
    ) = FieldConfig(platformUrl, "app1", clientIdentifier, requireUnlock)

    fun session(
        config: FieldConfig = config(),
        clockFn: () -> Long = clock::now,
        scope: CoroutineScope = this.scope,
        dispatcher: CoroutineDispatcher = Dispatchers.IO,
        storage: InMemoryStorage = this.storage,
    ): FieldSession {
        val s = FieldSession.create(config, Deps(storage, keys, FieldSession.defaultHttpClient(), clockFn, scope, dispatcher))
        s.addListener { events.add(it) }
        return s
    }

    fun close() { platform.stop() }
}

fun sha256b64u(s: String): String = Base64.getUrlEncoder().withoutPadding().encodeToString(MessageDigest.getInstance("SHA-256").digest(s.toByteArray()))
