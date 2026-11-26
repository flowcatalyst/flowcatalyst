import Foundation
import XCTest
@testable import FlowCatalystFieldCore

/// Unlock keys (docs/field-unlock-keys-spec.md section 12) against a fake platform that verifies the key proof and the
/// assertion as the real one does, with software keys behind the Secure Enclave's interface and a fake prompt.
/// Port of UnlockKeyTest.kt; the iOS differences (no attestation, the ATTESTATION_REQUIRED wording) are tested too.
final class UnlockKeyTests: XCTestCase {
    var h: Harness!
    var keys: SoftUnlockKeys!
    var p: FakePlatform { h.platform }
    let prompts = Locked<Int>(0)
    let prompt = UnlockKeyPrompt(title: "Sign in", cancel: "Use PIN")

    // What the fake platform holds and how it answers.
    let registeredJwk = Locked<JObject?>(nil)
    let registerAnswer = Locked<((Rec) -> FakeResponse)?>(nil)
    let emptyRegistrations = Locked<Int>(0)
    /// What the next login and refresh answer carry as `unlockKeys` (nil: a platform without unlock keys).
    let loginKeys = Locked<JObject?>(nil)
    let refreshKeys = Locked<JObject?>(nil)

    /// The token answer's `unlockKeys` (spec 2.3).
    func keys(_ window: Int64?, allowed: Bool = true, attestation: Bool = false) -> JObject {
        var k: JObject = ["allowed": allowed, "attestationRequired": attestation]
        if let window { k["registerExpiresIn"] = window }
        return k
    }
    let challengeAnswer = Locked<(() -> FakeResponse)?>(nil)
    let unlockAnswer = Locked<((Rec) -> FakeResponse)?>(nil)
    let revokeAnswer = Locked<(() -> FakeResponse)?>(nil)
    let issued = Locked<[String]>([])
    let revoked = Locked<[String]>([])
    let loginClaims = Locked<JObject>([:])
    let loginWorker = Locked<(String, String)>(("w1", "W001"))

    var finger: FakeUnlockPrompter { FakeUnlockPrompter(shown: prompts) { _, sig, data in try sig.sign(data) } }
    func refusing(_ code: String) -> FakeUnlockPrompter {
        FakeUnlockPrompter(shown: prompts) { _, _, _ in throw FieldAuthError(code: code) }
    }

    func claims(_ amr: [String], _ sid: String, worker: String = "w1") -> JObject {
        var c = p.deviceClaims("PERSONAL", id: "dev1")
        c["client_id"] = "clt_1"
        c["client_identifier"] = "acme"
        c["sub"] = worker
        c["sid"] = sid
        c["amr"] = amr
        return c
    }

    override func setUp() async throws {
        loginKeys.value = keys(600)
        refreshKeys.value = keys(600)
        keys = SoftUnlockKeys()
        h = Harness(keyLevel: "TEE", unlockKeys: keys)
        install()
    }

    override func tearDown() async throws {
        XCTAssertEqual(0, emptyRegistrations.value, "a registration without a key was sent")
        XCTAssertEqual(0, p.all("/field/auth/unlock-keys").filter { $0.body?.has("keyProof") != true }.count)
        h.close()
    }

