import XCTest
@testable import FlowCatalystFieldCore

/// Port of TimersTest.kt. Time is virtual: `TestClock.advance` wakes the session's timer tasks in deadline order.
final class TimersTests: XCTestCase {
    var h: Harness!
    var p: FakePlatform { h.platform }

    override func setUp() async throws {
        h = Harness()
        p.on("/field/auth/login") { _ in self.p.token() }
        p.on("/field/auth/refresh") { _ in self.p.token() }
    }

    override func tearDown() async throws { h.close() }

    private func advance(_ ms: Int64) async { await h.clock.advance(ms) }

    private func loginSession(_ cfg: FieldConfig? = nil) async throws -> FieldSession {
        let s = h.session(cfg)
        _ = try await s.login(LoginCredentials(workerCode: "W001", pin: "1234"))
        return s
    }

    // keepAliveRefreshesAtTheIdleMidpoint
    func testKeepAliveRefreshesAtTheIdleMidpoint() async throws {
        p.idleWindowMs.value = 30 * MIN // midpoint (15 min) is earlier than idle - 2 min
        p.absoluteWindowMs.value = 600 * MIN
        let s = try await loginSession()
        await s.keepAlive(true)
        await advance(15 * MIN - 1_000)
        XCTAssertEqual(0, p.count("/field/auth/refresh"))
        await advance(1_000)
        XCTAssertEqual(1, p.count("/field/auth/refresh"))
        // re-planned from the new idle deadline: next at +15 min again
        await advance(15 * MIN - 2_000)
        XCTAssertEqual(1, p.count("/field/auth/refresh"))
        await advance(2_000)
        XCTAssertEqual(2, p.count("/field/auth/refresh"))
    }

    // keepAliveUsesTheLeadWhenTheIdleWindowIsShort
    func testKeepAliveUsesTheLeadWhenTheIdleWindowIsShort() async throws {
        p.idleWindowMs.value = 3 * MIN // idle - 2 min = 1 min, midpoint = 1.5 min
        p.absoluteWindowMs.value = 600 * MIN
        let s = try await loginSession()
        await s.keepAlive(true)
        await advance(MIN - 1_000)
        XCTAssertEqual(0, p.count("/field/auth/refresh"))
        await advance(1_000)
        XCTAssertEqual(1, p.count("/field/auth/refresh"))
    }

    // keepAliveCustomLead
    func testKeepAliveCustomLead() async throws {
        p.idleWindowMs.value = 100 * MIN // lead 5 min => idle-5 = 95 min, midpoint 50 => midpoint wins
        p.absoluteWindowMs.value = 600 * MIN
        var cfg = h.config()
        cfg.keepAliveLead = 5 * 60
        let s = try await loginSession(cfg)
        await s.keepAlive(true)
        await advance(50 * MIN - 1_000)
        XCTAssertEqual(0, p.count("/field/auth/refresh"))
        await advance(1_000)
        XCTAssertEqual(1, p.count("/field/auth/refresh"))
    }

    // keepAliveOffStopsTheTimer
    func testKeepAliveOffStopsTheTimer() async throws {
        p.absoluteWindowMs.value = 600 * MIN
        let s = try await loginSession()
        await s.keepAlive(true)
        await advance(5 * MIN)
        await s.keepAlive(false)
        await advance(120 * MIN)
        XCTAssertEqual(0, p.count("/field/auth/refresh"))
    }

    // keepAliveStopsOnSessionEndAndResumesOnNextLogin
    func testKeepAliveStopsOnSessionEndAndResumesOnNextLogin() async throws {
        p.absoluteWindowMs.value = 600 * MIN
        let s = try await loginSession()
        await s.keepAlive(true)
        p.on("/field/auth/refresh") { _ in self.p.error(401, "INVALID_GRANT") }
        await advance(15 * MIN)
        XCTAssertEqual(1, p.count("/field/auth/refresh"))
        XCTAssertEqual(.signedOut, s.state)
        await advance(120 * MIN)
        XCTAssertEqual(1, p.count("/field/auth/refresh"), "no more attempts")
        p.on("/field/auth/refresh") { _ in self.p.token() }
        _ = try await s.login(LoginCredentials(workerCode: "W001", pin: "1234"))
        await advance(15 * MIN)
        XCTAssertEqual(2, p.count("/field/auth/refresh"), "the flag stayed on")
    }

