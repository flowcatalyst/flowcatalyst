package io.flowcatalyst.field

import android.app.Activity
import android.app.Application
import android.content.Context
import android.os.Bundle
import androidx.fragment.app.FragmentActivity
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import okhttp3.Headers
import okhttp3.HttpUrl
import okhttp3.HttpUrl.Companion.toHttpUrlOrNull
import okhttp3.Interceptor
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONArray
import org.json.JSONObject
import java.io.Closeable
import java.io.IOException
import java.time.OffsetDateTime
import java.util.concurrent.CopyOnWriteArrayList
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicLong
import kotlin.time.Duration
import kotlin.time.Duration.Companion.milliseconds
import kotlin.time.Duration.Companion.seconds

/** Everything the session talks to; swapped for fakes in tests. */
internal class Deps(
    val storage: SecureStorage,
    val keys: DeviceKeyProvider,
    val http: OkHttpClient,
    val clock: () -> Long,
    val scope: CoroutineScope,
    val dispatcher: CoroutineDispatcher,
)

internal class Signed(val accessToken: String, val proof: String)

/**
 * The process-wide owner of a field worker's session: device key, tokens, refresh, nonces. One per process; the
 * tracking service and any UI (including React Native) must run in the app's process and share it. No UI; never needs
 * the main thread. See README.md.
 */
class FieldSession private constructor(private val config: FieldConfig, private val d: Deps) {

    companion object {
        @Volatile private var instance: FieldSession? = null

        /** The process-wide owner. First call configures it; later calls must pass equal config. */
        @JvmStatic
        fun get(context: Context, config: FieldConfig): FieldSession {
            instance?.let { return verified(it, config) }
            synchronized(this) {
                instance?.let { return verified(it, config) }
                val app = context.applicationContext
                val dispatcher = Dispatchers.IO
                val s = FieldSession(
                    config,
                    Deps(
                        storage = AndroidKeystoreStorage(app),
                        keys = AndroidKeyProvider(),
                        http = defaultHttpClient(),
                        clock = System::currentTimeMillis,
                        scope = CoroutineScope(SupervisorJob() + dispatcher),
                        dispatcher = dispatcher,
                    ),
                )
                (app as? Application)?.registerActivityLifecycleCallbacks(Visibility(s))
                s.d.scope.launch { runCatching { s.restore() } }
                instance = s
                return s
            }
        }

        private fun verified(s: FieldSession, config: FieldConfig): FieldSession {
            check(s.config == config) { "FieldSession is already configured with a different FieldConfig" }
            return s
        }

        internal fun create(config: FieldConfig, deps: Deps): FieldSession = FieldSession(config, deps)

        internal fun defaultHttpClient(): OkHttpClient = OkHttpClient.Builder()
            .connectTimeout(15, TimeUnit.SECONDS)
            .readTimeout(30, TimeUnit.SECONDS)
            .writeTimeout(30, TimeUnit.SECONDS)
            .build()

        internal const val STORE_DEVICE = "fc.field.device"
        internal const val STORE_REFRESH = "fc.field.refresh"
        internal const val STORE_SESSION = "fc.field.session"
        internal const val STORE_SUPERVISOR = "fc.field.supervisor"
        internal const val STORE_SUPERVISOR_REFRESH = "fc.field.supervisor.refresh"

        /** Refresh first when the access token expires within this many ms. */
        internal const val REFRESH_SKEW_MS = 30_000L
        private const val QR_PREFIX = "fcfield:1:"
        private const val NONCE_PATH = "/field/auth/nonce"
        private val JSON = "application/json".toMediaType()

        /** Drops whitespace and control characters and upper-cases (scanned badges, sign-in codes). */
        @JvmStatic
        fun normaliseBadgeToken(scanned: String): String =
            scanned.replace(Regex("[\\s\\u0000-\\u001f\\u007f]+"), "").uppercase()
    }

    private class Access(val token: String, val expiresAt: Long)

    private class Supervisor(val session: Session, var refreshToken: String?, val access: Access?)

    private class Reply(val status: Int, val headers: Headers, val json: JSONObject?) {
        val error: String? get() = json?.optString("error")?.takeIf { it.isNotEmpty() }
    }

    // ── state ────────────────────────────────────────────────────────────

    private val lock = Any()
    private var platformUrl: String = config.platformUrl.trimEnd('/')
    private var device: DeviceRecord? = null
    private var sessionState: Session? = null
    private var access: Access? = null
    private var refreshToken: String? = null
    private var supervisor: Supervisor? = null
    private var issuedAt: Long = 0
    private var deviceKey: DeviceKey? = null
    private var loaded = false
    private var backgroundedAt: Long? = null

    private val loadMutex = Mutex()
    private val platformMutex = Mutex() // serialises platform calls so a single-use nonce is never shared
    private val refreshMutex = Mutex()
    private val refreshGen = AtomicLong()
    private val nonces = java.util.concurrent.ConcurrentHashMap<String, String>()
    private val listeners = CopyOnWriteArrayList<FieldListener>()
    private val _state = MutableStateFlow<FieldState>(FieldState.SignedOut)

