#if canImport(Security) && canImport(LocalAuthentication)
import Foundation
import LocalAuthentication
import Security
import FlowCatalystFieldCore

/// Session storage in the Keychain (Kotlin `AndroidKeystoreStorage`). It reads and writes the items that
/// `clients/field-react-native/ios/FcField.swift` (`SecureStoreKeychain`) uses: generic passwords, service
/// `io.flowcatalyst.field.store`, account = key, value = UTF-8, `kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly`
/// (no user authentication, so the session can refresh in the background and after a reboot). Like Android's
/// "unreadable reads as absent" the session treats a missing item as no session. Never logs keys or values.
public final class KeychainStorage: SecureStorage, @unchecked Sendable {
    /// The service name. Do not change it: the React Native module reads the same items.
    public static let defaultService = "io.flowcatalyst.field.store"

    private let service: String
    private let markerService: String
    private let lock = NSLock()

    public init(service: String = KeychainStorage.defaultService) {
        self.service = service
        self.markerService = service + ".bio-marker"
    }

    private func base(_ key: String, service s: String) -> [String: Any] {
        [kSecClass as String: kSecClassGenericPassword, kSecAttrService as String: s, kSecAttrAccount as String: key]
    }

    /// A context that never shows UI: an item that needs authentication fails instead of prompting from a background refresh.
    private func quietContext() -> LAContext {
        let ctx = LAContext()
        ctx.interactionNotAllowed = true
        return ctx
    }

    public func get(_ key: String) throws -> String? {
        lock.lock()
        defer { lock.unlock() }
        var q = base(key, service: service)
        q[kSecReturnData as String] = true
        q[kSecMatchLimit as String] = kSecMatchLimitOne
        q[kSecUseAuthenticationContext as String] = quietContext()
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
                // A biometric item (written by the React Native module) vanished while its marker remains: the
                // biometry set changed.
                SecItemDelete(base(key, service: markerService) as CFDictionary)
                throw FcFieldError(code: "KEY_INVALIDATED", message: "biometric enrolment changed")
            }
            return nil
        case errSecUserCanceled:
            throw FcFieldError(code: "BIOMETRIC_CANCELLED", message: "biometric prompt cancelled")
        case errSecAuthFailed:
            throw FcFieldError(code: "BIOMETRIC_UNAVAILABLE", message: "biometric authentication failed or unavailable")
        case errSecInteractionNotAllowed:
            // Before the first unlock after a restart, or an item that needs authentication.
            throw FcFieldError(code: "INTERNAL", message: "keychain item is not available now (device not unlocked since restart, or it needs authentication)")
        default:
            throw FcFieldError(code: "INTERNAL", message: "keychain error \(st)")
        }
    }

    public func put(_ key: String, value: String) throws {
        lock.lock()
        defer { lock.unlock() }
        let data = Data(value.utf8)
        let accessible = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
        // Update in place so a crash cannot leave the rotating refresh token deleted; same attributes as the
        // React Native module's delete-then-add.
        var uq = base(key, service: service)
        uq[kSecUseAuthenticationContext as String] = quietContext()
        let attrs: [String: Any] = [kSecValueData as String: data, kSecAttrAccessible as String: accessible]
        var st = SecItemUpdate(uq as CFDictionary, attrs as CFDictionary)
        if st != errSecSuccess {
            if st != errSecItemNotFound { SecItemDelete(base(key, service: service) as CFDictionary) } // e.g. a biometric item
            var q = base(key, service: service)
            q[kSecValueData as String] = data
            q[kSecAttrAccessible as String] = accessible
            st = SecItemAdd(q as CFDictionary, nil)
            guard st == errSecSuccess else { throw FcFieldError(code: "INTERNAL", message: "keychain error \(st)") }
        }
        SecItemDelete(base(key, service: markerService) as CFDictionary)
    }

    public func remove(_ key: String) throws {
        lock.lock()
        defer { lock.unlock() }
        SecItemDelete(base(key, service: service) as CFDictionary)
        SecItemDelete(base(key, service: markerService) as CFDictionary)
    }

    private func hasMarker(_ key: String) -> Bool {
        var q = base(key, service: markerService)
        q[kSecReturnData as String] = false
        return SecItemCopyMatching(q as CFDictionary, nil) == errSecSuccess
    }
}
#endif
