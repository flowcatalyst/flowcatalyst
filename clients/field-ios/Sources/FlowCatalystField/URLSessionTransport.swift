#if canImport(Darwin)
import Foundation
import FlowCatalystFieldCore

/// The HTTP transport over `URLSession` (Kotlin: the OkHttp client). The default session is ephemeral (no cookies, no
/// cache, no credential storage) with a 30 s idle timeout, like the Android client's read timeout.
public final class URLSessionTransport: HTTPTransport, @unchecked Sendable {
    private let session: URLSession

    public init(session: URLSession? = nil) { self.session = session ?? URLSessionTransport.makeSession() }

    public static func makeSession() -> URLSession {
        let c = URLSessionConfiguration.ephemeral
        c.timeoutIntervalForRequest = 30
        c.timeoutIntervalForResource = 120
        c.httpShouldSetCookies = false
        c.httpCookieStorage = nil
        c.urlCache = nil
        c.requestCachePolicy = .reloadIgnoringLocalCacheData
        return URLSession(configuration: c)
    }

    public func send(_ request: HTTPRequest) async throws -> HTTPResponse {
        guard let url = URL(string: request.url) else { throw URLError(.badURL) }
        var r = URLRequest(url: url)
        r.httpMethod = request.method
        for (k, v) in request.headers { r.setValue(v, forHTTPHeaderField: k) }
        r.httpBody = request.body
        do {
            let (data, response) = try await session.data(for: r)
            guard let http = response as? HTTPURLResponse else { throw URLError(.badServerResponse) }
            var headers: [String: String] = [:]
            for (k, v) in http.allHeaderFields { headers[String(describing: k)] = String(describing: v) }
            return HTTPResponse(status: http.statusCode, headers: headers, body: data)
        } catch let e as URLError where e.code == .cancelled && Task.isCancelled {
            throw CancellationError()
        }
    }
}
#endif
