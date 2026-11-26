import Foundation

/// The process-wide owner of a field worker's session: device key, tokens, refresh, nonces (Kotlin `FieldSession`).
/// No UI; never needs the main thread. One per process: the tracking code and any UI (including React Native) must
/// run in the app's process and share it. On Apple platforms obtain it with `FieldSession.get(config:)`.
///
/// An actor: its state is serialised, and every `await` is a point where another call may run, so the rules that
/// Kotlin enforces with mutexes (one platform call at a time, one refresh at a time) are explicit locks here.
public actor FieldSession {
    // MARK: constants

    static let storeDevice = "fc.field.device"
    static let storeRefresh = "fc.field.refresh"
    static let storeSession = "fc.field.session"
    static let storeSupervisor = "fc.field.supervisor"
    static let storeSupervisorRefresh = "fc.field.supervisor.refresh"
    /// The unlock key held on this phone: `{unlockKeyId, workerId, ..., alias}` (not secret).
    static let storeUnlockKey = "fc.field.unlockKey"
    /// Unlock key ids still to revoke on the platform (the phone deleted them while it could not reach it).
    static let storeUnlockRevokes = "fc.field.unlockKey.revoke"

    /// Refresh first when the access token expires within this many ms.
    static let refreshSkewMs: Int64 = 30_000
    private static let qrPrefix = "fcfield:1:"
    /// Hosts that may use `http:` (loopback names and addresses, and the Android emulator's host alias).
    private static let loopbackHosts: Set<String> = ["localhost", "127.0.0.1", "::1", "10.0.2.2"]
    private static let noncePath = "/field/auth/nonce"
    private static let retryAfterFailureMs: Int64 = 30_000
    private static let minGapMs: Int64 = 1_000

    // MARK: types

    private struct Access {
        let token: String
        let expiresAt: Int64
    }

    private final class Supervisor {
        let session: Session
        var refreshToken: String?
        let access: Access?
        init(session: Session, refreshToken: String?, access: Access?) {
            self.session = session
            self.refreshToken = refreshToken
            self.access = access
        }
    }

    private struct Reply {
        let status: Int
        let headers: [String: String]
        let json: JObject?
        var bytes: Data? = nil

        var error: String? {
            guard let j = json else { return nil }
            let e = j.optString("error")
            return e.isEmpty ? nil : e
        }

        func header(_ name: String) -> String? {
            headers.first { $0.key.caseInsensitiveCompare(name) == .orderedSame }?.value
        }
    }

    struct Signed {
        let accessToken: String
        let proof: String
    }

    // MARK: dependencies and state

    /// The configuration this owner was created with.
    public nonisolated let config: FieldConfig
    private let storage: SecureStorage
    private let keys: DeviceKeyProvider
    let transport: HTTPTransport
    private let clock: FieldClock
    private let unlockPrompter: UnlockPrompter?
    private let unlockKeys: UnlockKeyProvider
    private let unlockKeyPrompter: UnlockKeyPrompter?

    private var platformUrl: String
    private var device: DeviceRecord?
    private var sessionState: Session?
    private var access: Access?
    private var refreshToken: String?
    private var supervisor: Supervisor?
    private var issuedAt: Int64 = 0
    private var deviceKey: DeviceKey?
    private var loaded = false
    private var backgroundedAt: Int64?
    /// When this session's window to register an unlock key closes, on `clock.monotonicMilliseconds()`; nil for none.
    private var registerDeadline: Int64?

    private let platformMutex = AsyncMutex() // serialises platform calls so a single-use nonce is never shared
    private let refreshMutex = AsyncMutex()
    private var refreshGen = 0
    private var nonces: [String: String] = [:]
    private nonisolated let listeners = ListenerBox()
    private nonisolated let stateBox = StateBox()

    private var keepAliveOn = false
    private var keepAliveTask: Task<Void, Never>?
    private var keepAliveGen = 0
    private var warnTask: Task<Void, Never>?
    private var warnGen = 0
    private var warnedKey: String?
    private var warned: Set<Int64> = []
    private let warnThresholds: [Int64]

    /// - Parameters:
    ///   - unlockPrompter: shows the app-unlock prompt for `unlock(prompt:)`; nil makes it fail with
    ///     `BIOMETRIC_UNAVAILABLE`. The Apple target's `FieldSession.get(config:)` supplies the LocalAuthentication one.
    ///   - unlockKeys: the phone's unlock keys; nil means none (`unlockKeyAvailability` is `.unsupported`). The Apple
    ///     target's `FieldSession.get(config:)` supplies the Secure Enclave one.
    ///   - unlockKeyPrompter: the biometric prompt that authorises an unlock-key signature for `enableUnlockKey(prompt:)`
    ///     and `loginWithUnlockKey(prompt:)`; nil makes them fail with `UNLOCK_KEYS_UNSUPPORTED`.
    public init(
        config: FieldConfig,
        storage: SecureStorage,
        keys: DeviceKeyProvider,
        transport: HTTPTransport,
        clock: FieldClock = SystemClock(),
        unlockPrompter: UnlockPrompter? = nil,
        unlockKeys: UnlockKeyProvider? = nil,
        unlockKeyPrompter: UnlockKeyPrompter? = nil
    ) {
        self.config = config
        self.storage = storage
        self.keys = keys
        self.transport = transport
        self.clock = clock
        self.unlockPrompter = unlockPrompter
        self.unlockKeys = unlockKeys ?? NoUnlockKeys()
        self.unlockKeyPrompter = unlockKeyPrompter
        self.platformUrl = FieldSession.trimTrailingSlashes(config.platformUrl)
        var seen = Set<Int64>()
        self.warnThresholds = config.expiryWarnings.map { FieldSession.millis($0) }.filter { seen.insert($0).inserted }
    }

    // MARK: observation

    /// signedOut | signedIn(session) | locked(session). Restored from storage shortly after `FieldSession.get`; see `restore()`.
    public nonisolated var state: FieldState { stateBox.value }

    /// The current state, then every change.
    public nonisolated func stateUpdates() -> AsyncStream<FieldState> { stateBox.stream() }

    /// Registers a listener for `FieldEvent`s (called on arbitrary threads, synchronously where the event happens).
    @discardableResult
    public nonisolated func addListener(_ listener: @escaping @Sendable (FieldEvent) -> Void) -> FieldListenerToken {
        listeners.add(listener)
    }

    /// Loads the stored device and session (once) and returns the resulting state. Safe to call repeatedly.
    public func restore() throws -> FieldState {
        try ensureLoaded()
        return stateBox.value
    }

    /// The device record of this enrolment (what `enrol` returned, as stored), or nil when the device is not enrolled.
    /// Loads the stored state first if that has not happened yet. No network. Signing out, including
    /// `logout(scope: .deviceAll)`, does not remove it: the device stays enrolled.
    public func enrolledDevice() throws -> DeviceRecord? {
        try ensureLoaded()
        return device
    }

    private func emit(_ e: FieldEvent) { listeners.emit(e) }

    func emitSessionEnded(_ reason: String) { emit(.sessionEnded(reason)) }

    // MARK: helpers

    static func millis(_ seconds: TimeInterval) -> Int64 { Int64((seconds * 1000).rounded()) }

    static func trimTrailingSlashes(_ s: String) -> String {
        var t = Substring(s)
        while t.hasSuffix("/") { t = t.dropLast() }
        return String(t)
    }

    private func now() -> Int64 { clock.nowMilliseconds() }

    private func store<T>(_ block: () throws -> T) throws -> T {
        do {
            return try block()
        } catch let e as FieldAuthError {
            throw e
        } catch let e as FcFieldError {
            throw FieldAuthError(code: e.code, message: "secure storage failed: \(e.code)")
        } catch {
            throw FieldAuthError(code: "INTERNAL", message: "secure storage failed")
        }
    }

    private func ensureLoaded() throws {
        if loaded { return }
        try load()
        loaded = true
    }

    private func load() throws {
        if let dev = try store({ try storage.get(Self.storeDevice) }) {
            device = JSON.parseObject(dev).flatMap(deviceFromJson)
            // First-run provisioning: an enrolment QR set the platform URL; it must survive a restart.
            if platformUrl.isEmpty, let d = device { platformUrl = Self.trimTrailingSlashes(d.platformUrl) }
        }
        if let ses = try store({ try storage.get(Self.storeSession) }) {
            sessionState = JSON.parseObject(ses).flatMap(sessionFromJson)
        }
        if let sup = try store({ try storage.get(Self.storeSupervisor) }), sessionState != nil {
            supervisor = JSON.parseObject(sup).flatMap(sessionFromJson).map { Supervisor(session: $0, refreshToken: nil, access: nil) }
        }
        issuedAt = now()
        if let s = sessionState {
            let lockNow = config.requireUnlock.afterColdStart && lockEligible(s)
            stateBox.set(lockNow ? .locked(s) : .signedIn(s))
            if lockNow { emit(.locked(s)) }
            ensureTimers()
        }
    }

    private func requirePlatform() throws -> String {
        if platformUrl.isEmpty {
            throw FieldAuthError(code: "NOT_ENROLLED", message: "no platform URL: configure platformUrl or enrol this device first")
        }
        return platformUrl
    }

    /// True while someone is acting as the signed-in worker (act-as or a sign-in code).
    private func impersonating() -> Bool { supervisor != nil || sessionState?.actor != nil }

    private func lockEligible(_ s: Session) -> Bool {
        device?.mode == "PERSONAL" && s.deviceMode == .personal && s.actor == nil && supervisor == nil
    }

    /// Drops whitespace and control characters and upper-cases (scanned badges, sign-in codes).
    public static func normaliseBadgeToken(_ scanned: String) -> String {
        let kept = scanned.unicodeScalars.filter { u in
            let v = u.value
            let asciiSpace = v == 0x20 || (v >= 0x09 && v <= 0x0d)
            return !(asciiSpace || v < 0x20 || v == 0x7f)
        }
        var out = String.UnicodeScalarView()
        out.append(contentsOf: kept)
        return String(out).uppercased()
    }

    // MARK: public API

    /// Enrols this device. `qrOrCode` is the 10-character code or a whole `fcfield:1:` QR payload (which also sets the
    /// platform URL when none is configured; with one configured, a QR for another origin that is not in
    /// `FieldConfig.allowedPlatformOrigins` is refused with `PLATFORM_MISMATCH` and nothing is persisted).
    public func enrol(_ qrOrCode: String, info: DeviceInfo) async throws -> DeviceRecord {
        try ensureLoaded()
        var code = qrOrCode.trimmingCharacters(in: .whitespacesAndNewlines)
        if code.hasPrefix(Self.qrPrefix) {
            let q = try parseEnrolmentQr(code)
            let configured = originOf(Self.trimTrailingSlashes(config.platformUrl))
            if let configured {
                let qrOrigin = originOf(q.url)
                let allowed = [configured] + config.allowedPlatformOrigins.compactMap { originOf(Self.trimTrailingSlashes($0)) }
                if qrOrigin == nil || !allowed.contains(qrOrigin!) {
                    throw FieldAuthError(
                        code: "PLATFORM_MISMATCH",
                        message: "enrolment QR code is for \(qrOrigin ?? "an unreadable platform"), not the configured platform \(configured)"
                    )
                }
            }
            try requireHttpsOrNamedOrigin(q.url)
            platformUrl = q.url
            code = q.code
        }
        let base = try requirePlatform()
        let key = try publicKey()
        var body: JObject = [
            "code": code,
            "publicKeyJwk": ["kty": "EC", "crv": "P-256", "x": key.x, "y": key.y] as JObject,
            "platform": String(info.platform.prefix(40)),
            "model": String(info.model.prefix(120)),
        ]
        if let chain = key.attestationChain, !chain.isEmpty { body["attestationChain"] = chain }
        if let label = info.label { body["label"] = label }
        let reply = try await platformPost("/field/devices/enrol", body)
        let j = reply.json ?? [:]
        let appCodes = (j.optArray("appCodes") ?? []).map { ($0 as? String) ?? "\($0)" }
        let record = DeviceRecord(
            deviceId: j.optString("deviceId"),
            mode: j.optString("mode") == "SHARED" ? "SHARED" : "PERSONAL",
            clientId: j.optString("clientId"),
            appCodes: appCodes,
            platformUrl: base
        )
        device = record
        try store { try storage.put(Self.storeDevice, value: JSON.string(deviceToJson(record))) }
        // An unlock key belongs to the device row it was registered on; a new enrolment is a new row.
        if let k = storedUnlockKey(), k.info.deviceId != record.deviceId { forgetUnlockKey(k, revoke: false) }
        return record
    }

    /// Signs a worker in by worker code or badge. Works without enrolment: the key is created on first use.
    public func login(_ creds: LoginCredentials) async throws -> Session {
        try ensureLoaded()
        let badge = creds.badgeToken.map(Self.normaliseBadgeToken) ?? ""
        let hasCode = !(creds.workerCode ?? "").isEmpty
        if hasCode == !badge.isEmpty {
            throw FieldAuthError(code: "INVALID_REQUEST", message: "give either a worker code or a badge token")
        }
        let clientIdentifier = creds.clientIdentifier ?? config.clientIdentifier
        if device == nil && hasCode && (clientIdentifier ?? "").isEmpty {
            throw FieldAuthError(
                code: "INVALID_REQUEST",
                message: "clientIdentifier is required to sign in by worker code on a device that is not enrolled"
            )
        }
        var body: JObject = ["appCode": config.appCode]
        if hasCode { body["workerCode"] = creds.workerCode } else { body["badgeToken"] = badge }
        if let pin = creds.pin, !pin.isEmpty { body["pin"] = pin }
        if let ci = clientIdentifier, !ci.isEmpty { body["clientIdentifier"] = ci }
        do {
            let reply = try await platformPost("/field/auth/login", body, secrets: [creds.pin ?? "", badge])
            clearSupervisor()
            let session = try apply(reply.json, preserveLock: false)
            await flushPendingRevokes()
            return session
        } catch let e as FieldAuthError {
            throw noteFailure(e)
        }
    }

    /// Redeems a sign-in code an administrator created in the console. The session is the worker's, with the admin as actor.
    public func redeemSignInCode(_ code: String) async throws -> Session {
        try ensureLoaded()
        let c = Self.normaliseBadgeToken(code)
        if c.isEmpty { throw FieldAuthError(code: "INVALID_REQUEST", message: "a sign-in code is required") }
        do {
            let reply = try await platformPost("/field/auth/sign-in-code", ["code": c], secrets: [c])
            clearSupervisor()
            return try apply(reply.json, preserveLock: false)
        } catch let e as FieldAuthError {
            throw noteFailure(e)
        }
    }

    /// A supervisor acts as a worker on this device. The supervisor's own session is kept aside and the active
    /// session becomes the worker's; `endImpersonation()` returns to it. The PIN is the supervisor's.
    public func actAs(_ target: ActAsTarget) async throws -> Session {
        try ensureLoaded()
        if impersonating() { throw FieldAuthError(code: "IMPERSONATING", message: "end the current impersonation first") }
        let hasCode = !(target.workerCode ?? "").isEmpty
        let hasId = !(target.workerId ?? "").isEmpty
        if hasCode == hasId { throw FieldAuthError(code: "INVALID_REQUEST", message: "give either a worker code or a worker id") }
        var body: JObject = ["pin": target.pin]
        if hasCode { body["workerCode"] = target.workerCode } else { body["workerId"] = target.workerId }
        let reply: Reply
        do {
            reply = try await bearerPost("/field/auth/act-as", body, secrets: [target.pin])
        } catch let e as FieldAuthError {
            throw noteFailure(e)
        }
        guard let before = sessionState, let token = refreshToken else {
            throw FieldAuthError(code: "SESSION_EXPIRED", message: "there is no session")
        }
        let sup = Supervisor(session: before, refreshToken: token, access: access)
        try store {
            try storage.put(Self.storeSupervisorRefresh, value: token)
            try storage.put(Self.storeSupervisor, value: JSON.string(sessionToJson(before)))
        }
        supervisor = sup
        return try apply(reply.json, preserveLock: false)
    }

    /// Ends the impersonated session (logs out its family) and returns to the supervisor's, refreshing it if its
    /// access token has lapsed. For a session from a sign-in code there is nothing to return to: the session just
    /// ends and this returns nil.
    public func endImpersonation() async throws -> Session? {
        try ensureLoaded()
        if !impersonating() { throw FieldAuthError(code: "NOT_IMPERSONATING", message: "no one is being impersonated") }
        let sup = supervisor
        if let sup { _ = try supervisorToken(sup) } // fail before anything is torn down
        let token: String? = refreshToken ?? (try? store { try storage.get(Self.storeRefresh) })
        var failure: FieldAuthError?
        if let token {
            do {
                _ = try await platformPost(
                    "/field/auth/logout", ["refreshToken": token, "scope": "session"], secrets: [token])
            } catch let e as FieldAuthError {
                failure = e
            }
        }
        guard let sup else {
            endSession("logout")
            if let failure { throw failure }
            return nil
        }
        try switchToSupervisor(sup, reason: "logout")
        if access == nil || access!.expiresAt - now() < Self.refreshSkewMs { _ = try await refresh() }
        if let failure { throw failure }
        return sessionState
    }

    /// Changes the PIN, then refreshes (which drops a `pcr` token).
    public func changePin(current: String, new: String) async throws {
        try ensureLoaded()
        if impersonating() { throw FieldAuthError(code: "IMPERSONATING", message: "the PIN cannot be changed while impersonating") }
        do {
            _ = try await bearerPost("/field/auth/pin", ["currentPin": current, "newPin": new], secrets: [current, new])
        } catch let e as FieldAuthError {
            throw noteFailure(e)
        }
        _ = try await refresh()
    }

    // MARK: the worker's own photo

    private static func photoVersion(_ w: JObject) -> String? {
        guard let v = w["photoVersion"] as? String, !v.isEmpty else { return nil }
        return v
    }

    /// Reads the signed-in worker's own photo (`.thumb` by default). Pass `haveVersion` (the version of the bytes the
    /// device already holds): while it is still current the answer has `notModified` true and no bytes. Returns nil when
    /// the worker has no photo or the client has photos off (404 `PHOTO_NOT_FOUND`); throws `PHOTOS_DISABLED` when the
    /// platform has them off and `TOO_MANY_REQUESTS` (429, with `retryAfterSeconds`) when the hourly budget is spent.
    /// A supervisor acting as the worker may read it.
    public func getPhoto(size: PhotoSize = .thumb, haveVersion: String? = nil) async throws -> WorkerPhoto? {
        try ensureLoaded()
        var body: JObject = ["size": size.rawValue]
        if let v = haveVersion, !v.isEmpty { body["version"] = v }
        let reply: Reply
        do {
            reply = try await bearerPost("/field/worker/photo", body, secrets: [])
        } catch let e as FieldAuthError {
            if e.code == "PHOTO_NOT_FOUND" { return nil }
            throw e
        }
        var version: String?
        if let etag = reply.header("ETag") {
            let t = etag.trimmingCharacters(in: CharacterSet(charactersIn: "\" "))
            if let dash = t.firstIndex(of: "-"), t.distance(from: t.startIndex, to: dash) == 16 { version = String(t[..<dash]) }
        }
        if reply.status == 304 { return WorkerPhoto(bytes: nil, contentType: nil, version: version ?? haveVersion, notModified: true) }
        return WorkerPhoto(bytes: reply.bytes, contentType: reply.header("Content-Type"), version: version, notModified: false)
    }

    /// Sets (or replaces) the signed-in worker's own photo from a picture's bytes (JPEG, PNG or WebP: the platform decides
    /// the type from the bytes; `contentType` may only agree). Needs `session.worker.canSetOwnPhoto` (else
    /// `OWN_PHOTO_NOT_ALLOWED`); `IMPERSONATING` while someone acts as the worker. The platform's other refusals come as
    /// their codes (`PHOTO_EMPTY`, `PHOTO_TOO_LARGE`, `PHOTO_UNSUPPORTED_TYPE`, `PHOTO_TYPE_MISMATCH`,
    /// `PHOTO_DIMENSIONS_TOO_LARGE`, `PHOTO_UNREADABLE`, `PHOTO_BUSY`, `PHOTOS_DISABLED_FOR_CLIENT`, `PHOTOS_DISABLED`,
    /// `TOO_MANY_REQUESTS`). Returns the session with the new `worker.photoVersion` and emits `.sessionChanged`.
    @discardableResult
    public func setPhoto(_ bytes: Data, contentType: String? = nil) async throws -> Session {
        try ensureLoaded()
        if impersonating() { throw FieldAuthError(code: "IMPERSONATING", message: "the photo cannot be changed while someone acts as the worker") }
        let type = (contentType?.isEmpty == false) ? contentType! : "application/octet-stream"
        let reply = try await bearerPost("/field/worker/photo/set", [:], secrets: [], raw: RawBody(bytes: bytes, contentType: type))
        let v = reply.json?.optString("photoVersion") ?? ""
        return try withPhotoVersion(v.isEmpty ? nil : v)
    }

    /// Removes the signed-in worker's own photo (the same rule as setting it). Returns the session.
    @discardableResult
    public func removePhoto() async throws -> Session {
        try ensureLoaded()
        if impersonating() { throw FieldAuthError(code: "IMPERSONATING", message: "the photo cannot be changed while someone acts as the worker") }
        _ = try await bearerPost("/field/worker/photo/remove", [:], secrets: [])
        return try withPhotoVersion(nil)
    }

    /// The session with a new photo version, stored and announced.
    private func withPhotoVersion(_ version: String?) throws -> Session {
        guard let cur = sessionState else { throw FieldAuthError(code: "SESSION_EXPIRED", message: "there is no session") }
        let session = Session(
            worker: Worker(id: cur.worker.id, workerCode: cur.worker.workerCode, name: cur.worker.name, photoVersion: version, canSetOwnPhoto: cur.worker.canSetOwnPhoto),
            actor: cur.actor, test: cur.test, deviceMode: cur.deviceMode, deviceId: cur.deviceId, appCode: cur.appCode,
            expiresAt: cur.expiresAt, idleExpiresAt: cur.idleExpiresAt, refreshExpiresAt: cur.refreshExpiresAt,
            pinChangeRequired: cur.pinChangeRequired, clientId: cur.clientId, clientIdentifier: cur.clientIdentifier,
            amr: cur.amr, unlockKeys: cur.unlockKeys)
        sessionState = session
        try store { try storage.put(Self.storeSession, value: JSON.string(sessionToJson(session))) }
        publish(session, preserveLock: true)
        emit(.sessionChanged(session))
        return session
    }

    // MARK: unlock keys (docs/field-unlock-keys-spec.md section 12; Kotlin FieldSession, "unlock keys")

    /// Whether "sign in with Face ID / Touch ID next time" can be offered now, after a PIN sign-in on this enrolled
    /// PERSONAL phone. No network: computed from the phone's own state and the session's latest token answer
    /// (`unlockKeys`, spec 2.3), which says whether the client's policy allows unlock keys for this session, whether it
    /// requires attestation (which iOS cannot give: `.attestationRequired`), and how long the session may still
    /// register a key.
    public func unlockKeyAvailability() throws -> UnlockKeyAvailability {
        try ensureLoaded()
        return localUnlockAvailability(forEnable: false)
    }

    /// The unlock key held on this phone (whoever's it is), or nil. No network.
    public func unlockKey() throws -> UnlockKeyInfo? {
        try ensureLoaded()
        return storedUnlockKey()?.info
    }

    /// Makes the signed-in worker an unlock key in the Secure Enclave and registers it with the platform: call right
    /// after a PIN sign-in on an enrolled PERSONAL phone, within the session's register window
    /// (`unlockKeys.registerExpiresIn`), when the worker agreed. Checks the session's token answer first and refuses
    /// without a key, a request or a prompt when it does not allow a key now (`UNLOCK_KEYS_DISABLED`,
    /// `PIN_SIGN_IN_REQUIRED`, `ATTESTATION_REQUIRED`: iOS cannot attest a key). Shows one biometric prompt (the key
    /// proof is signed by the new key). Replaces the worker's own earlier key on this phone. Nothing is kept when it fails.
    public func enableUnlockKey(prompt: UnlockKeyPrompt) async throws -> UnlockKeyInfo {
        guard let p = unlockKeyPrompter else {
            throw FieldAuthError(code: UnlockKeyCodes.unlockKeysUnsupported, message: "no unlock-key prompter is configured")
        }
        return try await enableUnlockKey(using: p, prompt: prompt)
    }

    /// Signs the worker of this phone's unlock key in again without the PIN: asks the platform for a challenge first (a
    /// refusal answers without a prompt), then shows the biometric prompt, then exchanges the signed challenge for a new
    /// session, exactly as `login` produces (`amr` `["hwk","uv"]`). Every failure means "sign in with the PIN"; the
    /// codes say why and whether the key was kept (README.md, "Unlock keys").
    public func loginWithUnlockKey(prompt: UnlockKeyPrompt) async throws -> Session {
        guard let p = unlockKeyPrompter else {
            throw FieldAuthError(code: UnlockKeyCodes.unlockKeysUnsupported, message: "no unlock-key prompter is configured")
        }
        return try await loginWithUnlockKey(using: p, prompt: prompt)
    }

    /// Turns biometric sign-in off on this phone: deletes the key and revokes it on the platform (needs only the device,
    /// no session). When the platform cannot be reached the key is deleted at once and the revocation is sent with the
    /// next platform call. The current session goes on. Also the hand-over of a phone whose key is another worker's.
    public func disableUnlockKey() async throws {
        try ensureLoaded()
        if let k = storedUnlockKey() { forgetUnlockKey(k, revoke: true) }
        await flushPendingRevokes()
    }

    /// `enableUnlockKey(prompt:)` with a prompter of your own (Kotlin `enableUnlockKeyWith`).
    public func enableUnlockKey(using prompter: UnlockKeyPrompter, prompt: UnlockKeyPrompt) async throws -> UnlockKeyInfo {
        try ensureLoaded()
        // The session's token answer says whether it may (advisory; the platform checks again): before any key or prompt.
        let a = localUnlockAvailability(forEnable: true)
        if a != .available { throw availabilityError(a) }
        await flushPendingRevokes()
        guard let dev = device else { throw availabilityError(.notEnrolled) }
        guard let s = sessionState else { throw availabilityError(.notSignedIn) }
        let claims = decodeClaims(try await ensureAccess())
        let workerId = Self.claimString(claims, "sub") ?? s.worker.id
        guard Self.claimString(claims, "sid") != nil, let deviceId = Self.claimString(claims, "device_id"),
              workerId == s.worker.id, deviceId == dev.deviceId
        else { throw FieldAuthError(code: "PIN_SIGN_IN_REQUIRED", message: "sign in with the PIN on this device first") }
        let previous = storedUnlockKey()
        var keyAttempts = 0
        while true {
            let alias = "io.flowcatalyst.field.unlock.\(workerId).\(Dpop.randomId(bytes: 6))"
            let material = try unlockKeyCall { try unlockKeys.create(alias: alias) }
            do {
                if material.level != "TEE" && material.level != "STRONGBOX" {
                    throw FieldAuthError(code: UnlockKeyCodes.unlockKeysUnsupported, message: "the phone's keys are not held in secure hardware")
                }
                let reply = try await register(prompter, prompt, alias: alias, material: material, workerId: workerId, deviceId: deviceId)
                let j = reply.json ?? [:]
                let id = j.optString("unlockKeyId")
                if id.isEmpty { throw FieldAuthError(code: "INTERNAL", message: "malformed unlock key answer") }
                let wid = j.optString("workerId")
                let did = j.optString("deviceId")
                let verdict = j.optObject("attestation")?.optString("verdict") ?? ""
                let info = UnlockKeyInfo(
                    unlockKeyId: id, workerId: wid.isEmpty ? workerId : wid, workerCode: s.worker.workerCode,
                    workerName: s.worker.name, deviceId: did.isEmpty ? deviceId : did,
                    createdAt: Self.optTime(j, "createdAt") ?? now(), pinRequiredBy: Self.optTime(j, "pinRequiredBy"),
                    attestationVerdict: verdict.isEmpty ? "NONE" : verdict)
                try saveUnlockKey(StoredUnlockKey(info: info, alias: alias, proofFailures: 0))
                // The platform revoked the worker's earlier key (REPLACED); the phone deletes its private half.
                if let previous, previous.alias != alias { try? unlockKeys.delete(alias: previous.alias) }
                emit(.unlockKeyChanged(info))
                return info
            } catch let e as FieldAuthError {
                try? unlockKeys.delete(alias: alias)
                if e.code == "UNLOCK_KEY_EXISTS" && keyAttempts == 0 { // spec: make a new key
                    keyAttempts += 1
                    continue
                }
                throw onRegisterRefusal(e)
            } catch {
                try? unlockKeys.delete(alias: alias)
                throw error
            }
        }
    }

    /// POST /field/auth/unlock-keys with the key proof. The key proof signs the request's own DPoP nonce, so the nonce is
    /// taken first, the prompt shown without holding the platform lock, and the request sent with that nonce. A nonce
    /// that expired during a long prompt (`use_dpop_nonce`) costs one more prompt, once.
    private func register(
        _ prompter: UnlockKeyPrompter, _ prompt: UnlockKeyPrompt, alias: String, material: UnlockKeyMaterial,
        workerId: String, deviceId: String
    ) async throws -> Reply {
        var attempt = 0
        while true {
            let nonce = try await reservePlatformNonce()
            let signer = try unlockKeyCall { try unlockKeys.prepare(alias: alias) }
            let input = UnlockJws.keyProofSigningInput(
                baseUrl: try requirePlatform(), nonce: nonce, workerId: workerId, deviceId: deviceId, iat: now() / 1000)
            let raw = try await prompter.authorise(prompt, signature: signer, data: Data(input.utf8))
            var body: JObject = [
                "publicKeyJwk": ["kty": "EC", "crv": "P-256", "x": material.x, "y": material.y] as JObject,
                "keyProof": try UnlockJws.compact(input, raw),
            ]
            if let chain = material.attestationChain, !chain.isEmpty { body["attestationChain"] = chain }
            let bearer = try await ensureAccess()
            let reply = try await platformSendWithNonce(UnlockJws.registerPath, body, bearer: bearer, nonce: nonce)
            if reply.status == 401 && reply.error == "use_dpop_nonce" && attempt == 0 {
                attempt += 1
                continue
            }
            if reply.status >= 400 { throw replyError(reply, secrets: []) }
            return reply
        }
    }

    /// `loginWithUnlockKey(prompt:)` with a prompter of your own (Kotlin `loginWithUnlockKeyWith`).
    public func loginWithUnlockKey(using prompter: UnlockKeyPrompter, prompt: UnlockKeyPrompt) async throws -> Session {
        try ensureLoaded()
        guard let dev = device else { throw FieldAuthError(code: "NOT_ENROLLED", message: "unlock keys need an enrolled device") }
        await flushPendingRevokes()
        guard var rec = storedUnlockKey() else {
            throw FieldAuthError(code: UnlockKeyCodes.noUnlockKey, message: "there is no unlock key on this phone; sign in with the PIN")
        }
        if dev.mode != "PERSONAL" {
            forgetUnlockKey(rec, revoke: false)
            throw FieldAuthError(code: "SHARED_DEVICE", message: "unlock keys are not allowed on a shared device")
        }
        if rec.info.deviceId != dev.deviceId {
            forgetUnlockKey(rec, revoke: false)
            throw FieldAuthError(code: UnlockKeyCodes.noUnlockKey, message: "the unlock key belongs to an earlier enrolment; sign in with the PIN")
        }
        let b = unlockKeys.biometricStatus()
        if b != .available { throw availabilityError(b) }
        // A key the phone invalidated fails here, before the platform is asked or a prompt shown.
        var signer = try await prepareOrInvalidate(rec)
        var attempt = 0
        while true {
            let ch: Reply
            do {
                ch = try await platformPost(UnlockJws.challengePath, ["unlockKeyId": rec.info.unlockKeyId])
            } catch let e as FieldAuthError {
                throw await onUnlockRefusal(rec, e)
            }
            let challenge = ch.json?.optString("challenge") ?? ""
            if challenge.isEmpty { throw FieldAuthError(code: "INTERNAL", message: "malformed unlock challenge") }
            let pinBy = ch.json.flatMap { Self.optTime($0, "pinRequiredBy") }
            if pinBy != rec.info.pinRequiredBy {
                rec = StoredUnlockKey(info: rec.info.with(pinRequiredBy: pinBy), alias: rec.alias, proofFailures: rec.proofFailures)
                try saveUnlockKey(rec)
            }
            let input = UnlockJws.assertionSigningInput(
                baseUrl: try requirePlatform(), unlockKeyId: rec.info.unlockKeyId, challenge: challenge,
                workerId: rec.info.workerId, deviceId: rec.info.deviceId, appCode: config.appCode, iat: now() / 1000)
            let raw: Data
            do {
                raw = try await prompter.authorise(prompt, signature: signer, data: Data(input.utf8))
            } catch let e as FieldAuthError {
                if e.code == "KEY_INVALIDATED" { throw await invalidated(rec) }
                throw e
            }
            let reply: Reply
            do {
                reply = try await platformPost(UnlockJws.unlockPath, [
                    "unlockKeyId": rec.info.unlockKeyId, "appCode": config.appCode,
                    "assertion": try UnlockJws.compact(input, raw),
                ])
            } catch let e as FieldAuthError {
                // A slow prompt is not an attack: a new challenge and one more prompt.
                if e.code == "CHALLENGE_EXPIRED" && attempt == 0 {
                    attempt += 1
                    signer = try await prepareOrInvalidate(rec)
                    continue
                }
                throw await onUnlockRefusal(rec, e)
            }
            if rec.proofFailures != 0 { try? saveUnlockKey(StoredUnlockKey(info: rec.info, alias: rec.alias, proofFailures: 0)) }
            clearSupervisor()
            return try apply(reply.json, preserveLock: false)
        }
    }

    /// The availability from the phone's state and the session's latest token answer (Kotlin
    /// `localUnlockAvailability`). With `forEnable` the worker's own key does not stop it (enabling again replaces it).
    private func localUnlockAvailability(forEnable: Bool) -> UnlockKeyAvailability {
        let bio = unlockKeys.biometricStatus()
        if bio == .unsupported || bio == .osTooOld { return bio }
        guard let dev = device else { return .notEnrolled }
        if dev.mode != "PERSONAL" { return .sharedDevice }
        guard let s = sessionState else { return .notSignedIn }
        if impersonating() { return .impersonating }
        if s.deviceMode != .personal || s.deviceId != dev.deviceId { return .pinSignInRequired }
        let rec = storedUnlockKey()
        if let rec, rec.info.workerId != s.worker.id { return .otherWorker }
        if rec != nil && !forEnable { return .enabled }
        // The platform's word for this session: absent (a platform older than unlock keys) reads as not allowed.
        guard let keys = s.unlockKeys, keys.allowed else { return .policyOff }
        guard !s.pinChangeRequired, let deadline = registerDeadline, clock.monotonicMilliseconds() < deadline else {
            return .pinSignInRequired
        }
        if bio != .available { return bio }
        let deviceKey = try? publicKey()
        if deviceKey?.level == "SOFTWARE" { return .noSecureHardware }
        // Under REQUIRED a key needs an attestation chain; the Secure Enclave gives none (spec 6.1).
        if keys.attestationRequired && (!unlockKeys.attests || (deviceKey?.attestationChain ?? []).isEmpty) {
            return .attestationRequired
        }
        return .available
    }

    /// The side effects of a refused registration; returns the error to throw.
    private func onRegisterRefusal(_ e: FieldAuthError) -> FieldAuthError {
        switch e.code {
        case "UNLOCK_KEYS_DISABLED":
            // Switching the policy off revoked every key of the client on the platform.
            if let k = storedUnlockKey() { forgetUnlockKey(k, revoke: false) }
        case "ATTESTATION_REQUIRED":
            return FieldAuthError(
                code: e.code,
                message: "iOS cannot attest an unlock key, so a client whose policy requires attestation cannot use unlock keys on iOS; sign in with the PIN",
                status: e.status, retryAfterSeconds: e.retryAfterSeconds)
        case "WORKER_LOCKED", "WORKER_LOCKED_TEMPORARILY", "TOO_MANY_ATTEMPTS":
            return noteFailure(e)
        case "KEY_INVALIDATED":
            return FieldAuthError(code: UnlockKeyCodes.unlockKeyInvalidated, message: "the new unlock key was invalidated; try again")
        default: break
        }
        return e
    }

    /// The side effects of a refused challenge or unlock (spec section 10); returns the error to throw.
    private func onUnlockRefusal(_ rec: StoredUnlockKey, _ e: FieldAuthError) async -> FieldAuthError {
        switch e.code {
        case "UNLOCK_KEY_INVALID":
            // Unknown or revoked on the platform (also: the worker not active, or a key the attestation policy no
            // longer admits): it can never sign the worker in again. Revoked too, so a live key nobody can use does
            // not keep another worker from registering on this phone.
            forgetUnlockKey(rec, revoke: true)
            await flushPendingRevokes()
        case "UNLOCK_KEYS_DISABLED":
            forgetUnlockKey(rec, revoke: false)
        case "SHARED_DEVICE", "UNKNOWN_DEVICE", "DEVICE_REVOKED":
            forgetUnlockKey(rec, revoke: false)
        case "INVALID_UNLOCK_PROOF":
            let failures = rec.proofFailures + 1
            if failures >= 2 {
                forgetUnlockKey(rec, revoke: true)
                await flushPendingRevokes()
            } else {
                try? saveUnlockKey(StoredUnlockKey(info: rec.info, alias: rec.alias, proofFailures: failures))
            }
        case "WORKER_LOCKED", "WORKER_LOCKED_TEMPORARILY", "TOO_MANY_ATTEMPTS":
            return noteFailure(e)
        default:
            // Kept: PIN_REAUTH_REQUIRED (works again after the next PIN sign-in), PIN_RESET_REQUIRED,
            // ATTESTATION_STATUS_UNAVAILABLE (try the key again later), DEVICE_SUSPENDED, NO_APP_ACCESS, NETWORK, ...
            break
        }
        return e
    }

    private func prepareOrInvalidate(_ rec: StoredUnlockKey) async throws -> PreparedSignature {
        do {
            return try unlockKeys.prepare(alias: rec.alias)
        } catch let e as FcFieldError {
            if e.code == "KEY_INVALIDATED" { throw await invalidated(rec) }
            throw FieldAuthError(code: e.code, message: "the unlock key could not be used: \(e.code)")
        } catch let e as FieldAuthError {
            if e.code == "KEY_INVALIDATED" { throw await invalidated(rec) }
            throw e
        } catch {
            throw FieldAuthError(code: "INTERNAL", message: "the unlock key could not be used")
        }
    }

    /// The phone invalidated the key (Face ID / Touch ID enrolment changed, keychain loss): revoke, delete, PIN.
    private func invalidated(_ rec: StoredUnlockKey) async -> FieldAuthError {
        forgetUnlockKey(rec, revoke: true)
        await flushPendingRevokes()
        return FieldAuthError(
            code: UnlockKeyCodes.unlockKeyInvalidated,
            message: "the phone's Face ID or Touch ID changed, so its unlock key no longer works; sign in with the PIN")
    }

    private func availabilityError(_ a: UnlockKeyAvailability) -> FieldAuthError {
        switch a {
        case .notEnrolled: return FieldAuthError(code: "NOT_ENROLLED", message: "unlock keys need an enrolled device")
        case .sharedDevice: return FieldAuthError(code: "SHARED_DEVICE", message: "unlock keys are not allowed on a shared device")
        case .notSignedIn: return FieldAuthError(code: "SESSION_EXPIRED", message: "there is no session")
        case .impersonating: return FieldAuthError(code: "IMPERSONATING", message: "only the worker may make their unlock key")
        case .pinSignInRequired:
            return FieldAuthError(code: "PIN_SIGN_IN_REQUIRED", message: "sign in with the PIN on this device first (the session's register window is closed)")
        case .otherWorker:
            return FieldAuthError(code: UnlockKeyCodes.unlockKeyOtherWorker, message: "this phone's unlock key belongs to another worker")
        case .osTooOld, .noSecureHardware, .unsupported:
            return FieldAuthError(code: UnlockKeyCodes.unlockKeysUnsupported, message: "this phone cannot hold an unlock key")
        case .biometricNotEnrolled:
            return FieldAuthError(code: UnlockKeyCodes.biometricNotEnrolled, message: "no Face ID or Touch ID is enrolled on this phone")
        case .biometricUnavailable: return FieldAuthError(code: "BIOMETRIC_UNAVAILABLE", message: "biometric unavailable")
        case .policyOff: return FieldAuthError(code: "UNLOCK_KEYS_DISABLED", message: "the session's token answer does not allow unlock keys")
        case .attestationRequired:
            return FieldAuthError(
                code: "ATTESTATION_REQUIRED",
                message: "iOS cannot attest an unlock key, so a client whose policy requires attestation cannot use unlock keys on iOS")
        case .available, .enabled: return FieldAuthError(code: "INTERNAL", message: "not a refusal")
        }
    }

    private func unlockKeyCall<T>(_ block: () throws -> T) throws -> T {
        do {
            return try block()
        } catch let e as FieldAuthError {
            throw e
        } catch let e as FcFieldError {
            throw FieldAuthError(code: e.code, message: "unlock key failed: \(e.code)")
        } catch {
            throw FieldAuthError(code: "INTERNAL", message: "unlock key failed")
        }
    }

    // MARK: unlock-key state (survives restarts and sign-outs; nothing secret)

    private struct StoredUnlockKey {
        let info: UnlockKeyInfo
        let alias: String
        let proofFailures: Int
    }

    private func storedUnlockKey() -> StoredUnlockKey? {
        guard let raw = (try? storage.get(Self.storeUnlockKey)) ?? nil, let j = JSON.parseObject(raw),
              let id = j["unlockKeyId"] as? String, let wid = j["workerId"] as? String,
              let did = j["deviceId"] as? String, let alias = j["alias"] as? String
        else { return nil }
        let verdict = j.optString("attestationVerdict")
        let info = UnlockKeyInfo(
            unlockKeyId: id, workerId: wid, workerCode: j.optString("workerCode"), workerName: j.optString("workerName"),
            deviceId: did, createdAt: j.optLong("createdAt"),
            pinRequiredBy: j.isNull("pinRequiredBy") ? nil : JSON.int64(j["pinRequiredBy"]),
            attestationVerdict: verdict.isEmpty ? "NONE" : verdict)
        return StoredUnlockKey(info: info, alias: alias, proofFailures: Int(j.optLong("proofFailures")))
    }

    private func saveUnlockKey(_ r: StoredUnlockKey) throws {
        let i = r.info
        let j: JObject = [
            "unlockKeyId": i.unlockKeyId, "workerId": i.workerId, "workerCode": i.workerCode, "workerName": i.workerName,
            "deviceId": i.deviceId, "createdAt": i.createdAt, "pinRequiredBy": i.pinRequiredBy.map { $0 as Any } ?? NSNull(),
            "attestationVerdict": i.attestationVerdict, "alias": r.alias, "proofFailures": r.proofFailures,
        ]
        try store { try storage.put(Self.storeUnlockKey, value: JSON.string(j)) }
    }

    /// Deletes the key and its record; with `revoke`, queues its revocation on the platform (sent by `flushPendingRevokes`).
    private func forgetUnlockKey(_ rec: StoredUnlockKey, revoke: Bool) {
        try? unlockKeys.delete(alias: rec.alias)
        if revoke {
            var ids = pendingRevokes()
            if !ids.contains(rec.info.unlockKeyId) { ids.append(rec.info.unlockKeyId) }
            savePendingRevokes(ids)
        }
        try? storage.remove(Self.storeUnlockKey)
        emit(.unlockKeyChanged(nil))
    }

    private func pendingRevokes() -> [String] {
        guard let raw = (try? storage.get(Self.storeUnlockRevokes)) ?? nil,
              let a = try? JSONSerialization.jsonObject(with: Data(raw.utf8)) as? [Any]
        else { return [] }
        return a.compactMap { $0 as? String }
    }

    private func savePendingRevokes(_ ids: [String]) {
        if ids.isEmpty {
            try? storage.remove(Self.storeUnlockRevokes)
        } else if let d = try? JSONSerialization.data(withJSONObject: ids) {
            try? storage.put(Self.storeUnlockRevokes, value: String(decoding: d, as: UTF8.self))
        }
    }

    /// Sends the revocations the phone owes the platform (best effort; needs only the device). Dropped when the platform
    /// answered, or when it no longer knows the device or refuses it for good (nothing is left to revoke); kept for the
    /// next platform call after anything else (no network, a server error, a full budget, a suspended device).
    private func flushPendingRevokes() async {
        guard device != nil else { return }
        let ids = pendingRevokes()
        if ids.isEmpty { return }
        var done = Set<String>()
        for id in ids {
            do {
                _ = try await platformPost(UnlockJws.revokePath, ["unlockKeyId": id])
                done.insert(id)
            } catch let e as FieldAuthError {
                if e.code == "UNKNOWN_DEVICE" || e.code == "DEVICE_REVOKED" || e.code == "INVALID_REQUEST" {
                    done.insert(id)
                } else {
                    break
                }
            } catch {
                break
            }
        }
        // Read again: the actor may have queued another revocation while a request was out.
        savePendingRevokes(pendingRevokes().filter { !done.contains($0) })
    }

    private static func optTime(_ j: JObject, _ name: String) -> Int64? {
        if j.isNull(name) { return nil }
        let v = j.optString(name)
        return v.isEmpty ? nil : parseOffsetDateTimeMillis(v)
    }

    /// Ends the session (or every session on this device) and clears local state.
    public func logout(scope: LogoutScope = .session) async throws {
        try ensureLoaded()
        if scope == .deviceAll && device == nil {
            throw FieldAuthError(code: "INVALID_REQUEST", message: "device-all needs an enrolled device")
        }
        let sup = supervisor
        let token: String? = refreshToken ?? (try? store { try storage.get(Self.storeRefresh) })
        var failure: FieldAuthError?
        if let token {
            do {
                _ = try await platformPost(
                    "/field/auth/logout", ["refreshToken": token, "scope": scope.wire], secrets: [token])
            } catch let e as FieldAuthError {
                failure = e
            }
        }
        // Signing out while impersonating also ends the supervisor's session.
        if let sup {
            do {
                let st = try supervisorToken(sup)
                _ = try await platformPost("/field/auth/logout", ["refreshToken": st, "scope": "session"], secrets: [st])
            } catch {
                // best effort: the supervisor's local state is cleared below either way
            }
        }
        endSession("logout")
        if let failure { throw failure }
    }

    /// App unlock: shows the system prompt (biometrics or the device passcode) when the state is `locked`. Enrolled
    /// PERSONAL, non-impersonated sessions only (else BIOMETRIC_UNAVAILABLE). Does not touch the tokens.
    public func unlock(prompt: String) async throws -> Session {
        guard let p = unlockPrompter else {
            throw FieldAuthError(code: "BIOMETRIC_UNAVAILABLE", message: "no unlock prompter is configured")
        }
        return try await unlock(using: p, prompt: prompt)
    }

    /// `unlock(prompt:)` with a prompter of your own (Kotlin `unlockWith`).
    public func unlock(using prompter: UnlockPrompter, prompt: String) async throws -> Session {
        try ensureLoaded()
        guard let s = sessionState else {
            throw FieldAuthError(code: "SESSION_EXPIRED", message: "there is no stored session to unlock")
        }
        if !lockEligible(s) { throw FieldAuthError(code: "BIOMETRIC_UNAVAILABLE", message: "unlock is not available for this session") }
        guard case .locked = stateBox.value else { return s }
        try await prompter.prompt(prompt)
        guard let cur = sessionState else { throw FieldAuthError(code: "SESSION_EXPIRED", message: "the session ended") }
        stateBox.set(.signedIn(cur))
        return cur
    }

    /// Hosts report the app going to the background (the Apple target does it from UIKit notifications on iOS).
    public func noteBackgrounded() { backgroundedAt = now() }

    /// Counterpart of `noteBackgrounded()`: locks when the `UnlockPolicy.afterBackground` time has passed.
    public func noteForegrounded() {
        guard let since = backgroundedAt else { return }
        backgroundedAt = nil
        guard let limit = config.requireUnlock.afterBackground, let s = sessionState else { return }
        if case .signedIn = stateBox.value, now() - since >= Self.millis(limit), lockEligible(s) {
            stateBox.set(.locked(s))
            emit(.locked(s))
        }
    }

    /// While on, the owner refreshes before the idle deadline (at min(idleExpiresAt - keepAliveLead, idle midpoint)).
    /// A timer in the process: it only runs while the process does (iOS suspends a backgrounded app). Stops on
    /// session end and resumes on the next sign-in while still on.
    public func keepAlive(_ on: Bool) {
        keepAliveOn = on
        if !on {
            keepAliveGen += 1
            keepAliveTask?.cancel()
            keepAliveTask = nil
        }
        if on { ensureTimers() }
    }

    // MARK: refresh

    /// Rotates the refresh token. Single-flight: concurrent callers share one request.
    public func refresh() async throws -> Session { try await refreshFrom(refreshGen) }

    /// Refreshes unless a refresh (or sign-in) completed since `seen` was read; then the current session is returned.
    private func refreshFrom(_ seen: Int) async throws -> Session {
        try ensureLoaded()
        await refreshMutex.lock()
        defer { refreshMutex.unlock() }
        if refreshGen != seen, let s = sessionState { return s }
        return try await doRefresh()
    }

    /// For `perform`: refresh after an invalid_token unless the token it used was already replaced.
    func refreshIfStale(_ usedToken: String) async throws {
        let seen = refreshGen
        if access?.token == usedToken { _ = try await refreshFrom(seen) }
    }

    private func doRefresh() async throws -> Session {
        var token = refreshToken
        if token == nil { token = try store { try storage.get(Self.storeRefresh) } }
        guard let token else {
            endSession("SESSION_EXPIRED")
            throw FieldAuthError(code: "SESSION_EXPIRED", message: "there is no session to refresh")
        }
        do {
            let reply = try await platformPost("/field/auth/refresh", ["refreshToken": token], secrets: [token])
            return try apply(reply.json, preserveLock: true)
        } catch let e as FieldAuthError {
            if sessionEndingCodes.contains(e.code) {
                // An impersonation that ends on its own hands back to the supervisor.
                if let sup = supervisor { try switchToSupervisor(sup, reason: e.code) } else { endSession(e.code) }
            }
            throw e
        }
    }

    private func ensureAccess() async throws -> String {
        let seen = refreshGen
        if access == nil || access!.expiresAt - now() < Self.refreshSkewMs { _ = try await refreshFrom(seen) }
        guard let a = access else { throw FieldAuthError(code: "SESSION_EXPIRED", message: "there is no session") }
        return a.token
    }

    // MARK: proofs for other code

    /// `Authorization: DPoP` + `DPoP` headers for one request. Refreshes first if the access token is about to expire.
    public func headersFor(method: String, url: String) async throws -> [String: String] {
        let s = try await signedFor(method: method, url: url)
        return ["Authorization": "DPoP \(s.accessToken)", "DPoP": s.proof]
    }

    /// Signs again after a `401 use_dpop_nonce`, with the nonce from that response's `DPoP-Nonce` header.
    public func headersForRetry(method: String, url: String, nonce: String) async throws -> [String: String] {
        noteNonce(url: url, nonce: nonce)
        return try await headersFor(method: method, url: url)
    }

    /// Feeds a `DPoP-Nonce` seen elsewhere (any response from that origin).
    public func noteNonce(url: String, nonce: String) {
        guard let u = HTTPURL(url), !nonce.isEmpty else { return }
        nonces[u.origin] = nonce
    }

    func signedFor(method: String, url: String) async throws -> Signed {
        guard let target = HTTPURL(url) else { throw FieldAuthError(code: "INVALID_REQUEST", message: "not a valid URL") }
        let token = try await ensureAccess()
        let proof = try makeProof(htm: method.uppercased(), htu: target.htu, nonce: takeNonce(target), accessToken: token)
        return Signed(accessToken: token, proof: proof)
    }

    /// Platform `/field/` route nonces are single-use: dropped on use. Other origins' nonces stay until replaced.
    private func takeNonce(_ u: HTTPURL) -> String? {
        if let platform = HTTPURL(platformUrl), platform.origin == u.origin, u.encodedPath.hasPrefix("/field/") {
            return nonces.removeValue(forKey: u.origin)
        }
        return nonces[u.origin]
    }

    // MARK: platform calls

    private func publicKey() throws -> DeviceKey {
        if let k = deviceKey { return k }
        let k: DeviceKey
        do {
            k = try keys.getOrCreate()
        } catch let e as FieldAuthError {
            throw e
        } catch let e as FcFieldError {
            throw FieldAuthError(code: e.code, message: "secure storage failed: \(e.code)")
        } catch {
            throw FieldAuthError(code: "INTERNAL", message: "secure storage failed")
        }
        deviceKey = k
        return k
    }

    private func makeProof(htm: String, htu: String, nonce: String?, accessToken: String?) throws -> String {
        let key = try publicKey()
        do {
            return try Dpop.build(
                keys: keys, key: key, htm: htm, htu: htu, nonce: nonce, accessToken: accessToken,
                iatSeconds: now() / 1000)
        } catch let e as FieldAuthError {
            throw e
        } catch let e as FcFieldError {
            throw FieldAuthError(code: e.code, message: "signing failed: \(e.code)")
        } catch {
            throw FieldAuthError(code: "INTERNAL", message: "signing failed")
        }
    }

    /// A request body of bytes, sent as it is with its own content type (the photo upload).
    private struct RawBody {
        let bytes: Data
        let contentType: String
    }

    private func send(_ url: HTTPURL, _ urlString: String, body: JObject, proof: String, accessToken: String?, raw: RawBody? = nil) async throws -> Reply {
        var req = HTTPRequest(url: urlString, method: "POST", body: raw?.bytes ?? JSON.data(body))
        req.headers["Accept"] = "application/json, image/jpeg"
        req.headers["Content-Type"] = raw?.contentType ?? "application/json; charset=utf-8"
        req.headers["DPoP"] = proof
        if let accessToken { req.headers["Authorization"] = "DPoP \(accessToken)" }
        let res: HTTPResponse
        do {
            res = try await transport.send(req)
        } catch is CancellationError {
            throw CancellationError()
        } catch {
            // The original error is dropped on purpose: it may quote the request.
            throw FieldAuthError(code: "NETWORK", message: "the platform could not be reached")
        }
        if let n = res.header("DPoP-Nonce"), !n.isEmpty { nonces[url.origin] = n }
        if (200..<300).contains(res.status), res.header("Content-Type")?.lowercased().hasPrefix("image/") == true {
            return Reply(status: res.status, headers: res.headers, json: nil, bytes: res.body)
        }
        return Reply(status: res.status, headers: res.headers, json: JSON.parseObject(res.body))
    }

    private func primeNonce(_ base: String) async throws {
        let full = base + Self.noncePath
        guard let url = HTTPURL(full) else { throw FieldAuthError(code: "INVALID_REQUEST", message: "bad platform URL") }
        let proof = try makeProof(htm: "POST", htu: url.htu, nonce: nil, accessToken: nil)
        let reply = try await send(url, full, body: [:], proof: proof, accessToken: nil)
        if reply.status >= 400 { throw replyError(reply, secrets: []) }
    }

    /// One platform POST with nonce handling (prime when none is cached, one retry on use_dpop_nonce).
    private func platformPost(_ path: String, _ body: JObject, bearer: String? = nil, secrets: [String] = [], raw: RawBody? = nil) async throws -> Reply {
        await platformMutex.lock()
        defer { platformMutex.unlock() }
        let base = try requirePlatform()
        let full = base + path
        guard let url = HTTPURL(full) else { throw FieldAuthError(code: "INVALID_REQUEST", message: "bad platform URL") }
        if nonces[url.origin] == nil { try await primeNonce(base) }
        var attempt = 0
        while true {
            let proof = try makeProof(htm: "POST", htu: url.htu, nonce: takeNonce(url), accessToken: bearer)
            let reply = try await send(url, full, body: body, proof: proof, accessToken: bearer, raw: raw)
            if reply.status == 401 && reply.error == "use_dpop_nonce" && attempt == 0 {
                attempt += 1
                continue
            }
            if reply.status >= 400 { throw replyError(reply, secrets: secrets) }
            return reply
        }
    }

    /// Takes a platform nonce for a request whose body signs it too (the unlock key proof).
    private func reservePlatformNonce() async throws -> String {
        await platformMutex.lock()
        defer { platformMutex.unlock() }
        let base = try requirePlatform()
        guard let url = HTTPURL(base + UnlockJws.registerPath) else { throw FieldAuthError(code: "INVALID_REQUEST", message: "bad platform URL") }
        if let n = nonces.removeValue(forKey: url.origin) { return n }
        try await primeNonce(base)
        if let n = nonces.removeValue(forKey: url.origin) { return n }
        throw FieldAuthError(code: "INTERNAL", message: "the platform gave no nonce")
    }

    /// One platform POST with the given nonce and no retry; the caller reads the reply's status.
    private func platformSendWithNonce(_ path: String, _ body: JObject, bearer: String?, nonce: String) async throws -> Reply {
        await platformMutex.lock()
        defer { platformMutex.unlock() }
        let full = try requirePlatform() + path
        guard let url = HTTPURL(full) else { throw FieldAuthError(code: "INVALID_REQUEST", message: "bad platform URL") }
        let proof = try makeProof(htm: "POST", htu: url.htu, nonce: nonce, accessToken: bearer)
        return try await send(url, full, body: body, proof: proof, accessToken: bearer)
    }

    /// An authenticated platform POST; one retry with a fresh access token on invalid_token.
    private func bearerPost(_ path: String, _ body: JObject, secrets: [String], raw: RawBody? = nil) async throws -> Reply {
        do {
            return try await platformPost(path, body, bearer: try await ensureAccess(), secrets: secrets, raw: raw)
        } catch let e as FieldAuthError {
            if e.code != "invalid_token" { throw e }
            access = nil
            return try await platformPost(path, body, bearer: try await ensureAccess(), secrets: secrets, raw: raw)
        }
    }

    private func replyError(_ r: Reply, secrets: [String]) -> FieldAuthError {
        let code = r.error ?? "HTTP_\(r.status)"
        var message = r.json.map { $0.optString("message") }.flatMap { $0.isEmpty ? nil : $0 } ?? code
        for s in secrets where s.count >= 4 { message = message.replacingOccurrences(of: s, with: "***") }
        let retry = r.header("Retry-After").flatMap { Int($0.trimmingCharacters(in: .whitespaces)) }
        let violations = (r.json?.optArray("violations") ?? []).map { ($0 as? String) ?? "" }
        return FieldAuthError(code: code, message: message, status: r.status, retryAfterSeconds: retry, violations: violations)
    }

    private func noteFailure(_ e: FieldAuthError) -> FieldAuthError {
        switch e.code {
        case "WORKER_LOCKED": emit(.workerLocked(hard: true, retryAfterSeconds: nil))
        case "WORKER_LOCKED_TEMPORARILY", "TOO_MANY_ATTEMPTS": emit(.workerLocked(hard: false, retryAfterSeconds: e.retryAfterSeconds))
        default: break
        }
        return e
    }

    // MARK: session state

    /// Stores a token response and publishes the new session.
    private func apply(_ json: JObject?, preserveLock: Bool) throws -> Session {
        guard let t = json else { throw FieldAuthError(code: "INTERNAL", message: "malformed token response") }
        let accessToken = t.optString("accessToken")
        let newRefresh = t.optString("refreshToken")
        guard !accessToken.isEmpty, !newRefresh.isEmpty, let w = t.optObject("worker"), t.has("expiresIn") else {
            throw FieldAuthError(code: "INTERNAL", message: "malformed token response")
        }
        let now = now()
        let claims = decodeClaims(accessToken)
        var actor: Actor?
        if let act = claims?.optObject("act") {
            let sub = act.optString("sub")
            if !sub.isEmpty {
                let typ = act.optString("typ")
                actor = Actor(id: sub, type: typ.isEmpty ? "field_worker" : typ, name: act.optString("name"))
            }
        }
        let deviceMode: DeviceMode
        switch claims?.optString("device_mode") {
        case "PERSONAL": deviceMode = .personal
        case "SHARED": deviceMode = .shared
        case "UNENROLLED": deviceMode = .unenrolled
        default: deviceMode = deviceModeFromDevice()
        }
        let deviceId: String?
        if let c = claims {
            let id = c.optString("device_id")
            deviceId = id.isEmpty ? nil : id
        } else {
            deviceId = device?.deviceId
        }
        let idle = try parseTime(t.optString("idleExpiresAt"))
        let refreshExp = try parseTime(t.optString("refreshExpiresAt"))
        let session = Session(
            worker: Worker(
                id: w.optString("id"), workerCode: w.optString("workerCode"), name: w.optString("name"),
                photoVersion: Self.photoVersion(w), canSetOwnPhoto: w.optBool("canSetOwnPhoto", false)),
            actor: actor,
            test: claims?.optBool("test", false) == true,
            deviceMode: deviceMode,
            deviceId: deviceId,
            appCode: sessionState?.appCode ?? config.appCode,
            expiresAt: now + t.optLong("expiresIn") * 1000,
            idleExpiresAt: idle,
            refreshExpiresAt: refreshExp,
            pinChangeRequired: t.optBool("pinChangeRequired", false),
            clientId: Self.claimString(claims, "client_id"),
            clientIdentifier: Self.claimString(claims, "client_identifier"),
            amr: (claims?.optArray("amr") ?? []).map { ($0 as? String) ?? "" },
            unlockKeys: Self.unlockKeysOf(t, now: now)
        )
        // Counted from when this answer arrived, on the monotonic clock; every answer replaces the previous one.
        registerDeadline = Self.registerWindowSeconds(t).map { clock.monotonicMilliseconds() + $0 * 1000 }
        access = Access(token: accessToken, expiresAt: session.expiresAt)
        refreshToken = newRefresh
        sessionState = session
        issuedAt = now
        refreshGen += 1
        try store {
            try storage.put(Self.storeRefresh, value: newRefresh)
            try storage.put(Self.storeSession, value: JSON.string(sessionToJson(session)))
        }
        publish(session, preserveLock: preserveLock)
        emit(.sessionChanged(session))
        if session.pinChangeRequired { emit(.pinChangeRequired(session)) }
        ensureTimers()
        return session
    }

    /// The token answer's `unlockKeys.registerExpiresIn`, or nil.
    private static func registerWindowSeconds(_ t: JObject) -> Int64? {
        guard let u = t.optObject("unlockKeys"), !u.isNull("registerExpiresIn") else { return nil }
        return JSON.int64(u["registerExpiresIn"])
    }

    /// The token answer's `unlockKeys`; nil when the platform sent none.
    private static func unlockKeysOf(_ t: JObject, now: Int64) -> SessionUnlockKeys? {
        guard let u = t.optObject("unlockKeys") else { return nil }
        return SessionUnlockKeys(
            allowed: u.optBool("allowed", false), attestationRequired: u.optBool("attestationRequired", false),
            registerExpiresAt: registerWindowSeconds(t).map { now + $0 * 1000 })
    }

    private func deviceModeFromDevice() -> DeviceMode {
        switch device?.mode {
        case "PERSONAL": return .personal
        case "SHARED": return .shared
        default: return .unenrolled
        }
    }

    private func publish(_ session: Session, preserveLock: Bool) {
        var wasLocked = false
        if case .locked = stateBox.value { wasLocked = true }
        stateBox.set(preserveLock && wasLocked && lockEligible(session) ? .locked(session) : .signedIn(session))
    }

    /// Clears local session state and tells listeners.
    private func endSession(_ reason: String) {
        let had = sessionState != nil
        access = nil
        refreshToken = nil
        sessionState = nil
        supervisor = nil
        warnGen += 1
        warnTask?.cancel()
        warnTask = nil
        warnedKey = nil
        warned.removeAll()
        // best effort: the in-memory state is already gone
        try? storage.remove(Self.storeRefresh)
        try? storage.remove(Self.storeSession)
        clearSupervisorStore()
        registerDeadline = nil
        stateBox.set(.signedOut)
        if had {
            emit(.sessionEnded(reason))
            emit(.sessionChanged(nil))
        }
    }

    private func supervisorToken(_ sup: Supervisor) throws -> String {
        if let t = sup.refreshToken { return t }
        guard let t = try store({ try storage.get(Self.storeSupervisorRefresh) }) else {
            throw FieldAuthError(code: "SESSION_EXPIRED", message: "the supervisor session is gone")
        }
        sup.refreshToken = t
        return t
    }

    /// Makes the kept-aside supervisor session the active one.
    private func switchToSupervisor(_ sup: Supervisor, reason: String) throws {
        let token = try supervisorToken(sup)
        supervisor = nil
        access = sup.access
        refreshToken = token
        sessionState = sup.session
        registerDeadline = nil // the supervisor's window was not kept aside; its next refresh brings it
        issuedAt = now()
        refreshGen += 1
        try store {
            try storage.put(Self.storeRefresh, value: token)
            try storage.put(Self.storeSession, value: JSON.string(sessionToJson(sup.session)))
        }
        clearSupervisorStore()
        publish(sup.session, preserveLock: false)
        emit(.sessionEnded(reason))
        emit(.sessionChanged(sup.session))
        ensureTimers()
    }

    private func clearSupervisorStore() {
        try? storage.remove(Self.storeSupervisor) // best effort
        try? storage.remove(Self.storeSupervisorRefresh)
    }

    private func clearSupervisor() {
        supervisor = nil
        clearSupervisorStore()
    }

    // MARK: keepAlive and expiry warnings

    private func ensureTimers() {
        guard let s = sessionState else { return }
        if keepAliveOn && keepAliveTask == nil {
            keepAliveGen += 1
            let g = keepAliveGen
            keepAliveTask = Task { await self.keepAliveLoop(g) }
        }
        if !warnThresholds.isEmpty {
            let key = "\(s.worker.id):\(s.refreshExpiresAt)"
            if key != warnedKey {
                warned.removeAll()
                warnedKey = key
                warnGen += 1
                warnTask?.cancel()
                warnTask = nil
            }
            if warnTask == nil && warned.count < warnThresholds.count {
                warnGen += 1
                let g = warnGen
                warnTask = Task { await self.warnLoop(key, g) }
            }
        }
    }

    private func keepAliveLoop(_ gen: Int) async {
        let lead = Self.millis(config.keepAliveLead)
        while true {
            guard gen == keepAliveGen else { return }
            guard let s = sessionState, keepAliveOn else {
                keepAliveTask = nil
                return
            }
            let issued = issuedAt
            let target = min(s.idleExpiresAt - lead, issued + (s.idleExpiresAt - issued) / 2)
            let wait = target - now()
            if wait > 0 {
                do { try await clock.sleep(milliseconds: wait) } catch { return }
                guard gen == keepAliveGen else { return }
                guard let s2 = sessionState, s2.idleExpiresAt == s.idleExpiresAt else { continue } // refreshed elsewhere or ended: re-plan
            }
            do {
                _ = try await refresh()
            } catch is CancellationError {
                return
            } catch let e as FieldAuthError {
                if !sessionEndingCodes.contains(e.code) {
                    do { try await clock.sleep(milliseconds: Self.retryAfterFailureMs) } catch { return }
                }
            } catch {
                // not a library error: treated like a transient failure
            }
            do { try await clock.sleep(milliseconds: Self.minGapMs) } catch { return }
        }
    }

    private func warnLoop(_ key: String, _ gen: Int) async {
        defer { if warnGen == gen { warnTask = nil } }
        while true {
            guard gen == warnGen, let s = sessionState else { return }
            if "\(s.worker.id):\(s.refreshExpiresAt)" != key { return }
            let remaining = s.refreshExpiresAt - now()
            let pending = warnThresholds.filter { !warned.contains($0) }
            guard let longest = pending.max() else { return }
            let due = pending.filter { remaining <= $0 }
            if !due.isEmpty {
                warned.formUnion(due)
                emit(.expiryWarning(remaining: Double(max(remaining, 0)) / 1000))
                continue
            }
            do { try await clock.sleep(milliseconds: remaining - longest) } catch { return }
        }
    }

    // MARK: serialisation

    private func parseTime(_ s: String) throws -> Int64 {
        guard let t = parseOffsetDateTimeMillis(s) else {
            throw FieldAuthError(code: "INTERNAL", message: "malformed token response")
        }
        return t
    }

    private func decodeClaims(_ jwt: String) -> JObject? {
        let parts = jwt.split(separator: ".", omittingEmptySubsequences: false)
        guard parts.count == 3, !parts[1].isEmpty, let d = Base64URL.decode(String(parts[1])) else { return nil }
        return JSON.parseObject(d)
    }

    private func deviceToJson(_ r: DeviceRecord) -> JObject {
        ["deviceId": r.deviceId, "mode": r.mode, "clientId": r.clientId, "appCodes": r.appCodes, "platformUrl": r.platformUrl]
    }

    private func deviceFromJson(_ j: JObject) -> DeviceRecord? {
        guard let id = j["deviceId"] as? String, let mode = j["mode"] as? String else { return nil }
        return DeviceRecord(
            deviceId: id, mode: mode, clientId: j.optString("clientId"),
            appCodes: (j.optArray("appCodes") ?? []).compactMap { $0 as? String },
            platformUrl: j.optString("platformUrl"))
    }

    private func sessionToJson(_ s: Session) -> JObject {
        var j: JObject = [
            "worker": [
                "id": s.worker.id, "workerCode": s.worker.workerCode, "name": s.worker.name,
                "photoVersion": s.worker.photoVersion ?? NSNull(), "canSetOwnPhoto": s.worker.canSetOwnPhoto,
            ] as JObject,
            "actor": NSNull(),
            "test": s.test,
            "deviceMode": s.deviceMode.rawValue,
            "deviceId": NSNull(),
            "appCode": s.appCode,
            "expiresAt": s.expiresAt,
            "idleExpiresAt": s.idleExpiresAt,
            "refreshExpiresAt": s.refreshExpiresAt,
            "pinChangeRequired": s.pinChangeRequired,
        ]
        j["clientId"] = s.clientId ?? NSNull()
        j["clientIdentifier"] = s.clientIdentifier ?? NSNull()
        j["amr"] = s.amr
        // The register window is not stored: it is counted on the monotonic clock, which a restart does not carry.
        j["unlockKeys"] = s.unlockKeys.map { ["allowed": $0.allowed, "attestationRequired": $0.attestationRequired] as JObject } ?? NSNull()
        if let a = s.actor { j["actor"] = ["id": a.id, "type": a.type, "name": a.name] as JObject }
        if let d = s.deviceId { j["deviceId"] = d }
        return j
    }

    /// A claim that is a non-empty string, else nil (an absent claim is nil, never a guess).
    private static func claimString(_ o: JObject?, _ key: String) -> String? {
        guard let o = o, !o.isNull(key) else { return nil }
        let v = o.optString(key)
        return v.isEmpty ? nil : v
    }

    private func sessionFromJson(_ j: JObject) -> Session? {
        guard let w = j.optObject("worker"), let wid = w["id"] as? String,
              let appCode = j["appCode"] as? String,
              let expiresAt = JSON.int64(j["expiresAt"]), let idle = JSON.int64(j["idleExpiresAt"]),
              let refreshExp = JSON.int64(j["refreshExpiresAt"])
        else { return nil }
        var actor: Actor?
        if let a = j.optObject("actor"), let aid = a["id"] as? String {
            let typ = a["type"] as? String
            actor = Actor(id: aid, type: typ ?? "field_worker", name: a.optString("name"))
        }
        let mode = (j["deviceMode"] as? String).flatMap(DeviceMode.init(rawValue:)) ?? deviceModeFromDevice()
        let deviceId: String? = j.isNull("deviceId") ? device?.deviceId : (j["deviceId"] as? String)
        return Session(
            worker: Worker(
                id: wid, workerCode: w.optString("workerCode"), name: w.optString("name"),
                photoVersion: Self.photoVersion(w), canSetOwnPhoto: w.optBool("canSetOwnPhoto", false)),
            actor: actor,
            test: j.optBool("test", false),
            deviceMode: mode,
            deviceId: deviceId,
            appCode: appCode,
            expiresAt: expiresAt,
            idleExpiresAt: idle,
            refreshExpiresAt: refreshExp,
            pinChangeRequired: j.optBool("pinChangeRequired", false),
            clientId: Self.claimString(j, "clientId"),
            clientIdentifier: Self.claimString(j, "clientIdentifier"),
            amr: (j.optArray("amr") ?? []).compactMap { $0 as? String },
            unlockKeys: j.optObject("unlockKeys").map {
                SessionUnlockKeys(allowed: $0.optBool("allowed", false), attestationRequired: $0.optBool("attestationRequired", false))
            }
        )
    }

    // MARK: enrolment QR

    /// Parses `fcfield:1:<platformBaseUrl>:<code>`; the code is the last segment.
    private func parseEnrolmentQr(_ payload: String) throws -> (url: String, code: String) {
        let bad = FieldAuthError(code: "INVALID_ENROLMENT_CODE", message: "not a FlowCatalyst enrolment QR code")
        let p = payload.trimmingCharacters(in: .whitespacesAndNewlines)
        guard p.hasPrefix(Self.qrPrefix) else { throw bad }
        let rest = String(p.dropFirst(Self.qrPrefix.count))
        guard let colon = rest.lastIndex(of: ":"), colon > rest.startIndex else { throw bad }
        let url = Self.trimTrailingSlashes(String(rest[rest.startIndex..<colon]))
        let code = String(rest[rest.index(after: colon)...])
        if code.isEmpty || code.unicodeScalars.contains(where: { CharacterSet.whitespacesAndNewlines.contains($0) }) { throw bad }
        guard HTTPURL(url) != nil else { throw bad }
        return (url, code)
    }

    /// `http:` is accepted only for loopback hosts, or when the QR's origin is the configured platform's or one in
    /// `FieldConfig.allowedPlatformOrigins` (the integrator named that http origin on purpose).
    private func requireHttpsOrNamedOrigin(_ qrUrl: String) throws {
        guard let u = HTTPURL(qrUrl) else { return }
        if u.scheme != "http" || Self.loopbackHosts.contains(u.host) { return }
        let named = ([config.platformUrl] + config.allowedPlatformOrigins).compactMap { originOf(Self.trimTrailingSlashes($0)) }
        if let o = originOf(qrUrl), named.contains(o) { return }
        throw FieldAuthError(
            code: "INVALID_ENROLMENT_CODE",
            message: "enrolment QR code uses http: only https is accepted (http only for localhost, 127.0.0.1, [::1], 10.0.2.2, or an origin named in platformUrl / allowedPlatformOrigins)"
        )
    }

    /// scheme://host:port of a URL, or nil when it is not an http(s) URL.
    private func originOf(_ url: String) -> String? { HTTPURL(url)?.origin }
}
