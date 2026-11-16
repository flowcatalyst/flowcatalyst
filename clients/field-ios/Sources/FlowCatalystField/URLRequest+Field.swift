#if canImport(Darwin)
import Foundation
#if canImport(FlowCatalystFieldCore)
import FlowCatalystFieldCore
#endif

extension FieldSession {
    /// A copy of `request` with `Authorization: DPoP` and a `DPoP` proof for its method and URL (Kotlin `headersFor`).
    /// Refreshes first when the access token is about to expire. A proof is bound to one method, one URL and one
    /// nonce: sign when you send, never when you queue. For uploads from a file or a stream, or any transport that is
    /// not `URLSession.data(for:)`, use this and `noteNonce` / `headersForRetry` yourself.
    public func authorize(_ request: URLRequest) async throws -> URLRequest {
        guard let url = request.url?.absoluteString else { throw FieldAuthError(code: "INVALID_REQUEST", message: "not a valid URL") }
        let h = try await headersFor(method: request.httpMethod ?? "GET", url: url)
        var out = request
        for (k, v) in h { out.setValue(v, forHTTPHeaderField: k) }
        return out
    }

    /// Authorises and performs `request` with `URLSession` (Kotlin: the OkHttp `FieldInterceptor`; iOS has no
    /// interceptor chain, so the retry loop is a function). Retries once on `401 use_dpop_nonce` and once on
    /// `401 invalid_token` after a refresh; see `FieldSession.perform(_:using:oneShot:)` for the exact rules.
    /// A body given as `httpBodyStream` is read into memory first and then treated as one-shot (never retried).
    /// Throws `FieldAuthError`, or the transport's own error when the app's server cannot be reached.
    public func perform(_ request: URLRequest, session: URLSession? = nil) async throws -> (Data, HTTPURLResponse) {
        guard let url = request.url else { throw FieldAuthError(code: "INVALID_REQUEST", message: "not a valid URL") }
        var body = request.httpBody
        var oneShot = false
        if let stream = request.httpBodyStream {
            body = Self.drain(stream)
            oneShot = true
        }
        let core = HTTPRequest(
            url: url.absoluteString, method: request.httpMethod ?? "GET",
            headers: request.allHTTPHeaderFields ?? [:], body: body)
        let res = try await perform(core, using: URLSessionTransport(session: session), oneShot: oneShot)
        guard let http = HTTPURLResponse(url: url, statusCode: res.status, httpVersion: "HTTP/1.1", headerFields: res.headers) else {
            throw FieldAuthError(code: "INTERNAL", message: "malformed response")
        }
        return (res.body, http)
    }

    private static func drain(_ stream: InputStream) -> Data {
        stream.open()
        defer { stream.close() }
        var data = Data()
        var buf = [UInt8](repeating: 0, count: 16 * 1024)
        while stream.hasBytesAvailable {
            let n = stream.read(&buf, maxLength: buf.count)
            if n <= 0 { break }
            data.append(buf, count: n)
        }
        return data
    }
}
#endif
