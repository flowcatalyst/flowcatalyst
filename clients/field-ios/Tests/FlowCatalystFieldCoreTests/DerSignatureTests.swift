import XCTest
#if canImport(CryptoKit)
import CryptoKit
#else
import Crypto
#endif
@testable import FlowCatalystFieldCore

/// The DER vectors of clients/field-react-native/ios/Tests/FcFieldDerTests/DerSignatureTests.swift (the Kotlin
/// library has no DER test of its own), plus signatures from a real P-256 key in place of Keychain ones.
final class DerSignatureTests: XCTestCase {
    func hex(_ s: String) -> [UInt8] {
        var out: [UInt8] = []; var i = s.startIndex
        while i < s.endIndex { let j = s.index(i, offsetBy: 2); out.append(UInt8(s[i..<j], radix: 16)!); i = j }
        return out
    }

    func testShortValuesPadded() throws {
        let raw = try DerSignature.derToRaw(hex("3006020101020102"))
        XCTAssertEqual(raw.count, 64)
        XCTAssertEqual(raw[31], 1); XCTAssertEqual(raw[63], 2)
        XCTAssertTrue(raw[0..<31].allSatisfy { $0 == 0 })
    }

    func testSignPaddingStripped() throws {
        let r = "00" + String(repeating: "ff", count: 32)
        let s = "7f" + String(repeating: "00", count: 31)
        let raw = try DerSignature.derToRaw(hex("3045" + "0221" + r + "0220" + s))
        XCTAssertEqual(Array(raw[0..<32]), hex(String(repeating: "ff", count: 32)))
        XCTAssertEqual(Array(raw[32..<64]), hex(s))
    }

    func testRoundTripWithRealSignatures() throws {
        let key = P256.Signing.PrivateKey()
        for n in 0..<200 {
            let data = Data([UInt8(n & 0xff)])
            let sig = try key.signature(for: data)
            let der = [UInt8](sig.derRepresentation)
            let raw = try DerSignature.derToRaw(der)
            XCTAssertEqual(raw.count, 64)
            XCTAssertEqual(try DerSignature.rawToDer(raw), der)
            XCTAssertEqual(raw, [UInt8](sig.rawRepresentation))
            let back = try P256.Signing.ECDSASignature(derRepresentation: Data(try DerSignature.rawToDer(raw)))
            XCTAssertTrue(key.publicKey.isValidSignature(back, for: data))
        }
    }

    func testMalformedRejected() {
        for bad in ["", "30", "3006020101020202", "3106020101020102", "300602010102", "3004020100ff"] {
            XCTAssertThrowsError(try DerSignature.derToRaw(hex(bad)), bad)
        }
    }

    func testBase64Url() {
        XCTAssertEqual(DerSignature.toBase64Url([1, 2, 3]), "AQID")
        XCTAssertEqual(DerSignature.toBase64Url([1, 2]), "AQI")
        XCTAssertEqual(DerSignature.fromBase64Url("AQI"), [1, 2])
        XCTAssertNil(DerSignature.fromBase64Url("@@@"))
        XCTAssertEqual(Base64URL.encode(Data([0xfb, 0xff])), "-_8")
        XCTAssertEqual(Base64URL.decode("-_8"), Data([0xfb, 0xff]))
    }

    func testFixedLength() throws {
        XCTAssertEqual(try DerSignature.fixedLength([0] + [UInt8](repeating: 9, count: 32)).count, 32)
        XCTAssertEqual(try DerSignature.fixedLength([5]).last, 5)
    }
}
