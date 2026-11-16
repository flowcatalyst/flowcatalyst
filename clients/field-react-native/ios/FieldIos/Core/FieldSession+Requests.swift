import Foundation

extension FieldSession {
    /// Authorises and performs one request to your backend over `transport` (default: the session's own). Adds
    /// `Authorization: DPoP` and a proof to the request (Kotlin `FieldInterceptor`), then:
    ///
    /// - retries once on `401 use_dpop_nonce` (with the nonce from that response's `DPoP-Nonce`);
    /// - on `401 invalid_token` refreshes (exactly one refresh request however many callers need one) and retries once;
    ///   an `invalid_token` that survives a refresh emits `sessionEnded("invalid_token")` and the 401 is returned;
    /// - a refresh that ends the session (`INVALID_GRANT`, `SESSION_EXPIRED`, `DEVICE_REVOKED`, `DEVICE_SUSPENDED`,
    ///   `UNKNOWN_DEVICE`, `NO_APP_ACCESS`) surfaces as `FieldAuthError` after `sessionEnded` was emitted.
    ///
    /// Set `oneShot` for a body that cannot be sent twice: it is then never retried.
    public func perform(_ request: HTTPRequest, using transport: HTTPTransport? = nil, oneShot: Bool = false) async throws -> HTTPResponse {
        let t = transport ?? self.transport
        var nonceRetried = false
        var refreshRetried = false
        while true {
            let signed = try await signedFor(method: request.method, url: request.url)
            var req = request
            req.setHeader("Authorization", "DPoP \(signed.accessToken)")
            req.setHeader("DPoP", signed.proof)
            let resp = try await t.send(req)
            if let n = resp.header("DPoP-Nonce") { noteNonce(url: request.url, nonce: n) }
            if resp.status != 401 { return resp }
            let www = resp.header("WWW-Authenticate") ?? ""
            if www.contains("use_dpop_nonce") && !nonceRetried && !oneShot {
                nonceRetried = true
                continue
            }
            if www.contains("invalid_token") {
                if !refreshRetried && !oneShot {
                    refreshRetried = true
                    try await refreshIfStale(signed.accessToken)
                    continue
                }
                emitSessionEnded("invalid_token")
            }
            return resp
        }
    }
}
