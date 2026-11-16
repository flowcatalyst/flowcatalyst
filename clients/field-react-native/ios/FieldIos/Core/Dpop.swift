import Foundation
#if canImport(CryptoKit)
import CryptoKit
#else
import Crypto
#endif

/// DPoP proof construction (RFC 9449 shape), ES256 through the key provider.
enum Dpop {
    static func b64u(_ d: Data) -> String { Base64URL.encode(d) }

    /// base64url(SHA-256(token)), the proof's `ath`.
    static func accessTokenHash(_ token: String) -> String { b64u(Data(SHA256.hash(data: Data(token.utf8)))) }

    static func randomId(bytes: Int = 16) -> String {
        var g = SystemRandomNumberGenerator()
        return b64u(Data((0..<bytes).map { _ in UInt8.random(in: 0...255, using: &g) }))
    }

    /// Builds an ES256 DPoP proof signed through the key provider. Throws `FieldAuthError` INTERNAL when the
    /// provider returns a malformed signature; provider errors propagate as thrown.
    static func build(
        keys: DeviceKeyProvider,
        key: DeviceKey,
        htm: String,
        htu: String,
        nonce: String?,
        accessToken: String?,
        iatSeconds: Int64
    ) throws -> String {
        let header: JObject = [
            "typ": "dpop+jwt",
            "alg": "ES256",
            "jwk": ["kty": "EC", "crv": "P-256", "x": key.x, "y": key.y] as JObject,
        ]
        var claims: JObject = ["htm": htm, "htu": htu, "iat": iatSeconds, "jti": randomId(bytes: 16)]
        if let nonce, !nonce.isEmpty { claims["nonce"] = nonce }
        if let accessToken, !accessToken.isEmpty { claims["ath"] = accessTokenHash(accessToken) }
        let input = b64u(JSON.data(header)) + "." + b64u(JSON.data(claims))
        let sig = try keys.sign(Data(input.utf8))
        if sig.count != 64 { throw FieldAuthError(code: "INTERNAL", message: "the key provider returned a malformed signature") }
        return input + "." + b64u(sig)
    }

    /// RFC 7638 thumbprint of an EC P-256 public key.
    static func thumbprint(x: String, y: String) -> String {
        let canonical = "{\"crv\":\"P-256\",\"kty\":\"EC\",\"x\":\"\(x)\",\"y\":\"\(y)\"}"
        return b64u(Data(SHA256.hash(data: Data(canonical.utf8))))
    }
}
