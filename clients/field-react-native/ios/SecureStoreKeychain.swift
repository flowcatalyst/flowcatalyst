import Foundation
import LocalAuthentication
import Security

/// The biometric-capable key/value store behind the WebView bridge (`storeGet` / `storeSet` / `storeDelete`). Items
/// live under the Keychain service `io.flowcatalyst.field.store`; the session owner's own items share that service
/// (they are plain, no access control). Never logs values.
final class SecureStoreKeychain {
    private let service = "io.flowcatalyst.field.store"
    private let markerService = "io.flowcatalyst.field.store.bio-marker"

    private func base(_ key: String, service s: String) -> [String: Any] {
        [kSecClass as String: kSecClassGenericPassword, kSecAttrService as String: s, kSecAttrAccount as String: key]
    }

    func biometricAvailable() -> Bool {
        let ctx = LAContext()
        var err: NSError?
        return ctx.canEvaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, error: &err)
    }

    private func requireBiometricReady() throws {
        let ctx = LAContext()
        var err: NSError?
        if ctx.canEvaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, error: &err) { return }
        switch (err as? LAError)?.code {
        case .passcodeNotSet: throw FcFieldError(code: "NO_SECURE_LOCK_SCREEN", message: "device has no passcode")
        default: throw FcFieldError(code: "BIOMETRIC_UNAVAILABLE", message: "biometrics unavailable or not enrolled")
        }
    }

    func set(_ key: String, value: String, biometric: Bool) throws {
        delete(key)
        var q = base(key, service: service)
        q[kSecValueData as String] = Data(value.utf8)
        if biometric {
            try requireBiometricReady()
            var acErr: Unmanaged<CFError>?
            guard let ac = SecAccessControlCreateWithFlags(
                nil, kSecAttrAccessibleWhenPasscodeSetThisDeviceOnly, .biometryCurrentSet, &acErr)
            else { throw FcFieldError(code: "NO_SECURE_LOCK_SCREEN", message: "device has no passcode") }
            q[kSecAttrAccessControl as String] = ac
            // Marker (no auth) lets get() tell "never stored" from "invalidated by an enrolment change".
            var m = base(key, service: markerService)
            m[kSecValueData as String] = Data([1])
            m[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
            SecItemDelete(base(key, service: markerService) as CFDictionary)
            let ms = SecItemAdd(m as CFDictionary, nil)
            guard ms == errSecSuccess else { throw FcFieldError(code: "INTERNAL", message: "keychain error \(ms)") }
        } else {
            q[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
        }
        let st = SecItemAdd(q as CFDictionary, nil)
        guard st == errSecSuccess else {
            if biometric { SecItemDelete(base(key, service: markerService) as CFDictionary) }
            throw FcFieldError(code: "INTERNAL", message: "keychain error \(st)")
        }
    }

    func get(_ key: String, prompt: String?) throws -> String? {
        var q = base(key, service: service)
        q[kSecReturnData as String] = true
        q[kSecMatchLimit as String] = kSecMatchLimitOne
        let ctx = LAContext()
        ctx.localizedReason = prompt ?? "Unlock"
        q[kSecUseAuthenticationContext as String] = ctx
        var item: CFTypeRef?
        let st = SecItemCopyMatching(q as CFDictionary, &item)
        switch st {
        case errSecSuccess:
            guard let d = item as? Data, let s = String(data: d, encoding: .utf8) else {
                throw FcFieldError(code: "INTERNAL", message: "stored value unreadable")
            }
            return s
        case errSecItemNotFound:
            if hasMarker(key) {
                // Biometric item vanished while its marker remains: the biometry set changed.
                SecItemDelete(base(key, service: markerService) as CFDictionary)
                throw FcFieldError(code: "KEY_INVALIDATED", message: "biometric enrolment changed")
            }
            return nil
        case errSecUserCanceled:
            throw FcFieldError(code: "BIOMETRIC_CANCELLED", message: "biometric prompt cancelled")
        case errSecAuthFailed, errSecInteractionNotAllowed:
            throw FcFieldError(code: "BIOMETRIC_UNAVAILABLE", message: "biometric authentication failed or unavailable")
        default:
            throw FcFieldError(code: "INTERNAL", message: "keychain error \(st)")
        }
    }

    private func hasMarker(_ key: String) -> Bool {
        var q = base(key, service: markerService)
        q[kSecReturnData as String] = false
        return SecItemCopyMatching(q as CFDictionary, nil) == errSecSuccess
    }

    func delete(_ key: String) {
        SecItemDelete(base(key, service: service) as CFDictionary)
        SecItemDelete(base(key, service: markerService) as CFDictionary)
    }
}
