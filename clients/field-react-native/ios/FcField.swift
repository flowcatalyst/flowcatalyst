import Foundation
import LocalAuthentication
import React
import Security

struct FcFieldError: Error {
    let code: String   // KEY_INVALIDATED | BIOMETRIC_UNAVAILABLE | BIOMETRIC_CANCELLED | NO_SECURE_LOCK_SCREEN | INTERNAL
    let message: String
}

/// JS name "FcField". Rejections carry `code`. Nothing here logs values, data or signatures.
@objc(FcField)
final class FcField: NSObject {
    private let queue = DispatchQueue(label: "io.flowcatalyst.field", qos: .userInitiated)
    private let keys = DeviceKeyStore()
    private let store = SecureStoreKeychain()

    @objc static func requiresMainQueueSetup() -> Bool { false }

    private func run(_ resolve: @escaping RCTPromiseResolveBlock, _ reject: @escaping RCTPromiseRejectBlock,
                     _ block: @escaping () throws -> Any?) {
        queue.async {
            do {
                resolve(try block())
            } catch let e as FcFieldError {
                reject(e.code, e.message, nil)
            } catch {
                reject("INTERNAL", "unexpected native error", nil)
            }
        }
    }

    @objc func getOrCreateDeviceKey(_ resolve: @escaping RCTPromiseResolveBlock, reject: @escaping RCTPromiseRejectBlock) {
        run(resolve, reject) { try self.keys.getOrCreate() }
    }

    @objc func sign(_ dataB64u: String, resolve: @escaping RCTPromiseResolveBlock, reject: @escaping RCTPromiseRejectBlock) {
        run(resolve, reject) {
            guard let data = DerSignature.fromBase64Url(dataB64u) else { throw FcFieldError(code: "INTERNAL", message: "bad base64url") }
            return DerSignature.toBase64Url(try self.keys.sign(data))
        }
    }

    @objc func deleteDeviceKey(_ resolve: @escaping RCTPromiseResolveBlock, reject: @escaping RCTPromiseRejectBlock) {
        run(resolve, reject) { try self.keys.delete(); return nil }
    }

    @objc func storeGet(_ key: String, prompt: String?, resolve: @escaping RCTPromiseResolveBlock, reject: @escaping RCTPromiseRejectBlock) {
        run(resolve, reject) { try self.store.get(key, prompt: prompt) }
    }

    @objc func storeSet(_ key: String, value: String, requireBiometric: Bool, resolve: @escaping RCTPromiseResolveBlock, reject: @escaping RCTPromiseRejectBlock) {
        run(resolve, reject) { try self.store.set(key, value: value, biometric: requireBiometric); return nil }
    }

    @objc func storeDelete(_ key: String, resolve: @escaping RCTPromiseResolveBlock, reject: @escaping RCTPromiseRejectBlock) {
        run(resolve, reject) { self.store.delete(key); return nil }
    }

    @objc func biometricAvailable(_ resolve: @escaping RCTPromiseResolveBlock, reject: @escaping RCTPromiseRejectBlock) {
        run(resolve, reject) { self.store.biometricAvailable() }
    }
}

// MARK: - Device key

final class DeviceKeyStore {
    private let tag = Data("io.flowcatalyst.field.device".utf8)

    private func loadKey() throws -> SecKey? {
        let q: [String: Any] = [
            kSecClass as String: kSecClassKey,
            kSecAttrApplicationTag as String: tag,
            kSecAttrKeyType as String: kSecAttrKeyTypeECSECPrimeRandom,
            kSecReturnRef as String: true,
        ]
        var item: CFTypeRef?
        let st = SecItemCopyMatching(q as CFDictionary, &item)
        if st == errSecItemNotFound { return nil }
        guard st == errSecSuccess, let item = item else { throw FcFieldError(code: "INTERNAL", message: "keychain error \(st)") }
        return (item as! SecKey)
    }

    private func create() throws -> SecKey {
        var privateAttrs: [String: Any] = [
            kSecAttrIsPermanent as String: true,
            kSecAttrApplicationTag as String: tag,
        ]
        var attrs: [String: Any] = [
            kSecAttrKeyType as String: kSecAttrKeyTypeECSECPrimeRandom,
            kSecAttrKeySizeInBits as String: 256,
        ]
        #if targetEnvironment(simulator)
        // The simulator has no Secure Enclave: software Keychain key (reported as SOFTWARE).
        #else
        var acErr: Unmanaged<CFError>?
        guard let ac = SecAccessControlCreateWithFlags(
            nil, kSecAttrAccessibleWhenUnlockedThisDeviceOnly, .privateKeyUsage, &acErr)
        else { throw FcFieldError(code: "INTERNAL", message: "could not create access control") }
        privateAttrs[kSecAttrAccessControl as String] = ac
        attrs[kSecAttrTokenID as String] = kSecAttrTokenIDSecureEnclave
        #endif
        attrs[kSecPrivateKeyAttrs as String] = privateAttrs
        var err: Unmanaged<CFError>?
        guard let key = SecKeyCreateRandomKey(attrs as CFDictionary, &err) else {
            throw FcFieldError(code: "INTERNAL", message: "could not generate device key")
        }
        return key
    }

    func getOrCreate() throws -> [String: Any] {
        let key = try loadKey() ?? create()
        guard let pub = SecKeyCopyPublicKey(key), let ext = SecKeyCopyExternalRepresentation(pub, nil) as Data? else {
            throw FcFieldError(code: "INTERNAL", message: "could not read public key")
        }
        let b = [UInt8](ext)
        guard b.count == 65, b[0] == 0x04 else { throw FcFieldError(code: "INTERNAL", message: "unexpected public key format") }
        #if targetEnvironment(simulator)
        let level = "SOFTWARE"
        #else
        let level = "TEE"
        #endif
        return [
            "jwk": ["kty": "EC", "crv": "P-256",
                    "x": DerSignature.toBase64Url(Array(b[1..<33])),
                    "y": DerSignature.toBase64Url(Array(b[33..<65]))],
            "attestation": ["level": level],
        ]
    }

    func sign(_ data: [UInt8]) throws -> [UInt8] {
        // A missing key (e.g. Keychain restored onto another device) is KEY_INVALIDATED, never a crash.
        guard let key = try loadKey() else { throw FcFieldError(code: "KEY_INVALIDATED", message: "no device key") }
        var err: Unmanaged<CFError>?
        guard let sig = SecKeyCreateSignature(key, .ecdsaSignatureMessageX962SHA256, Data(data) as CFData, &err) as Data? else {
            throw FcFieldError(code: "KEY_INVALIDATED", message: "device key is no longer usable")
        }
        do { return try DerSignature.derToRaw([UInt8](sig)) }
        catch { throw FcFieldError(code: "INTERNAL", message: "malformed signature") }
    }

    func delete() throws {
        let q: [String: Any] = [
            kSecClass as String: kSecClassKey,
            kSecAttrApplicationTag as String: tag,
            kSecAttrKeyType as String: kSecAttrKeyTypeECSECPrimeRandom,
        ]
        let st = SecItemDelete(q as CFDictionary)
        if st != errSecSuccess && st != errSecItemNotFound { throw FcFieldError(code: "INTERNAL", message: "keychain error \(st)") }
    }
}

// MARK: - Secure store

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
