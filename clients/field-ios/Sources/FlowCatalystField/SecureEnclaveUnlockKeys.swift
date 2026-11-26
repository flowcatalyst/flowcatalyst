#if canImport(Security) && canImport(LocalAuthentication)
import Foundation
import LocalAuthentication
import Security
#if canImport(CryptoKit)
import CryptoKit
#endif
#if canImport(FlowCatalystFieldCore)
import FlowCatalystFieldCore
#endif

/// Unlock keys in the Secure Enclave (docs/field-unlock-keys-spec.md section 12.1; Kotlin `AndroidUnlockKeyProvider`):
/// a permanent P-256 key per alias, `kSecAttrAccessibleWhenPasscodeSetThisDeviceOnly` with `.privateKeyUsage` and
/// `.biometryCurrentSet`, so every signature needs Face ID or Touch ID and an enrolment change makes the key unusable.
/// No attestation (iOS cannot attest such a key: spec 6.1). No software fallback: without a Secure Enclave (the
/// simulator) there are no unlock keys. Never logs key material or signatures.
///
/// Invalidation is detected before any prompt or challenge in two ways: the key is gone (`errSecItemNotFound`), or the
/// biometric enrolment state LocalAuthentication reports (`domainState.biometry.stateHash`, earlier
/// `evaluatedPolicyDomainState`) differs from the one recorded when the key was made. `.biometryCurrentSet` makes the
/// key itself refuse to sign after a change; the recorded state lets the library say so without asking the platform
/// for a challenge or showing a prompt that cannot succeed.
public final class SecureEnclaveUnlockKeyProvider: UnlockKeyProvider, @unchecked Sendable {
    private static let lock = NSLock()
    private static let statePrefix = "io.flowcatalyst.field.unlock.biometryState."

    private let defaults: UserDefaults

    /// `defaults` keeps the (not secret) biometric enrolment state recorded with each key.
    public init(defaults: UserDefaults = .standard) { self.defaults = defaults }

    /// A Secure Enclave key has no attestation (spec 6.1): under a policy that requires one, iOS has no unlock keys.
    public var attests: Bool { false }

    public func biometricStatus() -> UnlockKeyAvailability {
        #if targetEnvironment(simulator)
        return .noSecureHardware
        #else
        #if canImport(CryptoKit)
        if !SecureEnclave.isAvailable { return .noSecureHardware }
        #endif
        let ctx = LAContext()
        var err: NSError?
        if ctx.canEvaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, error: &err) { return .available }
        switch (err as? LAError)?.code {
        case .biometryNotEnrolled?: return .biometricNotEnrolled
        default: return .biometricUnavailable
        }
        #endif
    }

    public func create(alias: String) throws -> UnlockKeyMaterial {
        Self.lock.lock()
        defer { Self.lock.unlock() }
        #if targetEnvironment(simulator)
        throw FcFieldError(code: UnlockKeyCodes.unlockKeysUnsupported, message: "the simulator has no Secure Enclave")
        #else
        var acErr: Unmanaged<CFError>?
        guard let ac = SecAccessControlCreateWithFlags(
            nil, kSecAttrAccessibleWhenPasscodeSetThisDeviceOnly, [.privateKeyUsage, .biometryCurrentSet], &acErr)
        else {
            acErr?.release()
            throw FcFieldError(code: "INTERNAL", message: "could not create access control")
        }
        let attrs: [String: Any] = [
            kSecAttrKeyType as String: kSecAttrKeyTypeECSECPrimeRandom,
            kSecAttrKeySizeInBits as String: 256,
            kSecAttrTokenID as String: kSecAttrTokenIDSecureEnclave,
            kSecPrivateKeyAttrs as String: [
                kSecAttrIsPermanent as String: true,
                kSecAttrApplicationTag as String: Data(alias.utf8),
                kSecAttrAccessControl as String: ac,
            ] as [String: Any],
        ]
        var err: Unmanaged<CFError>?
        guard let key = SecKeyCreateRandomKey(attrs as CFDictionary, &err) else {
            err?.release()
            throw FcFieldError(code: "INTERNAL", message: "could not make an unlock key")
        }
        guard let pub = SecKeyCopyPublicKey(key), let ext = SecKeyCopyExternalRepresentation(pub, nil) as Data? else {
            _ = try? deleteLocked(alias)
            throw FcFieldError(code: "INTERNAL", message: "could not read the unlock key")
        }
        let b = [UInt8](ext)
        guard b.count == 65, b[0] == 0x04 else {
            _ = try? deleteLocked(alias)
            throw FcFieldError(code: "INTERNAL", message: "unexpected public key format")
        }
        if let state = Self.biometryState() { defaults.set(state, forKey: Self.statePrefix + alias) }
        return UnlockKeyMaterial(
            x: DerSignature.toBase64Url(Array(b[1..<33])), y: DerSignature.toBase64Url(Array(b[33..<65])),
            attestationChain: nil, level: "TEE")
        #endif
    }

    public func prepare(alias: String) throws -> PreparedSignature {
        Self.lock.lock()
        defer { Self.lock.unlock() }
        guard try Self.loadKey(alias, context: nil) != nil else {
            throw FcFieldError(code: "KEY_INVALIDATED", message: "the unlock key is gone")
        }
        if let recorded = defaults.data(forKey: Self.statePrefix + alias), let now = Self.biometryState(), recorded != now {
            throw FcFieldError(code: "KEY_INVALIDATED", message: "the enrolled biometrics changed")
        }
        return SecureEnclaveSignature(alias: alias)
    }

    public func delete(alias: String) throws {
        Self.lock.lock()
        defer { Self.lock.unlock() }
        try deleteLocked(alias)
    }

    private func deleteLocked(_ alias: String) throws {
        defaults.removeObject(forKey: Self.statePrefix + alias)
        let q: [String: Any] = [
            kSecClass as String: kSecClassKey,
            kSecAttrApplicationTag as String: Data(alias.utf8),
            kSecAttrKeyType as String: kSecAttrKeyTypeECSECPrimeRandom,
        ]
        let st = SecItemDelete(q as CFDictionary)
        if st != errSecSuccess && st != errSecItemNotFound { throw FcFieldError(code: "INTERNAL", message: "keychain error \(st)") }
    }

    /// The key's reference; with `context`, one that signs under that (already evaluated) LocalAuthentication context.
    static func loadKey(_ alias: String, context: LAContext?) throws -> SecKey? {
        var q: [String: Any] = [
            kSecClass as String: kSecClassKey,
            kSecAttrApplicationTag as String: Data(alias.utf8),
            kSecAttrKeyType as String: kSecAttrKeyTypeECSECPrimeRandom,
            kSecReturnRef as String: true,
        ]
        if let context { q[kSecUseAuthenticationContext as String] = context }
        var item: CFTypeRef?
        let st = SecItemCopyMatching(q as CFDictionary, &item)
        if st == errSecItemNotFound { return nil }
        guard st == errSecSuccess, let item, CFGetTypeID(item) == SecKeyGetTypeID() else {
            throw FcFieldError(code: "INTERNAL", message: "keychain error \(st)")
        }
        return (item as! SecKey)
    }

    /// The enrolled biometrics' opaque state; nil when biometrics cannot be evaluated now.
    static func biometryState() -> Data? {
        let ctx = LAContext()
        var err: NSError?
        guard ctx.canEvaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, error: &err) else { return nil }
        if #available(iOS 18.0, macOS 15.0, *) { return ctx.domainState.biometry.stateHash }
        return ctx.evaluatedPolicyDomainState
    }
}