    private var keepAliveOn = false
    private var keepAliveJob: Job? = null
    private var warnJob: Job? = null
    private var warnedKey: String? = null
    private val warned: MutableSet<Long> = java.util.concurrent.ConcurrentHashMap.newKeySet()
    private val warnThresholds: List<Long> = config.expiryWarnings.map { it.inWholeMilliseconds }.distinct()

    /** SignedOut | SignedIn(session) | Locked(session). Restored from storage shortly after [get]; see [restore]. */
    val state: StateFlow<FieldState> get() = _state

    /** Registers a listener for [FieldEvent]s (called on arbitrary threads). Close it to unregister. */
    fun addListener(l: FieldListener): Closeable {
        listeners.add(l)
        return Closeable { listeners.remove(l) }
    }

    /** Loads the stored device and session (once) and returns the resulting state. Safe to call repeatedly. */
    suspend fun restore(): FieldState = withContext(d.dispatcher) { ensureLoaded(); _state.value }

    // ── events ───────────────────────────────────────────────────────────

    private fun emit(e: FieldEvent) {
        for (l in listeners) runCatching { l.onEvent(e) }
    }

    internal fun emitSessionEnded(reason: String) = emit(FieldEvent.SessionEnded(reason))

    // ── storage helpers ──────────────────────────────────────────────────

    private inline fun <T> store(block: () -> T): T = try {
        block()
    } catch (e: FieldAuthException) {
        throw e
    } catch (e: FcFieldException) {
        throw FieldAuthException(e.code, "secure storage failed: ${e.code}")
    } catch (e: Exception) {
        throw FieldAuthException("INTERNAL", "secure storage failed")
    }

    private fun now(): Long = d.clock()

    private suspend fun ensureLoaded() {
        if (loaded) return
        loadMutex.withLock {
            if (loaded) return
            withContext(d.dispatcher) { load() }
            loaded = true
        }
    }

    private fun load() {
        val dev = store { d.storage.get(STORE_DEVICE) }
        if (dev != null) {
            device = runCatching { deviceFromJson(JSONObject(dev)) }.getOrNull()
        }
        val ses = store { d.storage.get(STORE_SESSION) }
        if (ses != null) {
            sessionState = runCatching { sessionFromJson(JSONObject(ses)) }.getOrNull()
        }
        val sup = store { d.storage.get(STORE_SUPERVISOR) }
        if (sup != null && sessionState != null) {
            supervisor = runCatching { Supervisor(sessionFromJson(JSONObject(sup)), null, null) }.getOrNull()
        }
        issuedAt = now()
        val s = sessionState
        if (s != null) {
            val lockNow = config.requireUnlock.afterColdStart && lockEligible(s)
            _state.value = if (lockNow) FieldState.Locked(s) else FieldState.SignedIn(s)
            if (lockNow) emit(FieldEvent.Locked(s))
            ensureTimers()
        }
    }

    private fun requirePlatform(): String = platformUrl.ifEmpty {
        throw FieldAuthException("NOT_ENROLLED", "no platform URL: configure platformUrl or enrol this device first")
    }

    /** True while someone is acting as the signed-in worker (act-as or a sign-in code). */
    private fun impersonating(): Boolean = supervisor != null || sessionState?.actor != null

    private fun lockEligible(s: Session): Boolean =
        device?.mode == "PERSONAL" && s.deviceMode == DeviceMode.PERSONAL && s.actor == null && supervisor == null

    // ── public API ───────────────────────────────────────────────────────

    /**
     * Enrols this device. [qrOrCode] is the 10-character code or a whole `fcfield:1:` QR payload (which also sets
     * the platform URL).
     */
    suspend fun enrol(qrOrCode: String, info: DeviceInfo): DeviceRecord = withContext(d.dispatcher) {
        ensureLoaded()
        var code = qrOrCode.trim()
        if (code.startsWith(QR_PREFIX)) {
            val q = parseEnrolmentQr(code)
            platformUrl = q.first
            code = q.second
        }
        val base = requirePlatform()
        val key = publicKey()
        val body = JSONObject()
            .put("code", code)
            .put("publicKeyJwk", JSONObject().put("kty", "EC").put("crv", "P-256").put("x", key.x).put("y", key.y))
            .put("platform", info.platform.take(40))
            .put("model", info.model.take(120))
        key.attestationChain?.takeIf { it.isNotEmpty() }?.let { body.put("attestationChain", JSONArray(it)) }
        info.label?.let { body.put("label", it) }
        val reply = platformPost("/field/devices/enrol", body)
        val j = reply.json ?: JSONObject()
        val appCodes = j.optJSONArray("appCodes")?.let { a -> List(a.length()) { a.getString(it) } } ?: emptyList()
        val record = DeviceRecord(
            deviceId = j.optString("deviceId"),
            mode = if (j.optString("mode") == "SHARED") "SHARED" else "PERSONAL",
            clientId = j.optString("clientId"),
            appCodes = appCodes,
            platformUrl = base,
        )
        device = record
        store { d.storage.put(STORE_DEVICE, deviceToJson(record).toString()) }
        record
    }

