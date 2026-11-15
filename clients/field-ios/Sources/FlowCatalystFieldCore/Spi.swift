import Foundation

/// Key/value storage for session material (Kotlin `SecureStorage`). Implementations must be safe to call from any
/// thread. Calls are blocking and short; the session calls them from its own isolation.
public protocol SecureStorage: Sendable {
    func get(_ key: String) throws -> String?
    func put(_ key: String, value: String) throws
    func remove(_ key: String) throws
}

/// For tests and for hosts that supply their own storage. Not persistent.
public final class InMemoryStorage: SecureStorage, @unchecked Sendable {
    private let lock = NSLock()
    private var map: [String: String] = [:]

    public init() {}

    public func get(_ key: String) throws -> String? { lock.withLock { map[key] } }
    public func put(_ key: String, value: String) throws { lock.withLock { map[key] = value } }
    public func remove(_ key: String) throws { lock.withLock { _ = map.removeValue(forKey: key) } }
    public var keys: Set<String> { lock.withLock { Set(map.keys) } }
}

public struct DeviceKey: Equatable, Sendable {
    /// base64url, 32 bytes each.
    public let x: String
    public let y: String
    /// Base64 DER certificates, leaf first; nil when there is no attestation.
    public let attestationChain: [String]?
    /// STRONGBOX, TEE, SOFTWARE or NONE (unknown). On iOS: TEE for a Secure Enclave key, SOFTWARE in the simulator.
    public let level: String

    public init(x: String, y: String, attestationChain: [String]? = nil, level: String = "NONE") {
        self.x = x
        self.y = y
        self.attestationChain = attestationChain
        self.level = level
    }
}

/// The device's ES256 signing key (Kotlin `DeviceKeyProvider`). Failures are `FcFieldError`.
public protocol DeviceKeyProvider: Sendable {
    func getOrCreate() throws -> DeviceKey
    /// ES256 over SHA-256(data); returns raw r‖s (64 bytes).
    func sign(_ data: Data) throws -> Data
    func delete() throws
}

/// Shows the app-unlock prompt (Kotlin `UnlockPrompter`). Throws `FieldAuthError` (BIOMETRIC_CANCELLED,
/// BIOMETRIC_UNAVAILABLE, ...).
public protocol UnlockPrompter: Sendable {
    func prompt(_ text: String) async throws
}

/// The device clock and timer source (Kotlin: the `clock` lambda plus coroutine `delay`).
public protocol FieldClock: Sendable {
    /// Milliseconds since the epoch.
    func nowMilliseconds() -> Int64
    /// Suspends for `milliseconds`; throws `CancellationError` when the calling task is cancelled.
    func sleep(milliseconds: Int64) async throws
}

public struct SystemClock: FieldClock {
    public init() {}

    public func nowMilliseconds() -> Int64 { Int64((Date().timeIntervalSince1970 * 1000).rounded(.down)) }

    public func sleep(milliseconds: Int64) async throws {
        if milliseconds <= 0 { return }
        try await Task.sleep(nanoseconds: UInt64(milliseconds) * 1_000_000)
    }
}

public struct HTTPRequest: Sendable {
    public var url: String
    public var method: String
    public var headers: [String: String]
    public var body: Data?

    public init(url: String, method: String = "GET", headers: [String: String] = [:], body: Data? = nil) {
        self.url = url
        self.method = method
        self.headers = headers
        self.body = body
    }

    public func header(_ name: String) -> String? {
        headers.first { $0.key.caseInsensitiveCompare(name) == .orderedSame }?.value
    }

    /// Sets a header, replacing any existing one of the same name (any case).
    public mutating func setHeader(_ name: String, _ value: String) {
        for k in headers.keys where k.caseInsensitiveCompare(name) == .orderedSame { headers.removeValue(forKey: k) }
        headers[name] = value
    }
}

public struct HTTPResponse: Sendable {
    public var status: Int
    public var headers: [String: String]
    public var body: Data

    public init(status: Int, headers: [String: String] = [:], body: Data = Data()) {
        self.status = status
        self.headers = headers
        self.body = body
    }

    /// Case-insensitive.
    public func header(_ name: String) -> String? {
        headers.first { $0.key.caseInsensitiveCompare(name) == .orderedSame }?.value
    }
}

/// The HTTP transport. Throws any error when the host cannot be reached; the session reports it as `NETWORK`
/// (the original error is dropped on purpose: it may quote the request). Should not follow redirects silently
/// into another origin with credentials. Cancellation must surface as `CancellationError`.
public protocol HTTPTransport: Sendable {
    func send(_ request: HTTPRequest) async throws -> HTTPResponse
}
