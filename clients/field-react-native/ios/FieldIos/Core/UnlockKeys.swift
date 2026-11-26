import Foundation
#if canImport(CryptoKit)
import CryptoKit
#else
import Crypto
#endif

// Unlock keys (docs/field-unlock-keys-spec.md section 12; Kotlin UnlockKeys.kt): a per-worker ECDSA P-256 key in the
// phone's secure hardware, usable only after a biometric match, registered with the platform after a PIN sign-in on an
// enrolled PERSONAL device. Signing a fresh platform challenge with it signs the worker in again without the PIN.

/// Whether `enableUnlockKey` can be offered now (`FieldSession.unlockKeyAvailability`). Only `.available` means "offer
/// it"; `.enabled` means this worker already holds a key on this phone. Raw values are the Kotlin enum's names.
public enum UnlockKeyAvailability: String, Equatable, Sendable, CaseIterable {
    /// Offer "sign in with Face ID / Touch ID next time".
    case available = "AVAILABLE"
    /// The signed-in worker already holds an unlock key on this phone.
    case enabled = "ENABLED"
    /// The device is not enrolled.
    case notEnrolled = "NOT_ENROLLED"
    /// The device is enrolled SHARED: a shared phone's biometrics cannot say whose finger or face it was. Never offered.
    case sharedDevice = "SHARED_DEVICE"
    /// Nobody is signed in.
    case notSignedIn = "NOT_SIGNED_IN"
    /// Someone is acting as the worker (act-as or a console sign-in code).
    case impersonating = "IMPERSONATING"
    /// The session was not signed in with the worker's PIN on this device in the last 10 minutes (a badge alone, an
    /// unlock key, a session from before enrolment, a pending PIN change, or too long ago).
    case pinSignInRequired = "PIN_SIGN_IN_REQUIRED"
    /// Another worker's unlock key is on this phone (one per phone). `disableUnlockKey()` hands it over.
    case otherWorker = "OTHER_WORKER"
    /// The OS cannot make such a key (Android older than 11; not used on iOS).
    case osTooOld = "OS_TOO_OLD"
    /// No secure hardware: on iOS, no Secure Enclave (the simulator).
    case noSecureHardware = "NO_SECURE_HARDWARE"
    /// No Face ID or Touch ID is enrolled on the phone.
    case biometricNotEnrolled = "BIOMETRIC_NOT_ENROLLED"
    /// The phone has no biometric hardware, or it cannot be used now (no passcode, locked out).
    case biometricUnavailable = "BIOMETRIC_UNAVAILABLE"
    /// The platform said the worker's client has unlock keys switched off (remembered for a day).
    case policyOff = "POLICY_OFF"
    /// The platform refused this phone's key under the client's attestation policy (`422 ATTESTATION_REQUIRED`,
    /// remembered for a week). On iOS this is always the answer under `REQUIRED`: iOS cannot attest an unlock key.
    case attestationRequired = "ATTESTATION_REQUIRED"
    /// No unlock keys in this environment (no unlock-key provider or prompter configured).
    case unsupported = "UNSUPPORTED"
}

/// The unlock key held on this phone (not secret). `pinRequiredBy` is as of the platform's last answer, nil for none.
public struct UnlockKeyInfo: Equatable, Sendable {
    public let unlockKeyId: String
    public let workerId: String
    public let workerCode: String
    public let workerName: String
    public let deviceId: String
    /// ms since the epoch.
    public let createdAt: Int64
    /// When the platform will next ask for the PIN (ms since the epoch); nil when the policy forces no periodic PIN.
    public let pinRequiredBy: Int64?
    /// The platform's attestation verdict at registration: NONE on iOS (MALFORMED, UNTRUSTED or TRUSTED on Android).
    public let attestationVerdict: String

    public init(
        unlockKeyId: String, workerId: String, workerCode: String, workerName: String, deviceId: String,
        createdAt: Int64, pinRequiredBy: Int64?, attestationVerdict: String
    ) {
        self.unlockKeyId = unlockKeyId
        self.workerId = workerId
        self.workerCode = workerCode
        self.workerName = workerName
        self.deviceId = deviceId
        self.createdAt = createdAt
        self.pinRequiredBy = pinRequiredBy
        self.attestationVerdict = attestationVerdict
    }

    func with(pinRequiredBy p: Int64?) -> UnlockKeyInfo {
        UnlockKeyInfo(unlockKeyId: unlockKeyId, workerId: workerId, workerCode: workerCode, workerName: workerName,
                      deviceId: deviceId, createdAt: createdAt, pinRequiredBy: p, attestationVerdict: attestationVerdict)
    }
}

/// The words of the biometric prompt. `cancel` is the prompt's cancel button ("Use PIN"); the library has no words of
/// its own. On iOS `title` is LocalAuthentication's `localizedReason`.
public struct UnlockKeyPrompt: Equatable, Sendable {
    public var title: String
    public var cancel: String
    public var subtitle: String?