    /**
     * Signs a worker in by worker code or badge. Works without enrolment: the key is created on first use.
     */
    suspend fun login(creds: LoginCredentials): Session = withContext(d.dispatcher) {
        ensureLoaded()
        val badge = creds.badgeToken?.let(::normaliseBadgeToken) ?: ""
        val hasCode = !creds.workerCode.isNullOrEmpty()
        if (hasCode == badge.isNotEmpty()) {
            throw FieldAuthException("INVALID_REQUEST", "give either a worker code or a badge token")
        }
        val clientIdentifier = creds.clientIdentifier ?: config.clientIdentifier
        if (device == null && hasCode && clientIdentifier.isNullOrEmpty()) {
            throw FieldAuthException(
                "INVALID_REQUEST",
                "clientIdentifier is required to sign in by worker code on a device that is not enrolled",
            )
        }
        val body = JSONObject().put("appCode", config.appCode)
        if (hasCode) body.put("workerCode", creds.workerCode) else body.put("badgeToken", badge)
        if (!creds.pin.isNullOrEmpty()) body.put("pin", creds.pin)
        if (!clientIdentifier.isNullOrEmpty()) body.put("clientIdentifier", clientIdentifier)
        try {
            val reply = platformPost("/field/auth/login", body, secrets = listOf(creds.pin ?: "", badge))
            clearSupervisor()
            apply(reply.json, impersonated = false, preserveLock = false)
        } catch (e: FieldAuthException) {
            throw noteFailure(e)
        }
    }

    /** Redeems a sign-in code an administrator created in the console. The session is the worker's, with the admin as actor. */
    suspend fun redeemSignInCode(code: String): Session = withContext(d.dispatcher) {
        ensureLoaded()
        val c = normaliseBadgeToken(code)
        if (c.isEmpty()) throw FieldAuthException("INVALID_REQUEST", "a sign-in code is required")
        try {
            val reply = platformPost("/field/auth/sign-in-code", JSONObject().put("code", c), secrets = listOf(c))
            clearSupervisor()
            apply(reply.json, impersonated = true, preserveLock = false)
        } catch (e: FieldAuthException) {
            throw noteFailure(e)
        }
    }

    /**
     * A supervisor acts as a worker on this device. The supervisor's own session is kept aside and the active
     * session becomes the worker's; [endImpersonation] returns to it. The PIN is the supervisor's.
     */
    suspend fun actAs(target: ActAsTarget): Session = withContext(d.dispatcher) {
        ensureLoaded()
        if (impersonating()) throw FieldAuthException("IMPERSONATING", "end the current impersonation first")
        val hasCode = !target.workerCode.isNullOrEmpty()
        val hasId = !target.workerId.isNullOrEmpty()
        if (hasCode == hasId) throw FieldAuthException("INVALID_REQUEST", "give either a worker code or a worker id")
        val body = JSONObject().put("pin", target.pin)
        if (hasCode) body.put("workerCode", target.workerCode) else body.put("workerId", target.workerId)
        val reply = try {
            bearerPost("/field/auth/act-as", body, listOf(target.pin))
        } catch (e: FieldAuthException) {
            throw noteFailure(e)
        }
        val before = sessionState
        val token = refreshToken
        if (before == null || token == null) throw FieldAuthException("SESSION_EXPIRED", "there is no session")
        val sup = Supervisor(before, token, access)
        store {
            d.storage.put(STORE_SUPERVISOR_REFRESH, token)
            d.storage.put(STORE_SUPERVISOR, sessionToJson(before).toString())
        }
        supervisor = sup
        apply(reply.json, impersonated = true, preserveLock = false)
    }

    /**
     * Ends the impersonated session (logs out its family) and returns to the supervisor's, refreshing it if its
     * access token has lapsed. For a session from a sign-in code there is nothing to return to: the session just
     * ends and this returns null.
     */
    suspend fun endImpersonation(): Session? = withContext(d.dispatcher) {
        ensureLoaded()
        if (!impersonating()) throw FieldAuthException("NOT_IMPERSONATING", "no one is being impersonated")
        val sup = supervisor
        if (sup != null) supervisorToken(sup) // fail before anything is torn down
        val token = refreshToken ?: runCatching { store { d.storage.get(STORE_REFRESH) } }.getOrNull()
        var failure: FieldAuthException? = null
        if (token != null) {
            try {
                platformPost(
                    "/field/auth/logout",
                    JSONObject().put("refreshToken", token).put("scope", "session"),
                    secrets = listOf(token),
                )
            } catch (e: FieldAuthException) {
                failure = e
            }
        }
        if (sup == null) {
            endSession("logout")
            failure?.let { throw it }
            return@withContext null
        }
        switchToSupervisor(sup, "logout")
        val a = access
        if (a == null || a.expiresAt - now() < REFRESH_SKEW_MS) refresh()
        failure?.let { throw it }
        sessionState
    }

    /** Changes the PIN, then refreshes (which drops a `pcr` token). */
    suspend fun changePin(current: String, new: String) {
        withContext(d.dispatcher) {
            ensureLoaded()
            if (impersonating()) throw FieldAuthException("IMPERSONATING", "the PIN cannot be changed while impersonating")
            try {
                bearerPost(
                    "/field/auth/pin",
                    JSONObject().put("currentPin", current).put("newPin", new),
                    listOf(current, new),
                )
            } catch (e: FieldAuthException) {
                throw noteFailure(e)
            }
            refresh()
        }
    }

