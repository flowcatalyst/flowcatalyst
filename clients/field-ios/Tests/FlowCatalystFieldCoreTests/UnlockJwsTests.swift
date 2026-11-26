import Foundation
import XCTest
@testable import FlowCatalystFieldCore

/// The unlock-key signer against the platform's byte vectors (api/field-fixtures/unlock.json). Port of UnlockJwsTest.kt.
/// The package is also published on its own, without the repository's fixtures: then these tests are skipped.
final class UnlockJwsTests: XCTestCase {
    private var fixture: JObject!

    override func setUpWithError() throws {
        let url = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
            .deletingLastPathComponent().deletingLastPathComponent()
            .appendingPathComponent("api/field-fixtures/unlock.json")
        guard let data = try? Data(contentsOf: url) else { throw XCTSkip("api/field-fixtures/unlock.json is not here") }
        fixture = JSON.parseObject(data)
    }

    private func c(_ name: String) -> JObject {
        let cases = fixture.optArray("cases") as! [JObject]
        return cases.first { $0.optString("name") == name }!
    }

    private var base: String { fixture.optString("baseUrl") }
    private var key: JObject { fixture.optObject("unlockKey")! }
    private func iat(_ c: JObject) -> Int64 { c.optObject("claims")!.optLong("iat") }

    /// What the library builds from the values the platform expects (verifyWith).
    private func ours(_ c: JObject) -> String {
        let v = c.optObject("verifyWith")!
        if c.optString("kind") == "keyProof" {
            return UnlockJws.keyProofSigningInput(
                baseUrl: base, nonce: v.optString("nonce"), workerId: v.optString("sub"), deviceId: v.optString("deviceId"), iat: iat(c))
        }
        return UnlockJws.assertionSigningInput(
            baseUrl: base, unlockKeyId: v.optString("keyId"), challenge: v.optString("nonce"), workerId: v.optString("sub"),
            deviceId: v.optString("deviceId"), appCode: v.optString("app"), iat: iat(c))
    }

    func testKeyProofSigningInputIsTheVectorsByteForByte() {
        XCTAssertEqual(c("key_proof_valid").optString("signingInput"), ours(c("key_proof_valid")))
    }

    func testAssertionSigningInputIsTheVectorsByteForByte() {
        XCTAssertEqual(c("assertion_valid").optString("signingInput"), ours(c("assertion_valid")))
    }

    func testTheValidVectorsVerifyWithTheUnlockKeyAndTheOtherKeysDoesNot() throws {
        for name in ["key_proof_valid", "assertion_valid"] {
            let jws = c(name).optString("jws")
            XCTAssertTrue(verifyEs256(jws, x: key.optString("x"), y: key.optString("y")), name)
            let sig = Base64URL.decode(String(jws.split(separator: ".").last!))!
            XCTAssertEqual(jws, try UnlockJws.compact(ours(c(name)), sig))
        }
        XCTAssertFalse(verifyEs256(c("assertion_signed_by_another_key").optString("jws"), x: key.optString("x"), y: key.optString("y")))
    }

    func testTheLibraryCannotBuildAnyRefusedVariant() {
        for name in ["assertion_with_key_proof_typ", "assertion_with_dpop_typ", "assertion_other_kid", "assertion_other_app",
                     "assertion_key_proof_audience", "key_proof_with_kid", "key_proof_other_nonce"] {
            XCTAssertNotEqual("valid", c(name).optString("expect"))
            XCTAssertNotEqual(c(name).optString("signingInput"), ours(c(name)), name)
        }
    }

    func testHeadersHoldExactlyTheMembersTheSpecNames() {
        let proof = jwsHeader(ours(c("key_proof_valid")) + ".x")
        XCTAssertEqual(Set(["typ", "alg"]), Set(proof.keys))
        let assertion = jwsHeader(ours(c("assertion_valid")) + ".x")
        XCTAssertEqual(Set(["typ", "alg", "kid"]), Set(assertion.keys))
    }

