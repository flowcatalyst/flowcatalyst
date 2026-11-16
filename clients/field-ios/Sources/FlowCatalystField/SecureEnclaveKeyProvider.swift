#if canImport(Security)
import Foundation
import Security
#if canImport(FlowCatalystFieldCore)
import FlowCatalystFieldCore
#endif

/// The device's ES256 key in the Secure Enclave (Kotlin `AndroidKeyProvider`): a permanent EC P-256 key tagged
/// `io.flowcatalyst.field.device` (Secure Enclave, `kSecAttrAccessibleWhenUnlockedThisDeviceOnly` with
/// `.privateKeyUsage`; a software Keychain key in the simulator). The React Native module's key methods call this
/// class, so the session owner and the WebView bridge share one device key per app. Never logs key material or
/// signatures.
public final class SecureEnclaveKeyProvider: DeviceKeyProvider, @unchecked Sendable {
    /// The Keychain application tag. Do not change it: keys created under it are looked up by it.
    public static let keyTag = "io.flowcatalyst.field.device"

    private static let tag = Data(keyTag.utf8)
    // One lock per process: every provider instance shares it (like the Android `shared` DeviceKeyManager).
    private static let lock = NSLock()

    public init() {}

    private static func loadKey() throws -> SecKey? {
        let q: [String: Any] = [
            kSecClass as String: kSecClassKey,
            kSecAttrApplicationTag as String: tag,
            kSecAttrKeyType as String: kSecAttrKeyTypeECSECPrimeRandom,
            kSecReturnRef as String: true,
        ]
        var item: CFTypeRef?
        let st = SecItemCopyMatching(q as CFDictionary, &item)
        if st == errSecItemNotFound { return nil }
        guard st == errSecSuccess, let item, CFGetTypeID(item) == SecKeyGetTypeID() else {
            throw FcFieldError(code: "INTERNAL", message: "keychain error \(st)")
        }
        return (item as! SecKey)
    }

    private static func create() throws -> SecKey {
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
            err?.release()
            throw FcFieldError(code: "INTERNAL", message: "could not generate device key")
        }
        return key
    }

    public func getOrCreate() throws -> DeviceKey {
        Self.lock.lock()
        defer { Self.lock.unlock() }
        let key = try Self.loadKey() ?? Self.create()
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
        return DeviceKey(
            x: DerSignature.toBase64Url(Array(b[1..<33])),
            y: DerSignature.toBase64Url(Array(b[33..<65])),
            attestationChain: nil,
            level: level)
    }

    public func sign(_ data: Data) throws -> Data {
        Self.lock.lock()
        defer { Self.lock.unlock() }
        // A missing key (e.g. Keychain restored onto another device) is KEY_INVALIDATED, never a crash.
        guard let key = try Self.loadKey() else { throw FcFieldError(code: "KEY_INVALIDATED", message: "no device key") }
        var err: Unmanaged<CFError>?
        guard let sig = SecKeyCreateSignature(key, .ecdsaSignatureMessageX962SHA256, data as CFData, &err) as Data? else {
            let cf = err?.takeRetainedValue()
            if let cf, CFErrorGetDomain(cf) as String == NSOSStatusErrorDomain,
               CFErrorGetCode(cf) == Int(errSecInteractionNotAllowed) {
                // The key is only usable while the device is unlocked (`WhenUnlockedThisDeviceOnly`, as the React
                // Native module creates it). That is not an invalidation: do not tell the app to re-enrol.
                throw FcFieldError(code: "INTERNAL", message: "device key is unavailable while the device is locked")
            }
            throw FcFieldError(code: "KEY_INVALIDATED", message: "device key is no longer usable")
        }
        do { return Data(try DerSignature.derToRaw([UInt8](sig))) } catch {
            throw FcFieldError(code: "INTERNAL", message: "malformed signature")
        }
    }

    public func delete() throws {
        Self.lock.lock()
        defer { Self.lock.unlock() }
        let q: [String: Any] = [
            kSecClass as String: kSecClassKey,
            kSecAttrApplicationTag as String: Self.tag,
            kSecAttrKeyType as String: kSecAttrKeyTypeECSECPrimeRandom,
        ]
        let st = SecItemDelete(q as CFDictionary)
        if st != errSecSuccess && st != errSecItemNotFound { throw FcFieldError(code: "INTERNAL", message: "keychain error \(st)") }
    }
}
#endif