    func install() {
        loginClaims.value = claims(["pin"], "fam1")
        p.on("/field/devices/enrol") { _ in .json(201, #"{"deviceId":"dev1","mode":"PERSONAL","clientId":"clt_1","appCodes":["app1"]}"#) }
        p.on("/field/auth/login") { [self] _ in
            p.token(workerId: loginWorker.value.0, workerCode: loginWorker.value.1, claims: loginClaims.value, unlockKeys: loginKeys.value)
        }
        p.on("/field/auth/refresh") { [self] _ in p.token(claims: loginClaims.value, unlockKeys: refreshKeys.value) }
        p.on("/field/auth/logout") { _ in FakeResponse(status: 204) }
        p.on("/field/auth/unlock-keys") { [self] r in
            if r.body == nil || r.body!.isEmpty {
                emptyRegistrations.mutate { $0 += 1 } // never: a library must not probe (spec 12.1)
                return p.error(400, "INVALID_REQUEST", message: "publicKeyJwk and keyProof are required")
            }
            return registerAnswer.value?(r) ?? register(r)
        }
        p.on("/field/auth/unlock/challenge") { [self] _ in
            if let a = challengeAnswer.value { return a() }
            let c = issued.mutate { $0.append("challenge-\($0.count + 1)"); return $0.last! }
            return .json(200, JSON.string(["challenge": c, "expiresAt": "2030-01-01T00:00:00Z"]))
        }
        p.on("/field/auth/unlock") { [self] r in unlockAnswer.value?(r) ?? unlock(r) }
        p.on("/field/auth/unlock-keys/revoke") { [self] r in
            if let a = revokeAnswer.value { return a() }
            revoked.mutate { $0.append(r.body!.optString("unlockKeyId")) }
            return FakeResponse(status: 204)
        }
    }

    /// The platform's registration: the key proof verifies with publicKeyJwk and signs this request's DPoP nonce.
    func register(_ r: Rec) -> FakeResponse {
        let body = r.body!
        let jwk = body.optObject("publicKeyJwk")!
        let proof = body.optString("keyProof")
        let hd = jwsHeader(proof)
        let c = jwsClaims(proof)
        let ok = verifyEs256(proof, x: jwk.optString("x"), y: jwk.optString("y"))
            && Set(hd.keys) == ["typ", "alg"] && hd.optString("typ") == "fc-unlock-key+jwt"
            && c.optString("aud") == p.url + "/field/auth/unlock-keys" && c.optString("nonce") == r.proof!.nonce
            && c.optString("sub") == "w1" && c.optString("device_id") == "dev1" && r.bearer != nil
        if !ok { return p.error(401, "INVALID_UNLOCK_PROOF") }
        registeredJwk.value = jwk
        return .json(201, JSON.string([
            "unlockKeyId": "fwk_1", "deviceId": "dev1", "workerId": "w1", "createdAt": "2027-01-15T08:00:00Z",
            "pinRequiredBy": "2027-02-14T08:00:00Z", "attestation": ["format": "NONE", "verdict": "NONE"] as JObject,
        ]))
    }

    /// The platform's unlock: the assertion verifies with the registered key and signs the last challenge.
    func unlock(_ r: Rec) -> FakeResponse {
        let body = r.body!
        guard let jwk = registeredJwk.value else { return p.error(401, "UNLOCK_KEY_INVALID") }
        let a = body.optString("assertion")
        let hd = jwsHeader(a)
        let c = jwsClaims(a)
        let ok = verifyEs256(a, x: jwk.optString("x"), y: jwk.optString("y"))
            && hd.optString("typ") == "fc-unlock+jwt" && hd.optString("kid") == body.optString("unlockKeyId")
            && c.optString("aud") == p.url + "/field/auth/unlock" && c.optString("nonce") == issued.value.last
            && c.optString("sub") == "w1" && c.optString("device_id") == "dev1" && c.optString("app") == "app1"
            && body.optString("appCode") == "app1" && r.bearer == nil
        if !ok { return p.error(401, "INVALID_UNLOCK_PROOF") }
        return p.token(claims: claims(["hwk", "uv"], "fam2"), unlockKeys: keys(nil))
    }

    func signedIn() async throws -> FieldSession {
        let s = h.session()
        _ = try await s.enrol("ABCDE12345", info: DeviceInfo())
        _ = try await s.login(LoginCredentials(workerCode: "W001", pin: "1234"))
        return s
    }

    @discardableResult
    func enabled() async throws -> FieldSession {
        let s = try await signedIn()
        _ = try await s.enableUnlockKey(using: finger, prompt: prompt)
        return s
    }

    func login(_ s: FieldSession, _ sid: String) async throws {
        loginClaims.value = claims(["pin"], sid)
        _ = try await s.login(LoginCredentials(workerCode: "W001", pin: "1234"))
    }

    func keyChanged(_ e: FieldEvent, _ key: UnlockKeyInfo?) -> Bool {
        if case .unlockKeyChanged(let k) = e { return k == key } else { return false }
    }

    // MARK: availability

    func testAvailableRightAfterAPinSignInOnAnEnrolledPersonalPhone() async throws {
        let s = try await signedIn()
        let a = try await s.unlockKeyAvailability()
        XCTAssertEqual(.available, a)
    }

    func testNotOfferedOnAnUnenrolledPhoneOrWithNobodySignedIn() async throws {
        let s = h.session()
        var a = try await s.unlockKeyAvailability()
        XCTAssertEqual(.notEnrolled, a)
        _ = try await s.enrol("ABCDE12345", info: DeviceInfo())
        a = try await s.unlockKeyAvailability()
        XCTAssertEqual(.notSignedIn, a)
    }

    func testNeverOnASharedPhone() async throws {
        p.on("/field/devices/enrol") { _ in .json(201, #"{"deviceId":"dev1","mode":"SHARED","clientId":"c","appCodes":[]}"#) }
        var c = claims(["pin"], "f")
        c["device_mode"] = "SHARED"
        loginClaims.value = c
        let s = try await signedIn()
        let a = try await s.unlockKeyAvailability()
        XCTAssertEqual(.sharedDevice, a)
        await failsWith("SHARED_DEVICE") { _ = try await s.enableUnlockKey(using: self.finger, prompt: self.prompt) }
    }

    func signedInSession(_ s: FieldSession) async -> Session? {
        if case .signedIn(let ses) = s.state { return ses }
        return nil
    }

    func testTheRegisterWindowComesFromTheTokenAnswerAndRunsOut() async throws {
        let s = try await signedIn()
        let ses = await signedInSession(s)
        XCTAssertEqual(h.clock.now() + 600_000, ses?.unlockKeys?.registerExpiresAt)
        h.clock.jump(9 * MIN + 59_000)
        var a = try await s.unlockKeyAvailability()
        XCTAssertEqual(.available, a)
        h.clock.jump(1_000)
        a = try await s.unlockKeyAvailability()
        XCTAssertEqual(.pinSignInRequired, a)
        let before = p.requests.count
        await failsWith("PIN_SIGN_IN_REQUIRED") { _ = try await s.enableUnlockKey(using: self.finger, prompt: self.prompt) }
        XCTAssertEqual(0, prompts.value)
        XCTAssertEqual(before, p.requests.count)
        XCTAssertTrue(keys.aliases.isEmpty)
    }

    func testAnAnswerWithoutAWindowIsNoOffer() async throws {
        let s = try await signedIn()
        loginKeys.value = keys(nil) // a badge alone: allowed, no window
        loginClaims.value = claims(["badge"], "fam3")
        _ = try await s.login(LoginCredentials(badgeToken: "B1"))
        var a = try await s.unlockKeyAvailability()
        XCTAssertEqual(.pinSignInRequired, a)
        // a PIN change pending: the platform sends no window either, and the library checks it too
        loginKeys.value = keys(600)
        p.on("/field/auth/login") { [self] _ in p.token(claims: claims(["pin"], "fam5"), pinChangeRequired: true, unlockKeys: keys(600)) }
        _ = try await s.login(LoginCredentials(workerCode: "W001", pin: "1234"))
        a = try await s.unlockKeyAvailability()
        XCTAssertEqual(.pinSignInRequired, a)
        await failsWith("PIN_SIGN_IN_REQUIRED") { _ = try await s.enableUnlockKey(using: self.finger, prompt: self.prompt) }
        XCTAssertEqual(0, prompts.value)
    }

    func testNotAllowedOrAbsentIsPolicyOffAndEnablingSendsNothing() async throws {
        for answer in [keys(nil, allowed: false), nil] {
            loginKeys.value = answer
            let s = try await signedIn()
            let a = try await s.unlockKeyAvailability()
            XCTAssertEqual(.policyOff, a)
            let ses = await signedInSession(s)
            XCTAssertEqual(answer == nil ? nil : SessionUnlockKeys(allowed: false, attestationRequired: false), ses?.unlockKeys)
            let before = p.requests.count
            await failsWith("UNLOCK_KEYS_DISABLED") { _ = try await s.enableUnlockKey(using: self.finger, prompt: self.prompt) }
            XCTAssertEqual(before, p.requests.count)
            XCTAssertEqual(0, prompts.value)
            XCTAssertTrue(keys.aliases.isEmpty)
        }
    }

    func testARefreshsAnswerReplacesThePreviousOne() async throws {
        let s = try await signedIn()
        var a = try await s.unlockKeyAvailability()
        XCTAssertEqual(.available, a)
        refreshKeys.value = keys(nil, allowed: false) // the policy switched off since
        _ = try await s.refresh()
        a = try await s.unlockKeyAvailability()
        XCTAssertEqual(.policyOff, a)
        refreshKeys.value = keys(120) // on again; the refresh carries what is left of the window
        _ = try await s.refresh()
        a = try await s.unlockKeyAvailability()
        XCTAssertEqual(.available, a)
        h.clock.jump(2 * MIN)
        a = try await s.unlockKeyAvailability()
        XCTAssertEqual(.pinSignInRequired, a)
        refreshKeys.value = keys(nil)
        _ = try await s.refresh()
        a = try await s.unlockKeyAvailability()
        XCTAssertEqual(.pinSignInRequired, a)
    }

    func testARestartWaitsForTheNextRefreshForTheWindow() async throws {
        _ = try await signedIn()
        let s = h.session()
        guard case .signedIn(let restored) = try await s.restore() else { return XCTFail("not signed in") }
        XCTAssertEqual(SessionUnlockKeys(allowed: true, attestationRequired: false), restored.unlockKeys)
        var a = try await s.unlockKeyAvailability()
        XCTAssertEqual(.pinSignInRequired, a)
        refreshKeys.value = keys(300)
        _ = try await s.refresh()
        a = try await s.unlockKeyAvailability()
        XCTAssertEqual(.available, a)
    }

    func testAttestationRequiredIsNeverOfferedOnIOS() async throws {
        loginKeys.value = keys(600, attestation: true)
        let s = try await signedIn()
        let a = try await s.unlockKeyAvailability()
        XCTAssertEqual(.attestationRequired, a)
        let before = p.requests.count
        let e = await failsWith("ATTESTATION_REQUIRED") { _ = try await s.enableUnlockKey(using: self.finger, prompt: self.prompt) }
        XCTAssertTrue(e?.message.contains("iOS cannot attest an unlock key") == true)
        XCTAssertEqual(before, p.requests.count)
        XCTAssertEqual(0, prompts.value)
        XCTAssertTrue(keys.aliases.isEmpty)
    }

    func testAttestationRequiredOnAProviderThatCanAttest() async throws {
        // The shared rule (the Android case): a provider that attests and a device key with a chain may offer it.
        loginKeys.value = keys(600, attestation: true)
        h.close()
        h = Harness(keyLevel: "TEE", keyChain: ["ZGV2", "cm9vdA=="], unlockKeys: keys)
        install()
        loginKeys.value = keys(600, attestation: true)
        keys.canAttest.value = true
        let s = try await signedIn()
        var a = try await s.unlockKeyAvailability()
        XCTAssertEqual(.available, a)
        keys.canAttest.value = false
        a = try await s.unlockKeyAvailability()
        XCTAssertEqual(.attestationRequired, a)
    }

    func testEnablingSendsExactlyTheRegistration() async throws {
        let s = try await signedIn()
        let before = p.requests.count
        _ = try await s.enableUnlockKey(using: finger, prompt: prompt)
        let sent = p.requests.dropFirst(before).map(\.path).filter { $0 != "/field/auth/nonce" }
        XCTAssertEqual(["/field/auth/unlock-keys"], Array(sent))
        XCTAssertEqual(true, p.all("/field/auth/unlock-keys").first?.body?.has("keyProof"))
        XCTAssertEqual(1, p.count("/field/auth/unlock-keys"))
    }

    func testNeverForAnImpersonatedSession() async throws {
        let s = h.session()
        _ = try await s.enrol("ABCDE12345", info: DeviceInfo())
        var c = claims(["code", "imp"], "fam9")
        c["act"] = ["sub": "adm", "typ": "principal", "name": "A"] as JObject
        p.on("/field/auth/sign-in-code") { [self] _ in p.token(claims: c) }
        _ = try await s.redeemSignInCode("CODE1234")
        let a = try await s.unlockKeyAvailability()
        XCTAssertEqual(.impersonating, a)
        await failsWith("IMPERSONATING") { _ = try await s.enableUnlockKey(using: self.finger, prompt: self.prompt) }
    }

    func testThePhonesOwnLimits() async throws {
        let s = try await signedIn()
        for (status, code) in [(UnlockKeyAvailability.biometricNotEnrolled, "BIOMETRIC_NOT_ENROLLED"),
                               (.biometricUnavailable, "BIOMETRIC_UNAVAILABLE"),
                               (.noSecureHardware, "UNLOCK_KEYS_UNSUPPORTED")] {
            keys.biometric.value = status
            let a = try await s.unlockKeyAvailability()
            XCTAssertEqual(status, a)
            await failsWith(code) { _ = try await s.enableUnlockKey(using: self.finger, prompt: self.prompt) }
        }
        XCTAssertEqual(0, prompts.value)
        XCTAssertTrue(keys.aliases.isEmpty)
    }

    func testASimulatorDeviceKeyIsNotOffered() async throws {
        h.close()
        h = Harness(keyLevel: "SOFTWARE", unlockKeys: keys)
        install()
        let a = try await signedIn().unlockKeyAvailability()
        XCTAssertEqual(.noSecureHardware, a)
    }

    func testAKeyOutsideSecureHardwareIsDeletedBeforeAnyPrompt() async throws {
        let s = try await signedIn()
        keys.level.value = "SOFTWARE"
        await failsWith("UNLOCK_KEYS_UNSUPPORTED") { _ = try await s.enableUnlockKey(using: self.finger, prompt: self.prompt) }
        XCTAssertEqual(0, prompts.value)
        XCTAssertTrue(keys.aliases.isEmpty)
    }

    func testWithoutAProviderThereAreNoUnlockKeys() async throws {
        h.close()
        h = Harness(keyLevel: "TEE")
        install()
        let s = try await signedIn()
        let a = try await s.unlockKeyAvailability()
        XCTAssertEqual(.unsupported, a)
        await failsWith("UNLOCK_KEYS_UNSUPPORTED") { _ = try await s.enableUnlockKey(using: self.finger, prompt: self.prompt) }
        // and the convenience forms, with no prompter configured
        await failsWith("UNLOCK_KEYS_UNSUPPORTED") { _ = try await s.enableUnlockKey(prompt: self.prompt) }
        await failsWith("UNLOCK_KEYS_UNSUPPORTED") { _ = try await s.loginWithUnlockKey(prompt: self.prompt) }
    }

    // MARK: enabling

    func testEnablingMakesAKeyAndRegistersItWithOnePromptAndNoAttestation() async throws {
        let s = try await signedIn()
        let before = h.events.all.count
        let info = try await s.enableUnlockKey(using: finger, prompt: prompt)
        XCTAssertEqual(1, prompts.value)
        XCTAssertEqual("fwk_1", info.unlockKeyId)
        XCTAssertEqual("w1", info.workerId)
        XCTAssertEqual("W001", info.workerCode)
        XCTAssertEqual("dev1", info.deviceId)
        XCTAssertEqual("NONE", info.attestationVerdict)
        XCTAssertEqual(parseOffsetDateTimeMillis("2027-02-14T08:00:00Z"), info.pinRequiredBy)

        let reg = p.all("/field/auth/unlock-keys").last { $0.body?.has("keyProof") == true }!
        XCTAssertNil(reg.body!["attestationChain"]) // iOS cannot attest an unlock key (spec 6.1)
        XCTAssertEqual(Set(["kty", "crv", "x", "y"]), Set(reg.body!.optObject("publicKeyJwk")!.keys))
        XCTAssertNotEqual(reg.body!.optObject("publicKeyJwk")!.optString("x"), reg.proof!.jwk.optString("x")) // not the device key

        var a = try await s.unlockKeyAvailability()
        XCTAssertEqual(.enabled, a)
        let k1 = try await s.unlockKey()
        XCTAssertEqual(info, k1)
        let k2 = try await h.session().unlockKey()
        XCTAssertEqual(info, k2) // kept across a restart
        XCTAssertTrue(h.events.all.dropFirst(before).contains { keyChanged($0, info) })
        a = try await s.unlockKeyAvailability()
        XCTAssertEqual(.enabled, a)
    }

    func testAProviderChainIsSentWhenThereIsOne() async throws {
        keys.chain.value = ["bGVhZg==", "cm9vdA=="]
        let s = try await signedIn()
        _ = try await s.enableUnlockKey(using: finger, prompt: prompt)
        let reg = p.all("/field/auth/unlock-keys").last { $0.body?.has("keyProof") == true }!
        XCTAssertEqual(["bGVhZg==", "cm9vdA=="], reg.body!["attestationChain"] as? [String])
    }

    func testNothingIsKeptWhenThePromptIsCancelled() async throws {
        let s = try await signedIn()
        await failsWith("BIOMETRIC_CANCELLED") { _ = try await s.enableUnlockKey(using: self.refusing("BIOMETRIC_CANCELLED"), prompt: self.prompt) }
        XCTAssertTrue(keys.aliases.isEmpty)
        let k = try await s.unlockKey()
        XCTAssertNil(k)
        XCTAssertEqual(0, p.all("/field/auth/unlock-keys").filter { $0.body?.has("keyProof") == true }.count)
    }

    func testTheRegistrationRefusals() async throws {
        let s = try await signedIn()
        for (status, code) in [(409, "UNLOCK_KEY_OTHER_WORKER"), (403, "PIN_SIGN_IN_REQUIRED"), (403, "WORKER_UNAVAILABLE"),
                               (401, "INVALID_UNLOCK_PROOF"), (503, "ATTESTATION_STATUS_UNAVAILABLE"), (400, "INVALID_PUBLIC_KEY")] {
            registerAnswer.value = { [self] _ in p.error(status, code, retryAfter: status == 503 ? 300 : nil) }
            let e = await failsWith(code) { _ = try await s.enableUnlockKey(using: self.finger, prompt: self.prompt) }
            if status == 503 { XCTAssertEqual(300, e?.retryAfterSeconds) }
            XCTAssertTrue(keys.aliases.isEmpty, code)
            let k = try await s.unlockKey()
            XCTAssertNil(k)
        }
    }

    func testTheServersAttestationRefusalSaysPlainlyThatIOSCannotAndIsNotRemembered() async throws {
        let s = try await signedIn()
        registerAnswer.value = { [self] _ in p.error(422, "ATTESTATION_REQUIRED", message: "the attestation is NONE") }
        let e = await failsWith("ATTESTATION_REQUIRED") { _ = try await s.enableUnlockKey(using: self.finger, prompt: self.prompt) }
        XCTAssertTrue(e?.message.contains("iOS cannot attest an unlock key") == true)
        XCTAssertEqual(422, e?.status)
        XCTAssertTrue(keys.aliases.isEmpty)
        // the token answer is the only source: it said nothing about attestation, so the offer stands
        let a = try await s.unlockKeyAvailability()
        XCTAssertEqual(.available, a)
    }

    func testTheServersPolicyRefusalAtRegistrationForgetsAnOldKey() async throws {
        let s = try await enabled()
        registerAnswer.value = { [self] _ in p.error(403, "UNLOCK_KEYS_DISABLED") }
        await failsWith("UNLOCK_KEYS_DISABLED") { _ = try await s.enableUnlockKey(using: self.finger, prompt: self.prompt) }
        let k = try await s.unlockKey()
        XCTAssertNil(k)
        XCTAssertTrue(keys.aliases.isEmpty)
    }

    func testAWorkerLockReportsTheEvent() async throws {
        let s = try await signedIn()
        registerAnswer.value = { [self] _ in p.error(429, "WORKER_LOCKED_TEMPORARILY", retryAfter: 60) }
        await failsWith("WORKER_LOCKED_TEMPORARILY") { _ = try await s.enableUnlockKey(using: self.finger, prompt: self.prompt) }
        XCTAssertTrue(h.events.contains(.workerLocked(hard: false, retryAfterSeconds: 60)))
    }

    func testAKeyThatExistsAlreadyIsMadeAgainOnce() async throws {
        let s = try await signedIn()
        let n = Locked(0)
        registerAnswer.value = { [self] r in n.mutate { $0 += 1; return $0 } == 1 ? p.error(409, "UNLOCK_KEY_EXISTS") : register(r) }
        let info = try await s.enableUnlockKey(using: finger, prompt: prompt)
        XCTAssertEqual("fwk_1", info.unlockKeyId)
        XCTAssertEqual(2, prompts.value)
        XCTAssertEqual(1, keys.aliases.count)
        XCTAssertEqual(1, keys.deleted.value.count)
    }

    func testANonceThatExpiredDuringTheFirstPromptCostsOneMorePrompt() async throws {
        let s = try await signedIn()
        let n = Locked(0)
        registerAnswer.value = { [self] r in
            if n.mutate({ $0 += 1; return $0 }) == 1 {
                return FakeResponse.json(401, #"{"error":"use_dpop_nonce"}"#).header("WWW-Authenticate", "DPoP error=\"use_dpop_nonce\"")
            }
            return register(r)
        }
        let info = try await s.enableUnlockKey(using: finger, prompt: prompt)
        XCTAssertEqual("fwk_1", info.unlockKeyId)
        XCTAssertEqual(2, prompts.value)
    }

    func testEnablingAgainReplacesTheWorkersOwnKey() async throws {
        let s = try await enabled()
        let first = keys.aliases.first!
        _ = try await s.enableUnlockKey(using: finger, prompt: prompt)
        XCTAssertEqual(1, keys.aliases.count)
        XCTAssertTrue(keys.deleted.value.contains(first))
        XCTAssertFalse(keys.aliases.contains(first))
    }

    func testAnotherWorkersKeyIsNotReplacedSilently() async throws {
        let s = try await enabled()
        loginWorker.value = ("w2", "W002")
        loginClaims.value = claims(["pin"], "famB", worker: "w2")
        _ = try await s.login(LoginCredentials(workerCode: "W002", pin: "5678"))
        var a = try await s.unlockKeyAvailability()
        XCTAssertEqual(.otherWorker, a)
        await failsWith("UNLOCK_KEY_OTHER_WORKER") { _ = try await s.enableUnlockKey(using: self.finger, prompt: self.prompt) }
        XCTAssertEqual(1, prompts.value)
        // the hand-over: the phone's holder disables the key, then registers their own
        try await s.disableUnlockKey()
        XCTAssertEqual(["fwk_1"], revoked.value)
        a = try await s.unlockKeyAvailability()
        XCTAssertEqual(.available, a)
    }

    // MARK: signing in with the key

    func testSigningInAsksForTheChallengeBeforeThePromptAndGivesASessionAsALoginDoes() async throws {
        try await enabled().logout()
        prompts.value = 0
        let s = h.session()
        let before = p.requests.count
        let order = Locked(false)
        let pr = FakeUnlockPrompter(shown: prompts) { [self] got, sig, data in
            order.value = p.count("/field/auth/unlock/challenge") == 1 && p.count("/field/auth/unlock") == 0
            XCTAssertEqual(prompt, got)
            return try sig.sign(data)
        }
        let ses = try await s.loginWithUnlockKey(using: pr, prompt: prompt)
        XCTAssertTrue(order.value)
        XCTAssertEqual(1, prompts.value)
        XCTAssertEqual(["hwk", "uv"], ses.amr)
        XCTAssertEqual("w1", ses.worker.id)
        XCTAssertEqual(.signedIn(ses), s.state)
        let hdr = try await s.headersFor(method: "GET", url: h.app.url + "/x")
        XCTAssertNotNil(hdr["Authorization"])
        let paths = p.requests.dropFirst(before).map(\.path).filter { $0 != "/field/auth/nonce" }
        XCTAssertEqual(["/field/auth/unlock/challenge", "/field/auth/unlock"], Array(paths))
        XCTAssertTrue(h.events.contains(.sessionChanged(ses)))
        // amr survives a restart
        if case .signedIn(let restored) = try await h.session().restore() { XCTAssertEqual(["hwk", "uv"], restored.amr) } else { XCTFail() }
    }

    func testASessionFromAnUnlockCannotMakeANewKey() async throws {
        try await enabled()
        let s = h.session()
        _ = try await s.loginWithUnlockKey(using: finger, prompt: prompt)
        try await s.disableUnlockKey()
        let a = try await s.unlockKeyAvailability()
        XCTAssertEqual(.pinSignInRequired, a)
    }

    func testARefusedChallengeNeverShowsAPrompt() async throws {
        try await enabled()
        prompts.value = 0
        let s = h.session()
        for (status, code) in [(401, "PIN_REAUTH_REQUIRED"), (403, "PIN_RESET_REQUIRED"), (503, "ATTESTATION_STATUS_UNAVAILABLE"),
                               (423, "WORKER_LOCKED"), (429, "WORKER_LOCKED_TEMPORARILY"), (429, "TOO_MANY_ATTEMPTS"),
                               (403, "DEVICE_SUSPENDED")] {
            challengeAnswer.value = { [self] in p.error(status, code, retryAfter: status == 503 || status == 429 ? 30 : nil) }
            let e = await failsWith(code) { _ = try await s.loginWithUnlockKey(using: self.finger, prompt: self.prompt) }
            if status == 503 { XCTAssertEqual(30, e?.retryAfterSeconds) }
            let k = try await s.unlockKey()
            XCTAssertNotNil(k, code)
        }
        XCTAssertEqual(0, prompts.value)
        XCTAssertEqual(0, p.count("/field/auth/unlock"))
        XCTAssertTrue(h.events.contains(.workerLocked(hard: true, retryAfterSeconds: nil)))
        XCTAssertTrue(revoked.value.isEmpty)
    }

    func testAnUnknownOrRevokedKeyIsDeletedAndRevoked() async throws {
        try await enabled()
        let s = h.session()
        challengeAnswer.value = { [self] in p.error(401, "UNLOCK_KEY_INVALID") }
        await failsWith("UNLOCK_KEY_INVALID") { _ = try await s.loginWithUnlockKey(using: self.finger, prompt: self.prompt) }
        let k = try await s.unlockKey()
        XCTAssertNil(k)
        XCTAssertTrue(keys.aliases.isEmpty)
        XCTAssertEqual(["fwk_1"], revoked.value)
        XCTAssertTrue(h.events.all.contains { keyChanged($0, nil) })
        await failsWith("NO_UNLOCK_KEY") { _ = try await s.loginWithUnlockKey(using: self.finger, prompt: self.prompt) }
    }

    func testPolicyOffDeletesTheKey() async throws {
        try await enabled()
        let s = h.session()
        challengeAnswer.value = { [self] in p.error(403, "UNLOCK_KEYS_DISABLED") }
        await failsWith("UNLOCK_KEYS_DISABLED") { _ = try await s.loginWithUnlockKey(using: self.finger, prompt: self.prompt) }
        let k = try await s.unlockKey()
        XCTAssertNil(k)
        XCTAssertTrue(keys.aliases.isEmpty)
        XCTAssertTrue(revoked.value.isEmpty) // the policy revoked it already
    }

    func testARevokedOrUnknownDeviceDeletesTheKey() async throws {
        for (status, code) in [(403, "DEVICE_REVOKED"), (401, "UNKNOWN_DEVICE"), (403, "SHARED_DEVICE")] {
            challengeAnswer.value = nil
            loginClaims.value = claims(["pin"], "fam-\(code)")
            try await enabled()
            let s = h.session()
            challengeAnswer.value = { [self] in p.error(status, code) }
            await failsWith(code) { _ = try await s.loginWithUnlockKey(using: self.finger, prompt: self.prompt) }
            let k = try await s.unlockKey()
            XCTAssertNil(k, code)
        }
    }

    func testAKeyThePhoneInvalidatedIsRevokedAndDeletedBeforeAnyChallengeOrPrompt() async throws {
        try await enabled()
        prompts.value = 0
        keys.invalidated.value = keys.aliases
        let s = h.session()
        await failsWith("UNLOCK_KEY_INVALIDATED") { _ = try await s.loginWithUnlockKey(using: self.finger, prompt: self.prompt) }
        XCTAssertEqual(0, prompts.value)
        XCTAssertEqual(0, p.count("/field/auth/unlock/challenge"))
        XCTAssertEqual(["fwk_1"], revoked.value)
        let k = try await s.unlockKey()
        XCTAssertNil(k)
        XCTAssertTrue(keys.aliases.isEmpty)
    }

    func testAnInvalidationReportedByThePromptIsHandledTheSameWay() async throws {
        try await enabled()
        let s = h.session()
        await failsWith("UNLOCK_KEY_INVALIDATED") { _ = try await s.loginWithUnlockKey(using: self.refusing("KEY_INVALIDATED"), prompt: self.prompt) }
        XCTAssertEqual(["fwk_1"], revoked.value)
        let k = try await s.unlockKey()
        XCTAssertNil(k)
        XCTAssertEqual(0, p.count("/field/auth/unlock"))
    }

    func testACancelledOrLockedOutPromptKeepsTheKey() async throws {
        try await enabled()
        let s = h.session()
        for code in ["BIOMETRIC_CANCELLED", "BIOMETRIC_LOCKOUT"] {
            await failsWith(code) { _ = try await s.loginWithUnlockKey(using: self.refusing(code), prompt: self.prompt) }
            let k = try await s.unlockKey()
            XCTAssertNotNil(k)
        }
        XCTAssertEqual(0, p.count("/field/auth/unlock"))
    }

    func testNoBiometricEnrolledAsksNothing() async throws {
        try await enabled()
        keys.biometric.value = .biometricNotEnrolled
        let s = h.session()
        let before = p.requests.count
        await failsWith("BIOMETRIC_NOT_ENROLLED") { _ = try await s.loginWithUnlockKey(using: self.finger, prompt: self.prompt) }
        XCTAssertEqual(before, p.requests.count)
        let k = try await s.unlockKey()
        XCTAssertNotNil(k)
    }

    func testAnExpiredChallengeIsAskedForAgainOnce() async throws {
        try await enabled()
        prompts.value = 0
        let n = Locked(0)
        unlockAnswer.value = { [self] r in n.mutate { $0 += 1; return $0 } == 1 ? p.error(401, "CHALLENGE_EXPIRED") : unlock(r) }
        let ses = try await h.session().loginWithUnlockKey(using: finger, prompt: prompt)
        XCTAssertEqual(["hwk", "uv"], ses.amr)
        XCTAssertEqual(2, prompts.value)
        XCTAssertEqual(2, p.count("/field/auth/unlock/challenge"))
        unlockAnswer.value = { [self] _ in p.error(401, "CHALLENGE_EXPIRED") }
        await failsWith("CHALLENGE_EXPIRED") { _ = try await self.h.session().loginWithUnlockKey(using: self.finger, prompt: self.prompt) }
    }

    func testABadSignatureTwiceDeletesTheKey() async throws {
        try await enabled()
        let s = h.session()
        unlockAnswer.value = { [self] _ in p.error(401, "INVALID_UNLOCK_PROOF") }
        await failsWith("INVALID_UNLOCK_PROOF") { _ = try await s.loginWithUnlockKey(using: self.finger, prompt: self.prompt) }
        var k = try await s.unlockKey()
        XCTAssertNotNil(k)
        await failsWith("INVALID_UNLOCK_PROOF") { _ = try await self.h.session().loginWithUnlockKey(using: self.finger, prompt: self.prompt) }
        k = try await h.session().unlockKey()
        XCTAssertNil(k)
        XCTAssertEqual(["fwk_1"], revoked.value)
    }

    func testASuccessResetsTheBadSignatureCount() async throws {
        try await enabled()
        let s = h.session()
        let n = Locked(0)
        unlockAnswer.value = { [self] r in n.mutate { $0 += 1; return $0 } % 2 == 1 ? p.error(401, "INVALID_UNLOCK_PROOF") : unlock(r) }
        await failsWith("INVALID_UNLOCK_PROOF") { _ = try await s.loginWithUnlockKey(using: self.finger, prompt: self.prompt) }
        _ = try await s.loginWithUnlockKey(using: finger, prompt: prompt)
        await failsWith("INVALID_UNLOCK_PROOF") { _ = try await s.loginWithUnlockKey(using: self.finger, prompt: self.prompt) }
        let k = try await s.unlockKey()
        XCTAssertNotNil(k)
    }

    func testTheOtherUnlockRefusalsKeepTheKey() async throws {
        try await enabled()
        let s = h.session()
        for (status, code) in [(401, "PIN_REAUTH_REQUIRED"), (403, "NO_APP_ACCESS"), (423, "WORKER_LOCKED")] {
            unlockAnswer.value = { [self] _ in p.error(status, code) }
            await failsWith(code) { _ = try await s.loginWithUnlockKey(using: self.finger, prompt: self.prompt) }
            let k = try await s.unlockKey()
            XCTAssertNotNil(k, code)
        }
    }

    func testThePinDeadlineFollowsTheChallenge() async throws {
        try await enabled()
        challengeAnswer.value = { [self] in
            issued.mutate { $0.append("c-x") }
            return .json(200, JSON.string(["challenge": "c-x", "expiresAt": "2030-01-01T00:00:00Z"]))
        }
        let s = h.session()
        _ = try await s.loginWithUnlockKey(using: finger, prompt: prompt)
        let k = try await s.unlockKey()
        XCTAssertNil(k?.pinRequiredBy) // no periodic PIN: absent on the wire
    }

    func testNoKeyOrUnenrolled() async throws {
        let s = h.session()
        await failsWith("NOT_ENROLLED") { _ = try await s.loginWithUnlockKey(using: self.finger, prompt: self.prompt) }
        _ = try await s.enrol("ABCDE12345", info: DeviceInfo())
        await failsWith("NO_UNLOCK_KEY") { _ = try await s.loginWithUnlockKey(using: self.finger, prompt: self.prompt) }
    }

    func testTheKeySurvivesASignOutAndARestart() async throws {
        let s = try await enabled()
        try await s.logout()
        var k = try await s.unlockKey()
        XCTAssertNotNil(k)
        k = try await h.session().unlockKey()
        XCTAssertNotNil(k)
        let ses = try await h.session().loginWithUnlockKey(using: finger, prompt: prompt)
        XCTAssertEqual("w1", ses.worker.id)
    }

    func testANewEnrolmentForgetsTheKeyOfTheOldDeviceRow() async throws {
        let s = try await enabled()
        p.on("/field/devices/enrol") { _ in .json(201, #"{"deviceId":"dev2","mode":"PERSONAL","clientId":"clt_1","appCodes":["app1"]}"#) }
        _ = try await s.enrol("ABCDE99999", info: DeviceInfo())
        let k = try await s.unlockKey()
        XCTAssertNil(k)
        XCTAssertTrue(keys.aliases.isEmpty)
    }

    // MARK: disabling

    func testDisablingRevokesWithTheDeviceAloneAndDeletesTheKey() async throws {
        let s = try await enabled()
        try await s.logout()
        try await s.disableUnlockKey()
        XCTAssertEqual(["fwk_1"], revoked.value)
        let r = p.all("/field/auth/unlock-keys/revoke").first!
        XCTAssertNil(r.bearer)
        XCTAssertTrue(r.proof!.valid())
        let k = try await s.unlockKey()
        XCTAssertNil(k)
        XCTAssertTrue(keys.aliases.isEmpty)
        try await s.disableUnlockKey() // nothing left: no request
        XCTAssertEqual(1, p.count("/field/auth/unlock-keys/revoke"))
    }

    func testDisablingOfflineDeletesAtOnceAndRevokesLater() async throws {
        let s = try await enabled()
        h.net.offline.value = true
        try await s.disableUnlockKey()
        let k = try await s.unlockKey()
        XCTAssertNil(k)
        XCTAssertTrue(keys.aliases.isEmpty)
        XCTAssertTrue(revoked.value.isEmpty)
        h.net.offline.value = false
        // the next platform call sends it (here: the next sign-in, also from a new process)
        _ = try await h.session().login(LoginCredentials(workerCode: "W001", pin: "1234"))
        XCTAssertEqual(["fwk_1"], revoked.value)
        _ = try await h.session().login(LoginCredentials(workerCode: "W001", pin: "1234"))
        XCTAssertEqual(["fwk_1"], revoked.value) // sent once
    }

    func testARevocationTheDeviceCanNoLongerSendIsDropped() async throws {
        let s = try await enabled()
        revokeAnswer.value = { [self] in p.error(403, "DEVICE_REVOKED") }
        try await s.disableUnlockKey()
        revokeAnswer.value = nil
        _ = try await s.login(LoginCredentials(workerCode: "W001", pin: "1234"))
        XCTAssertTrue(revoked.value.isEmpty)
        XCTAssertEqual(1, p.count("/field/auth/unlock-keys/revoke"))
    }

    func testTheAppUnlockIsSeparateAndUnchanged() async throws {
        let s = try await enabled()
        let shown = Locked(false)
        struct P: UnlockPrompter {
            let shown: Locked<Bool>
            func prompt(_ text: String) async throws { shown.value = true }
        }
        _ = try await s.unlock(using: P(shown: shown), prompt: "x") // not locked: returns at once, no prompt, no key used
        XCTAssertFalse(shown.value)
        XCTAssertEqual(0, p.count("/field/auth/unlock/challenge"))
    }

    func testNoPinOrSecretInAnyUnlockRequest() async throws {
        let s = try await enabled()
        try await s.logout()
        _ = try await h.session().loginWithUnlockKey(using: finger, prompt: prompt)
        for r in p.requests where r.path.contains("unlock") {
            let text = String(decoding: r.raw, as: UTF8.self)
            XCTAssertFalse(text.contains("1234"))
            XCTAssertFalse(text.contains("fwr_"))
        }
    }
}
