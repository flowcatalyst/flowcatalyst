import Foundation
import XCTest
#if canImport(CryptoKit)
import CryptoKit
#else
import Crypto
#endif
@testable import FlowCatalystFieldCore

// Port of clients/field-android/src/test/kotlin/io/flowcatalyst/field/TestSupport.kt, with an in-memory network in
// place of MockWebServer and a virtual clock in place of kotlinx-coroutines-test.

let BASE_TIME: Int64 = 1_800_000_000_000
let MIN: Int64 = 60_000

/// Counts clock and network calls so the virtual clock can tell when the session's timer tasks have gone quiet.
final class Activity: @unchecked Sendable {
    private let lock = NSLock()
    private var n = 0
    func bump() { lock.withLock { n += 1 } }
    var value: Int { lock.withLock { n } }
}

final class Locked<T>: @unchecked Sendable {
    private let lock = NSLock()
    private var v: T
    init(_ v: T) { self.v = v }
    var value: T {
        get { lock.withLock { v } }
        set { lock.withLock { v = newValue } }
    }
    func mutate<R>(_ f: (inout T) -> R) -> R { lock.withLock { f(&v) } }
}

func sha256b64u(_ s: String) -> String { Base64URL.encode(Data(SHA256.hash(data: Data(s.utf8)))) }

// MARK: - keys

/// A software P-256 key behind the same interface the Secure Enclave key uses.
final class SoftKeyProvider: DeviceKeyProvider, @unchecked Sendable {
    let privateKey = P256.Signing.PrivateKey()
    let key: DeviceKey

    init(level: String = "NONE", chain: [String]? = nil) {
        let x963 = [UInt8](privateKey.publicKey.x963Representation)
        key = DeviceKey(
            x: DerSignature.toBase64Url(Array(x963[1..<33])),
            y: DerSignature.toBase64Url(Array(x963[33..<65])), attestationChain: chain, level: level)
    }

    func getOrCreate() throws -> DeviceKey { key }

    func sign(_ data: Data) throws -> Data {
        let sig = try privateKey.signature(for: data)
        return Data(try DerSignature.derToRaw([UInt8](sig.derRepresentation)))
    }

    func delete() throws {}
}

/// A parsed DPoP proof; `valid()` is true when the ES256 signature verifies against the embedded jwk.
struct Proof {
    let jwt: String
    let parts: [String]
    let header: JObject
    let claims: JObject

    init(_ jwt: String) {
        self.jwt = jwt
        parts = jwt.split(separator: ".", omittingEmptySubsequences: false).map(String.init)
        header = JSON.parseObject(Base64URL.decode(parts[0])!)!
        claims = JSON.parseObject(Base64URL.decode(parts[1])!)!
    }

    var nonce: String? { claims.has("nonce") ? claims.optString("nonce") : nil }
    var jwk: JObject { header.optObject("jwk")! }

    func valid() -> Bool {
        guard let x = Base64URL.decode(jwk.optString("x")), let y = Base64URL.decode(jwk.optString("y")),
              let sigBytes = Base64URL.decode(parts[2]), sigBytes.count == 64,
              let pub = try? P256.Signing.PublicKey(x963Representation: Data([4]) + x + y),
              let sig = try? P256.Signing.ECDSASignature(rawRepresentation: sigBytes)
        else { return false }
        return pub.isValidSignature(sig, for: Data((parts[0] + "." + parts[1]).utf8))
    }
}

// MARK: - clock

/// The test clock. `jump` moves it by hand (timers stay asleep, as real delays would); `advance` moves virtual time
/// and wakes every sleeper whose deadline passes, in order, letting the session's timer tasks settle between wake-ups.
final class TestClock: FieldClock, @unchecked Sendable {
    private struct Sleeper {
        let id: Int
        let deadline: Int64
        let cont: CheckedContinuation<Void, Error>
    }

    private let lock = NSLock()
    private var nowMs: Int64
    private var sleepers: [Sleeper] = []
    private var nextId = 0
    let activity: Activity

    init(start: Int64 = BASE_TIME, activity: Activity = Activity()) {
        nowMs = start
        self.activity = activity
    }

    func nowMilliseconds() -> Int64 {
        activity.bump()
        return lock.withLock { nowMs }
    }

    func now() -> Int64 { lock.withLock { nowMs } }
    func jump(_ ms: Int64) { lock.withLock { nowMs += ms } }

