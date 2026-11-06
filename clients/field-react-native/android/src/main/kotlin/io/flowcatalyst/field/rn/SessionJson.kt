package io.flowcatalyst.field.rn

import io.flowcatalyst.field.FieldAuthException
import io.flowcatalyst.field.FieldConfig
import io.flowcatalyst.field.UnlockPolicy
import io.flowcatalyst.field.DeviceInfo
import io.flowcatalyst.field.DeviceRecord
import io.flowcatalyst.field.Session
import io.flowcatalyst.field.LogoutScope
import io.flowcatalyst.field.FieldState
import io.flowcatalyst.field.FieldEvent
import io.flowcatalyst.field.LoginCredentials
import io.flowcatalyst.field.ActAsTarget
import com.facebook.react.bridge.Arguments
import com.facebook.react.bridge.ReadableMap
import com.facebook.react.bridge.ReadableType
import com.facebook.react.bridge.WritableArray
import com.facebook.react.bridge.WritableMap
import kotlin.time.Duration
import kotlin.time.Duration.Companion.milliseconds

/** Conversions between the JS shapes (field-js Session etc.) and field-android's models. Nothing here logs. */
internal object SessionJson {

    fun session(s: Session): WritableMap = Arguments.createMap().apply {
        putMap("worker", Arguments.createMap().apply {
            putString("id", s.worker.id)
            putString("workerCode", s.worker.workerCode)
            putString("name", s.worker.name)
        })
        val a = s.actor
        if (a == null) putNull("actor") else putMap("actor", Arguments.createMap().apply {
            putString("id", a.id)
            putString("type", a.type)
            putString("name", a.name)
        })
        putBoolean("test", s.test)
        putString("deviceMode", s.deviceMode.name)
        if (s.deviceId == null) putNull("deviceId") else putString("deviceId", s.deviceId)
        putString("appCode", s.appCode)
        putDouble("expiresAt", s.expiresAt.toDouble())
        putDouble("idleExpiresAt", s.idleExpiresAt.toDouble())
        putDouble("refreshExpiresAt", s.refreshExpiresAt.toDouble())
        putBoolean("pinChangeRequired", s.pinChangeRequired)
    }

    fun sessionOrNull(s: Session?): WritableMap? = s?.let { session(it) }

    fun device(d: DeviceRecord): WritableMap = Arguments.createMap().apply {
        putString("deviceId", d.deviceId)
        putString("mode", d.mode)
        putString("clientId", d.clientId)
        putArray("appCodes", Arguments.createArray().also { arr -> d.appCodes.forEach { arr.pushString(it) } })
        putString("platformUrl", d.platformUrl)
    }

    fun state(st: FieldState, device: DeviceRecord? = null): WritableMap = Arguments.createMap().apply {
        when (st) {
            is FieldState.SignedOut -> { putString("state", "SignedOut"); putNull("session") }
            is FieldState.SignedIn -> { putString("state", "SignedIn"); putMap("session", session(st.session)) }
            is FieldState.Locked -> { putString("state", "Locked"); putMap("session", session(st.session)) }
        }
        if (device == null) putNull("device") else putMap("device", device(device))
    }

    fun headers(h: Map<String, String>): WritableMap = Arguments.createMap().also { m ->
        h.forEach { (k, v) -> m.putString(k, v) }
    }

    fun event(e: FieldEvent): Pair<String, WritableMap> = when (e) {
        is FieldEvent.SessionChanged -> "SessionChanged" to Arguments.createMap().also {
            val s = sessionOrNull(e.session)
            if (s == null) it.putNull("session") else it.putMap("session", s)
        }
        is FieldEvent.SessionEnded -> "SessionEnded" to Arguments.createMap().also { it.putString("reason", e.reason) }
        is FieldEvent.PinChangeRequired -> "PinChangeRequired" to Arguments.createMap().also { it.putMap("session", session(e.session)) }
        is FieldEvent.ExpiryWarning -> "ExpiryWarning" to Arguments.createMap().also {
            it.putDouble("remainingMs", e.remaining.inWholeMilliseconds.toDouble())
        }
        is FieldEvent.Locked -> "Locked" to Arguments.createMap().also { it.putMap("session", session(e.session)) }
        is FieldEvent.WorkerLocked -> "WorkerLocked" to Arguments.createMap().also {
            it.putBoolean("hard", e.hard)
            e.retryAfterSeconds?.let { s -> it.putInt("retryAfterSeconds", s) }
        }
    }

    // ── JS -> Kotlin ─────────────────────────────────────────────────────

    private fun ReadableMap.str(k: String): String? =
        if (hasKey(k) && !isNull(k) && getType(k) == ReadableType.String) getString(k) else null

    private fun ReadableMap.num(k: String): Double? =
        if (hasKey(k) && !isNull(k) && getType(k) == ReadableType.Number) getDouble(k) else null

    private fun ReadableMap.bool(k: String): Boolean? =
        if (hasKey(k) && !isNull(k) && getType(k) == ReadableType.Boolean) getBoolean(k) else null

    fun config(m: ReadableMap): FieldConfig {
        val platformUrl = m.str("platformUrl") ?: throw FieldAuthException("INVALID_REQUEST", "platformUrl is required")
        val appCode = m.str("appCode") ?: throw FieldAuthException("INVALID_REQUEST", "appCode is required")
        val unlock = if (m.hasKey("requireUnlock") && !m.isNull("requireUnlock")) {
            val u = m.getMap("requireUnlock")!!
            UnlockPolicy(
                afterColdStart = u.bool("afterColdStart") ?: false,
                afterBackground = u.num("afterBackgroundMs")?.let { it.toLong().milliseconds },
            )
        } else UnlockPolicy.NEVER
        val defaults = FieldConfig(platformUrl, appCode)
        val warnings: List<Duration> = if (m.hasKey("expiryWarningsMs") && !m.isNull("expiryWarningsMs")) {
            val a = m.getArray("expiryWarningsMs")!!
            (0 until a.size()).map { a.getDouble(it).toLong().milliseconds }
        } else defaults.expiryWarnings
        return FieldConfig(
            platformUrl = platformUrl,
            appCode = appCode,
            clientIdentifier = m.str("clientIdentifier"),
            requireUnlock = unlock,
            expiryWarnings = warnings,
            keepAliveLead = m.num("keepAliveLeadMs")?.let { it.toLong().milliseconds } ?: defaults.keepAliveLead,
        )
    }

    fun deviceInfo(m: ReadableMap): DeviceInfo {
        val d = DeviceInfo()
        return DeviceInfo(
            platform = m.str("platform") ?: d.platform,
            model = m.str("model") ?: d.model,
            label = m.str("label"),
        )
    }

    fun credentials(m: ReadableMap): LoginCredentials = LoginCredentials(
        workerCode = m.str("workerCode"),
        badgeToken = m.str("badgeToken"),
        pin = m.str("pin"),
        clientIdentifier = m.str("clientIdentifier"),
    )

    fun actAsTarget(m: ReadableMap): ActAsTarget = ActAsTarget(
        workerCode = m.str("workerCode"),
        workerId = m.str("workerId"),
        pin = m.str("pin") ?: throw FieldAuthException("INVALID_REQUEST", "the supervisor PIN is required"),
    )

    fun logoutScope(s: String): LogoutScope = when (s) {
        "session" -> LogoutScope.SESSION
        "device-all" -> LogoutScope.DEVICE_ALL
        else -> throw FieldAuthException("INVALID_REQUEST", "unknown logout scope")
    }

    fun array(l: List<String>): WritableArray = Arguments.createArray().also { a -> l.forEach { a.pushString(it) } }
}
