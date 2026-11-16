import Foundation
import Security

/// What a promise rejection carries. `message` never contains a PIN or a token. Same fields as the Android bridge.
struct FcFieldRejection {
    let code: String
    let message: String
    var status: Int?
    var retryAfterSeconds: Int?
    var violations: [String] = []

    /// FieldAuthError keeps its code; a key/store FcFieldError keeps its code; anything else is INTERNAL.
    static func classify(_ error: Error) -> FcFieldRejection {
        if let e = error as? FieldAuthError {
            // The library words this one for Swift callers; the bridge uses the Android wording.
            let message = e.code == "CONFIG_MISMATCH" ? "FieldSession is already configured with a different config" : e.message
            return FcFieldRejection(code: e.code, message: message, status: e.status,
                                    retryAfterSeconds: e.retryAfterSeconds, violations: e.violations)
        }
        if let e = error as? FcFieldError { return FcFieldRejection(code: e.code, message: e.message) }
        return FcFieldRejection(code: "INTERNAL", message: "unexpected native error")
    }

    /// The rejection's user-info, as `FieldClient`'s `mapNativeError` reads it.
    var userInfo: [String: Any] {
        var m: [String: Any] = [:]
        if let status { m["status"] = status }
        if let retryAfterSeconds { m["retryAfterSeconds"] = retryAfterSeconds }
        if !violations.isEmpty { m["violations"] = violations }
        return m
    }
}

/// The Swift half of the `FcField` TurboModule (the Objective-C++ class `FcField` forwards every spec method here).
/// The session methods delegate to the process-wide field-ios `FieldSession`; the key/store methods serve the WebView
/// bridge, and use the same `SecureEnclaveKeyProvider` key as the session, so there is one device key per app.
/// Nothing here logs values, PINs, tokens, data or signatures.
@objc(FcFieldHost)
public final class FcFieldHost: NSObject {
    /// Set by the Objective-C++ module: delivers a native event (name, payload) to `NativeEventEmitter`.
    @objc public var eventSink: ((String, [String: Any]) -> Void)?

    private let lock = NSLock()
    private var session: FieldSession?
    private var subscription: FieldListenerToken?

    private let keys = SecureEnclaveKeyProvider()
    private let store = SecureStoreKeychain()

    public override init() { super.init() }

    deinit { subscription?.cancel() }

    @objc public func invalidate() {
        lock.lock()
        let s = subscription
        subscription = nil
        lock.unlock()
        s?.cancel()
    }

    // MARK: dispatch

    public typealias Resolve = (Any?) -> Void
    public typealias Reject = (String, String, [String: Any]) -> Void

    private func launch(_ resolve: @escaping Resolve, _ reject: @escaping Reject, _ block: @escaping () async throws -> Any?) {
        Task {
            do {
                resolve(try await block() ?? NSNull())
            } catch is CancellationError {
                // cancellation is not an error
            } catch {
                let r = FcFieldRejection.classify(error)
                reject(r.code, r.message, r.userInfo)
            }
        }
    }

    /// Key and Keychain calls block (a biometric prompt waits for the user), so they run on their own serial queue,
    /// never on the Swift concurrency pool.
    private let ioQueue = DispatchQueue(label: "io.flowcatalyst.field.io", qos: .userInitiated)

    private func blocking(_ resolve: @escaping Resolve, _ reject: @escaping Reject, _ block: @escaping () throws -> Any?) {
        ioQueue.async {
            do {
                resolve(try block() ?? NSNull())
            } catch {
                let r = FcFieldRejection.classify(error)
                reject(r.code, r.message, r.userInfo)
            }
        }
    }

    private func owner() throws -> FieldSession {
        lock.lock()
        defer { lock.unlock() }
        guard let s = session else { throw FieldAuthError(code: "NOT_CONFIGURED", message: "configure() must be called first") }
        return s
    }

    private func emit(_ e: FieldEvent) {
        let (name, payload) = FcFieldJson.event(e)
        eventSink?(name, payload)
    }

    // MARK: session owner

    @objc public func configure(_ config: [String: Any], resolve: @escaping Resolve, reject: @escaping Reject) {
        launch(resolve, reject) { [self] in
            let s = try FieldSession.get(config: try FieldBridgeConfig.fieldConfig(from: config))
            lock.lock()
            if session !== s || subscription == nil {
                subscription?.cancel()
                subscription = s.addListener { [weak self] e in self?.emit(e) }
            }
            session = s
            lock.unlock()
            return nil
        }
    }

    @objc public func restore(_ resolve: @escaping Resolve, reject: @escaping Reject) {
        launch(resolve, reject) { [self] in
            let s = try owner()
            return FcFieldJson.state(try await s.restore(), device: try await s.enrolledDevice())
        }
    }

    @objc public func getState(_ resolve: @escaping Resolve, reject: @escaping Reject) {
        launch(resolve, reject) { [self] in
            let s = try owner()
            return FcFieldJson.state(s.state, device: try await s.enrolledDevice())
        }
    }

    @objc public func enrol(_ qrOrCode: String, info: [String: Any], resolve: @escaping Resolve, reject: @escaping Reject) {
        launch(resolve, reject) { [self] in
            FcFieldJson.device(try await owner().enrol(qrOrCode, info: FcFieldJson.deviceInfo(info)))
        }
    }