    func sleep(milliseconds: Int64) async throws {
        activity.bump()
        if milliseconds <= 0 { return }
        let id: Int = lock.withLock { nextId += 1; return nextId }
        try await withTaskCancellationHandler {
            try await withCheckedThrowingContinuation { (cont: CheckedContinuation<Void, Error>) in
                let cancelled: Bool = lock.withLock {
                    if Task.isCancelled { return true }
                    sleepers.append(Sleeper(id: id, deadline: nowMs + milliseconds, cont: cont))
                    return false
                }
                if cancelled { cont.resume(throwing: CancellationError()) }
            }
        } onCancel: {
            let s: Sleeper? = lock.withLock {
                guard let i = sleepers.firstIndex(where: { $0.id == id }) else { return nil }
                return sleepers.remove(at: i)
            }
            s?.cont.resume(throwing: CancellationError())
        }
    }

    /// Waits until the clock and the network have been idle for a few milliseconds of real time.
    func settle() async {
        var quiet = 0
        var last = activity.value
        while quiet < 4 {
            try? await Task.sleep(nanoseconds: 2_000_000)
            let a = activity.value
            if a == last { quiet += 1 } else { quiet = 0; last = a }
        }
    }

    func advance(_ ms: Int64) async {
        await settle()
        let target = lock.withLock { nowMs + ms }
        while true {
            let next: Sleeper? = lock.withLock {
                guard let i = sleepers.indices.filter({ sleepers[$0].deadline <= target })
                    .min(by: { sleepers[$0].deadline < sleepers[$1].deadline })
                else { return nil }
                let s = sleepers.remove(at: i)
                nowMs = max(nowMs, s.deadline)
                return s
            }
            guard let s = next else { break }
            s.cont.resume()
            await settle()
        }
        lock.withLock { nowMs = target }
        await settle()
    }

    /// Resumes every sleeper with a cancellation (test teardown).
    func cancelAll() {
        let all: [Sleeper] = lock.withLock { let a = sleepers; sleepers = []; return a }
        for s in all { s.cont.resume(throwing: CancellationError()) }
    }
}

// MARK: - network

struct Rec {
    let method: String
    let path: String
    let headers: [String: String]
    let body: JObject?
    var raw: Data = Data()

    func header(_ name: String) -> String? { headers.first { $0.key.caseInsensitiveCompare(name) == .orderedSame }?.value }
    var proof: Proof? { header("DPoP").map { Proof($0) } }
    var bearer: String? { header("Authorization").map { $0.hasPrefix("DPoP ") ? String($0.dropFirst(5)) : $0 } }
}

struct FakeResponse {
    var status = 200
    var body = ""
    var headers: [String: String] = [:]
    var delayMs: Int = 0
    var bytes: Data? = nil

    func header(_ k: String, _ v: String) -> FakeResponse { var r = self; r.headers[k] = v; return r }
    func delayed(_ ms: Int) -> FakeResponse { var r = self; r.delayMs = ms; return r }
    static func json(_ status: Int, _ body: String) -> FakeResponse { FakeResponse(status: status, body: body) }
}

struct Unreachable: Error {}

/// Routes by origin; a host without a route is unreachable (the transport throws).
final class FakeNetwork: HTTPTransport, @unchecked Sendable {
    private let routes = Locked<[String: (Rec) -> FakeResponse]>([:])
    let activity: Activity
    /// While true every request fails before an answer: the library sees no network.
    let offline = Locked<Bool>(false)

    init(activity: Activity) { self.activity = activity }

    func mount(origin: String, _ h: @escaping (Rec) -> FakeResponse) { routes.mutate { $0[origin] = h } }

    func send(_ request: HTTPRequest) async throws -> HTTPResponse {
        activity.bump()
        if offline.value { throw Unreachable() }
        guard let url = HTTPURL(request.url), let route = routes.value[url.origin] else { throw Unreachable() }
        let comps = URLComponents(string: request.url)
        let path = (comps?.percentEncodedPath ?? "/") + (comps?.percentEncodedQuery.map { "?" + $0 } ?? "")
        let rec = Rec(
            method: request.method, path: path, headers: request.headers,
            body: request.body.flatMap { JSON.parseObject($0) }, raw: request.body ?? Data())
        let resp = route(rec)
        if resp.delayMs > 0 { try await Task.sleep(nanoseconds: UInt64(resp.delayMs) * 1_000_000) }
        activity.bump()
        return HTTPResponse(status: resp.status, headers: resp.headers, body: resp.bytes ?? Data(resp.body.utf8))
    }
}