    /** Ends the session (or every session on this device) and clears local state. */
    suspend fun logout(scope: LogoutScope = LogoutScope.SESSION) {
        withContext(d.dispatcher) {
            ensureLoaded()
            if (scope == LogoutScope.DEVICE_ALL && device == null) {
                throw FieldAuthException("INVALID_REQUEST", "device-all needs an enrolled device")
            }
            val sup = supervisor
            val token = refreshToken ?: runCatching { store { d.storage.get(STORE_REFRESH) } }.getOrNull()
            var failure: FieldAuthException? = null
            if (token != null) {
                try {
                    platformPost(
                        "/field/auth/logout",
                        JSONObject().put("refreshToken", token).put("scope", scope.wire),
                        secrets = listOf(token),
                    )
                } catch (e: FieldAuthException) {
                    failure = e
                }
            }
            // Signing out while impersonating also ends the supervisor's session.
            if (sup != null) {
                try {
                    val st = supervisorToken(sup)
                    platformPost(
                        "/field/auth/logout",
                        JSONObject().put("refreshToken", st).put("scope", "session"),
                        secrets = listOf(st),
                    )
                } catch (e: FieldAuthException) {
                    // best effort: the supervisor's local state is cleared below either way
                }
            }
            endSession("logout")
            failure?.let { throw it }
        }
    }

    /**
     * App unlock (R3): shows the system prompt (BIOMETRIC_STRONG or DEVICE_CREDENTIAL) when the state is [FieldState.Locked].
     * Enrolled PERSONAL, non-impersonated sessions only (else BIOMETRIC_UNAVAILABLE). Does not touch the tokens.
     */
    suspend fun unlock(activity: FragmentActivity, prompt: String): Session = unlockWith(BiometricUnlocker(activity), prompt)

    internal suspend fun unlockWith(prompter: UnlockPrompter, prompt: String): Session {
        ensureLoaded()
        val s = sessionState ?: throw FieldAuthException("SESSION_EXPIRED", "there is no stored session to unlock")
        if (!lockEligible(s)) throw FieldAuthException("BIOMETRIC_UNAVAILABLE", "unlock is not available for this session")
        if (_state.value !is FieldState.Locked) return s
        prompter.prompt(prompt)
        val cur = sessionState ?: throw FieldAuthException("SESSION_EXPIRED", "the session ended")
        _state.value = FieldState.SignedIn(cur)
        return cur
    }

    /** Hosts without activity lifecycle callbacks (or tests) report the app going to the background. */
    fun noteBackgrounded() {
        backgroundedAt = now()
    }

    /** Counterpart of [noteBackgrounded]: locks when the [UnlockPolicy.afterBackground] time has passed. */
    fun noteForegrounded() {
        val since = backgroundedAt ?: return
        backgroundedAt = null
        val limit = config.requireUnlock.afterBackground ?: return
        val s = sessionState ?: return
        if (_state.value is FieldState.SignedIn && now() - since >= limit.inWholeMilliseconds && lockEligible(s)) {
            _state.value = FieldState.Locked(s)
            emit(FieldEvent.Locked(s))
        }
    }

    /**
     * While on, the owner refreshes before the idle deadline (at min(idleExpiresAt - keepAliveLead, idle midpoint)).
     * Uses no Android alarm: the app's foreground service keeps the process alive. Stops on session end and resumes
     * on the next sign-in while still on.
     */
    fun keepAlive(on: Boolean) {
        synchronized(lock) {
            keepAliveOn = on
            if (!on) {
                keepAliveJob?.cancel()
                keepAliveJob = null
            }
        }
        if (on) ensureTimers()
    }

    // ── refresh ──────────────────────────────────────────────────────────

    /** Rotates the refresh token. Single-flight: concurrent callers share one request. */
    suspend fun refresh(): Session = refreshFrom(refreshGen.get())

    /** Refreshes unless a refresh (or sign-in) completed since [seen] was read; then the current session is returned. */
    private suspend fun refreshFrom(seen: Long): Session = withContext(d.dispatcher) {
        ensureLoaded()
        refreshMutex.withLock {
            if (refreshGen.get() != seen) sessionState?.let { return@withLock it }
            doRefresh()
        }
    }

    /** For the interceptor: refresh after an invalid_token unless the token it used was already replaced. */
    internal suspend fun refreshIfStale(usedToken: String) {
        val seen = refreshGen.get()
        if (access?.token == usedToken) refreshFrom(seen)
    }

