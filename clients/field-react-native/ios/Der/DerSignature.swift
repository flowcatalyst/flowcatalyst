import Foundation

public enum DerError: Error { case malformed }

/// Pure helpers (Foundation only) so they are unit-testable on the host.
public enum DerSignature {
    /// ASN.1 DER ECDSA signature -> raw r‖s, each left-padded to `size` bytes.
    public static func derToRaw(_ der: [UInt8], size: Int = 32) throws -> [UInt8] {
        var i = 0
        func need(_ n: Int) throws { if n < 0 || i + n > der.count { throw DerError.malformed } }
        func readLen() throws -> Int {
            try need(1)
            let b = Int(der[i]); i += 1
            if b < 0x80 { return b }
            let n = b & 0x7f
            if n == 0 || n > 2 { throw DerError.malformed }
            try need(n)
            var len = 0
            for _ in 0..<n { len = (len << 8) | Int(der[i]); i += 1 }
            return len
        }
        try need(1)
        guard der[i] == 0x30 else { throw DerError.malformed }
        i += 1
        let seqLen = try readLen()
        guard i + seqLen == der.count else { throw DerError.malformed }
        func readInt() throws -> [UInt8] {
            try need(1)
            guard der[i] == 0x02 else { throw DerError.malformed }
            i += 1
            let len = try readLen()
            try need(len)
            var start = i
            let end = i + len
            i = end
            while start < end - 1 && der[start] == 0 { start += 1 }
            let n = end - start
            if n > size || n <= 0 { throw DerError.malformed }
            return [UInt8](repeating: 0, count: size - n) + der[start..<end]
        }
        let r = try readInt()
        let s = try readInt()
        guard i == der.count else { throw DerError.malformed }
        return r + s
    }

    public static func rawToDer(_ raw: [UInt8]) throws -> [UInt8] {
        guard !raw.isEmpty, raw.count % 2 == 0 else { throw DerError.malformed }
        let half = raw.count / 2
        func enc(_ v: ArraySlice<UInt8>) -> [UInt8] {
            var body = Array(v.drop(while: { $0 == 0 }))
            if body.isEmpty { body = [0] }
            if body[0] & 0x80 != 0 { body.insert(0, at: 0) }
            return [0x02, UInt8(body.count)] + body
        }
        let seq = enc(raw[0..<half]) + enc(raw[half...])
        let hdr: [UInt8] = seq.count < 0x80 ? [0x30, UInt8(seq.count)] : [0x30, 0x81, UInt8(seq.count)]
        return hdr + seq
    }

    /// Left-pad (or strip a leading zero) to exactly `size` bytes.
    public static func fixedLength(_ v: [UInt8], size: Int = 32) throws -> [UInt8] {
        var s = v[...]
        while s.count > size && s.first == 0 { s = s.dropFirst() }
        guard s.count <= size else { throw DerError.malformed }
        return [UInt8](repeating: 0, count: size - s.count) + s
    }

    public static func toBase64Url(_ b: [UInt8]) -> String {
        Data(b).base64EncodedString()
            .replacingOccurrences(of: "+", with: "-")
            .replacingOccurrences(of: "/", with: "_")
            .replacingOccurrences(of: "=", with: "")
    }

    public static func fromBase64Url(_ s: String) -> [UInt8]? {
        var t = s.replacingOccurrences(of: "-", with: "+").replacingOccurrences(of: "_", with: "/")
        while t.count % 4 != 0 { t += "=" }
        return Data(base64Encoded: t).map { [UInt8]($0) }
    }
}
