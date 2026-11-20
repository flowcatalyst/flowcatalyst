import Foundation

/// When the app (UI) must be unlocked again. Applies to enrolled PERSONAL, non-impersonated sessions only
/// (Kotlin `UnlockPolicy`). Durations are seconds.
public struct UnlockPolicy: Equatable, Sendable {
    /// Lock a restored session after a process start.
    public var afterColdStart: Bool
    /// Lock after the app has been in the background this long; nil = never.
    public var afterBackground: TimeInterval?

    public init(afterColdStart: Bool = false, afterBackground: TimeInterval? = nil) {
        self.afterColdStart = afterColdStart
        self.afterBackground = afterBackground
    }

    public static let never = UnlockPolicy(afterColdStart: false, afterBackground: nil)
}

/// Kotlin `FieldConfig`. Durations are seconds.
public struct FieldConfig: Equatable, Sendable {
    /// The platform's public base URL. An enrolment QR payload for a different origin (scheme + host + port) is
    /// refused with `PLATFORM_MISMATCH` unless the origin is in `allowedPlatformOrigins`. With an empty URL the first
    /// QR is adopted (first-run provisioning).
    public var platformUrl: String
    public var appCode: String
    /// The client's identifier; needed to sign in by worker code on a device that is not enrolled.
    public var clientIdentifier: String?
    public var requireUnlock: UnlockPolicy
    /// Time before the absolute expiry at which an `expiryWarning` event fires (once each per family).
    public var expiryWarnings: [TimeInterval]
    /// keepAlive refreshes this long before the idle deadline.
    public var keepAliveLead: TimeInterval
    /// Additional platform origins (e.g. `https://fc-eu.example.com`) an enrolment QR may name. Default: none.
    public var allowedPlatformOrigins: [String]

    public init(
        platformUrl: String,
        appCode: String,
        clientIdentifier: String? = nil,
        requireUnlock: UnlockPolicy = .never,
        expiryWarnings: [TimeInterval] = [30 * 60, 10 * 60],
        keepAliveLead: TimeInterval = 2 * 60,
        allowedPlatformOrigins: [String] = []
    ) {
        self.platformUrl = platformUrl
        self.appCode = appCode
        self.clientIdentifier = clientIdentifier
        self.requireUnlock = requireUnlock
        self.expiryWarnings = expiryWarnings
        self.keepAliveLead = keepAliveLead
        self.allowedPlatformOrigins = allowedPlatformOrigins
    }
}

public struct DeviceInfo: Equatable, Sendable {
    public var platform: String
    public var model: String
    public var label: String?

    public init(platform: String = "ios", model: String = "unknown", label: String? = nil) {
        self.platform = platform
        self.model = model
        self.label = label
    }
}

public struct DeviceRecord: Equatable, Sendable {
    public let deviceId: String
    /// "PERSONAL" or "SHARED".
    public let mode: String
    public let clientId: String
    public let appCodes: [String]
    public let platformUrl: String

    public init(deviceId: String, mode: String, clientId: String, appCodes: [String], platformUrl: String) {
        self.deviceId = deviceId
        self.mode = mode
        self.clientId = clientId
        self.appCodes = appCodes
        self.platformUrl = platformUrl
    }
}

/// How the device is known to the platform; `unenrolled` = a key the platform has not enrolled.
public enum DeviceMode: String, Equatable, Sendable {
    case personal = "PERSONAL"
    case shared = "SHARED"
    case unenrolled = "UNENROLLED"
}

public struct Worker: Equatable, Sendable {
    public let id: String
    public let workerCode: String
    public let name: String
    /// The version of the worker's photo; nil when there is none, or photos are off for the client or the platform.
    /// It follows the platform's answer at every sign-in and refresh (a `.sessionChanged` is emitted each time), so a
    /// photo changed or removed elsewhere shows as a different value: fetch it again with `getPhoto`.
    public let photoVersion: String?
    /// True only when photos are on for the platform and the client, the client lets a worker set their own, and the
    /// session is the worker's own (not a supervisor or an admin acting as them). Show a "change photo" control only then.
    public let canSetOwnPhoto: Bool

    public init(id: String, workerCode: String, name: String, photoVersion: String? = nil, canSetOwnPhoto: Bool = false) {
        self.id = id
        self.workerCode = workerCode
        self.name = name
        self.photoVersion = photoVersion
        self.canSetOwnPhoto = canSetOwnPhoto
    }
}

/// Which rendition of the worker's photo: `.thumb` is at most 128 px on its longest side, `.full` at most 512 px.
public enum PhotoSize: String, Sendable {
    case thumb
    case full
}