    private suspend fun doRefresh(): Session {
        var token = refreshToken
        if (token == null) token = store { d.storage.get(STORE_REFRESH) }
        if (token == null) {
            endSession("SESSION_EXPIRED")
            throw FieldAuthException("SESSION_EXPIRED", "there is no session to refresh")
        }
        try {
            val reply = platformPost("/field/auth/refresh", JSONObject().put("refreshToken", token), secrets = listOf(token))
            return apply(reply.json, impersonated = false, preserveLock = true)
        } catch (e: FieldAuthException) {
            if (e.code in SESSION_ENDING) {
                // An impersonation that ends on its own hands back to the supervisor.
                val sup = supervisor
                if (sup != null) switchToSupervisor(sup, e.code) else endSession(e.code)
            }
            throw e
        }
    }

    private suspend fun ensureAccess(): String {
        val seen = refreshGen.get()
        val a = access
        if (a == null || a.expiresAt - now() < REFRESH_SKEW_MS) refreshFrom(seen)
        return access?.token ?: throw FieldAuthException("SESSION_EXPIRED", "there is no session")
    }

    // ── proofs for other code ────────────────────────────────────────────

    /** `Authorization: DPoP` + `DPoP` headers for one request. Refreshes first if the access token is about to expire. */
    suspend fun headersFor(method: String, url: String): Map<String, String> = signedFor(method, url).toHeaders()

    /** Signs again after a `401 use_dpop_nonce`, with the nonce from that response's `DPoP-Nonce` header. */
    suspend fun headersForRetry(method: String, url: String, nonce: String): Map<String, String> {
        noteNonce(url, nonce)
        return headersFor(method, url)
    }

    /** Feeds a `DPoP-Nonce` seen elsewhere (any response from that origin). */
    fun noteNonce(url: String, nonce: String) {
        val u = url.toHttpUrlOrNull() ?: return
        if (nonce.isNotEmpty()) nonces[originOf(u)] = nonce
    }

    /** Blocking form for Java and non-coroutine callers. Never on the main thread. */
    fun headersForBlocking(method: String, url: String): Map<String, String> = runBlocking { headersFor(method, url) }

    /** An OkHttp interceptor that signs requests, retries on nonce and invalid_token. Add it as an application interceptor. */
    fun interceptor(): Interceptor = FieldInterceptor(this)

    private fun Signed.toHeaders(): Map<String, String> = mapOf("Authorization" to "DPoP $accessToken", "DPoP" to proof)

    internal suspend fun signedFor(method: String, url: String): Signed = withContext(d.dispatcher) {
        val target = url.toHttpUrlOrNull() ?: throw FieldAuthException("INVALID_REQUEST", "not a valid URL")
        val token = ensureAccess()
        val proof = proof(method.uppercase(), Dpop.htu(target), takeNonce(target), token)
        Signed(token, proof)
    }

    private fun originOf(u: HttpUrl): String = "${u.scheme}://${u.host}:${u.port}"

    /** Platform `/field/ routes` nonces are single-use: dropped on use. Other origins' nonces stay until replaced. */
    private fun takeNonce(u: HttpUrl): String? {
        val origin = originOf(u)
        val platform = platformUrl.toHttpUrlOrNull()
        return if (platform != null && originOf(platform) == origin && u.encodedPath.startsWith("/field/")) {
            nonces.remove(origin)
        } else {
            nonces[origin]
        }
    }

    // ── platform calls ───────────────────────────────────────────────────

    private fun publicKey(): DeviceKey {
        deviceKey?.let { return it }
        val k = try {
            d.keys.getOrCreate()
        } catch (e: FieldAuthException) {
            throw e
        } catch (e: FcFieldException) {
            throw FieldAuthException(e.code, "secure storage failed: ${e.code}")
        } catch (e: Exception) {
            throw FieldAuthException("INTERNAL", "secure storage failed")
        }
        deviceKey = k
        return k
    }

    private fun proof(htm: String, htu: String, nonce: String?, accessToken: String?): String {
        val key = publicKey()
        try {
            return Dpop.build(d.keys, key, htm, htu, nonce, accessToken, now() / 1000)
        } catch (e: FieldAuthException) {
            throw e
        } catch (e: FcFieldException) {
            throw FieldAuthException(e.code, "signing failed: ${e.code}")
        } catch (e: Exception) {
            throw FieldAuthException("INTERNAL", "signing failed")
        }
    }

    private fun send(url: HttpUrl, body: JSONObject, proof: String, accessToken: String?): Reply {
        val rb = Request.Builder()
            .url(url)
            .header("Accept", "application/json")
            .header("DPoP", proof)
            .post(body.toString().toRequestBody(JSON))
        if (accessToken != null) rb.header("Authorization", "DPoP $accessToken")
        try {
            d.http.newCall(rb.build()).execute().use { res ->
                res.header("DPoP-Nonce")?.takeIf { it.isNotEmpty() }?.let { nonces[originOf(url)] = it }
                var json: JSONObject? = null
                try {
                    val text = res.body?.string()
                    if (!text.isNullOrEmpty()) json = JSONObject(text)
                } catch (e: Exception) {
                    json = null
                }
                return Reply(res.code, res.headers, json)
            }
        } catch (e: IOException) {
            // The original error is dropped on purpose: it may quote the request.
            throw FieldAuthException("NETWORK", "the platform could not be reached")
        }
    }

