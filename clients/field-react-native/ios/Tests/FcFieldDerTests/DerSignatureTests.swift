import XCTest
import Security
@testable import FcFieldDer

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

    func testRoundTripWithRealKeychainSignatures() throws {
        let attrs: [String: Any] = [kSecAttrKeyType as String: kSecAttrKeyTypeECSECPrimeRandom,
                                    kSecAttrKeySizeInBits as String: 256]
        var err: Unmanaged<CFError>?
        let key = try XCTUnwrap(SecKeyCreateRandomKey(attrs as CFDictionary, &err))
        for n in 0..<200 {
            let data = Data([UInt8(n & 0xff)])
            let der = try XCTUnwrap(SecKeyCreateSignature(key, .ecdsaSignatureMessageX962SHA256, data as CFData, &err)) as Data
            let raw = try DerSignature.derToRaw([UInt8](der))
            XCTAssertEqual(raw.count, 64)
            XCTAssertEqual(try DerSignature.rawToDer(raw), [UInt8](der))
            let pub = try XCTUnwrap(SecKeyCopyPublicKey(key))
            XCTAssertTrue(SecKeyVerifySignature(pub, .ecdsaSignatureMessageX962SHA256, data as CFData, Data(try DerSignature.rawToDer(raw)) as CFData, &err))
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
    }

    func testFixedLength() throws {
        XCTAssertEqual(try DerSignature.fixedLength([0] + [UInt8](repeating: 9, count: 32)).count, 32)
        XCTAssertEqual(try DerSignature.fixedLength([5]).last, 5)
    }
}
