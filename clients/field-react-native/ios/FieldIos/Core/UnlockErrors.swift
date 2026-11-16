import Foundation

/// The mapping from LocalAuthentication's `LAError` codes to the library's unlock errors, kept here (it is plain
/// integers) so it is tested on every platform. Same table as Android's `BiometricUnlocker.mapError`.
public enum UnlockErrorMapping {
    /// - Parameter rawCode: `LAError.Code.rawValue`; nil when `canEvaluatePolicy` failed without an `LAError`.
    public static func map(laErrorRawCode rawCode: Int?) -> FieldAuthError {
        switch rawCode {
        case -2, -4, -9, -3: // userCancel, systemCancel, appCancel, userFallback
            return FieldAuthError(code: "BIOMETRIC_CANCELLED", message: "unlock cancelled")
        case -5: // passcodeNotSet
            return FieldAuthError(code: "NO_SECURE_LOCK_SCREEN", message: "device has no passcode")
        case -6, -7, -8: // biometryNotAvailable, biometryNotEnrolled, biometryLockout
            return FieldAuthError(code: "BIOMETRIC_UNAVAILABLE", message: "biometric unavailable")
        case nil:
            return FieldAuthError(code: "BIOMETRIC_UNAVAILABLE", message: "no biometric or passcode is available")
        case let c?:
            return FieldAuthError(code: "INTERNAL", message: "unlock error \(c)")
        }
    }
}
