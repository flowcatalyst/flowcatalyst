import XCTest
@testable import FlowCatalystFieldCore

/// Port of WireFlowTest.kt (the enrolment, sign-in, act-as, PIN, logout and storage flows and their exact requests).
final class WireFlowTests: XCTestCase {
    var h: Harness!
    var p: FakePlatform { h.platform }

    override func setUp() async throws { h = Harness() }
    override func tearDown() async throws { h.close() }

    private func ok(_ claims: JObject = [:]) -> (Rec) -> FakeResponse { { _ in self.p.token(claims: claims) } }
    private let enrolOk = FakeResponse.json(201, #"{"deviceId":"dev1","mode":"PERSONAL","clientId":"c1","appCodes":["app1"]}"#)

    private func enrolOkFor(appCodes: String = "[]", mode: String = "PERSONAL") {
        p.on("/field/devices/enrol") { _ in .json(201, #"{"deviceId":"dev1","mode":"\#(mode)","clientId":"c1","appCodes":\#(appCodes)}"#) }
    }

    // MARK: enrolment

    // enrolByQrSetsPlatformUrlSendsKeyAndPersistsDevice
    func testEnrolByQrSetsPlatformUrlSendsKeyAndPersistsDevice() async throws {
        p.on("/field/devices/enrol") { _ in self.enrolOk }
        let s = h.session(h.config(platformUrl: "")) // no configured URL: the QR is adopted
        let rec = try await s.enrol("fcfield:1:\(p.url):ABCDE12345", info: DeviceInfo(platform: "android", model: "Pixel 9", label: "van 3"))
        XCTAssertEqual(DeviceRecord(deviceId: "dev1", mode: "PERSONAL", clientId: "c1", appCodes: ["app1"], platformUrl: p.url), rec)
        // the nonce route is primed first, then the single-use nonce goes into the enrol proof
        XCTAssertEqual(["/field/auth/nonce", "/field/devices/enrol"], p.requests.map(\.path))
        let body = try XCTUnwrap(p.all("/field/devices/enrol").first?.body)
        XCTAssertEqual("ABCDE12345", body.optString("code"))
        XCTAssertEqual("android", body.optString("platform"))
        XCTAssertEqual("Pixel 9", body.optString("model"))
        XCTAssertEqual("van 3", body.optString("label"))
        XCTAssertEqual(h.keys.key.x, body.optObject("publicKeyJwk")?.optString("x"))
        XCTAssertEqual(h.keys.key.y, body.optObject("publicKeyJwk")?.optString("y"))
        XCTAssertEqual("EC", body.optObject("publicKeyJwk")?.optString("kty"))
        XCTAssertEqual("P-256", body.optObject("publicKeyJwk")?.optString("crv"))
        XCTAssertFalse(body.has("attestationChain"))
        XCTAssertNotNil(try h.storage.get(FieldSession.storeDevice))
    }

    // qrForAnotherOriginThanConfiguredIsRefusedAndNothingPersisted
    func testQrForAnotherOriginThanConfiguredIsRefusedAndNothingPersisted() async throws {
        let s = h.session(h.config(platformUrl: "https://configured.example.com"))
        await failsWith("PLATFORM_MISMATCH") { _ = try await s.enrol("fcfield:1:\(p.url):ABCDE12345", info: DeviceInfo()) }
        XCTAssertTrue(p.requests.isEmpty)
        XCTAssertNil(try h.storage.get(FieldSession.storeDevice))
    }

    // qrForTheConfiguredOriginIsAccepted
    func testQrForTheConfiguredOriginIsAccepted() async throws {
        enrolOkFor()
        let rec = try await h.session().enrol("fcfield:1:\(p.url):ABCDE12345", info: DeviceInfo())
        XCTAssertEqual(p.url, rec.platformUrl)
    }

    // qrForAnAllowedOriginIsAccepted
    func testQrForAnAllowedOriginIsAccepted() async throws {
        enrolOkFor()
        var cfg = h.config(platformUrl: "https://configured.example.com")
        cfg.allowedPlatformOrigins = [p.url]
        let rec = try await h.session(cfg).enrol("fcfield:1:\(p.url):ABCDE12345", info: DeviceInfo())
        XCTAssertEqual(p.url, rec.platformUrl)
    }

    // httpQrIsRefusedUnlessLoopback
    func testHttpQrIsRefusedUnlessLoopback() async throws {
        let s = h.session(h.config(platformUrl: ""))
        await failsWith("INVALID_ENROLMENT_CODE") { _ = try await s.enrol("fcfield:1:http://example.com:ABCDE12345", info: DeviceInfo()) }
        enrolOkFor()
        // the fake platform is on a loopback host over http: accepted
        let rec = try await h.session(h.config(platformUrl: "")).enrol("fcfield:1:\(p.url):ABCDE12345", info: DeviceInfo())
        XCTAssertEqual(p.url, rec.platformUrl)
    }

    // A name under .invalid never resolves (RFC 2606); here no route exists for it, so the request fails at once.
    private let lan = "http://dev-platform.invalid:8080"

    private func assertPassesPolicy(_ s: FieldSession, _ qr: String) async {
        // getting past the policy means the request is attempted (and fails with a network error)
        do {
            _ = try await s.enrol(qr, info: DeviceInfo())
        } catch let e as FieldAuthError {
            XCTAssertNotEqual(e.code, "INVALID_ENROLMENT_CODE")
            XCTAssertNotEqual(e.code, "PLATFORM_MISMATCH")
            XCTAssertEqual(e.code, "NETWORK")
        } catch { XCTFail("\(error)") }
    }

    // httpQrForTheConfiguredHttpOriginPassesThePolicy
    func testHttpQrForTheConfiguredHttpOriginPassesThePolicy() async {
        await assertPassesPolicy(h.session(h.config(platformUrl: lan)), "fcfield:1:\(lan):ABCDE12345")
    }

    // httpQrForAnAllowedOriginPassesThePolicy
    func testHttpQrForAnAllowedOriginPassesThePolicy() async {
        var cfg = h.config(platformUrl: "https://configured.example.com")
        cfg.allowedPlatformOrigins = [lan]
        await assertPassesPolicy(h.session(cfg), "fcfield:1:\(lan):ABCDE12345")
    }

    // lanHttpQrWithNoConfiguredUrlIsRefused
    func testLanHttpQrWithNoConfiguredUrlIsRefused() async {
        await failsWith("INVALID_ENROLMENT_CODE") { _ = try await h.session(h.config(platformUrl: "")).enrol("fcfield:1:\(lan):ABCDE12345", info: DeviceInfo()) }
        XCTAssertTrue(p.requests.isEmpty)
    }

    // configuredHttpsOriginWithHttpQrForTheSameHostIsAMismatch
    func testConfiguredHttpsOriginWithHttpQrForTheSameHostIsAMismatch() async {
        await failsWith("PLATFORM_MISMATCH") {
            _ = try await h.session(h.config(platformUrl: "https://dev-platform.invalid:8080")).enrol("fcfield:1:\(lan):ABCDE12345", info: DeviceInfo())
        }
    }

    // enrolWithPlainCodeUsesConfiguredPlatform
    func testEnrolWithPlainCodeUsesConfiguredPlatform() async throws {
        enrolOkFor(mode: "SHARED")
        let rec = try await h.session().enrol("  ABCDE12345 ", info: DeviceInfo())
        XCTAssertEqual("SHARED", rec.mode)
        XCTAssertEqual("ABCDE12345", p.all("/field/devices/enrol").first?.body?.optString("code"))
    }

    // badQrIsInvalidEnrolmentCode
    func testBadQrIsInvalidEnrolmentCode() async {
        for bad in ["fcfield:1:nocode", "fcfield:1:ftp://x.example:CODE", "fcfield:1:\(p.url):", "fcfield:1:\(p.url):CO DE"] {
            await failsWith("INVALID_ENROLMENT_CODE") { _ = try await h.session().enrol(bad, info: DeviceInfo()) }
        }
        XCTAssertTrue(p.requests.isEmpty)
    }

    // enrolErrorCodePassesThrough
    func testEnrolErrorCodePassesThrough() async {
        p.on("/field/devices/enrol") { _ in self.p.error(401, "INVALID_ENROLMENT_CODE") }
        let e = await failsWith("INVALID_ENROLMENT_CODE") { _ = try await h.session().enrol("ABCDE12345", info: DeviceInfo()) }
        XCTAssertEqual(401, e?.status)
    }

    // MARK: login

    // unenrolledLoginByWorkerCodeNeedsClientIdentifier
    func testUnenrolledLoginByWorkerCodeNeedsClientIdentifier() async {
        let s = h.session(h.config(clientIdentifier: nil))
        await failsWith("INVALID_REQUEST") { _ = try await s.login(LoginCredentials(workerCode: "W001", pin: "1234")) }
        XCTAssertTrue(p.requests.isEmpty)
    }

    // unenrolledLoginWorksAndIsUnenrolled
    func testUnenrolledLoginWorksAndIsUnenrolled() async throws {
        p.on("/field/auth/login", ok(["device_mode": "UNENROLLED"]))
        let s = h.session()
        let ses = try await s.login(LoginCredentials(workerCode: "W001", pin: "1234"))
        XCTAssertEqual(.unenrolled, ses.deviceMode)
        XCTAssertNil(ses.deviceId)
        XCTAssertNil(ses.actor)
        XCTAssertEqual("W001", ses.worker.workerCode)
        XCTAssertEqual("app1", ses.appCode)
        XCTAssertEqual(h.clock.now() + 600_000, ses.expiresAt)
        let body = try XCTUnwrap(p.all("/field/auth/login").first?.body)
        XCTAssertEqual("app1", body.optString("appCode"))
        XCTAssertEqual("W001", body.optString("workerCode"))
        XCTAssertEqual("1234", body.optString("pin"))
        XCTAssertEqual("acme", body.optString("clientIdentifier"))
        XCTAssertFalse(body.has("badgeToken"))
        XCTAssertEqual(.signedIn(ses), s.state)
        XCTAssertTrue(h.events.contains(.sessionChanged(ses)))
    }

    // perCallClientIdentifierOverridesConfig
    func testPerCallClientIdentifierOverridesConfig() async throws {
        p.on("/field/auth/login", ok())
        _ = try await h.session().login(LoginCredentials(workerCode: "W001", pin: "1234", clientIdentifier: "other"))
        XCTAssertEqual("other", p.all("/field/auth/login").first?.body?.optString("clientIdentifier"))
    }

    // enrolledLoginDoesNotNeedClientIdentifierAndTakesDeviceFromClaims
    func testEnrolledLoginDoesNotNeedClientIdentifierAndTakesDeviceFromClaims() async throws {
        enrolOkFor(appCodes: #"["app1"]"#)
        p.on("/field/auth/login", ok(p.deviceClaims("PERSONAL", id: "dev1")))
        let s = h.session(h.config(clientIdentifier: nil))
        _ = try await s.enrol("ABCDE12345", info: DeviceInfo())
        let ses = try await s.login(LoginCredentials(workerCode: "W001", pin: "1234"))
        XCTAssertEqual(.personal, ses.deviceMode)
        XCTAssertEqual("dev1", ses.deviceId)
        XCTAssertFalse(try XCTUnwrap(p.all("/field/auth/login").first?.body).has("clientIdentifier"))
    }

    // badgeLoginNormalisesAndSendsBadgeToken
    func testBadgeLoginNormalisesAndSendsBadgeToken() async throws {
        p.on("/field/auth/login", ok())
        _ = try await h.session().login(LoginCredentials(badgeToken: "  ab-12\r\n", pin: "1234"))
        let body = try XCTUnwrap(p.all("/field/auth/login").first?.body)
        XCTAssertEqual("AB-12", body.optString("badgeToken"))
        XCTAssertFalse(body.has("workerCode"))
    }

    // badgeOnlyLoginOmitsPin
    func testBadgeOnlyLoginOmitsPin() async throws {
        p.on("/field/auth/login", ok())
        _ = try await h.session().login(LoginCredentials(badgeToken: "AB12"))
        XCTAssertFalse(try XCTUnwrap(p.all("/field/auth/login").first?.body).has("pin"))
    }

    // loginNeedsExactlyOneOfCodeAndBadge
    func testLoginNeedsExactlyOneOfCodeAndBadge() async {
        let s = h.session()
        await failsWith("INVALID_REQUEST") { _ = try await s.login(LoginCredentials(pin: "1234")) }
        await failsWith("INVALID_REQUEST") { _ = try await s.login(LoginCredentials(workerCode: "W", badgeToken: "B", pin: "1234")) }
    }

    // wrongPinKeepsCodeAndNeverEchoesPin
    func testWrongPinKeepsCodeAndNeverEchoesPin() async {
        p.on("/field/auth/login") { _ in self.p.error(401, "INVALID_CREDENTIALS", message: "wrong pin 987654") }
        let e = await failsWith("INVALID_CREDENTIALS") { _ = try await h.session().login(LoginCredentials(workerCode: "W001", pin: "987654")) }
        XCTAssertFalse(e?.message.contains("987654") ?? true)
        XCTAssertEqual(401, e?.status)
    }

    // lockoutsRaiseWorkerLockedEvents
    func testLockoutsRaiseWorkerLockedEvents() async {
        let s = h.session()
        p.on("/field/auth/login") { _ in self.p.error(429, "WORKER_LOCKED_TEMPORARILY", retryAfter: 30) }
        let e = await failsWith("WORKER_LOCKED_TEMPORARILY") { _ = try await s.login(LoginCredentials(workerCode: "W", pin: "1234")) }
        XCTAssertEqual(30, e?.retryAfterSeconds)
        XCTAssertTrue(h.events.contains(.workerLocked(hard: false, retryAfterSeconds: 30)))
        p.on("/field/auth/login") { _ in self.p.error(423, "WORKER_LOCKED") }
        await failsWith("WORKER_LOCKED") { _ = try await s.login(LoginCredentials(workerCode: "W", pin: "1234")) }
        XCTAssertTrue(h.events.contains(.workerLocked(hard: true, retryAfterSeconds: nil)))
        p.on("/field/auth/login") { _ in self.p.error(429, "TOO_MANY_ATTEMPTS", retryAfter: 60) }
        await failsWith("TOO_MANY_ATTEMPTS") { _ = try await s.login(LoginCredentials(workerCode: "W", pin: "1234")) }
        XCTAssertTrue(h.events.contains(.workerLocked(hard: false, retryAfterSeconds: 60)))
    }

    // pinResetRequiredIsTypedAndNotRetriedOrLocked
    func testPinResetRequiredIsTypedAndNotRetriedOrLocked() async {
        let s = h.session()
        p.on("/field/auth/login") { _ in self.p.error(403, "PIN_RESET_REQUIRED", message: "pin reset required") }
        let e = await failsWith("PIN_RESET_REQUIRED") { _ = try await s.login(LoginCredentials(workerCode: "W", pin: "1234")) }
        XCTAssertEqual(true, e?.isPinResetRequired)
        XCTAssertEqual(403, e?.status)
        XCTAssertEqual(1, p.count("/field/auth/login"))
        XCTAssertFalse(h.events.all.contains { if case .workerLocked = $0 { return true } else { return false } })
        XCTAssertFalse(h.events.all.contains { if case .sessionEnded = $0 { return true } else { return false } })
        XCTAssertEqual(.signedOut, s.state)
        XCTAssertFalse(FieldAuthError(code: "INVALID_CREDENTIALS").isPinResetRequired)
    }

    // actAsPinResetRequiredLeavesTheSupervisorSession
    func testActAsPinResetRequiredLeavesTheSupervisorSession() async throws {
        let s = h.session()
        let sup = try await supervisorLogin(s)
        p.on("/field/auth/act-as") { _ in self.p.error(403, "PIN_RESET_REQUIRED", message: "pin reset required") }
        let e = await failsWith("PIN_RESET_REQUIRED") { _ = try await s.actAs(ActAsTarget(workerCode: "W002", pin: "9999")) }
        XCTAssertEqual(true, e?.isPinResetRequired)
        XCTAssertEqual(1, p.count("/field/auth/act-as"))
        XCTAssertEqual(.signedIn(sup), s.state)
    }

    // pinPolicyViolationsAreReturned
    func testPinPolicyViolationsAreReturned() async throws {
        p.on("/field/auth/login", ok())
        p.on("/field/auth/pin") { _ in .json(422, #"{"error":"PIN_POLICY","message":"no","violations":["TOO_SHORT","SEQUENTIAL"]}"#) }
        let s = h.session()
        _ = try await s.login(LoginCredentials(workerCode: "W001", pin: "1234"))
        let e = await failsWith("PIN_POLICY") { try await s.changePin(current: "1234", new: "1111") }
        XCTAssertEqual(["TOO_SHORT", "SEQUENTIAL"], e?.violations)
    }

    // pinChangeRequiredEmitsEvent
    func testPinChangeRequiredEmitsEvent() async throws {
        p.on("/field/auth/login") { _ in self.p.token(pinChangeRequired: true) }
        let ses = try await h.session().login(LoginCredentials(workerCode: "W001", pin: "1234"))
        XCTAssertTrue(ses.pinChangeRequired)
        XCTAssertTrue(h.events.contains(.pinChangeRequired(ses)))
    }

    // networkFailureIsNetworkError
    func testNetworkFailureIsNetworkError() async {
        let s = h.session(h.config(platformUrl: "http://127.0.0.1:1"))
        await failsWith("NETWORK") { _ = try await s.login(LoginCredentials(workerCode: "W001", pin: "1234")) }
    }

    // malformedTokenResponseIsInternal
    func testMalformedTokenResponseIsInternal() async {
        p.on("/field/auth/login") { _ in .json(200, #"{"accessToken":"x"}"#) }
        await failsWith("INTERNAL") { _ = try await h.session().login(LoginCredentials(workerCode: "W001", pin: "1234")) }
    }

    // MARK: sign-in code

    private var adminActor: JObject { ["act": ["sub": "adm1", "typ": "principal", "name": "Ada Admin"] as JObject] }

    // signInCodeRedeemsWithActorAndNoSupervisor
    func testSignInCodeRedeemsWithActorAndNoSupervisor() async throws {
        p.on("/field/auth/sign-in-code", ok(adminActor))
        let s = h.session()
        let ses = try await s.redeemSignInCode(" abcd-1234\n")
        XCTAssertEqual("ABCD-1234", p.all("/field/auth/sign-in-code").first?.body?.optString("code"))
        XCTAssertEqual(Actor(id: "adm1", type: "principal", name: "Ada Admin"), ses.actor)
        XCTAssertNil(try h.storage.get(FieldSession.storeSupervisor))
        // nothing to return to: ending the impersonation just ends the session
        p.on("/field/auth/logout") { _ in FakeResponse(status: 204) }
        let back = try await s.endImpersonation()
        XCTAssertNil(back)
        XCTAssertEqual(.signedOut, s.state)
        XCTAssertEqual("session", p.all("/field/auth/logout").first?.body?.optString("scope"))
    }

    // emptySignInCodeIsRejected
    func testEmptySignInCodeIsRejected() async {
        await failsWith("INVALID_REQUEST") { _ = try await h.session().redeemSignInCode("  \n") }
    }

    // endImpersonationWithoutImpersonationFails
    func testEndImpersonationWithoutImpersonationFails() async throws {
        p.on("/field/auth/login", ok())
        let s = h.session()
        _ = try await s.login(LoginCredentials(workerCode: "W001", pin: "1234"))
        await failsWith("NOT_IMPERSONATING") { _ = try await s.endImpersonation() }
    }

    // MARK: act-as, supervisor kept aside

    private func supervisorLogin(_ s: FieldSession) async throws -> Session {
        p.on("/field/auth/login") { _ in self.p.token(workerId: "sup", workerCode: "S001") }
        p.on("/field/auth/act-as") { _ in
            self.p.token(workerId: "w2", workerCode: "W002", claims: ["act": ["sub": "sup", "typ": "field_worker", "name": "Name S001"] as JObject])
        }
        p.on("/field/auth/logout") { _ in FakeResponse(status: 204) }
        return try await s.login(LoginCredentials(workerCode: "S001", pin: "9999"))
    }

    // actAsKeepsSupervisorAsideAndEndImpersonationRestoresIt
    func testActAsKeepsSupervisorAsideAndEndImpersonationRestoresIt() async throws {
        let s = h.session()
        let sup = try await supervisorLogin(s)
        let worker = try await s.actAs(ActAsTarget(workerCode: "W002", pin: "9999"))
        XCTAssertEqual("W002", worker.worker.workerCode)
        XCTAssertEqual(Actor(id: "sup", type: "field_worker", name: "Name S001"), worker.actor)
        // act-as is a bearer call by the supervisor, bound to its access token
        let req = try XCTUnwrap(p.all("/field/auth/act-as").first)
        let bearer = try XCTUnwrap(req.bearer)
        XCTAssertEqual(sha256b64u(bearer), req.proof?.claims.optString("ath"))
        XCTAssertEqual("W002", req.body?.optString("workerCode"))
        XCTAssertFalse(try XCTUnwrap(req.body).has("workerId"))
        // the supervisor is kept aside in storage, the active refresh token is the worker's
        XCTAssertNotNil(try h.storage.get(FieldSession.storeSupervisor))
        XCTAssertEqual("fwr_1", try h.storage.get(FieldSession.storeSupervisorRefresh))
        XCTAssertEqual("fwr_2", try h.storage.get(FieldSession.storeRefresh))
        // a second act-as is refused
        await failsWith("IMPERSONATING") { _ = try await s.actAs(ActAsTarget(workerId: "w3", pin: "9999")) }
        await failsWith("IMPERSONATING") { try await s.changePin(current: "1", new: "2") }

        let back = try await s.endImpersonation()
        XCTAssertEqual(sup, back)
        // the worker's family was logged out with the worker's token
        XCTAssertEqual("fwr_2", p.all("/field/auth/logout").first?.body?.optString("refreshToken"))
        // the supervisor's access token was still valid: no refresh
        XCTAssertEqual(0, p.count("/field/auth/refresh"))
        XCTAssertEqual("fwr_1", try h.storage.get(FieldSession.storeRefresh))
        XCTAssertNil(try h.storage.get(FieldSession.storeSupervisor))
        XCTAssertNil(try h.storage.get(FieldSession.storeSupervisorRefresh))
        XCTAssertEqual(.signedIn(sup), s.state)
    }

    // actAsByWorkerIdAndValidation
    func testActAsByWorkerIdAndValidation() async throws {
        let s = h.session()
        _ = try await supervisorLogin(s)
        await failsWith("INVALID_REQUEST") { _ = try await s.actAs(ActAsTarget(pin: "9999")) }
        await failsWith("INVALID_REQUEST") { _ = try await s.actAs(ActAsTarget(workerCode: "a", workerId: "b", pin: "9999")) }
        _ = try await s.actAs(ActAsTarget(workerId: "w2", pin: "9999"))
        let body = try XCTUnwrap(p.all("/field/auth/act-as").first?.body)
        XCTAssertEqual("w2", body.optString("workerId"))
        XCTAssertFalse(body.has("workerCode"))
    }

    // endImpersonationRefreshesALapsedSupervisorToken
    func testEndImpersonationRefreshesALapsedSupervisorToken() async throws {
        let s = h.session()
        _ = try await supervisorLogin(s)
        p.on("/field/auth/refresh") { _ in self.p.token(workerId: "sup", workerCode: "S001") }
        _ = try await s.actAs(ActAsTarget(workerCode: "W002", pin: "9999"))
        h.clock.jump(11 * MIN) // the supervisor's access token (10 min) has lapsed
        let back = try await XCTUnwrapAsync(try await s.endImpersonation())
        XCTAssertEqual("S001", back.worker.workerCode)
        XCTAssertEqual("fwr_1", p.all("/field/auth/refresh").first?.body?.optString("refreshToken"))
        XCTAssertEqual(1, p.count("/field/auth/refresh"))
        XCTAssertGreaterThan(back.expiresAt, h.clock.now())
    }

    // supervisorSurvivesProcessRestart
    func testSupervisorSurvivesProcessRestart() async throws {
        let s = h.session()
        let sup = try await supervisorLogin(s)
        _ = try await s.actAs(ActAsTarget(workerCode: "W002", pin: "9999"))
        // a "new process": fresh session over the same storage
        let s2 = h.session()
        let st = try await s2.restore()
        guard case .signedIn(let restored) = st else { return XCTFail("expected signedIn, got \(st)") }
        XCTAssertEqual("W002", restored.worker.workerCode)
        XCTAssertNotNil(restored.actor)
        await failsWith("IMPERSONATING") { _ = try await s2.actAs(ActAsTarget(workerCode: "W003", pin: "9999")) }
        p.on("/field/auth/refresh") { _ in self.p.token(workerId: "sup", workerCode: "S001") }
        let back = try await XCTUnwrapAsync(try await s2.endImpersonation())
        XCTAssertEqual(sup.worker, back.worker)
        // the worker was logged out with the restored worker token, the supervisor refreshed with its stored one
        XCTAssertEqual("fwr_2", p.all("/field/auth/logout").first?.body?.optString("refreshToken"))
        XCTAssertEqual("fwr_1", p.all("/field/auth/refresh").first?.body?.optString("refreshToken"))
        XCTAssertNil(try h.storage.get(FieldSession.storeSupervisor))
    }

    // lostSupervisorTokenFailsBeforeTearingDown
    func testLostSupervisorTokenFailsBeforeTearingDown() async throws {
        let s = h.session()
        _ = try await supervisorLogin(s)
        _ = try await s.actAs(ActAsTarget(workerCode: "W002", pin: "9999"))
        let s2 = h.session()
        _ = try await s2.restore()
        try h.storage.remove(FieldSession.storeSupervisorRefresh)
        await failsWith("SESSION_EXPIRED") { _ = try await s2.endImpersonation() }
        XCTAssertEqual(0, p.count("/field/auth/logout"))
        if case .signedIn = s2.state {} else { XCTFail("expected signedIn") }
    }

    // impersonationEndingOnItsOwnHandsBackToSupervisor
    func testImpersonationEndingOnItsOwnHandsBackToSupervisor() async throws {
        let s = h.session()
        let sup = try await supervisorLogin(s)
        _ = try await s.actAs(ActAsTarget(workerCode: "W002", pin: "9999"))
        p.on("/field/auth/refresh") { _ in self.p.error(401, "SESSION_EXPIRED") }
        h.clock.jump(11 * MIN)
        await failsWith("SESSION_EXPIRED") { _ = try await s.refresh() }
        XCTAssertEqual(.signedIn(sup), s.state)
        XCTAssertTrue(h.events.contains(.sessionEnded("SESSION_EXPIRED")))
        XCTAssertEqual("fwr_1", try h.storage.get(FieldSession.storeRefresh))
    }

    // MARK: PIN change, logout

    // changePinIsABearerCallThenRefreshes
    func testChangePinIsABearerCallThenRefreshes() async throws {
        p.on("/field/auth/login", ok())
        p.on("/field/auth/pin") { _ in FakeResponse(status: 204) }
        p.on("/field/auth/refresh") { _ in self.p.token() }
        let s = h.session()
        _ = try await s.login(LoginCredentials(workerCode: "W001", pin: "1234"))
        try await s.changePin(current: "1234", new: "8642")
        let req = try XCTUnwrap(p.all("/field/auth/pin").first)
        XCTAssertEqual("1234", req.body?.optString("currentPin"))
        XCTAssertEqual("8642", req.body?.optString("newPin"))
        XCTAssertTrue(try XCTUnwrap(req.proof).claims.has("ath"))
        XCTAssertEqual(1, p.count("/field/auth/refresh"))
    }

    // logoutSendsScopeAndClearsEverything
    func testLogoutSendsScopeAndClearsEverything() async throws {
        p.on("/field/auth/login", ok())
        p.on("/field/auth/logout") { _ in FakeResponse(status: 204) }
        let s = h.session()
        _ = try await s.login(LoginCredentials(workerCode: "W001", pin: "1234"))
        try await s.logout()
        let body = try XCTUnwrap(p.all("/field/auth/logout").first?.body)
        XCTAssertEqual("fwr_1", body.optString("refreshToken"))
        XCTAssertEqual("session", body.optString("scope"))
        XCTAssertEqual(.signedOut, s.state)
        XCTAssertNil(try h.storage.get(FieldSession.storeRefresh))
        XCTAssertNil(try h.storage.get(FieldSession.storeSession))
        XCTAssertTrue(h.events.contains(.sessionEnded("logout")))
        XCTAssertTrue(h.events.contains(.sessionChanged(nil)))
    }

    // enrolledDevice: nil when fresh, the record after enrol, the same record from a new session over the same
    // storage, and still the record after a device-all logout (the device stays enrolled)
    func testEnrolledDeviceAccessor() async throws {
        p.on("/field/auth/login", ok())
        enrolOkFor(mode: "SHARED")
        p.on("/field/auth/logout") { _ in FakeResponse(status: 204) }
        let s = h.session()
        let fresh = try await s.enrolledDevice()
        XCTAssertNil(fresh)
        let rec = try await s.enrol("ABCDE12345", info: DeviceInfo())
        let afterEnrol = try await s.enrolledDevice()
        XCTAssertEqual(rec, afterEnrol)
        XCTAssertEqual(DeviceRecord(deviceId: "dev1", mode: "SHARED", clientId: "c1", appCodes: [], platformUrl: p.url), rec)

        // a new owner over the same storage reads it back without any network call
        let before = p.requests.count
        let reopened = h.session(storage: h.storage)
        let restored = try await reopened.enrolledDevice()
        XCTAssertEqual(rec, restored)
        XCTAssertEqual(before, p.requests.count)

        _ = try await s.login(LoginCredentials(workerCode: "W001", pin: "1234"))
        try await s.logout(scope: .deviceAll)
        XCTAssertEqual(.signedOut, s.state)
        let afterLogout = try await s.enrolledDevice()
        XCTAssertEqual(rec, afterLogout)
        let afterLogoutStored = try await h.session(storage: h.storage).enrolledDevice()
        XCTAssertEqual(rec, afterLogoutStored)
    }

    // deviceAllLogoutNeedsEnrolmentAndUsesWireScope
    func testDeviceAllLogoutNeedsEnrolmentAndUsesWireScope() async throws {
        p.on("/field/auth/login", ok())
        enrolOkFor(mode: "SHARED")
        p.on("/field/auth/logout") { _ in FakeResponse(status: 204) }
        let s = h.session()
        _ = try await s.login(LoginCredentials(workerCode: "W001", pin: "1234"))
        await failsWith("INVALID_REQUEST") { try await s.logout(scope: .deviceAll) }
        _ = try await s.enrol("ABCDE12345", info: DeviceInfo())
        try await s.logout(scope: .deviceAll)
        XCTAssertEqual("device-all", p.all("/field/auth/logout").first?.body?.optString("scope"))
    }

    // logoutClearsLocalStateEvenWhenThePlatformIsDown
    func testLogoutClearsLocalStateEvenWhenThePlatformIsDown() async throws {
        p.on("/field/auth/login", ok())
        let s = h.session()
        _ = try await s.login(LoginCredentials(workerCode: "W001", pin: "1234"))
        p.on("/field/auth/logout") { _ in self.p.error(500, "INTERNAL") }
        await failsWith("INTERNAL") { try await s.logout() }
        XCTAssertEqual(.signedOut, s.state)
        XCTAssertNil(try h.storage.get(FieldSession.storeRefresh))
    }

    // MARK: refresh, storage

    // refreshRotatesAndPersistsTheToken
    func testRefreshRotatesAndPersistsTheToken() async throws {
        p.on("/field/auth/login", ok())
        p.on("/field/auth/refresh") { _ in self.p.token() }
        let s = h.session()
        _ = try await s.login(LoginCredentials(workerCode: "W001", pin: "1234"))
        h.clock.jump(5 * MIN)
        _ = try await s.refresh()
        _ = try await s.refresh()
        XCTAssertEqual("fwr_1", p.all("/field/auth/refresh")[0].body?.optString("refreshToken"))
        XCTAssertEqual("fwr_2", p.all("/field/auth/refresh")[1].body?.optString("refreshToken"))
        XCTAssertEqual("fwr_3", try h.storage.get(FieldSession.storeRefresh))
    }

    // storageRoundTripRestoresSessionAndRefreshesFromStoredToken
    func testStorageRoundTripRestoresSessionAndRefreshesFromStoredToken() async throws {
        var claims = p.deviceClaims("PERSONAL", id: "dev1")
        claims["test"] = true
        p.on("/field/auth/login", ok(claims))
        p.on("/field/auth/refresh") { _ in self.p.token(claims: claims) }
        let s = h.session()
        let ses = try await s.login(LoginCredentials(workerCode: "W001", pin: "1234"))
        XCTAssertTrue(ses.test)
        let s2 = h.session()
        XCTAssertEqual(.signedOut, s2.state) // not loaded yet
        let restored = try await s2.restore()
        XCTAssertEqual(.signedIn(ses), restored)
        // the access token is not persisted: the first use refreshes with the stored token
        let headers = try await s2.headersFor(method: "GET", url: "\(p.url)/x")
        XCTAssertNotNil(headers["DPoP"])
        XCTAssertEqual("fwr_1", p.all("/field/auth/refresh").first?.body?.optString("refreshToken"))
        XCTAssertEqual("fwr_2", try h.storage.get(FieldSession.storeRefresh))
    }

    // nothingSensitiveInPlainSessionJson
    func testNothingSensitiveInPlainSessionJson() async throws {
        p.on("/field/auth/login", ok())
        _ = try await h.session().login(LoginCredentials(workerCode: "W001", pin: "1234"))
        let all = try h.storage.keys.map { try h.storage.get($0)! }.joined(separator: ",")
        XCTAssertFalse(all.contains("1234"))
        XCTAssertFalse(try XCTUnwrap(h.storage.get(FieldSession.storeSession)).contains("fwr_"))
    }

    // refreshWithoutAnySessionFails
    func testRefreshWithoutAnySessionFails() async {
        await failsWith("SESSION_EXPIRED") { _ = try await h.session().refresh() }
        await failsWith("SESSION_EXPIRED") { _ = try await h.session().headersFor(method: "GET", url: "\(p.url)/x") }
        XCTAssertTrue(h.events.isEmpty, "nothing to end, nothing to announce")
    }

    // MARK: Swift additions (not in Kotlin)

    // The Kotlin library never restores the QR-adopted platform URL after a restart (field-js does: auth.ts load()).
    func testQrAdoptedPlatformUrlSurvivesARestart() async throws {
        enrolOkFor()
        p.on("/field/auth/login", ok())
        let s = h.session(h.config(platformUrl: ""))
        _ = try await s.enrol("fcfield:1:\(p.url):ABCDE12345", info: DeviceInfo())
        let s2 = h.session(h.config(platformUrl: ""))
        _ = try await s2.restore()
        let ses = try await s2.login(LoginCredentials(workerCode: "W001", pin: "1234"))
        XCTAssertEqual("W001", ses.worker.workerCode)
    }

    // The stored values are the same JSON field-js and the Android library write, so a session survives a switch of library.
    func testRestoresSessionAndDeviceWrittenByAnotherLibrary() async throws {
        try h.storage.put(FieldSession.storeDevice, value: #"{"deviceId":"dev9","mode":"PERSONAL","clientId":"c1","appCodes":["app1"],"platformUrl":"https://p.example.com"}"#)
        try h.storage.put(FieldSession.storeSession, value: #"{"worker":{"id":"w1","workerCode":"W001","name":"Ann"},"actor":null,"test":false,"deviceMode":"PERSONAL","deviceId":null,"appCode":"app1","expiresAt":1800000600000,"idleExpiresAt":1800001800000,"refreshExpiresAt":1800003600000,"pinChangeRequired":false}"#)
        let st = try await h.session().restore()
        guard case .signedIn(let ses) = st else { return XCTFail("\(st)") }
        XCTAssertEqual("dev9", ses.deviceId, "a null deviceId falls back to the device record")
        XCTAssertEqual(1_800_003_600_000, ses.refreshExpiresAt)
        // and the other direction: what we write parses as that JSON
        p.on("/field/auth/login", ok())
        let s = h.session()
        _ = try await s.login(LoginCredentials(workerCode: "W001", pin: "1234"))
        let j = try XCTUnwrap(JSON.parseObject(try XCTUnwrap(h.storage.get(FieldSession.storeSession))))
        for k in ["worker", "actor", "test", "deviceMode", "deviceId", "appCode", "expiresAt", "idleExpiresAt", "refreshExpiresAt", "pinChangeRequired"] {
            XCTAssertTrue(j.has(k), k)
        }
        XCTAssertTrue(j.isNull("actor"))
        XCTAssertTrue(j.isNull("deviceId"))
    }

    func testNormaliseBadgeToken() {
        XCTAssertEqual("AB-12", FieldSession.normaliseBadgeToken("  ab-12\r\n"))
        XCTAssertEqual("ABCD1234", FieldSession.normaliseBadgeToken("ab\tcd\u{0}12\u{7f}34"))
    }

    func testTimestampParsingAndFormattingRoundTrip() {
        XCTAssertEqual(1_800_000_000_000, parseOffsetDateTimeMillis("2027-01-15T08:00:00Z"))
        XCTAssertEqual(1_800_000_000_123, parseOffsetDateTimeMillis("2027-01-15T08:00:00.123456789Z"))
        XCTAssertEqual(1_800_000_000_000, parseOffsetDateTimeMillis("2027-01-15T10:00:00+02:00"))
        XCTAssertEqual(1_800_000_000_000, parseOffsetDateTimeMillis("2027-01-15T07:30:00-00:30"))
        XCTAssertEqual(0, parseOffsetDateTimeMillis("1970-01-01T00:00Z"))
        XCTAssertNil(parseOffsetDateTimeMillis(""))
        XCTAssertNil(parseOffsetDateTimeMillis("2027-01-15 08:00:00Z"))
        XCTAssertNil(parseOffsetDateTimeMillis("2027-13-15T08:00:00Z"))
        for ms: Int64 in [0, 1, 999, 1_800_000_000_000, 1_800_000_000_123, 4_102_444_800_000] {
            XCTAssertEqual(ms, parseOffsetDateTimeMillis(formatInstant(ms)))
        }
    }
}

/// `XCTUnwrap` for an async expression.
func XCTUnwrapAsync<T>(_ e: @autoclosure () async throws -> T?, file: StaticString = #filePath, line: UInt = #line) async throws -> T {
    let v = try await e()
    return try XCTUnwrap(v, file: file, line: line)
}
