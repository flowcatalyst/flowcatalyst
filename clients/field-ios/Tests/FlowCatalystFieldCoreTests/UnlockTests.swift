import XCTest
@testable import FlowCatalystFieldCore

/// Port of UnlockTest.kt.
final class UnlockTests: XCTestCase {
    var h: Harness!
    var p: FakePlatform { h.platform }
    let policy = UnlockPolicy(afterColdStart: true, afterBackground: 5 * 60)

    override func setUp() async throws {
        h = Harness()
        p.on("/field/auth/refresh") { _ in self.p.token(claims: self.p.deviceClaims()) }
    }

    override func tearDown() async throws { h.close() }

    private struct Prompter: UnlockPrompter {
        let body: @Sendable (String) async throws -> Void
        func prompt(_ text: String) async throws { try await body(text) }
    }

    private let ok = Prompter { _ in }

    @discardableResult
    private func enrolledLogin(claims: JObject? = nil, cfg: FieldConfig? = nil) async throws -> FieldSession {
        p.on("/field/devices/enrol") { _ in .json(201, #"{"deviceId":"dev1","mode":"PERSONAL","clientId":"c1","appCodes":["app1"]}"#) }
        let c = claims ?? p.deviceClaims("PERSONAL", id: "dev1")
        p.on("/field/auth/login") { _ in self.p.token(claims: c) }
        let s = h.session(cfg ?? h.config(requireUnlock: policy))
        _ = try await s.enrol("ABCDE12345", info: DeviceInfo())
        _ = try await s.login(LoginCredentials(workerCode: "W001", pin: "1234"))
        return s
    }

    private func isLocked(_ e: FieldEvent) -> Bool { if case .locked = e { return true } else { return false } }

    // freshLoginIsNotLockedButColdStartIs
    func testFreshLoginIsNotLockedButColdStartIs() async throws {
        let s = try await enrolledLogin()
        if case .signedIn = s.state {} else { XCTFail("expected signedIn") }
        let s2 = h.session(h.config(requireUnlock: policy))
        let st = try await s2.restore()
        if case .locked = st {} else { XCTFail("expected locked, got \(st)") }
        XCTAssertTrue(h.events.all.contains(where: isLocked))
    }

    // lockedDoesNotBlockHeadersOrRefresh
    func testLockedDoesNotBlockHeadersOrRefresh() async throws {
        try await enrolledLogin()
        let s2 = h.session(h.config(requireUnlock: policy))
        let st = try await s2.restore()
        if case .locked = st {} else { XCTFail("expected locked") }
        let hdr = try await s2.headersFor(method: "GET", url: "https://api.example.com/x") // refreshes in the background
        XCTAssertNotNil(hdr["DPoP"])
        XCTAssertEqual(1, p.count("/field/auth/refresh"))
        if case .locked = s2.state {} else { XCTFail("a refresh does not unlock") }
    }

    // unlockClearsTheLock
    func testUnlockClearsTheLock() async throws {
        try await enrolledLogin()
        let s2 = h.session(h.config(requireUnlock: policy))
        _ = try await s2.restore()
        let shown = Locked<String?>(nil)
        let ses = try await s2.unlock(using: Prompter { shown.value = $0 }, prompt: "Unlock fulfil")
        XCTAssertEqual("Unlock fulfil", shown.value)
        XCTAssertEqual(.signedIn(ses), s2.state)
    }

    // cancelledPromptStaysLocked
    func testCancelledPromptStaysLocked() async throws {
        try await enrolledLogin()
        let s2 = h.session(h.config(requireUnlock: policy))
        _ = try await s2.restore()
        await failsWith("BIOMETRIC_CANCELLED") {
            _ = try await s2.unlock(using: Prompter { _ in throw FieldAuthError(code: "BIOMETRIC_CANCELLED") }, prompt: "x")
        }
        if case .locked = s2.state {} else { XCTFail("expected locked") }
    }

    // backgroundTimeLocksOnlyAfterTheLimit
    func testBackgroundTimeLocksOnlyAfterTheLimit() async throws {
        let s = try await enrolledLogin()
        await s.noteBackgrounded()
        h.clock.jump(4 * MIN)
        await s.noteForegrounded()
        if case .signedIn = s.state {} else { XCTFail("expected signedIn") }
        await s.noteBackgrounded()
        h.clock.jump(5 * MIN)
        await s.noteForegrounded()
        if case .locked = s.state {} else { XCTFail("expected locked") }
    }

    // unenrolledNeverLocks
    func testUnenrolledNeverLocks() async throws {
        var claims = p.deviceClaims("UNENROLLED")
        claims.removeValue(forKey: "device_id")
        p.on("/field/auth/login") { _ in self.p.token(claims: claims) }
        _ = try await h.session(h.config(requireUnlock: policy)).login(LoginCredentials(workerCode: "W001", pin: "1234"))
        let s2 = h.session(h.config(requireUnlock: policy))
        let st = try await s2.restore()
        if case .signedIn = st {} else { XCTFail("expected signedIn") }
        await failsWith("BIOMETRIC_UNAVAILABLE") { _ = try await s2.unlock(using: ok, prompt: "x") }
    }

    // sharedDeviceNeverLocks
    func testSharedDeviceNeverLocks() async throws {
        try await enrolledLogin(claims: p.deviceClaims("SHARED", id: "dev1"))
        let st = try await h.session(h.config(requireUnlock: policy)).restore()
        if case .signedIn = st {} else { XCTFail("expected signedIn") }
    }

    // impersonatedSessionNeverLocks
    func testImpersonatedSessionNeverLocks() async throws {
        let act: JObject = [
            "act": ["sub": "adm", "typ": "principal", "name": "A"] as JObject,
            "device_mode": "PERSONAL", "device_id": "dev1",
        ]
        p.on("/field/auth/sign-in-code") { _ in self.p.token(claims: act) }
        let s = h.session(h.config(requireUnlock: policy))
        _ = try await s.redeemSignInCode("CODE1234")
        let st = try await h.session(h.config(requireUnlock: policy)).restore()
        if case .signedIn = st {} else { XCTFail("expected signedIn") }
        await s.noteBackgrounded(); h.clock.jump(10 * MIN); await s.noteForegrounded()
        if case .signedIn = s.state {} else { XCTFail("expected signedIn") }
    }

    // Swift addition. Kotlin's impersonatedSessionNeverLocks never enrols the device, so the session is ineligible for
    // lack of a device record whatever the actor rule says; here the device is enrolled PERSONAL and only the actor
    // (and the kept-aside supervisor) keep the lock off.
    func testImpersonatedSessionOnAnEnrolledDeviceNeverLocks() async throws {
        let act: JObject = [
            "act": ["sub": "adm", "typ": "principal", "name": "A"] as JObject,
            "device_mode": "PERSONAL", "device_id": "dev1",
        ]
        p.on("/field/devices/enrol") { _ in .json(201, #"{"deviceId":"dev1","mode":"PERSONAL","clientId":"c1","appCodes":["app1"]}"#) }
        p.on("/field/auth/sign-in-code") { _ in self.p.token(claims: act) }
        let s = h.session(h.config(requireUnlock: policy))
        _ = try await s.enrol("ABCDE12345", info: DeviceInfo())
        _ = try await s.redeemSignInCode("CODE1234")
        let st = try await h.session(h.config(requireUnlock: policy)).restore()
        if case .signedIn = st {} else { XCTFail("a restored impersonated session must not lock, got \(st)") }
        await s.noteBackgrounded(); h.clock.jump(10 * MIN); await s.noteForegrounded()
        if case .signedIn = s.state {} else { XCTFail("backgrounding must not lock an impersonated session") }
    }

    // unlockWhenNotLockedJustReturnsTheSession
    func testUnlockWhenNotLockedJustReturnsTheSession() async throws {
        let s = try await enrolledLogin()
        let prompted = Locked(false)
        _ = try await s.unlock(using: Prompter { _ in prompted.value = true }, prompt: "x")
        XCTAssertFalse(prompted.value)
    }

    // neverPolicyNeverLocks
    func testNeverPolicyNeverLocks() async throws {
        try await enrolledLogin(cfg: h.config())
        let st = try await h.session(h.config()).restore()
        if case .signedIn = st {} else { XCTFail("expected signedIn") }
    }

    // sameConfigIsRequiredByGet (the Apple target's `get` compares configs with this equality)
    func testSameConfigIsRequiredByGet() {
        XCTAssertEqual(h.config(), h.config())
        XCTAssertNotEqual(h.config(), h.config(clientIdentifier: "other"))
        XCTAssertNotEqual(h.config(requireUnlock: policy), h.config())
    }

    // Swift addition: `unlock(prompt:)` with no prompter configured, and with no stored session.
    func testUnlockWithoutPrompterOrSession() async throws {
        let s = h.session()
        await failsWith("BIOMETRIC_UNAVAILABLE") { _ = try await s.unlock(prompt: "x") }
        await failsWith("SESSION_EXPIRED") { _ = try await s.unlock(using: ok, prompt: "x") }
    }

    // Port of BiometricUnlocker.mapError's table, for the LocalAuthentication codes (LAError.Code raw values).
    func testLocalAuthenticationErrorMapping() {
        let table: [(Int?, String)] = [
            (-2, "BIOMETRIC_CANCELLED"), (-4, "BIOMETRIC_CANCELLED"), (-9, "BIOMETRIC_CANCELLED"), (-3, "BIOMETRIC_CANCELLED"),
            (-5, "NO_SECURE_LOCK_SCREEN"),
            (-6, "BIOMETRIC_UNAVAILABLE"), (-7, "BIOMETRIC_UNAVAILABLE"), (-8, "BIOMETRIC_UNAVAILABLE"), (nil, "BIOMETRIC_UNAVAILABLE"),
            (-1, "INTERNAL"), (-10, "INTERNAL"), (-1004, "INTERNAL"),
        ]
        for (raw, code) in table { XCTAssertEqual(code, UnlockErrorMapping.map(laErrorRawCode: raw).code, "\(String(describing: raw))") }
        XCTAssertEqual("unlock error -1", UnlockErrorMapping.map(laErrorRawCode: -1).message)
    }
}
