import XCTest
@testable import FlowCatalystFieldCore

/// The client the worker belongs to, read from the access token's claims.
final class ClientTests: XCTestCase {
    var h: Harness!
    var p: FakePlatform { h.platform }

    override func setUp() async throws { h = Harness() }
    override func tearDown() async throws { h.close() }

    private func claims(_ extra: JObject = [:], id: String = "clt_1", identifier: String = "acme") -> JObject {
        var c = p.deviceClaims("PERSONAL", id: "dev1")
        c["client_id"] = id
        c["client_identifier"] = identifier
        for (k, v) in extra { c[k] = v }
        return c
    }

    private let adminActor: JObject = ["act": ["sub": "adm1", "typ": "principal", "name": "Ada Admin"] as JObject]

    private func login(_ s: FieldSession) async throws -> Session {
        try await s.login(LoginCredentials(workerCode: "W001", pin: "1234"))
    }

    func testSignInTakesTheClientFromTheClaims() async throws {
        let c = claims()
        p.on("/field/auth/login") { _ in self.p.token(claims: c) }
        let ses = try await login(h.session())
        XCTAssertEqual("clt_1", ses.clientId)
        XCTAssertEqual("acme", ses.clientIdentifier)
    }

    func testRefreshKeepsIt() async throws {
        let first = claims()
        let second = claims(id: "clt_2", identifier: "globex")
        p.on("/field/auth/login") { _ in self.p.token(claims: first) }
        p.on("/field/auth/refresh") { _ in self.p.token(claims: second) }
        let s = h.session()
        _ = try await login(s)
        let ses = try await s.refresh()
        XCTAssertEqual("clt_2", ses.clientId)
        XCTAssertEqual("globex", ses.clientIdentifier)
    }

    func testARestoredSessionHasIt() async throws {
        let c = claims()
        p.on("/field/auth/login") { _ in self.p.token(claims: c) }
        _ = try await login(h.session())
        let st = try await h.session().restore()
        guard case .signedIn(let ses) = st else { return XCTFail("\(st)") }
        XCTAssertEqual("clt_1", ses.clientId)
        XCTAssertEqual("acme", ses.clientIdentifier)
    }

    func testAnImpersonatedSessionHasTheWorkersClient() async throws {
        let c = claims(adminActor)
        p.on("/field/auth/sign-in-code") { _ in self.p.token(claims: c) }
        let ses = try await h.session().redeemSignInCode("ABCD-1234")
        XCTAssertNotNil(ses.actor)
        XCTAssertEqual("clt_1", ses.clientId)
        XCTAssertEqual("acme", ses.clientIdentifier)

        let plain = claims()
        p.on("/field/auth/login") { _ in self.p.token(workerId: "sup", workerCode: "S001", claims: plain) }
        p.on("/field/auth/act-as") { _ in self.p.token(workerId: "w2", workerCode: "W002", claims: c) }
        let s = h.session()
        _ = try await s.login(LoginCredentials(workerCode: "S001", pin: "9999"))
        let worker = try await s.actAs(ActAsTarget(workerCode: "W002", pin: "9999"))
        XCTAssertEqual("acme", worker.clientIdentifier)
    }

    func testAChangedPhotoKeepsTheClient() async throws {
        let c = claims()
        p.on("/field/auth/login") { _ in self.p.token(claims: c, photoVersion: "0123456789abcdef", canSetOwnPhoto: true) }
        let s = h.session()
        _ = try await login(s)
        p.on("/field/worker/photo/remove") { _ in .json(200, #"{"photoVersion":null}"#) }
        let after = try await s.removePhoto()
        XCTAssertEqual("acme", after.clientIdentifier)
        XCTAssertEqual("clt_1", after.clientId)
    }

    func testAnAbsentClaimIsNil() async throws {
        let c = p.deviceClaims("PERSONAL", id: "dev1")
        p.on("/field/auth/login") { _ in self.p.token(claims: c) }
        let ses = try await login(h.session())
        XCTAssertNil(ses.clientId)
        XCTAssertNil(ses.clientIdentifier)
    }

    func testAnEmptyClaimIsNil() async throws {
        let c = claims(identifier: "")
        p.on("/field/auth/login") { _ in self.p.token(claims: c) }
        let ses = try await login(h.session())
        XCTAssertEqual("clt_1", ses.clientId)
        XCTAssertNil(ses.clientIdentifier)
    }

    func testASessionStoredBeforeTheFieldsExistedReadsAsNilAndTheNextRefreshFillsThem() async throws {
        let c = claims()
        p.on("/field/auth/login") { _ in self.p.token(claims: c) }
        p.on("/field/auth/refresh") { _ in self.p.token(claims: c) }
        _ = try await login(h.session())
        var stored = try XCTUnwrap(JSON.parseObject(try XCTUnwrap(h.storage.get(FieldSession.storeSession))))
        XCTAssertTrue(stored.has("clientIdentifier"))
        stored.removeValue(forKey: "clientId")
        stored.removeValue(forKey: "clientIdentifier")
        try h.storage.put(FieldSession.storeSession, value: JSON.string(stored))
        let s2 = h.session()
        guard case .signedIn(let restored) = try await s2.restore() else { return XCTFail("not signed in") }
        XCTAssertNil(restored.clientId)
        XCTAssertNil(restored.clientIdentifier)
        let ses = try await s2.refresh()
        XCTAssertEqual("acme", ses.clientIdentifier)
    }
}
