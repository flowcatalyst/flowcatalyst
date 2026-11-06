package io.flowcatalyst.field

import java.io.IOException
import kotlin.time.Duration
import kotlin.time.Duration.Companion.minutes

/** When the app (UI) must be unlocked again. Applies to enrolled PERSONAL, non-impersonated sessions only. */
data class UnlockPolicy(
    /** Lock a restored session after a process start. */
    val afterColdStart: Boolean = false,
    /** Lock after the app has been in the background (no started activity) this long; null = never. */
    val afterBackground: Duration? = null,
) {
    companion object {
        val NEVER = UnlockPolicy(afterColdStart = false, afterBackground = null)
    }
}

data class FieldConfig(
    /** The platform's public base URL. An enrolment QR payload overrides it. */
    val platformUrl: String,
    val appCode: String,
    /** The client's identifier; needed to sign in by worker code on a device that is not enrolled. */
    val clientIdentifier: String? = null,
    val requireUnlock: UnlockPolicy = UnlockPolicy.NEVER,
    /** Time before the absolute expiry at which an [FieldEvent.ExpiryWarning] fires (once each per family). */
    val expiryWarnings: List<Duration> = listOf(30.minutes, 10.minutes),
    /** keepAlive refreshes this long before the idle deadline. */
    val keepAliveLead: Duration = 2.minutes,
)

data class DeviceInfo(
    val platform: String = "android",
    val model: String = "unknown",
    val label: String? = null,
)

data class DeviceRecord(
    val deviceId: String,
    /** "PERSONAL" or "SHARED". */
    val mode: String,
    val clientId: String,
    val appCodes: List<String>,
    val platformUrl: String,
)

/** How the device is known to the platform; UNENROLLED = a key the platform has not enrolled. */
enum class DeviceMode { PERSONAL, SHARED, UNENROLLED }

data class Worker(val id: String, val workerCode: String, val name: String)

/** Who is acting as the worker (impersonation): a supervisor or a console admin. */
data class Actor(val id: String, val type: String, val name: String)

/** All times are ms since the epoch on the device clock. */
data class Session(
    val worker: Worker,
    /** Non-null while someone else is acting as the worker. */
    val actor: Actor?,
    /** A test identity. */
    val test: Boolean,
    val deviceMode: DeviceMode,
    /** Null on an unenrolled device. */
    val deviceId: String?,
    val appCode: String,
    /** When the access token expires. */
    val expiresAt: Long,
    val idleExpiresAt: Long,
    val refreshExpiresAt: Long,
    val pinChangeRequired: Boolean,
)

/** Exactly one of [workerCode] / [badgeToken]. The PIN is passed through: never stored, never logged. */
data class LoginCredentials(
    val workerCode: String? = null,
    val badgeToken: String? = null,
    val pin: String? = null,
    val clientIdentifier: String? = null,
) {
    override fun toString(): String = "LoginCredentials(workerCode=$workerCode, badgeToken=***, pin=***)"
}

/** Exactly one of [workerCode] / [workerId]; [pin] is the supervisor's. */
data class ActAsTarget(
    val workerCode: String? = null,
    val workerId: String? = null,
    val pin: String,
) {
    override fun toString(): String = "ActAsTarget(workerCode=$workerCode, workerId=$workerId, pin=***)"
}

enum class LogoutScope(internal val wire: String) {
    SESSION("session"),
    DEVICE_ALL("device-all"),
}

sealed class FieldState {
    object SignedOut : FieldState()
    data class SignedIn(val session: Session) : FieldState()
    /** Signed in, but the app UI must call [FieldSession.unlock]. Tokens and headers still work. */
    data class Locked(val session: Session) : FieldState()
}

sealed class FieldEvent {
    /** [session] is null on sign-out. */
    data class SessionChanged(val session: Session?) : FieldEvent()
    /** [reason] is "logout", "invalid_token" or a platform code (INVALID_GRANT, SESSION_EXPIRED, DEVICE_REVOKED, ...). */
    data class SessionEnded(val reason: String) : FieldEvent()
    data class PinChangeRequired(val session: Session) : FieldEvent()
    data class ExpiryWarning(val remaining: Duration) : FieldEvent()
    /** The app lock engaged (see [UnlockPolicy]). */
    data class Locked(val session: Session) : FieldEvent()
    /** The worker is locked out of sign-in. [retryAfterSeconds] is null for a hard lock. */
    data class WorkerLocked(val hard: Boolean, val retryAfterSeconds: Int?) : FieldEvent()
}

fun interface FieldListener {
    fun onEvent(event: FieldEvent)
}

/**
 * Every failure of the library. [code] is a platform code (INVALID_CREDENTIALS, INVALID_GRANT, SESSION_EXPIRED,
 * DEVICE_REVOKED, ...), a lower-case DPoP code (invalid_token, use_dpop_nonce, invalid_dpop_proof), or one of the
 * library's own (NETWORK, NOT_ENROLLED, IMPERSONATING, ...). An [IOException] so OkHttp interceptors can throw it.
 * The message never contains a PIN or a refresh token.
 */
class FieldAuthException(
    val code: String,
    message: String? = null,
    val status: Int? = null,
    val retryAfterSeconds: Int? = null,
    val violations: List<String> = emptyList(),
) : IOException(message ?: code)

/** The platform codes that end the local session. */
internal val SESSION_ENDING: Set<String> = setOf(
    "INVALID_GRANT", "SESSION_EXPIRED", "DEVICE_REVOKED", "DEVICE_SUSPENDED", "UNKNOWN_DEVICE", "NO_APP_ACCESS",
)