/// The signed-in worker's own photo as `getPhoto` reads it. `bytes` is the JPEG, nil when `notModified`; `version` is the
/// photo's version (also `session.worker.photoVersion`): pass it as `haveVersion` next time.
public struct WorkerPhoto: Equatable, Sendable {
    public let bytes: Data?
    public let contentType: String?
    public let version: String?
    /// True when the platform answered 304 to `haveVersion`: the bytes the device holds are current.
    public let notModified: Bool

    public init(bytes: Data?, contentType: String?, version: String?, notModified: Bool) {
        self.bytes = bytes
        self.contentType = contentType
        self.version = version
        self.notModified = notModified
    }
}

/// Who is acting as the worker (impersonation): a supervisor or a console admin.
public struct Actor: Equatable, Sendable {
    public let id: String
    public let type: String
    public let name: String

    public init(id: String, type: String, name: String) {
        self.id = id
        self.type = type
        self.name = name
    }
}

/// All times are ms since the epoch on the device clock.
public struct Session: Equatable, Sendable {
    public let worker: Worker
    /// Non-nil while someone else is acting as the worker.
    public let actor: Actor?
    /// A test identity.
    public let test: Bool
    public let deviceMode: DeviceMode
    /// Nil on an unenrolled device.
    public let deviceId: String?
    public let appCode: String
    /// When the access token expires.
    public let expiresAt: Int64
    public let idleExpiresAt: Int64
    public let refreshExpiresAt: Int64
    public let pinChangeRequired: Bool

    public init(
        worker: Worker, actor: Actor?, test: Bool, deviceMode: DeviceMode, deviceId: String?, appCode: String,
        expiresAt: Int64, idleExpiresAt: Int64, refreshExpiresAt: Int64, pinChangeRequired: Bool
    ) {
        self.worker = worker
        self.actor = actor
        self.test = test
        self.deviceMode = deviceMode
        self.deviceId = deviceId
        self.appCode = appCode
        self.expiresAt = expiresAt
        self.idleExpiresAt = idleExpiresAt
        self.refreshExpiresAt = refreshExpiresAt
        self.pinChangeRequired = pinChangeRequired
    }
}

/// Exactly one of `workerCode` / `badgeToken`. The PIN is passed through: never stored, never logged.
public struct LoginCredentials: CustomStringConvertible, Sendable {
    public var workerCode: String?
    public var badgeToken: String?
    public var pin: String?
    public var clientIdentifier: String?

    public init(workerCode: String? = nil, badgeToken: String? = nil, pin: String? = nil, clientIdentifier: String? = nil) {
        self.workerCode = workerCode
        self.badgeToken = badgeToken
        self.pin = pin
        self.clientIdentifier = clientIdentifier
    }

    public var description: String { "LoginCredentials(workerCode=\(workerCode ?? "nil"), badgeToken=***, pin=***)" }
}

/// Exactly one of `workerCode` / `workerId`; `pin` is the supervisor's.
public struct ActAsTarget: CustomStringConvertible, Sendable {
    public var workerCode: String?
    public var workerId: String?
    public var pin: String

    public init(workerCode: String? = nil, workerId: String? = nil, pin: String) {
        self.workerCode = workerCode
        self.workerId = workerId
        self.pin = pin
    }

    public var description: String { "ActAsTarget(workerCode=\(workerCode ?? "nil"), workerId=\(workerId ?? "nil"), pin=***)" }
}

public enum LogoutScope: Sendable {
    case session
    case deviceAll

    var wire: String {
        switch self {
        case .session: return "session"
        case .deviceAll: return "device-all"
        }
    }
}

public enum FieldState: Equatable, Sendable {
    case signedOut
    case signedIn(Session)
    /// Signed in, but the app UI must call `FieldSession.unlock`. Tokens and headers still work.
    case locked(Session)
}

public enum FieldEvent: Equatable, Sendable {
    /// `session` is nil on sign-out.
    case sessionChanged(Session?)
    /// `reason` is "logout", "invalid_token" or a platform code (INVALID_GRANT, SESSION_EXPIRED, DEVICE_REVOKED, ...).
    case sessionEnded(String)
    case pinChangeRequired(Session)
    /// `remaining` is seconds until the absolute expiry.
    case expiryWarning(remaining: TimeInterval)
    /// The app lock engaged (see `UnlockPolicy`).
    case locked(Session)
    /// The worker is locked out of sign-in. `retryAfterSeconds` is nil for a hard lock.
    case workerLocked(hard: Bool, retryAfterSeconds: Int?)
}
