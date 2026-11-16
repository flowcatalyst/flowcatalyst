#if canImport(LocalAuthentication)
import Foundation
import LocalAuthentication
#if canImport(FlowCatalystFieldCore)
import FlowCatalystFieldCore
#endif

/// App unlock through LocalAuthentication (Kotlin `BiometricUnlocker`): Face ID or Touch ID, falling back to the
/// device passcode (`deviceOwnerAuthentication`, the counterpart of BIOMETRIC_STRONG or DEVICE_CREDENTIAL). No
/// Keychain item is gated on it. The app's Info.plist needs `NSFaceIDUsageDescription` for Face ID.
public struct LocalAuthenticationUnlocker: UnlockPrompter {
    public init() {}

    public func prompt(_ text: String) async throws {
        let ctx = LAContext()
        var err: NSError?
        guard ctx.canEvaluatePolicy(.deviceOwnerAuthentication, error: &err) else {
            throw Self.map((err as? LAError)?.code)
        }
        do {
            let ok = try await ctx.evaluatePolicy(.deviceOwnerAuthentication, localizedReason: text)
            if !ok { throw FieldAuthError(code: "INTERNAL", message: "unlock failed") }
        } catch let e as LAError {
            throw Self.map(e.code)
        }
    }

    private static func map(_ code: LAError.Code?) -> FieldAuthError {
        UnlockErrorMapping.map(laErrorRawCode: code?.rawValue)
    }
}
#endif