/// A fake platform: enforces single-use nonces on every /field/ route except the nonce route, puts a fresh
/// DPoP-Nonce on every response, records every request.
final class FakePlatform: @unchecked Sendable {
    let origin: String
    var url: String { origin }
    private let clockFn: Locked<() -> Int64>
    private let reqs = Locked<[Rec]>([])
    private let handlers = Locked<[String: (Rec) -> FakeResponse]>([:])
    private let nonces = Locked<Set<String>>([])
    private let seq = Locked<Int>(0)
    private let tokenSeq = Locked<Int>(0)
    private let refreshSeq = Locked<Int>(0)
    let failNextNonceChecks = Locked<Int>(0)
    let idleWindowMs = Locked<Int64>(30 * MIN)
    let absoluteWindowMs = Locked<Int64>(60 * MIN)
    let accessLifetimeSec = Locked<Int>(600)

    init(origin: String, clock: @escaping () -> Int64) {
        self.origin = origin
        clockFn = Locked(clock)
        on("/field/auth/nonce") { _ in .json(200, #"{"nonce":"x","expiresAt":"2030-01-01T00:00:00Z"}"#) }
    }

    var requests: [Rec] { reqs.value }
    func on(_ path: String, _ h: @escaping (Rec) -> FakeResponse) { handlers.mutate { $0[path] = h } }
    func count(_ path: String) -> Int { requests.filter { $0.path == path }.count }
    func all(_ path: String) -> [Rec] { requests.filter { $0.path == path } }

    private func freshNonce() -> String {
        let n = "nonce-" + String(seq.mutate { $0 += 1; return $0 })
        nonces.mutate { _ = $0.insert(n) }
        return n
    }

    func dispatch(_ rec: Rec) -> FakeResponse {
        reqs.mutate { $0.append(rec) }
        if rec.path.hasPrefix("/field/") && rec.path != "/field/auth/nonce" {
            let n = rec.proof?.nonce
            let forced = failNextNonceChecks.mutate { v -> Bool in
                if v > 0 { v -= 1; return true }
                return false
            }
            let known = n.map { nn in nonces.mutate { $0.remove(nn) != nil } } ?? false
            if forced || n == nil || !known {
                return FakeResponse.json(401, #"{"error":"use_dpop_nonce"}"#)
                    .header("WWW-Authenticate", "DPoP error=\"use_dpop_nonce\"")
                    .header("DPoP-Nonce", freshNonce())
            }
        }
        let h = handlers.value[rec.path]
        let resp = h?(rec) ?? .json(404, #"{"error":"NOT_FOUND"}"#)
        return resp.header("DPoP-Nonce", freshNonce())
    }

    /// A FieldTokenResponse. The access token is an (unsigned) JWT with `claims`; `jti` is unique per call.
    func token(
        workerId: String = "w1", workerCode: String = "W001", claims: JObject = [:],
        pinChangeRequired: Bool = false, refreshExpiresAt: Int64? = nil,
        photoVersion: String? = nil, canSetOwnPhoto: Bool = false, unlockKeys: JObject? = nil
    ) -> FakeResponse {
        let n = tokenSeq.mutate { $0 += 1; return $0 }
        var c = claims
        c["jti"] = "at-\(n)"
        let jwt = Base64URL.encode(Data(#"{"alg":"none"}"#.utf8)) + "." + Base64URL.encode(JSON.data(c)) + ".sig"
        let now = clockFn.value()
        var body: JObject = [
            "accessToken": jwt,
            "tokenType": "DPoP",
            "expiresIn": accessLifetimeSec.value,
            "refreshToken": "fwr_" + String(refreshSeq.mutate { $0 += 1; return $0 }),
            "refreshExpiresAt": formatInstant(refreshExpiresAt ?? (now + absoluteWindowMs.value)),
            "idleExpiresAt": formatInstant(now + idleWindowMs.value),
            "worker": [
                "id": workerId, "workerCode": workerCode, "name": "Name \(workerCode)",
                "photoVersion": photoVersion ?? NSNull(), "canSetOwnPhoto": canSetOwnPhoto,
            ] as JObject,
            "pinChangeRequired": pinChangeRequired,
        ]
        if let unlockKeys { body["unlockKeys"] = unlockKeys }
        return .json(200, JSON.string(body))
    }

    func error(_ status: Int, _ code: String, message: String? = nil, retryAfter: Int? = nil) -> FakeResponse {
        var r = FakeResponse.json(status, JSON.string(["error": code, "message": message ?? code]))
        if let retryAfter { r.headers["Retry-After"] = String(retryAfter) }
        return r
    }

    func deviceClaims(_ mode: String = "PERSONAL", id: String = "dev1") -> JObject { ["device_mode": mode, "device_id": id] }
}

/// An application backend: reusable nonce, accepts tokens per `accept`.
final class FakeApp: @unchecked Sendable {
    let origin: String
    var url: String { origin }
    private let reqs = Locked<[Rec]>([])
    let requireNonce = Locked<Bool>(true)
    let accept = Locked<(String) -> Bool>({ _ in true })
    private let current = "app-nonce-1"

    init(origin: String) { self.origin = origin }

    var requests: [Rec] { reqs.value }

    func dispatch(_ rec: Rec) -> FakeResponse {
        reqs.mutate { $0.append(rec) }
        if requireNonce.value && rec.proof?.nonce != current {
            return FakeResponse(status: 401).header("WWW-Authenticate", "DPoP error=\"use_dpop_nonce\"").header("DPoP-Nonce", current)
        }
        if rec.bearer == nil || !accept.value(rec.bearer!) {
            return FakeResponse(status: 401).header("WWW-Authenticate", "DPoP error=\"invalid_token\"").header("DPoP-Nonce", current)
        }
        return FakeResponse.json(200, #"{"ok":true}"#).header("DPoP-Nonce", current)
    }
}

// MARK: - harness

final class EventLog: @unchecked Sendable {
    private let items = Locked<[FieldEvent]>([])
    func add(_ e: FieldEvent) { items.mutate { $0.append(e) } }
    var all: [FieldEvent] { items.value }
    func clear() { items.value = [] }
    func contains(_ e: FieldEvent) -> Bool { all.contains(e) }
    var warnings: [FieldEvent] { all.filter { if case .expiryWarning = $0 { return true } else { return false } } }
    var isEmpty: Bool { all.isEmpty }
}

final class Harness: @unchecked Sendable {
    let activity = Activity()
    let clock: TestClock
    let storage = InMemoryStorage()
    let keys: SoftKeyProvider
    let unlockKeys: UnlockKeyProvider?
    let unlockKeyPrompter: UnlockKeyPrompter?
    let net: FakeNetwork
    let platform: FakePlatform
    let app: FakeApp
    let events = EventLog()

    init(
        keyLevel: String = "NONE", keyChain: [String]? = nil, unlockKeys: UnlockKeyProvider? = nil,
        unlockKeyPrompter: UnlockKeyPrompter? = nil
    ) {
        keys = SoftKeyProvider(level: keyLevel, chain: keyChain)
        self.unlockKeys = unlockKeys
        self.unlockKeyPrompter = unlockKeyPrompter
        clock = TestClock(activity: activity)
        net = FakeNetwork(activity: activity)
        let c = clock
        platform = FakePlatform(origin: "http://127.0.0.1:18080", clock: { c.now() })
        app = FakeApp(origin: "http://127.0.0.1:18081")
        net.mount(origin: platform.origin) { [platform] in platform.dispatch($0) }
        net.mount(origin: app.origin) { [app] in app.dispatch($0) }
    }

    func config(
        clientIdentifier: String? = "acme", requireUnlock: UnlockPolicy = .never, platformUrl: String? = nil
    ) -> FieldConfig {
        FieldConfig(platformUrl: platformUrl ?? platform.url, appCode: "app1", clientIdentifier: clientIdentifier, requireUnlock: requireUnlock)
    }

    func session(_ config: FieldConfig? = nil, storage: InMemoryStorage? = nil, transport: HTTPTransport? = nil) -> FieldSession {
        let log = events
        let s = FieldSession(
            config: config ?? self.config(), storage: storage ?? self.storage, keys: keys,
            transport: transport ?? net, clock: clock, unlockKeys: unlockKeys, unlockKeyPrompter: unlockKeyPrompter)
        s.addListener { log.add($0) }
        return s
    }

    func close() { clock.cancelAll() }
}

// MARK: - assertions

extension XCTestCase {
    /// Runs `block`, expects a `FieldAuthError` with `code`, returns it.
    @discardableResult
    func failsWith(
        _ code: String, file: StaticString = #filePath, line: UInt = #line, _ block: () async throws -> Void
    ) async -> FieldAuthError? {
        do {
            try await block()
            XCTFail("expected FieldAuthError \(code)", file: file, line: line)
        } catch let e as FieldAuthError {
            XCTAssertEqual(code, e.code, file: file, line: line)
            return e
        } catch {
            XCTFail("expected FieldAuthError \(code), got \(error)", file: file, line: line)
        }
        return nil
    }
}

// MARK: - unlock keys

/// ES256 verification of a compact JWS against a P-256 public key given as JWK x / y (base64url).
func verifyEs256(_ jws: String, x: String, y: String) -> Bool {
    let parts = jws.split(separator: ".", omittingEmptySubsequences: false).map(String.init)
    guard parts.count == 3, let xd = Base64URL.decode(x), let yd = Base64URL.decode(y),
          let sigBytes = Base64URL.decode(parts[2]), sigBytes.count == 64,
          let pub = try? P256.Signing.PublicKey(x963Representation: Data([4]) + xd + yd),
          let sig = try? P256.Signing.ECDSASignature(rawRepresentation: sigBytes)
    else { return false }
    return pub.isValidSignature(sig, for: Data((parts[0] + "." + parts[1]).utf8))
}

func jwsHeader(_ jws: String) -> JObject { JSON.parseObject(Base64URL.decode(String(jws.split(separator: ".")[0]))!)! }
func jwsClaims(_ jws: String) -> JObject { JSON.parseObject(Base64URL.decode(String(jws.split(separator: ".")[1]))!)! }

/// Software P-256 unlock keys behind the Secure Enclave's interface; tests flip `biometric`, `level` and `invalidated`.
final class SoftUnlockKeys: UnlockKeyProvider, @unchecked Sendable {
    let biometric = Locked<UnlockKeyAvailability>(.available)
    let level = Locked<String>("TEE")
    let chain = Locked<[String]?>(nil)
    let keys = Locked<[String: P256.Signing.PrivateKey]>([:])
    let invalidated = Locked<Set<String>>([])
    let deleted = Locked<[String]>([])
    let created = Locked<Int>(0)

    func biometricStatus() -> UnlockKeyAvailability { biometric.value }
    /// The Secure Enclave's answer by default (no attestation); a test may say yes to check the attesting path.
    let canAttest = Locked<Bool>(false)
    var attests: Bool { canAttest.value }

    func create(alias: String) throws -> UnlockKeyMaterial {
        let k = P256.Signing.PrivateKey()
        keys.mutate { $0[alias] = k }
        created.mutate { $0 += 1 }
        let b = [UInt8](k.publicKey.x963Representation)
        return UnlockKeyMaterial(
            x: DerSignature.toBase64Url(Array(b[1..<33])), y: DerSignature.toBase64Url(Array(b[33..<65])),
            attestationChain: chain.value, level: level.value)
    }

    func prepare(alias: String) throws -> PreparedSignature {
        guard let k = keys.value[alias], !invalidated.value.contains(alias) else {
            throw FcFieldError(code: "KEY_INVALIDATED", message: "gone")
        }
        return SoftSignature(key: k)
    }

    func delete(alias: String) throws {
        keys.mutate { _ = $0.removeValue(forKey: alias) }
        deleted.mutate { $0.append(alias) }
    }

    var aliases: Set<String> { Set(keys.value.keys) }
}

struct SoftSignature: PreparedSignature {
    let key: P256.Signing.PrivateKey
    func sign(_ data: Data) throws -> Data {
        Data(try DerSignature.derToRaw([UInt8](try key.signature(for: data).derRepresentation)))
    }
}

/// A prompt that signs (a "biometric match") or throws, counting how often it was shown.
struct FakeUnlockPrompter: UnlockKeyPrompter {
    let shown: Locked<Int>
    let body: @Sendable (UnlockKeyPrompt, PreparedSignature, Data) throws -> Data

    func authorise(_ prompt: UnlockKeyPrompt, signature: PreparedSignature, data: Data) async throws -> Data {
        shown.mutate { $0 += 1 }
        return try body(prompt, signature, data)
    }
}
