import XCTest
@testable import FlowCatalystFieldCore

/// Port of RefreshAndInterceptorTest.kt. The OkHttp interceptor becomes `FieldSession.perform` (the same loop over
/// `HTTPRequest` / `HTTPResponse`); the Apple target's URLRequest functions are thin wrappers around it.
final class RefreshAndPerformTests: XCTestCase {
    var h: Harness!
    var p: FakePlatform { h.platform }
    var app: FakeApp { h.app }

    override func setUp() async throws {
        h = Harness()
        p.on("/field/auth/login") { _ in self.p.token() }
    }

    override func tearDown() async throws { h.close() }

    private func loggedIn() async throws -> FieldSession {
        let s = h.session()
        _ = try await s.login(LoginCredentials(workerCode: "W001", pin: "1234"))
        return s
    }

    private func get(_ s: FieldSession, _ path: String = "/api/x", base: String? = nil) async throws -> HTTPResponse {
        try await s.perform(HTTPRequest(url: (base ?? app.url) + path, method: "GET"))
    }

    // MARK: single flight

    // concurrentRefreshesShareOneRequest
    func testConcurrentRefreshesShareOneRequest() async throws {
        let s = try await loggedIn()
        p.on("/field/auth/refresh") { _ in self.p.token().delayed(400) }
        let all = try await withThrowingTaskGroup(of: Session.self) { group -> [Session] in
            for _ in 1...20 { group.addTask { try await s.refresh() } }
            var out: [Session] = []
            for try await x in group { out.append(x) }
            return out
        }
        XCTAssertEqual(1, p.count("/field/auth/refresh"))
        XCTAssertEqual(1, Set(all.map(\.expiresAt)).count, "everyone got the same session")
    }

    // twoCoroutinesAndTheInterceptorTriggerExactlyOneRefresh
    func testTwoTasksAndPerformTriggerExactlyOneRefresh() async throws {
        let s = try await loggedIn()
        app.requireNonce.value = false
        h.clock.jump(11 * MIN) // the access token has lapsed: every path wants a refresh
        p.on("/field/auth/refresh") { _ in self.p.token().delayed(500) }
        async let performed = self.get(s)
        async let a = s.headersFor(method: "GET", url: "\(app.url)/a")
        async let b = s.headersFor(method: "POST", url: "\(app.url)/b")
        let (resp, ha, hb) = try await (performed, a, b)
        XCTAssertEqual(200, resp.status)
        XCTAssertEqual(1, p.count("/field/auth/refresh"))
        // everyone signed with the same, refreshed access token
        XCTAssertEqual(ha["Authorization"], hb["Authorization"])
        XCTAssertEqual(ha["Authorization"], "DPoP " + (app.requests.first?.bearer ?? "?"))
        XCTAssertEqual(1, app.requests.count)
    }

    // freshTokenMeansNoRefresh
    func testFreshTokenMeansNoRefresh() async throws {
        let s = try await loggedIn()
        _ = try await s.headersFor(method: "GET", url: "\(app.url)/a")
        _ = try await s.headersFor(method: "GET", url: "\(app.url)/a")
        XCTAssertEqual(0, p.count("/field/auth/refresh"))
    }

    // tokenExpiringWithin30sRefreshesFirst
    func testTokenExpiringWithin30sRefreshesFirst() async throws {
        let s = try await loggedIn()
        p.on("/field/auth/refresh") { _ in self.p.token() }
        h.clock.jump(10 * MIN - 31_000)
        _ = try await s.headersFor(method: "GET", url: "\(app.url)/a")
        XCTAssertEqual(0, p.count("/field/auth/refresh"))
        h.clock.jump(2_000)
        _ = try await s.headersFor(method: "GET", url: "\(app.url)/a")
        XCTAssertEqual(1, p.count("/field/auth/refresh"))
    }

    // MARK: session-ended mapping

    // sessionEndingCodesEndTheSessionAndFailTheRequest
    func testSessionEndingCodesEndTheSessionAndFailTheRequest() async throws {
        for (status, code) in [(401, "INVALID_GRANT"), (401, "SESSION_EXPIRED"), (403, "DEVICE_REVOKED"), (403, "DEVICE_SUSPENDED")] {
            let hh = Harness()
            defer { hh.close() }
            hh.platform.on("/field/auth/login") { _ in hh.platform.token() }
            hh.platform.on("/field/auth/refresh") { _ in hh.platform.error(status, code) }
            let s = hh.session()
            _ = try await s.login(LoginCredentials(workerCode: "W001", pin: "1234"))
            hh.app.requireNonce.value = false
            hh.clock.jump(11 * MIN)
            await failsWith(code) { _ = try await s.perform(HTTPRequest(url: hh.app.url + "/x")) }
            XCTAssertEqual(0, hh.app.requests.count, "never reached the app")
            XCTAssertEqual(.signedOut, s.state)
            XCTAssertTrue(hh.events.contains(.sessionEnded(code)), "\(code): \(hh.events.all)")
            XCTAssertNil(try hh.storage.get(FieldSession.storeRefresh))
            XCTAssertNil(try hh.storage.get(FieldSession.storeSession))
        }
    }

