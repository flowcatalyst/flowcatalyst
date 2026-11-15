import Foundation

/// The subset of an http(s) URL the library needs (Kotlin uses OkHttp's `HttpUrl`).
struct HTTPURL {
    let scheme: String
    let host: String
    let port: Int
    /// Percent-encoded, dot segments removed, never empty.
    let encodedPath: String

    init?(_ string: String) {
        guard let c = URLComponents(string: string.trimmingCharacters(in: .whitespacesAndNewlines)),
              let scheme = c.scheme?.lowercased(), scheme == "http" || scheme == "https",
              var host = c.host?.lowercased(), !host.isEmpty
        else { return nil }
        if host.hasPrefix("[") && host.hasSuffix("]") { host = String(host.dropFirst().dropLast()) }
        let port = c.port ?? (scheme == "https" ? 443 : 80)
        guard (1...65535).contains(port) else { return nil }
        self.scheme = scheme
        self.host = host
        self.port = port
        self.encodedPath = HTTPURL.removeDotSegments(c.percentEncodedPath)
    }

    /// `scheme://host:port`, port always explicit (the nonce cache key and the origin policy).
    var origin: String { "\(scheme)://\(host):\(port)" }

    private var hostForDisplay: String { host.contains(":") ? "[\(host)]" : host }
    private var defaultPort: Int { scheme == "https" ? 443 : 80 }

    /// The DPoP `htu`: the request URL without query or fragment.
    var htu: String {
        "\(scheme)://\(hostForDisplay)\(port == defaultPort ? "" : ":\(port)")\(encodedPath)"
    }

    private static func removeDotSegments(_ path: String) -> String {
        if path.isEmpty { return "/" }
        var out: [Substring] = []
        let segs = path.split(separator: "/", omittingEmptySubsequences: false)
        for (i, seg) in segs.enumerated() {
            if seg == "." {
                if i == segs.count - 1 { out.append("") }
            } else if seg == ".." {
                if out.count > 1 { out.removeLast() }
                if i == segs.count - 1 { out.append("") }
            } else {
                out.append(seg)
            }
        }
        let joined = out.joined(separator: "/")
        return joined.hasPrefix("/") ? joined : "/" + joined
    }
}