    private fun primeNonce(base: String) {
        val url = (base + NONCE_PATH).toHttpUrlOrNull() ?: throw FieldAuthException("INVALID_REQUEST", "bad platform URL")
        val reply = send(url, JSONObject(), proof("POST", Dpop.htu(url), null, null), null)
        if (reply.status >= 400) throw replyError(reply, emptyList())
    }

    /** One platform POST with nonce handling (prime when none is cached, one retry on use_dpop_nonce). */
    private suspend fun platformPost(
        path: String,
        body: JSONObject,
        bearer: String? = null,
        secrets: List<String> = emptyList(),
    ): Reply = platformMutex.withLock {
        val base = requirePlatform()
        val url = (base + path).toHttpUrlOrNull() ?: throw FieldAuthException("INVALID_REQUEST", "bad platform URL")
        if (nonces[originOf(url)] == null) primeNonce(base)
        var attempt = 0
        while (true) {
            val proof = proof("POST", Dpop.htu(url), takeNonce(url), bearer)
            val reply = send(url, body, proof, bearer)
            if (reply.status == 401 && reply.error == "use_dpop_nonce" && attempt == 0) {
                attempt++
                continue
            }
            if (reply.status >= 400) throw replyError(reply, secrets)
            return@withLock reply
        }
        error("unreachable")
    }

    /** An authenticated platform POST; one retry with a fresh access token on invalid_token. */
    private suspend fun bearerPost(path: String, body: JSONObject, secrets: List<String>): Reply {
        suspend fun call(): Reply = platformPost(path, body, bearer = ensureAccess(), secrets = secrets)
        try {
            return call()
        } catch (e: FieldAuthException) {
            if (e.code != "invalid_token") throw e
            access = null
            return call()
        }
    }

    private fun replyError(r: Reply, secrets: List<String>): FieldAuthException {
        val code = r.error ?: "HTTP_${r.status}"
        var message = r.json?.optString("message")?.takeIf { it.isNotEmpty() } ?: code
        for (s in secrets) if (s.length >= 4) message = message.replace(s, "***")
        val retry = r.headers["Retry-After"]?.trim()?.toIntOrNull()
        val violations = r.json?.optJSONArray("violations")?.let { a -> List(a.length()) { a.optString(it) } } ?: emptyList()
        return FieldAuthException(code, message, r.status, retry, violations)
    }

    private fun noteFailure(e: FieldAuthException): FieldAuthException {
        when (e.code) {
            "WORKER_LOCKED" -> emit(FieldEvent.WorkerLocked(hard = true, retryAfterSeconds = null))
            "WORKER_LOCKED_TEMPORARILY", "TOO_MANY_ATTEMPTS" -> emit(FieldEvent.WorkerLocked(hard = false, retryAfterSeconds = e.retryAfterSeconds))
        }
        return e
    }

    // ── session state ────────────────────────────────────────────────────

    /** Stores a token response and publishes the new session. */
    private fun apply(json: JSONObject?, impersonated: Boolean, preserveLock: Boolean): Session {
        val t = json ?: throw FieldAuthException("INTERNAL", "malformed token response")
        val accessToken = t.optString("accessToken").takeIf { it.isNotEmpty() }
        val newRefresh = t.optString("refreshToken").takeIf { it.isNotEmpty() }
        val w = t.optJSONObject("worker")
        if (accessToken == null || newRefresh == null || w == null || !t.has("expiresIn")) {
            throw FieldAuthException("INTERNAL", "malformed token response")
        }
        val now = now()
        val claims = decodeClaims(accessToken)
        val act = claims?.optJSONObject("act")
        val actor = act?.optString("sub")?.takeIf { it.isNotEmpty() }?.let {
            Actor(it, act.optString("typ").ifEmpty { "field_worker" }, act.optString("name"))
        }
        val deviceMode = when (claims?.optString("device_mode")) {
            "PERSONAL" -> DeviceMode.PERSONAL
            "SHARED" -> DeviceMode.SHARED
            "UNENROLLED" -> DeviceMode.UNENROLLED
            else -> when (device?.mode) {
                "PERSONAL" -> DeviceMode.PERSONAL
                "SHARED" -> DeviceMode.SHARED
                else -> DeviceMode.UNENROLLED
            }
        }
        val deviceId: String? = when {
            claims?.optString("device_id")?.isNotEmpty() == true -> claims.optString("device_id")
            claims != null -> null
            else -> device?.deviceId
        }
        val session = Session(
            worker = Worker(w.optString("id"), w.optString("workerCode"), w.optString("name")),
            actor = actor,
            test = claims?.optBoolean("test", false) == true,
            deviceMode = deviceMode,
            deviceId = deviceId,
            appCode = sessionState?.appCode ?: config.appCode,
            expiresAt = now + t.optLong("expiresIn") * 1000,
            idleExpiresAt = parseTime(t.optString("idleExpiresAt")),
            refreshExpiresAt = parseTime(t.optString("refreshExpiresAt")),
            pinChangeRequired = t.optBoolean("pinChangeRequired", false),
        )
        synchronized(lock) {
            access = Access(accessToken, session.expiresAt)
            refreshToken = newRefresh
            sessionState = session
            issuedAt = now
        }
        refreshGen.incrementAndGet()
        store {
            d.storage.put(STORE_REFRESH, newRefresh)
            d.storage.put(STORE_SESSION, sessionToJson(session).toString())
        }
        publish(session, preserveLock)
        emit(FieldEvent.SessionChanged(session))
        if (session.pinChangeRequired) emit(FieldEvent.PinChangeRequired(session))
        ensureTimers()
        return session
    }