    // otherRefreshFailuresKeepTheSession
    func testOtherRefreshFailuresKeepTheSession() async throws {
        let s = try await loggedIn()
        p.on("/field/auth/refresh") { _ in self.p.error(503, "UNAVAILABLE") }
        await failsWith("UNAVAILABLE") { _ = try await s.refresh() }
        if case .signedIn = s.state {} else { XCTFail("expected signedIn") }
        XCTAssertNotNil(try h.storage.get(FieldSession.storeRefresh))
        XCTAssertFalse(h.events.all.contains { if case .sessionEnded = $0 { return true } else { return false } })
    }

    // unknownDeviceAndNoAppAccessAlsoEndTheSession
    func testUnknownDeviceAndNoAppAccessAlsoEndTheSession() async throws {
        for code in ["UNKNOWN_DEVICE", "NO_APP_ACCESS"] {
            let s = try await loggedIn()
            p.on("/field/auth/refresh") { _ in self.p.error(401, code) }
            await failsWith(code) { _ = try await s.refresh() }
            XCTAssertEqual(.signedOut, s.state)
        }
    }

    // MARK: perform (interceptor): nonce and invalid_token

    // interceptorRetriesOnceOnUseDpopNonce
    func testPerformRetriesOnceOnUseDpopNonce() async throws {
        let s = try await loggedIn()
        let r = try await get(s)
        XCTAssertEqual(200, r.status)
        XCTAssertEqual(2, app.requests.count)
        XCTAssertNil(app.requests[0].proof?.nonce)
        XCTAssertEqual("app-nonce-1", app.requests[1].proof?.nonce)
        // the cached application nonce is reused by the next request: no retry
        _ = try await get(s)
        XCTAssertEqual(3, app.requests.count)
        XCTAssertEqual("app-nonce-1", app.requests[2].proof?.nonce)
        XCTAssertEqual(0, p.count("/field/auth/refresh"))
    }

    // nonceRetryHappensOnlyOnce
    func testNonceRetryHappensOnlyOnce() async throws {
        let s = try await loggedIn()
        let n = Locked(0)
        let calls = Locked(0)
        h.net.mount(origin: "http://127.0.0.1:18082") { _ in
            calls.mutate { $0 += 1 }
            let i = n.mutate { v -> Int in v += 1; return v - 1 }
            return FakeResponse(status: 401).header("WWW-Authenticate", "DPoP error=\"use_dpop_nonce\"").header("DPoP-Nonce", "n\(i)")
        }
        // an app that always demands a different nonce: first retry still fails, second 401 is returned as is
        let r = try await get(s, "/x", base: "http://127.0.0.1:18082")
        XCTAssertEqual(401, r.status)
        XCTAssertEqual(2, calls.value)
    }

    // invalidTokenRefreshesOnceAndRetriesOnce
    func testInvalidTokenRefreshesOnceAndRetriesOnce() async throws {
        let s = try await loggedIn()
        app.requireNonce.value = false
        let first = String(try await s.headersFor(method: "GET", url: "\(app.url)/x")["Authorization"]!.dropFirst(5))
        app.accept.value = { $0 != first }
        p.on("/field/auth/refresh") { _ in self.p.token() }
        let r = try await get(s)
        XCTAssertEqual(200, r.status)
        XCTAssertEqual(1, p.count("/field/auth/refresh"))
        XCTAssertEqual(2, app.requests.count)
        XCTAssertTrue(app.requests[0].bearer == first && app.requests[1].bearer != first)
    }

    // invalidTokenThatSurvivesARefreshEmitsSessionEnded
    func testInvalidTokenThatSurvivesARefreshEmitsSessionEnded() async throws {
        let s = try await loggedIn()
        app.requireNonce.value = false
        app.accept.value = { _ in false }
        p.on("/field/auth/refresh") { _ in self.p.token() }
        let r = try await get(s)
        XCTAssertEqual(401, r.status)
        XCTAssertEqual(1, p.count("/field/auth/refresh"))
        XCTAssertEqual(2, app.requests.count)
        XCTAssertTrue(h.events.contains(.sessionEnded("invalid_token")))
    }

    // invalidTokenAfterAnotherPathAlreadyRefreshedDoesNotRefreshAgain
    func testInvalidTokenAfterAnotherPathAlreadyRefreshedDoesNotRefreshAgain() async throws {
        let s = try await loggedIn()
        app.requireNonce.value = false
        let first = String(try await s.headersFor(method: "GET", url: "\(app.url)/x")["Authorization"]!.dropFirst(5))
        p.on("/field/auth/refresh") { _ in self.p.token() }
        _ = try await s.refresh() // someone else replaced the token
        let before = p.count("/field/auth/refresh")
        app.accept.value = { $0 != first }
        // a stale in-flight request: simulate by refreshIfStale with the old token
        try await s.refreshIfStale(first)
        XCTAssertEqual(before, p.count("/field/auth/refresh"))
    }