    @objc public func login(_ creds: [String: Any], resolve: @escaping Resolve, reject: @escaping Reject) {
        launch(resolve, reject) { [self] in
            FcFieldJson.session(try await owner().login(FcFieldJson.credentials(creds)))
        }
    }

    @objc public func redeemSignInCode(_ code: String, resolve: @escaping Resolve, reject: @escaping Reject) {
        launch(resolve, reject) { [self] in FcFieldJson.session(try await owner().redeemSignInCode(code)) }
    }

    @objc public func actAs(_ target: [String: Any], resolve: @escaping Resolve, reject: @escaping Reject) {
        launch(resolve, reject) { [self] in
            FcFieldJson.session(try await owner().actAs(try FcFieldJson.actAsTarget(target)))
        }
    }

    @objc public func endImpersonation(_ resolve: @escaping Resolve, reject: @escaping Reject) {
        launch(resolve, reject) { [self] in
            let s = try await owner().endImpersonation()
            return ["session": s.map { FcFieldJson.session($0) } ?? NSNull()] as [String: Any]
        }
    }

    @objc public func changePin(_ currentPin: String, newPin: String, resolve: @escaping Resolve, reject: @escaping Reject) {
        launch(resolve, reject) { [self] in
            try await owner().changePin(current: currentPin, new: newPin)
            return nil
        }
    }

    @objc public func logout(_ scope: String, resolve: @escaping Resolve, reject: @escaping Reject) {
        launch(resolve, reject) { [self] in
            let sc = try FcFieldJson.logoutScope(scope)
            try await owner().logout(scope: sc)
            return nil
        }
    }

    @objc public func unlock(_ prompt: String, resolve: @escaping Resolve, reject: @escaping Reject) {
        launch(resolve, reject) { [self] in FcFieldJson.session(try await owner().unlock(prompt: prompt)) }
    }

    @objc public func refresh(_ resolve: @escaping Resolve, reject: @escaping Reject) {
        launch(resolve, reject) { [self] in FcFieldJson.session(try await owner().refresh()) }
    }

    @objc public func headersFor(_ method: String, url: String, resolve: @escaping Resolve, reject: @escaping Reject) {
        launch(resolve, reject) { [self] in try await owner().headersFor(method: method, url: url) }
    }

    @objc public func headersForRetry(_ method: String, url: String, nonce: String, resolve: @escaping Resolve, reject: @escaping Reject) {
        launch(resolve, reject) { [self] in try await owner().headersForRetry(method: method, url: url, nonce: nonce) }
    }

    // The two methods without a promise run through one ordered chain: a later call is never applied before an
    // earlier one (keepAlive(true) then keepAlive(false) ends off).
    private let ordered = OrderedTaskChain()

    @objc public func noteNonce(_ url: String, nonce: String) {
        guard let s = try? owner() else { return }
        ordered.enqueue { await s.noteNonce(url: url, nonce: nonce) }
    }

    @objc public func keepAlive(_ on: Bool) {
        guard let s = try? owner() else { return }
        ordered.enqueue { await s.keepAlive(on) }
    }

    // MARK: key / store (WebView bridge protocol v1, one device key per app)

    @objc public func getOrCreateDeviceKey(_ resolve: @escaping Resolve, reject: @escaping Reject) {
        blocking(resolve, reject) { [self] in
            let k = try keys.getOrCreate()
            var attestation: [String: Any] = ["level": k.level]
            if let chain = k.attestationChain { attestation["chain"] = chain }
            return ["jwk": ["kty": "EC", "crv": "P-256", "x": k.x, "y": k.y], "attestation": attestation] as [String: Any]
        }
    }

    @objc public func sign(_ dataB64u: String, resolve: @escaping Resolve, reject: @escaping Reject) {
        blocking(resolve, reject) { [self] in
            guard let data = Base64URL.decode(dataB64u) else { throw FcFieldError(code: "INTERNAL", message: "bad base64url") }
            return Base64URL.encode(try keys.sign(data))
        }
    }

    @objc public func deleteDeviceKey(_ resolve: @escaping Resolve, reject: @escaping Reject) {
        blocking(resolve, reject) { [self] in
            try keys.delete()
            return nil
        }
    }

    @objc public func storeGet(_ key: String, prompt: String?, resolve: @escaping Resolve, reject: @escaping Reject) {
        blocking(resolve, reject) { [self] in try store.get(key, prompt: prompt) }
    }

    @objc public func storeSet(_ key: String, value: String, requireBiometric: Bool, resolve: @escaping Resolve, reject: @escaping Reject) {
        blocking(resolve, reject) { [self] in
            try store.set(key, value: value, biometric: requireBiometric)
            return nil
        }
    }

    @objc public func storeDelete(_ key: String, resolve: @escaping Resolve, reject: @escaping Reject) {
        blocking(resolve, reject) { [self] in
            store.delete(key)
            return nil
        }
    }

    @objc public func biometricAvailable(_ resolve: @escaping Resolve, reject: @escaping Reject) {
        blocking(resolve, reject) { [self] in store.biometricAvailable() }
    }
}