    func testAttestationChallengeIsTheVectors() {
        let a = fixture.optObject("attestationChallenge")!
        let parts = a.optString("input").replacingOccurrences(of: "fc-unlock-key-attestation:v1\n", with: "")
            .split(separator: "\n").map(String.init)
        let got = UnlockJws.attestationChallenge(sid: parts[0], workerId: parts[1], deviceId: parts[2])
        XCTAssertEqual(32, got.count)
        XCTAssertEqual(a.optString("hex"), got.map { String(format: "%02x", $0) }.joined())
    }

    func testATrailingSlashOnTheBaseUrlChangesNothing() {
        let v = c("key_proof_valid").optObject("verifyWith")!
        XCTAssertEqual(
            c("key_proof_valid").optString("signingInput"),
            UnlockJws.keyProofSigningInput(baseUrl: base + "/", nonce: v.optString("nonce"), workerId: v.optString("sub"),
                                           deviceId: v.optString("deviceId"), iat: iat(c("key_proof_valid"))))
    }
}

/// The pure parts that need no fixture.
final class UnlockJwsPureTests: XCTestCase {
    func testJsonEscapesOnlyWhatJsonNeeds() {
        XCTAssertEqual(#"{"a":"x/y\"z\\\n\u0001","n":5}"#, UnlockJws.json([("a", .string("x/y\"z\\\n\u{01}")), ("n", .int(5))]))
    }

    func testASignatureOfTheWrongLengthIsRefused() {
        XCTAssertThrowsError(try UnlockJws.compact("a.b", Data(count: 70))) { e in
            XCTAssertEqual("INTERNAL", (e as? FieldAuthError)?.code)
        }
    }

    func testTheLAErrorTable() {
        XCTAssertEqual("BIOMETRIC_CANCELLED", UnlockKeyErrorMapping.map(laErrorRawCode: -2).code) // userCancel
        XCTAssertEqual("BIOMETRIC_CANCELLED", UnlockKeyErrorMapping.map(laErrorRawCode: -4).code) // systemCancel
        XCTAssertEqual("BIOMETRIC_CANCELLED", UnlockKeyErrorMapping.map(laErrorRawCode: -9).code) // appCancel
        XCTAssertEqual("BIOMETRIC_CANCELLED", UnlockKeyErrorMapping.map(laErrorRawCode: -3).code) // userFallback
        XCTAssertEqual("BIOMETRIC_CANCELLED", UnlockKeyErrorMapping.map(laErrorRawCode: -1).code) // authenticationFailed
        XCTAssertEqual("BIOMETRIC_LOCKOUT", UnlockKeyErrorMapping.map(laErrorRawCode: -8).code)
        XCTAssertEqual("BIOMETRIC_NOT_ENROLLED", UnlockKeyErrorMapping.map(laErrorRawCode: -7).code)
        XCTAssertEqual("BIOMETRIC_UNAVAILABLE", UnlockKeyErrorMapping.map(laErrorRawCode: -6).code)
        XCTAssertEqual("NO_SECURE_LOCK_SCREEN", UnlockKeyErrorMapping.map(laErrorRawCode: -5).code)
        XCTAssertEqual("BIOMETRIC_UNAVAILABLE", UnlockKeyErrorMapping.map(laErrorRawCode: nil).code)
        XCTAssertEqual("INTERNAL", UnlockKeyErrorMapping.map(laErrorRawCode: -10).code)
    }

    func testAvailabilityRawValuesAreTheKotlinNames() {
        XCTAssertEqual(
            ["AVAILABLE", "ENABLED", "NOT_ENROLLED", "SHARED_DEVICE", "NOT_SIGNED_IN", "IMPERSONATING", "PIN_SIGN_IN_REQUIRED",
             "OTHER_WORKER", "OS_TOO_OLD", "NO_SECURE_HARDWARE", "BIOMETRIC_NOT_ENROLLED", "BIOMETRIC_UNAVAILABLE",
             "POLICY_OFF", "ATTESTATION_REQUIRED", "UNSUPPORTED"],
            UnlockKeyAvailability.allCases.map(\.rawValue))
    }
}
