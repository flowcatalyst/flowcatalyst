package io.flowcatalyst.ingestor.functions.context;

import io.flowcatalyst.framework.httpclient.ClientResponse;
import io.flowcatalyst.framework.httpclient.HttpClientException;
import io.flowcatalyst.framework.httpclient.RequestBuilder;
import io.flowcatalyst.framework.httpclient.RequestMethod;
import io.flowcatalyst.framework.httpclient.ServiceClient;
import io.flowcatalyst.function.HttpCall;
import io.flowcatalyst.function.HttpCallRefusedException;
import io.flowcatalyst.function.HttpCaller;
import io.flowcatalyst.function.HttpReply;
import io.flowcatalyst.function.HttpResponseTooLargeException;
import java.io.IOException;
import java.net.ConnectException;
import java.net.URI;
import java.net.URLDecoder;
import java.net.http.HttpTimeoutException;
import java.nio.charset.StandardCharsets;
import java.time.Clock;
import java.time.Duration;
import java.util.List;
import java.util.Locale;
import java.util.Map;
import java.util.Objects;

/// [HttpCaller] over the framework's HTTP client (so a function's outbound calls are the ingestor's own
/// observable calls, see [OutboundClients]): the target host must be on `describe.httpAllow`
/// ([HttpAllowlist]); `https` only, except a loopback host; redirects are never followed (the function
/// sees the 3xx itself); the request timeout is `min(call.timeout() or [#DEFAULT_CALL_TIMEOUT], time left
/// before the invocation's own deadline)` ([InvocationDeadline]); a response over 16 MiB fails the call
/// ([HttpResponseTooLargeException]). Methods are the framework client's: GET, HEAD, POST, PUT, PATCH, DELETE
/// and OPTIONS.
public final class AllowlistHttpCaller implements HttpCaller {

    /// The host's own default call timeout, used when [HttpCall#timeout()] is `null` — generous enough never to
    /// be the binding constraint in practice; the invocation deadline is almost always the tighter of the two.
    public static final Duration DEFAULT_CALL_TIMEOUT = Duration.ofSeconds(30);

    private final OutboundClients clients;
    private final HttpAllowlist allowlist;
    private final Clock clock;

    public AllowlistHttpCaller(OutboundClients clients, List<String> httpAllow, Clock clock) {
        this.clients = Objects.requireNonNull(clients, "clients");
        this.allowlist = new HttpAllowlist(httpAllow);
        this.clock = Objects.requireNonNull(clock, "clock");
    }

    @Override
    public HttpReply send(HttpCall call) throws Exception {
        Objects.requireNonNull(call, "call");
        URI uri;
        try {
            uri = URI.create(call.url());
        } catch (RuntimeException e) {
            throw new HttpCallRefusedException(call.url(), "not a valid URL");
        }
        String host = uri.getHost();
        String scheme = uri.getScheme();
        boolean loopback = HttpAllowlist.isLoopback(host);
        if (!"https".equalsIgnoreCase(scheme) && !(loopback && "http".equalsIgnoreCase(scheme))) {
            throw new HttpCallRefusedException(String.valueOf(host),
                    "scheme '" + scheme + "' is not permitted (https only, except loopback)");
        }
        if (!allowlist.allows(host)) {
            throw new HttpCallRefusedException(String.valueOf(host), "not on this function's httpAllow list");
        }
        RequestMethod method;
        try {
            method = RequestMethod.valueOf(call.method().toUpperCase(Locale.ROOT));
        } catch (IllegalArgumentException e) {
            throw new HttpCallRefusedException(String.valueOf(host), "method '" + call.method() + "' is not supported");
        }

        Duration remaining = InvocationDeadline.remaining(clock);
        Duration ceiling = call.timeout() != null ? call.timeout() : DEFAULT_CALL_TIMEOUT;
        Duration timeout = remaining.compareTo(ceiling) < 0 ? remaining : ceiling;
        if (timeout.isZero() || timeout.isNegative()) {
            throw new HttpCallRefusedException(String.valueOf(host), "no time left before the invocation deadline");
        }

        if (uri.getRawUserInfo() != null) {
            throw new HttpCallRefusedException(String.valueOf(host), "credentials in the URL are not permitted");
        }
        ServiceClient client;
        try {
            client = clients.forOrigin(URI.create(scheme.toLowerCase(Locale.ROOT) + "://" + uri.getRawAuthority()));
        } catch (IllegalArgumentException e) {
            throw new HttpCallRefusedException(String.valueOf(host), "the URL is not allowed");
        }
        RequestBuilder request;
        try {
            String path = uri.getRawPath() == null || uri.getRawPath().isEmpty() ? "/" : uri.getRawPath();
            request = client.request(method, path).retryable(false).timeout(timeout);
            String rawQuery = uri.getRawQuery();
            if (rawQuery != null && !rawQuery.isEmpty()) {
                for (String pair : rawQuery.split("&")) {
                    if (pair.isEmpty()) {
                        continue;
                    }
                    int eq = pair.indexOf('=');
                    request.query(
                            URLDecoder.decode(eq < 0 ? pair : pair.substring(0, eq), StandardCharsets.UTF_8),
                            eq < 0 ? "" : URLDecoder.decode(pair.substring(eq + 1), StandardCharsets.UTF_8));
                }
            }
            String contentType = null;
            for (Map.Entry<String, List<String>> header : call.headers().entrySet()) {
                if (header.getKey().equalsIgnoreCase("Content-Type") && !header.getValue().isEmpty()) {
                    contentType = header.getValue().get(0);
                    continue;
                }
                for (String value : header.getValue()) {
                    request.header(header.getKey(), value);
                }
            }
            byte[] body = call.body();
            if (body.length > 0) {
                request.body(body, contentType == null ? "application/octet-stream" : contentType);
            } else if (contentType != null) {
                request.header("Content-Type", contentType);
            }
        } catch (IllegalArgumentException e) {
            // A header or path the client refuses (Host, Content-Length, a dot segment, ...).
            throw new HttpCallRefusedException(String.valueOf(host), "the request is not allowed: " + e.getMessage());
        }

        ClientResponse response;
        try {
            response = request.send();
        } catch (HttpClientException e) {
            switch (e.kind()) {
                case RESPONSE_TOO_LARGE -> throw new HttpResponseTooLargeException(String.valueOf(host));
                case INTERRUPTED -> {
                    Thread.currentThread().interrupt();
                    throw new IOException("interrupted", e);
                }
                // The JDK's own exception types, as a function that used java.net.http would catch them.
                case TIMEOUT -> throw new HttpTimeoutException(e.getMessage());
                case CONNECT -> throw new ConnectException(e.getMessage());
                default -> throw new IOException(e.getMessage(), e);
            }
        }
        return new HttpReply(response.status(), response.headers(), response.bodyBytes());
    }
}