    // nonceSeenInAResponseFeedsLaterProofs
    func testNonceSeenInAResponseFeedsLaterProofs() async throws {
        let s = try await loggedIn()
        await s.noteNonce(url: "\(app.url)/api/x?y=1", nonce: "seen-elsewhere")
        let h1 = try await s.headersFor(method: "GET", url: "\(app.url)/api/z")
        XCTAssertEqual("seen-elsewhere", Proof(h1["DPoP"]!).nonce)
        // application nonces are reusable
        let h2 = try await s.headersFor(method: "GET", url: "\(app.url)/api/z")
        XCTAssertEqual("seen-elsewhere", Proof(h2["DPoP"]!).nonce)
        // a nonce is per origin
        let h3 = try await s.headersFor(method: "GET", url: "http://127.0.0.1:9/api")
        XCTAssertNil(Proof(h3["DPoP"]!).nonce)
    }

    // headersForRetryUsesTheGivenNonce
    func testHeadersForRetryUsesTheGivenNonce() async throws {
        let s = try await loggedIn()
        let hdr = try await s.headersForRetry(method: "GET", url: "\(app.url)/api/x", nonce: "fresh-1")
        XCTAssertEqual("fresh-1", Proof(hdr["DPoP"]!).nonce)
    }

    // platformRouteNoncesAreSingleUse
    func testPlatformRouteNoncesAreSingleUse() async throws {
        let s = try await loggedIn()
        await s.noteNonce(url: "\(p.url)/field/auth/pin", nonce: "single")
        let h1 = try await s.headersFor(method: "POST", url: "\(p.url)/field/auth/pin")
        XCTAssertEqual("single", Proof(h1["DPoP"]!).nonce)
        let h2 = try await s.headersFor(method: "POST", url: "\(p.url)/field/auth/pin")
        XCTAssertNil(Proof(h2["DPoP"]!).nonce)
    }

    // (no Kotlin counterpart of headersForBlockingWorksOffTheMainThread: Swift has no blocking form)

    // MARK: Swift additions (the interceptor's one-shot rule and header handling)

    // Kotlin's interceptor does not retry a one-shot body (RequestBody.isOneShot()).
    func testOneShotBodiesAreNotRetried() async throws {
        let s = try await loggedIn()
        let r = try await s.perform(HTTPRequest(url: "\(app.url)/up", method: "POST", body: Data("x".utf8)), oneShot: true)
        XCTAssertEqual(401, r.status, "the use_dpop_nonce 401 is returned, not retried")
        XCTAssertEqual(1, app.requests.count)
    }

    func testPerformReplacesCallerAuthorizationAndPreservesTheBody() async throws {
        let s = try await loggedIn()
        app.requireNonce.value = false
        var req = HTTPRequest(url: "\(app.url)/up", method: "POST", headers: ["authorization": "Bearer stale", "X-A": "1"], body: Data("payload".utf8))
        req.setHeader("dpop", "stale")
        let r = try await s.perform(req)
        XCTAssertEqual(200, r.status)
        let sent = try XCTUnwrap(app.requests.first)
        XCTAssertTrue(sent.header("Authorization")!.hasPrefix("DPoP "))
        XCTAssertEqual("1", sent.header("X-A"))
        XCTAssertEqual("POST", try XCTUnwrap(sent.proof).claims.optString("htm"))
    }

    // MARK: platform nonce handling

    // platformCallsPrimeTheNonceThenUseTheResponseNonce
    func testPlatformCallsPrimeTheNonceThenUseTheResponseNonce() async throws {
        p.on("/field/auth/refresh") { _ in self.p.token() }
        let s = try await loggedIn()
        _ = try await s.refresh()
        _ = try await s.refresh()
        // login primes once; refreshes reuse the nonce each response handed over
        XCTAssertEqual(1, p.count("/field/auth/nonce"))
        XCTAssertEqual(["/field/auth/nonce", "/field/auth/login", "/field/auth/refresh", "/field/auth/refresh"], p.requests.map(\.path))
        let nonces = p.requests.filter { $0.path != "/field/auth/nonce" }.map { $0.proof?.nonce }
        XCTAssertEqual(nonces.count, Set(nonces.map { $0 ?? "nil" }).count, "never shared")
    }

    // platformRetriesOnceOnUseDpopNonce
    func testPlatformRetriesOnceOnUseDpopNonce() async throws {
        p.failNextNonceChecks.value = 1
        let s = h.session()
        _ = try await s.login(LoginCredentials(workerCode: "W001", pin: "1234"))
        XCTAssertEqual(2, p.count("/field/auth/login"))
    }

    // platformDoesNotRetryTwice
    func testPlatformDoesNotRetryTwice() async {
        p.failNextNonceChecks.value = 2
        let s = h.session()
        await failsWith("use_dpop_nonce") { _ = try await s.login(LoginCredentials(workerCode: "W001", pin: "1234")) }
        XCTAssertEqual(2, p.count("/field/auth/login"))
    }
}