/// A Secure Enclave signature: the key is looked up again under the prompt's evaluated `LAContext`, so the Enclave
/// signs with that one biometric match and shows no second prompt.
struct SecureEnclaveSignature: PreparedSignature {
    let alias: String

    /// Signs under the system's own prompt (no evaluated context). The library's prompter uses `sign(_:context:)`.
    func sign(_ data: Data) throws -> Data { try sign(data, context: nil) }

    func sign(_ data: Data, context: LAContext?) throws -> Data {
        guard let key = try SecureEnclaveUnlockKeyProvider.loadKey(alias, context: context) else {
            throw FcFieldError(code: "KEY_INVALIDATED", message: "the unlock key is gone")
        }
        var err: Unmanaged<CFError>?
        guard let sig = SecKeyCreateSignature(key, .ecdsaSignatureMessageX962SHA256, data as CFData, &err) as Data? else {
            let cf = err?.takeRetainedValue()
            if let cf, CFErrorGetDomain(cf) as String == LAErrorDomain {
                throw UnlockKeyErrorMapping.map(laErrorRawCode: CFErrorGetCode(cf))
            }
            if let cf, CFErrorGetDomain(cf) as String == NSOSStatusErrorDomain, CFErrorGetCode(cf) == Int(errSecItemNotFound) {
                throw FcFieldError(code: "KEY_INVALIDATED", message: "the unlock key no longer works")
            }
            throw FcFieldError(code: "INTERNAL", message: "unlock signing failed")
        }
        do { return Data(try DerSignature.derToRaw([UInt8](sig))) } catch {
            throw FcFieldError(code: "INTERNAL", message: "malformed signature")
        }
    }
}

/// The unlock key's prompt (Kotlin `BiometricUnlockKeyPrompter`): LocalAuthentication with biometrics only
/// (`deviceOwnerAuthenticationWithBiometrics`, no passcode fallback: `localizedFallbackTitle` is empty), then the
/// Secure Enclave signs under that evaluated context. `title` is the `localizedReason`, `cancel` the cancel button. The
/// app's Info.plist needs `NSFaceIDUsageDescription` for Face ID.
public struct LocalAuthenticationUnlockKeyPrompter: UnlockKeyPrompter {
    public init() {}

    public func authorise(_ prompt: UnlockKeyPrompt, signature: PreparedSignature, data: Data) async throws -> Data {
        let ctx = LAContext()
        ctx.localizedCancelTitle = prompt.cancel
        ctx.localizedFallbackTitle = ""
        ctx.touchIDAuthenticationAllowableReuseDuration = 0
        var err: NSError?
        guard ctx.canEvaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, error: &err) else {
            throw UnlockKeyErrorMapping.map(laErrorRawCode: (err as? LAError)?.code.rawValue)
        }
        do {
            let ok = try await ctx.evaluatePolicy(.deviceOwnerAuthenticationWithBiometrics, localizedReason: prompt.title)
            if !ok { throw FieldAuthError(code: "BIOMETRIC_CANCELLED", message: "the biometric prompt was cancelled") }
        } catch let e as LAError {
            throw UnlockKeyErrorMapping.map(laErrorRawCode: e.code.rawValue)
        }
        do {
            if let se = signature as? SecureEnclaveSignature { return try se.sign(data, context: ctx) }
            return try signature.sign(data)
        } catch let e as FcFieldError {
            throw FieldAuthError(code: e.code, message: "unlock signing failed: \(e.code)")
        }
    }
}
#endif
