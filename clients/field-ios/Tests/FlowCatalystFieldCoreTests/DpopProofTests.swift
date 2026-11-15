import XCTest
@testable import FlowCatalystFieldCore

/// Port of DpopProofTest.kt: verifies proofs the way a server would (signature against the embedded jwk, claims, thumbprint).
final class DpopProofTests: XCTestCase {
    var h: Harness!
    var p: FakePlatform { h.platform }

    override func setUp() async throws { h = Harness() }
    override func tearDown() async throws { h.close() }

    private func thumbprintIndependently(_ x: String, _ y: String) -> String {
        sha256b64u("{\"crv\":\"P-256\",\"kty\":\"EC\",\"x\":\"\(x)\",\"y\":\"\(y)\"}")
    }

    // DpopProofTest.loginProofVerifies
    func testLoginProofVerifies() async throws {
        p.on("/field/auth/login") { _ in self.p.token() }
        _ = try await h.session().login(LoginCredentials(workerCode: "W001", pin: "1234"))
        let login = p.all("/field/auth/login")
        XCTAssertEqual(login.count, 1)
        let proof = try XCTUnwrap(login[0].proof)
        XCTAssertEqual("dpop+jwt", proof.header.optString("typ"))
        XCTAssertEqual("ES256", proof.header.optString("alg"))
        XCTAssertTrue(proof.valid(), "signature verifies")
        XCTAssertEqual("POST", proof.claims.optString("htm"))
        XCTAssertEqual("\(p.url)/field/auth/login", proof.claims.optString("htu"))
        XCTAssertEqual(h.clock.now() / 1000, proof.claims.optLong("iat"))
        XCTAssertGreaterThanOrEqual(proof.claims.optString("jti").count, 16)
        XCTAssertTrue(try XCTUnwrap(proof.nonce).hasPrefix("nonce-"))
        XCTAssertFalse(proof.claims.has("ath"), "no token yet")
        // the jwk is exactly the public key; its thumbprint is the RFC 7638 jkt
        XCTAssertEqual(h.keys.key.x, proof.jwk.optString("x"))
        XCTAssertEqual(h.keys.key.y, proof.jwk.optString("y"))
        XCTAssertEqual("EC", proof.jwk.optString("kty"))
        XCTAssertEqual("P-256", proof.jwk.optString("crv"))
        XCTAssertFalse(proof.jwk.has("d"))
        XCTAssertEqual(thumbprintIndependently(h.keys.key.x, h.keys.key.y), Dpop.thumbprint(x: h.keys.key.x, y: h.keys.key.y))
        // the nonce proof (no nonce yet) verifies as well
        let prime = try XCTUnwrap(p.all("/field/auth/nonce").first?.proof)
        XCTAssertEqual(p.all("/field/auth/nonce").count, 1)
        XCTAssertTrue(prime.valid())
        XCTAssertNil(prime.nonce)
    }

    // DpopProofTest.headersForProofBindsMethodUrlTokenAndNonce
    func testHeadersForProofBindsMethodUrlTokenAndNonce() async throws {
        p.on("/field/auth/login") { _ in self.p.token() }
        let s = h.session()
        _ = try await s.login(LoginCredentials(workerCode: "W001", pin: "1234"))
        await s.noteNonce(url: "https://api.example.com/x", nonce: "srv-nonce")
        let hdr = try await s.headersFor(method: "post", url: "https://api.example.com/v1/items?limit=5#frag")
        let token = String(try XCTUnwrap(hdr["Authorization"]).dropFirst("DPoP ".count))
        let proof = Proof(try XCTUnwrap(hdr["DPoP"]))
        XCTAssertTrue(proof.valid())
        XCTAssertEqual("POST", proof.claims.optString("htm"))
        XCTAssertEqual("https://api.example.com/v1/items", proof.claims.optString("htu"), "no query, no fragment")
        XCTAssertEqual("srv-nonce", proof.nonce)
        XCTAssertEqual(sha256b64u(token), proof.claims.optString("ath"))
        XCTAssertEqual(h.clock.now() / 1000, proof.claims.optLong("iat"))
        // jti differs per proof
        let again = Proof(try await s.headersFor(method: "post", url: "https://api.example.com/v1/items")["DPoP"]!)
        XCTAssertNotEqual(proof.claims.optString("jti"), again.claims.optString("jti"))
        XCTAssertTrue(again.valid())
    }

    // DpopProofTest.aTamperedProofDoesNotVerify
    func testATamperedProofDoesNotVerify() async throws {
        p.on("/field/auth/login") { _ in self.p.token() }
        let s = h.session()
        _ = try await s.login(LoginCredentials(workerCode: "W001", pin: "1234"))
        let jwt = try await s.headersFor(method: "GET", url: "https://api.example.com/x")["DPoP"]!
        let parts = jwt.split(separator: ".").map(String.init)
        let claims = String(decoding: Base64URL.decode(parts[1])!, as: UTF8.self).replacingOccurrences(of: "GET", with: "PUT")
        let forged = parts[0] + "." + Base64URL.encode(Data(claims.utf8)) + "." + parts[2]
        XCTAssertFalse(Proof(forged).valid())
    }

    // Not in Kotlin (it has no HTU unit test): the htu normalisation rules, explicitly.
    func testHtuNormalisation() {
        XCTAssertEqual(HTTPURL("https://API.Example.com:443/a/b?x=1#f")?.htu, "https://api.example.com/a/b")
        XCTAssertEqual(HTTPURL("http://example.com:80")?.htu, "http://example.com/")
        XCTAssertEqual(HTTPURL("http://127.0.0.1:8080/field/x/../y/./z")?.htu, "http://127.0.0.1:8080/field/y/z")
        XCTAssertEqual(HTTPURL("https://example.com:8443/p%20q")?.htu, "https://example.com:8443/p%20q")
        XCTAssertEqual(HTTPURL("https://example.com/a?x=1")?.origin, "https://example.com:443")
        XCTAssertNil(HTTPURL("ftp://example.com/x"))
        XCTAssertNil(HTTPURL("not a url"))
    }

    // Not in Kotlin: ath and nonce rules of the builder itself (empty values are omitted).
    func testEmptyNonceAndTokenAreOmitted() throws {
        let jwt = try Dpop.build(keys: h.keys, key: h.keys.key, htm: "GET", htu: "https://a.example/x", nonce: "", accessToken: "", iatSeconds: 5)
        let proof = Proof(jwt)
        XCTAssertFalse(proof.claims.has("nonce"))
        XCTAssertFalse(proof.claims.has("ath"))
        XCTAssertEqual(5, proof.claims.optLong("iat"))
        XCTAssertTrue(proof.valid())
    }

    // Not in Kotlin: a key provider returning a signature of the wrong length is INTERNAL.
    func testMalformedSignatureFromProviderIsInternal() {
        struct Short: DeviceKeyProvider {
            func getOrCreate() throws -> DeviceKey { DeviceKey(x: "x", y: "y") }
            func sign(_ data: Data) throws -> Data { Data(count: 10) }
            func delete() throws {}
        }
        XCTAssertThrowsError(try Dpop.build(keys: Short(), key: DeviceKey(x: "x", y: "y"), htm: "GET", htu: "u", nonce: nil, accessToken: nil, iatSeconds: 1)) {
            XCTAssertEqual(($0 as? FieldAuthError)?.code, "INTERNAL")
        }
    }
}