    public init(title: String, cancel: String, subtitle: String? = nil) {
        self.title = title
        self.cancel = cancel
        self.subtitle = subtitle
    }
}

/// A new unlock key's public half, its attestation chain (base64 DER, leaf first; always nil on iOS) and where it lives.
public struct UnlockKeyMaterial: Equatable, Sendable {
    public let x: String
    public let y: String
    public let attestationChain: [String]?
    /// STRONGBOX, TEE, SOFTWARE or NONE (unknown). On iOS: TEE for a Secure Enclave key.
    public let level: String

    public init(x: String, y: String, attestationChain: [String]? = nil, level: String) {
        self.x = x
        self.y = y
        self.attestationChain = attestationChain
        self.level = level
    }
}

/// One signature over an unlock key, readied without a prompt (Kotlin `PreparedSignature`).
public protocol PreparedSignature: Sendable {
    /// ES256 over SHA-256(data); returns raw r‖s (64 bytes). Failures are `FcFieldError`.
    func sign(_ data: Data) throws -> Data
}

/// The phone's unlock keys (the Secure Enclave; fakes in tests). Failures are `FcFieldError`.
public protocol UnlockKeyProvider: Sendable {
    /// `.available`, `.biometricNotEnrolled`, `.biometricUnavailable`, `.noSecureHardware` (or `.osTooOld`).
    func biometricStatus() -> UnlockKeyAvailability
    /// Makes a new key under `alias`, bound to the currently enrolled biometrics. iOS sends no attestation, so there is
    /// no attestation challenge (spec 6.1).
    func create(alias: String) throws -> UnlockKeyMaterial
    /// Readies `alias` for one signature, no prompt. `KEY_INVALIDATED` when the biometrics changed or the key is gone.
    func prepare(alias: String) throws -> PreparedSignature
    func delete(alias: String) throws
}

/// Shows the biometric prompt for `signature` and returns its signature of `data` (raw r‖s). Throws `FieldAuthError`.
public protocol UnlockKeyPrompter: Sendable {
    func authorise(_ prompt: UnlockKeyPrompt, signature: PreparedSignature, data: Data) async throws -> Data
}

/// For hosts without unlock keys (and the default): every call refuses.
struct NoUnlockKeys: UnlockKeyProvider {
    func biometricStatus() -> UnlockKeyAvailability { .unsupported }
    func create(alias: String) throws -> UnlockKeyMaterial {
        throw FcFieldError(code: UnlockKeyCodes.unlockKeysUnsupported, message: "no unlock keys here")
    }
    func prepare(alias: String) throws -> PreparedSignature { throw FcFieldError(code: "KEY_INVALIDATED", message: "no unlock keys here") }
    func delete(alias: String) throws {}
}

/// The library's own codes for unlock keys (the platform's codes pass through as they are). Same as Kotlin.
public enum UnlockKeyCodes {
    /// No unlock key is held on this phone: sign in with the PIN.
    public static let noUnlockKey = "NO_UNLOCK_KEY"
    /// The phone invalidated the key (Face ID / Touch ID enrolment changed, or the key is gone): PIN, and offer again.
    public static let unlockKeyInvalidated = "UNLOCK_KEY_INVALIDATED"
    /// This phone or environment cannot hold an unlock key (no Secure Enclave, no provider configured).
    public static let unlockKeysUnsupported = "UNLOCK_KEYS_UNSUPPORTED"
    /// No Face ID or Touch ID is enrolled on the phone.
    public static let biometricNotEnrolled = "BIOMETRIC_NOT_ENROLLED"
    /// Too many failed biometric tries: the phone locked its biometric until its passcode is entered.
    public static let biometricLockout = "BIOMETRIC_LOCKOUT"
    /// Another worker's unlock key is on this phone; disable it first (a hand-over), or let that worker keep it.
    public static let unlockKeyOtherWorker = "UNLOCK_KEY_OTHER_WORKER"
}

