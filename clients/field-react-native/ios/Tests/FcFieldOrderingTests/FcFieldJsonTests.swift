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

    func testTheSessionCarriesHowItWasSignedIn() {
        let s = Session(
            worker: Worker(id: "w1", workerCode: "W001", name: "Ann"), actor: nil, test: false, deviceMode: .personal,
            deviceId: "d1", appCode: "app1", expiresAt: 1, idleExpiresAt: 2, refreshExpiresAt: 3, pinChangeRequired: false,
            amr: ["hwk", "uv"])
        XCTAssertEqual(["hwk", "uv"], FcFieldJson.session(s)["amr"] as? [String])
        XCTAssertEqual([], FcFieldJson.session(session(clientId: nil, clientIdentifier: nil))["amr"] as? [String])
    }

    private let key = UnlockKeyInfo(
        unlockKeyId: "fwk_1", workerId: "w1", workerCode: "W001", workerName: "Ann", deviceId: "d1",
        createdAt: 1_800_000_000_000, pinRequiredBy: nil, attestationVerdict: "NONE")

    func testAnUnlockKeyIsTheAndroidShape() {
        let j = FcFieldJson.unlockKey(key)
        XCTAssertEqual(Set(["unlockKeyId", "workerId", "workerCode", "workerName", "deviceId", "createdAt", "pinRequiredBy",
                            "attestationVerdict"]), Set(j.keys))
        XCTAssertEqual(1_800_000_000_000, j["createdAt"] as? Double)
        XCTAssertTrue(j["pinRequiredBy"] is NSNull)
        XCTAssertEqual("NONE", j["attestationVerdict"] as? String)
        let withDeadline = UnlockKeyInfo(
            unlockKeyId: "fwk_1", workerId: "w1", workerCode: "W001", workerName: "Ann", deviceId: "d1",
            createdAt: 1, pinRequiredBy: 2, attestationVerdict: "NONE")
        XCTAssertEqual(2, FcFieldJson.unlockKey(withDeadline)["pinRequiredBy"] as? Double)
        XCTAssertTrue(FcFieldJson.unlockKeyOrNull(nil)["key"] is NSNull)
        XCTAssertEqual("fwk_1", (FcFieldJson.unlockKeyOrNull(key)["key"] as? [String: Any])?["unlockKeyId"] as? String)
    }

    func testTheUnlockKeyEvent() {
        let (name, payload) = FcFieldJson.event(.unlockKeyChanged(key))
        XCTAssertEqual("UnlockKeyChanged", name)
        XCTAssertEqual("fwk_1", (payload["key"] as? [String: Any])?["unlockKeyId"] as? String)
        let (_, gone) = FcFieldJson.event(.unlockKeyChanged(nil))
        XCTAssertTrue(gone["key"] is NSNull)
    }

    func testThePromptNeedsATitleAndACancelLabel() throws {
        let p = try FcFieldJson.unlockKeyPrompt(["title": "Sign in", "cancel": "Use PIN", "subtitle": "W001"])
        XCTAssertEqual(UnlockKeyPrompt(title: "Sign in", cancel: "Use PIN", subtitle: "W001"), p)
        XCTAssertNil(try FcFieldJson.unlockKeyPrompt(["title": "Sign in", "cancel": "Use PIN"]).subtitle)
        XCTAssertThrowsError(try FcFieldJson.unlockKeyPrompt(["title": "Sign in"])) { e in
            XCTAssertEqual("INVALID_REQUEST", (e as? FieldAuthError)?.code)
        }
    }

    func testTheAvailabilityCrossesAsItsName() {
        XCTAssertEqual("PIN_SIGN_IN_REQUIRED", UnlockKeyAvailability.pinSignInRequired.rawValue)
    }
}
