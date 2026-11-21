import XCTest
@testable import FcFieldBridge

/// The Session as the bridge hands it to JavaScript.
final class FcFieldJsonTests: XCTestCase {
    private func session(clientId: String?, clientIdentifier: String?) -> Session {
        Session(
            worker: Worker(id: "w1", workerCode: "W001", name: "Ann", photoVersion: nil, canSetOwnPhoto: false),
            actor: nil, test: false, deviceMode: .personal, deviceId: "d1", appCode: "app1",
            expiresAt: 1, idleExpiresAt: 2, refreshExpiresAt: 3, pinChangeRequired: false,
            clientId: clientId, clientIdentifier: clientIdentifier)
    }

    func testTheSessionCarriesTheClient() {
        let j = FcFieldJson.session(session(clientId: "clt_1", clientIdentifier: "acme"))
        XCTAssertEqual("clt_1", j["clientId"] as? String)
        XCTAssertEqual("acme", j["clientIdentifier"] as? String)
    }

    func testAnAbsentClientIsNullNotMissing() {
        let j = FcFieldJson.session(session(clientId: nil, clientIdentifier: nil))
        XCTAssertTrue(j["clientId"] is NSNull)
        XCTAssertTrue(j["clientIdentifier"] is NSNull)
    }
}