    // keepAliveRetriesAfterATransientFailure
    func testKeepAliveRetriesAfterATransientFailure() async throws {
        p.absoluteWindowMs.value = 600 * MIN
        let s = try await loginSession()
        await s.keepAlive(true)
        p.on("/field/auth/refresh") { _ in self.p.error(503, "UNAVAILABLE") }
        await advance(15 * MIN)
        XCTAssertEqual(1, p.count("/field/auth/refresh"))
        p.on("/field/auth/refresh") { _ in self.p.token() }
        await advance(31 * 1000)
        XCTAssertEqual(2, p.count("/field/auth/refresh"))
        if case .signedIn = s.state {} else { XCTFail("expected signedIn") }
    }

    // keepAliveRefreshOnARestoredSessionHappensImmediatelyWhenDue
    func testKeepAliveRefreshOnARestoredSessionHappensImmediatelyWhenDue() async throws {
        p.absoluteWindowMs.value = 600 * MIN
        _ = try await loginSession()
        await advance(29 * MIN) // idle deadline (30 min) is 1 min away: inside the 2 min lead
        let s2 = h.session()
        _ = try await s2.restore()
        await s2.keepAlive(true)
        await advance(1)
        XCTAssertEqual(1, p.count("/field/auth/refresh"))
    }

    // MARK: expiry warnings

    private func remainings() -> [TimeInterval] {
        h.events.warnings.compactMap { if case .expiryWarning(let r) = $0 { return r } else { return nil } }
    }

    // warningsFireOncePerThresholdPerFamily
    func testWarningsFireOncePerThresholdPerFamily() async throws {
        p.absoluteWindowMs.value = 60 * MIN
        let s = try await loginSession()
        await advance(30 * MIN - 1_000)
        XCTAssertEqual(0, h.events.warnings.count)
        await advance(1_000)
        XCTAssertEqual([1800.0], remainings())
        await advance(20 * MIN - 1_000)
        XCTAssertEqual(1, h.events.warnings.count)
        await advance(1_000)
        XCTAssertEqual([1800.0, 600.0], remainings())
        // a refresh stays in the same family: nothing fires again
        p.on("/field/auth/refresh") { _ in self.p.token(refreshExpiresAt: BASE_TIME + 60 * MIN) }
        _ = try await s.refresh()
        await advance(30 * MIN)
        XCTAssertEqual(2, h.events.warnings.count)
    }

    // aNewFamilyWarnsAgain
    func testANewFamilyWarnsAgain() async throws {
        p.absoluteWindowMs.value = 60 * MIN
        let s = try await loginSession()
        await advance(55 * MIN)
        XCTAssertEqual(2, h.events.warnings.count)
        _ = try await s.login(LoginCredentials(workerCode: "W001", pin: "1234")) // new absolute deadline
        await advance(30 * MIN)
        XCTAssertEqual(3, h.events.warnings.count)
        await advance(20 * MIN)
        XCTAssertEqual(4, h.events.warnings.count)
    }

    // customThresholds
    func testCustomThresholds() async throws {
        p.absoluteWindowMs.value = 60 * MIN
        var cfg = h.config()
        cfg.expiryWarnings = [300.0]
        _ = try await loginSession(cfg)
        await advance(55 * MIN - 1_000)
        XCTAssertEqual(0, h.events.warnings.count)
        await advance(1_000)
        XCTAssertEqual([300.0], remainings())
    }

    // restoredSessionAlreadyInsideBothThresholdsWarnsOnce
    func testRestoredSessionAlreadyInsideBothThresholdsWarnsOnce() async throws {
        p.absoluteWindowMs.value = 60 * MIN
        _ = try await loginSession()
        await advance(55 * MIN)
        h.events.clear()
        let s2 = h.session()
        _ = try await s2.restore()
        await h.clock.settle()
        XCTAssertEqual([300.0], remainings())
        await advance(10 * MIN)
        XCTAssertEqual(1, h.events.warnings.count)
    }

    // noWarningsAfterLogout
    func testNoWarningsAfterLogout() async throws {
        p.absoluteWindowMs.value = 60 * MIN
        p.on("/field/auth/logout") { _ in FakeResponse(status: 204) }
        let s = try await loginSession()
        try await s.logout()
        await advance(120 * MIN)
        XCTAssertEqual(0, h.events.warnings.count)
    }
}
