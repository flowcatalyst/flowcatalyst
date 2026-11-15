import Foundation

/// A failure of a [DeviceKeyProvider] or [SecureStorage] (Kotlin `FcFieldException`). `code` is one of
/// KEY_INVALIDATED, BIOMETRIC_UNAVAILABLE, BIOMETRIC_CANCELLED, NO_SECURE_LOCK_SCREEN, INTERNAL.
public struct FcFieldError: Error, Equatable, CustomStringConvertible {
    public let code: String
    public let message: String

    public init(code: String, message: String) {
        self.code = code
        self.message = message
    }

    public var description: String { "FcFieldError(\(code): \(message))" }
}

/// Every failure of the library (Kotlin `FieldAuthException`). `code` is a platform code (INVALID_CREDENTIALS,
/// INVALID_GRANT, SESSION_EXPIRED, DEVICE_REVOKED, ...), a lower-case DPoP code (invalid_token, use_dpop_nonce,
/// invalid_dpop_proof), or one of the library's own (NETWORK, NOT_ENROLLED, IMPERSONATING, ...). The message never
/// contains a PIN or a refresh token.
public struct FieldAuthError: Error, Equatable, LocalizedError, CustomStringConvertible {
    public static let pinResetRequired = "PIN_RESET_REQUIRED"

    public let code: String
    public let message: String
    public let status: Int?
    public let retryAfterSeconds: Int?
    public let violations: [String]

    public init(
        code: String,
        message: String? = nil,
        status: Int? = nil,
        retryAfterSeconds: Int? = nil,
        violations: [String] = []
    ) {
        self.code = code
        self.message = message ?? code
        self.status = status
        self.retryAfterSeconds = retryAfterSeconds
        self.violations = violations
    }

    /// True for 403 `PIN_RESET_REQUIRED` (from login and actAs): the worker's PIN was set under an app key that has
    /// since been removed. Not a wrong PIN; retrying cannot help. Show "Ask your supervisor to reset your PIN". The
    /// device stays enrolled, no `WorkerLocked` event is raised and no session ends.
    public var isPinResetRequired: Bool { code == FieldAuthError.pinResetRequired }

    public var errorDescription: String? { message }
    public var description: String { "FieldAuthError(\(code): \(message))" }
}

/// The platform codes that end the local session.
let sessionEndingCodes: Set<String> = [
    "INVALID_GRANT", "SESSION_EXPIRED", "DEVICE_REVOKED", "DEVICE_SUSPENDED", "UNKNOWN_DEVICE", "NO_APP_ACCESS",
]