    private fun publish(session: Session, preserveLock: Boolean) {
        val wasLocked = _state.value is FieldState.Locked
        _state.value = if (preserveLock && wasLocked && lockEligible(session)) FieldState.Locked(session) else FieldState.SignedIn(session)
    }

    /** Clears local session state and tells listeners. */
    private fun endSession(reason: String) {
        val had: Boolean
        synchronized(lock) {
            had = sessionState != null
            access = null
            refreshToken = null
            sessionState = null
            supervisor = null
            warnJob?.cancel()
            warnJob = null
            warnedKey = null
            warned.clear()
        }
        try {
            d.storage.remove(STORE_REFRESH)
            d.storage.remove(STORE_SESSION)
        } catch (e: Exception) {
            // best effort: the in-memory state is already gone
        }
        clearSupervisorStore()
        _state.value = FieldState.SignedOut
        if (had) {
            emit(FieldEvent.SessionEnded(reason))
            emit(FieldEvent.SessionChanged(null))
        }
    }

    private fun supervisorToken(sup: Supervisor): String {
        sup.refreshToken?.let { return it }
        val t = store { d.storage.get(STORE_SUPERVISOR_REFRESH) }
            ?: throw FieldAuthException("SESSION_EXPIRED", "the supervisor session is gone")
        sup.refreshToken = t
        return t
    }

    /** Makes the kept-aside supervisor session the active one. */
    private fun switchToSupervisor(sup: Supervisor, reason: String) {
        val token = supervisorToken(sup)
        synchronized(lock) {
            supervisor = null
            access = sup.access
            refreshToken = token
            sessionState = sup.session
            issuedAt = now()
        }
        refreshGen.incrementAndGet()
        store {
            d.storage.put(STORE_REFRESH, token)
            d.storage.put(STORE_SESSION, sessionToJson(sup.session).toString())
        }
        clearSupervisorStore()
        publish(sup.session, preserveLock = false)
        emit(FieldEvent.SessionEnded(reason))
        emit(FieldEvent.SessionChanged(sup.session))
        ensureTimers()
    }

    private fun clearSupervisorStore() {
        try {
            d.storage.remove(STORE_SUPERVISOR)
            d.storage.remove(STORE_SUPERVISOR_REFRESH)
        } catch (e: Exception) {
            // best effort
        }
    }

    private fun clearSupervisor() {
        supervisor = null
        clearSupervisorStore()
    }

    // ── keepAlive and expiry warnings ────────────────────────────────────

    private fun ensureTimers() {
        synchronized(lock) {
            val s = sessionState ?: return
            if (keepAliveOn && keepAliveJob?.isActive != true) {
                keepAliveJob = d.scope.launch { keepAliveLoop() }
            }
            if (warnThresholds.isNotEmpty()) {
                val key = "${s.worker.id}:${s.refreshExpiresAt}"
                if (key != warnedKey) {
                    warned.clear()
                    warnedKey = key
                    warnJob?.cancel()
                    warnJob = null
                }
                if (warnJob?.isActive != true && warned.size < warnThresholds.size) {
                    warnJob = d.scope.launch { warnLoop(key) }
                }
            }
        }
    }

    private suspend fun keepAliveLoop() {
        while (true) {
            val s: Session
            val issued: Long
            synchronized(lock) {
                val cur = sessionState
                if (cur == null || !keepAliveOn) {
                    keepAliveJob = null
                    return
                }
                s = cur
                issued = issuedAt
            }
            val target = minOf(s.idleExpiresAt - config.keepAliveLead.inWholeMilliseconds, issued + (s.idleExpiresAt - issued) / 2)
            val wait = target - now()
            if (wait > 0) {
                delay(wait)
                val s2 = sessionState
                if (s2 == null || s2.idleExpiresAt != s.idleExpiresAt) continue // refreshed elsewhere or ended: re-plan
            }
            try {
                refresh()
            } catch (e: CancellationException) {
                throw e
            } catch (e: FieldAuthException) {
                if (e.code !in SESSION_ENDING) delay(RETRY_AFTER_FAILURE)
            }
            delay(MIN_GAP)
        }
    }

    private suspend fun warnLoop(key: String) {
        val me = currentCoroutineContext()[Job]
        try {
            while (true) {
                val s = sessionState ?: return
                if ("${s.worker.id}:${s.refreshExpiresAt}" != key) return
                val remaining = s.refreshExpiresAt - now()
                val pending = warnThresholds.filter { it !in warned }
                if (pending.isEmpty()) return
                val due = pending.filter { remaining <= it }
                if (due.isNotEmpty()) {
                    warned.addAll(due)
                    emit(FieldEvent.ExpiryWarning(remaining.coerceAtLeast(0).milliseconds))
                    continue
                }
                delay(remaining - pending.max())
            }
        } finally {
            synchronized(lock) { if (warnJob === me) warnJob = null }
        }
    }