/// LocalAuthentication's `LAError` codes to the library's codes for the unlock-key prompt, kept here as plain integers
/// so it is tested on every platform (the Kotlin `BiometricErrors.forUnlockKey` table). Unlike the app unlock, there is
/// no passcode fallback, so a lockout is its own code.
public enum UnlockKeyErrorMapping {
    /// - Parameter rawCode: `LAError.Code.rawValue`; nil when `canEvaluatePolicy` failed without an `LAError`.
    public static func map(laErrorRawCode rawCode: Int?) -> FieldAuthError {
        switch rawCode {
        case -2, -4, -9, -3, -1: // userCancel, systemCancel, appCancel, userFallback, authenticationFailed
            return FieldAuthError(code: "BIOMETRIC_CANCELLED", message: "the biometric prompt was cancelled")
        case -8: // biometryLockout
            return FieldAuthError(code: UnlockKeyCodes.biometricLockout, message: "too many failed biometric attempts")
        case -7: // biometryNotEnrolled
            return FieldAuthError(code: UnlockKeyCodes.biometricNotEnrolled, message: "no Face ID or Touch ID is enrolled")
        case -6: // biometryNotAvailable
            return FieldAuthError(code: "BIOMETRIC_UNAVAILABLE", message: "biometric unavailable")
        case -5: // passcodeNotSet
            return FieldAuthError(code: "NO_SECURE_LOCK_SCREEN", message: "device has no passcode")
        case nil:
            return FieldAuthError(code: "BIOMETRIC_UNAVAILABLE", message: "no biometric is available")
        case let c?:
            return FieldAuthError(code: "INTERNAL", message: "biometric error \(c)")
        }
    }
}

/// The unlock-key JWS of section 3 (pure, tested against api/field-fixtures/unlock.json).
enum UnlockJws {
    static let keyProofType = "fc-unlock-key+jwt"
    static let assertionType = "fc-unlock+jwt"
    static let registerPath = "/field/auth/unlock-keys"
    static let challengePath = "/field/auth/unlock/challenge"
    static let unlockPath = "/field/auth/unlock"
    static let revokePath = "/field/auth/unlock-keys/revoke"

    enum Value {
        case string(String)
        case int(Int64)
    }

    /// `base64url(header) . base64url(claims)` of a key proof, members in the spec's order.
    static func keyProofSigningInput(baseUrl: String, nonce: String, workerId: String, deviceId: String, iat: Int64) -> String {
        signingInput(
            [("typ", .string(keyProofType)), ("alg", .string("ES256"))],
            [("aud", .string(FieldSession.trimTrailingSlashes(baseUrl) + registerPath)), ("nonce", .string(nonce)),
             ("sub", .string(workerId)), ("device_id", .string(deviceId)), ("iat", .int(iat))])
    }

    /// `base64url(header) . base64url(claims)` of an unlock assertion, members in the spec's order.
    static func assertionSigningInput(
        baseUrl: String, unlockKeyId: String, challenge: String, workerId: String, deviceId: String, appCode: String, iat: Int64
    ) -> String {
        signingInput(
            [("typ", .string(assertionType)), ("alg", .string("ES256")), ("kid", .string(unlockKeyId))],
            [("aud", .string(FieldSession.trimTrailingSlashes(baseUrl) + unlockPath)), ("nonce", .string(challenge)),
             ("sub", .string(workerId)), ("device_id", .string(deviceId)), ("app", .string(appCode)), ("iat", .int(iat))])
    }

    /// The compact JWS: the signing input and the raw 64-byte signature.
    static func compact(_ signingInput: String, _ rawSignature: Data) throws -> String {
        if rawSignature.count != 64 {
            throw FieldAuthError(code: "INTERNAL", message: "the unlock key returned a malformed signature")
        }
        return signingInput + "." + Base64URL.encode(rawSignature)
    }

    /// SHA-256 of `fc-unlock-key-attestation:v1\n<sid>\n<workerId>\n<deviceId>` (UTF-8): Android's
    /// setAttestationChallenge. iOS has no key attestation and never sends it; kept so the vectors are checked here too.
    static func attestationChallenge(sid: String, workerId: String, deviceId: String) -> Data {
        Data(SHA256.hash(data: Data("fc-unlock-key-attestation:v1\n\(sid)\n\(workerId)\n\(deviceId)".utf8)))
    }

    private static func signingInput(_ header: [(String, Value)], _ claims: [(String, Value)]) -> String {
        Base64URL.encode(Data(json(header).utf8)) + "." + Base64URL.encode(Data(json(claims).utf8))
    }

    /// A JSON object with its members in the given order and no escapes beyond what JSON needs (JSONSerialization
    /// reorders members and, without an option, writes `/` as `\/`, so it cannot produce the vectors' bytes).
    static func json(_ members: [(String, Value)]) -> String {
        var out = "{"
        for (i, (k, v)) in members.enumerated() {
            if i > 0 { out += "," }
            out += string(k) + ":"
            switch v {
            case .string(let s): out += string(s)
            case .int(let n): out += String(n)
            }
        }
        return out + "}"
    }

    private static func string(_ s: String) -> String {
        var out = "\""
        for u in s.unicodeScalars {
            switch u {
            case "\"": out += "\\\""
            case "\\": out += "\\\\"
            case "\n": out += "\\n"
            case "\r": out += "\\r"
            case "\t": out += "\\t"
            default:
                if u.value < 0x20 {
                    let hex = String(u.value, radix: 16)
                    out += "\\u" + String(repeating: "0", count: 4 - hex.count) + hex
                } else {
                    out.unicodeScalars.append(u)
                }
            }
        }
        return out + "\""
    }
}