    // ── app visibility (lock after background) ───────────────────────────

    private class Visibility(private val s: FieldSession) : Application.ActivityLifecycleCallbacks {
        private var started = 0
        override fun onActivityStarted(activity: Activity) {
            if (started++ == 0) s.noteForegrounded()
        }

        override fun onActivityStopped(activity: Activity) {
            if (--started == 0) s.noteBackgrounded()
        }

        override fun onActivityCreated(activity: Activity, savedInstanceState: Bundle?) {}
        override fun onActivityResumed(activity: Activity) {}
        override fun onActivityPaused(activity: Activity) {}
        override fun onActivitySaveInstanceState(activity: Activity, outState: Bundle) {}
        override fun onActivityDestroyed(activity: Activity) {}
    }

    // ── serialisation ────────────────────────────────────────────────────

    private fun parseTime(s: String): Long = try {
        OffsetDateTime.parse(s).toInstant().toEpochMilli()
    } catch (e: Exception) {
        throw FieldAuthException("INTERNAL", "malformed token response")
    }

    private fun decodeClaims(jwt: String): JSONObject? {
        val parts = jwt.split(".")
        if (parts.size != 3 || parts[1].isEmpty()) return null
        return try {
            JSONObject(String(Dpop.b64uDecode(parts[1]), Charsets.UTF_8))
        } catch (e: Exception) {
            null
        }
    }

    private fun deviceToJson(r: DeviceRecord) = JSONObject()
        .put("deviceId", r.deviceId).put("mode", r.mode).put("clientId", r.clientId)
        .put("appCodes", JSONArray(r.appCodes)).put("platformUrl", r.platformUrl)

    private fun deviceFromJson(j: JSONObject) = DeviceRecord(
        deviceId = j.getString("deviceId"),
        mode = j.getString("mode"),
        clientId = j.optString("clientId"),
        appCodes = j.optJSONArray("appCodes")?.let { a -> List(a.length()) { a.getString(it) } } ?: emptyList(),
        platformUrl = j.optString("platformUrl"),
    )

    private fun sessionToJson(s: Session): JSONObject = JSONObject()
        .put("worker", JSONObject().put("id", s.worker.id).put("workerCode", s.worker.workerCode).put("name", s.worker.name))
        .put("actor", s.actor?.let { JSONObject().put("id", it.id).put("type", it.type).put("name", it.name) } ?: JSONObject.NULL)
        .put("test", s.test)
        .put("deviceMode", s.deviceMode.name)
        .put("deviceId", s.deviceId ?: JSONObject.NULL)
        .put("appCode", s.appCode)
        .put("expiresAt", s.expiresAt)
        .put("idleExpiresAt", s.idleExpiresAt)
        .put("refreshExpiresAt", s.refreshExpiresAt)
        .put("pinChangeRequired", s.pinChangeRequired)

    private fun sessionFromJson(j: JSONObject): Session {
        val w = j.getJSONObject("worker")
        val a = j.optJSONObject("actor")
        return Session(
            worker = Worker(w.getString("id"), w.optString("workerCode"), w.optString("name")),
            actor = a?.let { Actor(it.getString("id"), it.optString("type", "field_worker"), it.optString("name")) },
            test = j.optBoolean("test", false),
            deviceMode = runCatching { DeviceMode.valueOf(j.getString("deviceMode")) }.getOrNull()
                ?: when (device?.mode) {
                    "PERSONAL" -> DeviceMode.PERSONAL
                    "SHARED" -> DeviceMode.SHARED
                    else -> DeviceMode.UNENROLLED
                },
            deviceId = if (j.isNull("deviceId")) device?.deviceId else j.getString("deviceId"),
            appCode = j.getString("appCode"),
            expiresAt = j.getLong("expiresAt"),
            idleExpiresAt = j.getLong("idleExpiresAt"),
            refreshExpiresAt = j.getLong("refreshExpiresAt"),
            pinChangeRequired = j.optBoolean("pinChangeRequired", false),
        )
    }

    /** Parses `fcfield:1:<platformBaseUrl>:<code>`; the code is the last segment. Returns (platformUrl, code). */
    private fun parseEnrolmentQr(payload: String): Pair<String, String> {
        fun bad() = FieldAuthException("INVALID_ENROLMENT_CODE", "not a FlowCatalyst enrolment QR code")
        val p = payload.trim()
        if (!p.startsWith(QR_PREFIX)) throw bad()
        val rest = p.substring(QR_PREFIX.length)
        val i = rest.lastIndexOf(':')
        if (i <= 0) throw bad()
        val url = rest.substring(0, i).trimEnd('/')
        val code = rest.substring(i + 1)
        if (code.isEmpty() || code.any { it.isWhitespace() }) throw bad()
        val u = url.toHttpUrlOrNull() ?: throw bad()
        if (u.scheme != "https" && u.scheme != "http") throw bad()
        return url to code
    }
}

private val RETRY_AFTER_FAILURE: Duration = 30.seconds
private val MIN_GAP: Duration = 1.seconds
