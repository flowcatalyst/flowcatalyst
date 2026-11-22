package io.flowcatalyst.sdk.http;

import tools.jackson.core.JacksonException;
import tools.jackson.databind.JavaType;
import tools.jackson.databind.JsonNode;
import tools.jackson.databind.ObjectMapper;
import io.flowcatalyst.sdk.auth.ClientCredentialsTokenManager;
import io.flowcatalyst.sdk.auth.TokenProvider;
import io.flowcatalyst.sdk.error.FlowCatalystException;
import io.flowcatalyst.sdk.error.SdkError;
import java.io.IOException;
import java.net.ConnectException;
import java.net.NoRouteToHostException;
import java.net.URI;
import java.net.URLEncoder;
import java.net.UnknownHostException;
import java.net.http.HttpClient;
import java.net.http.HttpConnectTimeoutException;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.net.http.HttpTimeoutException;
import java.nio.channels.UnresolvedAddressException;
import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.util.Collection;
import java.util.Collections;
import java.util.IdentityHashMap;
import java.util.LinkedHashMap;
import java.util.Locale;
import java.util.Map;
import java.util.Set;
import javax.net.ssl.SSLHandshakeException;

/**
 * Authenticated HTTP execution shared by all resources: bearer injection,
 * exponential-backoff retry, a one-shot token refresh on 401
 * (client-credentials mode only), and typed error mapping. Blocking by
 * design — run on virtual threads to scale.
 *
 * <h2>The retry rule</h2>
 *
 * A request is <em>repeatable</em> when its method is GET, HEAD, OPTIONS, PUT or
 * DELETE, or when the caller marks it repeatable for that one call ({@link
 * #postRepeatable}, or the {@code repeatable} argument of {@link #exchange} and
 * {@link #rawAuthenticated}). Then:
 *
 * <ul>
 *   <li>408 and 429 (refused before processing) are retried for every request;
 *   <li>502, 503 and 504 are retried for a repeatable request; for any other the
 *       platform may have applied it, so it is not sent again and the call fails
 *       with {@link SdkError.OutcomeUnknown};
 *   <li>a transport failure before anything was sent (DNS, connection refused,
 *       connect timeout, TLS handshake) is retried for every request;
 *   <li>a transport failure after the request may have been sent (reset, EOF,
 *       the request timeout) is retried for a repeatable request, and is {@link
 *       SdkError.OutcomeUnknown} for any other;
 *   <li>a DELETE that answers 404 on a retry after an attempt with an unknown
 *       outcome succeeds with no body (the first attempt most likely deleted it);
 *   <li>an interrupt is never retried.
 * </ul>
 *
 * {@code retryAttempts} counts retries after the first request, with {@code
 * retryDelay} doubling each time; 0 turns retries off.
 */
public final class Transport {

    /** Refused before the platform processed the request: safe to send again whatever it is. */
    private static final Set<Integer> REFUSED_BEFORE_PROCESSING = Set.of(408, 429);

    /** The gateway lost the answer, or the platform was unavailable: the request may have been applied. */
    private static final Set<Integer> GATEWAY_OUTCOME_UNKNOWN = Set.of(502, 503, 504);

    /** Methods whose repeat leaves the same state (RFC 9110 §9.2.2). */
    private static final Set<String> REPEATABLE_METHODS = Set.of("GET", "HEAD", "OPTIONS", "PUT", "DELETE");

    private final HttpClient http;
    private final ObjectMapper mapper = Json.newMapper();
    private final String baseUrl;
    private final TokenProvider tokenProvider;
    private final ClientCredentialsTokenManager tokenManager; // null in user-token mode
    private final Duration timeout;
    private final int retryAttempts;
    private final Duration retryDelay;

    public Transport(
            String baseUrl,
            TokenProvider tokenProvider,
            ClientCredentialsTokenManager tokenManager,
            Duration timeout,
            int retryAttempts,
            Duration retryDelay) {
        this.baseUrl = baseUrl;
        this.tokenProvider = tokenProvider;
        this.tokenManager = tokenManager;
        this.timeout = timeout;
        this.retryAttempts = retryAttempts;
        this.retryDelay = retryDelay;
        this.http = HttpClient.newBuilder().connectTimeout(timeout).build();
    }

    public ObjectMapper mapper() {
        return mapper;
    }

    /**
     * A request body of bytes, sent as they are with their own content type (an image) instead of
     * being serialised as JSON.
     */
    public record Bytes(byte[] data, String contentType) {}

    /**
     * What an endpoint that answers bytes returns, as the response type: a 2xx keeps the body, its
     * content type and its ETag; a 304 (to a request that sent If-None-Match) is a success with
     * {@code notModified} true and no bytes.
     */
    public record Binary(byte[] data, String contentType, String etag, boolean notModified) {}

    /** What comes back from one attempt: the body is kept as bytes, and read as text where a text is meant. */
    private record Reply(int statusCode, byte[] bytes, java.net.http.HttpHeaders headers) {
        String body() {
            return new String(bytes, StandardCharsets.UTF_8);
        }
    }

    // ── Convenience methods used by resources ───────────────────────

    public <T> T get(String path, Map<String, Object> query, Class<T> type) {
        return execute("GET", baseUrl + path, query, null, javaType(type), true);
    }

    public <T> T get(String path, Map<String, Object> query, JavaType type) {
        return execute("GET", baseUrl + path, query, null, type, true);
    }

    public <T> T post(String path, Object body, Class<T> type) {
        return execute("POST", baseUrl + path, null, body, javaType(type), true);
    }

    public <T> T post(String path, Map<String, Object> query, Object body, Class<T> type) {
        return execute("POST", baseUrl + path, query, body, javaType(type), true);
    }

    /**
     * A POST that may be sent again after a gateway status (502/503/504) or a
     * transport failure after sending, because a repeat leaves the same state
     * (a declarative sync, a read-only POST, a batch whose every item carries an
     * idempotency key). Mark only such a call: an ordinary {@link #post} is not
     * repeated, and fails with {@link SdkError.OutcomeUnknown} instead.
     */
    public <T> T postRepeatable(String path, Object body, Class<T> type) {
        return execute("POST", baseUrl + path, null, body, javaType(type), true, null, true);
    }

    /** {@link #postRepeatable(String, Object, Class)} with a query string. */
    public <T> T postRepeatable(String path, Map<String, Object> query, Object body, Class<T> type) {
        return execute("POST", baseUrl + path, query, body, javaType(type), true, null, true);
    }

    public <T> T put(String path, Object body, Class<T> type) {
        return execute("PUT", baseUrl + path, null, body, javaType(type), true);
    }

    public <T> T put(String path, Map<String, Object> query, Object body, Class<T> type) {
        return execute("PUT", baseUrl + path, query, body, javaType(type), true);
    }

    /**
     * A GET whose answer is bytes ({@link Binary}); {@code headers} (a null value is not sent) carry an
     * {@code If-None-Match}.
     */
    public Binary getBinary(String path, Map<String, Object> query, Map<String, String> headers) {
        return execute("GET", baseUrl + path, query, null, javaType(Binary.class), true, headers, false);
    }

    public <T> T delete(String path, Class<T> type) {
        return execute("DELETE", baseUrl + path, null, null, javaType(type), true);
    }

    /**
     * A platform call that also sends {@code headers} (a null value is not sent), for the few
     * operations that read a request header such as {@code X-On-Behalf-Of}.
     */
    public <T> T exchange(
            String method,
            String path,
            Map<String, Object> query,
            Object body,
            Map<String, String> headers,
            Class<T> type) {
        return execute(method, baseUrl + path, query, body, javaType(type), true, headers, false);
    }

    /**
     * {@link #exchange(String, String, Map, Object, Map, Class)} with an explicit
     * mark: {@code repeatable} true lets a POST (or another method that is not
     * repeatable by itself) be sent again on 502/503/504 or a transport failure
     * after sending. Set it only for a call whose repeat leaves the same state.
     * GET, HEAD, OPTIONS, PUT and DELETE are repeatable whatever it says.
     */
    public <T> T exchange(
            String method,
            String path,
            Map<String, Object> query,
            Object body,
            Map<String, String> headers,
            Class<T> type,
            boolean repeatable) {
        return execute(method, baseUrl + path, query, body, javaType(type), true, headers, repeatable);
    }

    /**
     * Authenticated request against an absolute URL: the message router, a
     * separate process that verifies the same platform bearer token (spec
     * router-api-auth.md rule 8). Same one-shot refresh on 401 as the platform
     * calls.
     */
    public <T> T rawAuthenticated(
            String method, String url, Map<String, Object> query, Object body, JavaType type) {
        return execute(method, url, query, body, type, true);
    }

    /**
     * {@link #rawAuthenticated(String, String, Map, Object, JavaType)} with an
     * explicit {@code repeatable} mark, as for {@link #exchange(String, String,
     * Map, Object, Map, Class, boolean)}.
     */
    public <T> T rawAuthenticated(
            String method,
            String url,
            Map<String, Object> query,
            Object body,
            JavaType type,
            boolean repeatable) {
        return execute(method, url, query, body, type, true, null, repeatable);
    }

    /** URL-encode a path segment. */
    public static String enc(String segment) {
        return URLEncoder.encode(segment, StandardCharsets.UTF_8).replace("+", "%20");
    }

    private JavaType javaType(Class<?> type) {
        return type == null ? null : mapper.getTypeFactory().constructType(type);
    }

    public JavaType listOf(Class<?> element) {
        return mapper.getTypeFactory().constructCollectionType(java.util.List.class, element);
    }

    public JavaType mapOf(Class<?> key, Class<?> value) {
        return mapper.getTypeFactory().constructMapType(java.util.Map.class, key, value);
    }

    // ── Core execution ──────────────────────────────────────────────

    private <T> T execute(
            String method,
            String url,
            Map<String, Object> query,
            Object body,
            JavaType responseType,
            boolean authenticated) {
        return execute(method, url, query, body, responseType, authenticated, null, false);
    }

    @SuppressWarnings("unchecked")
    private <T> T execute(
            String method,
            String url,
            Map<String, Object> query,
            Object body,
            JavaType responseType,
            boolean authenticated,
            Map<String, String> headers,
            boolean markedRepeatable) {
        String fullUrl = url + queryString(query);
        String verb = method.toUpperCase(Locale.ROOT);
        boolean repeatable = markedRepeatable || REPEATABLE_METHODS.contains(verb);
        String token = null;
        boolean canRefreshToken = authenticated && tokenManager != null;
        // An earlier attempt of this call may have been applied (a gateway status or a
        // transport failure after sending), so a 404 to a DELETE now means it is gone.
        boolean earlierOutcomeUnknown = false;

        if (authenticated) {
            token = acquireToken();
        }

        int attempt = 0;
        while (true) {
            Reply response;
            try {
                response = send(method, fullUrl, body, token, headers);
            } catch (InterruptedException e) {
                // Never retried: the caller asked this thread to stop.
                Thread.currentThread().interrupt();
                throw new FlowCatalystException(new SdkError.Network("Request interrupted", e));
            } catch (IOException e) {
                boolean nothingSent = nothingSent(e);
                if (attempt < retryAttempts && (nothingSent || repeatable)) {
                    earlierOutcomeUnknown |= !nothingSent;
                    sleep(retryDelay.multipliedBy(1L << attempt));
                    attempt++;
                    continue;
                }
                if (nothingSent || repeatable) {
                    throw transportError(e);
                }
                throw new FlowCatalystException(new SdkError.OutcomeUnknown(
                        verb + " " + fullUrl + " failed after the request may have been sent ("
                                + describe(e) + "): it may have been applied and was not repeated",
                        verb, fullUrl, null, null, e));
            }
            int status = response.statusCode();

            boolean binary = responseType != null && responseType.getRawClass() == Binary.class;
            if (binary && status == 304) {
                return (T) new Binary(null, null, response.headers().firstValue("ETag").orElse(null), true);
            }
            if (status >= 200 && status < 300) {
                if (binary) {
                    return (T) new Binary(
                            response.bytes(),
                            response.headers().firstValue("Content-Type").orElse(null),
                            response.headers().firstValue("ETag").orElse(null),
                            false);
                }
                return (T) parseResponse(response.body(), status, responseType);
            }

            // One-shot transparent refresh on 401 (client-credentials mode only).
            if (status == 401 && attempt == 0 && canRefreshToken) {
                token = tokenManager.refreshToken();
                attempt++;
                continue;
            }

            if (status == 404 && "DELETE".equals(verb) && earlierOutcomeUnknown) {
                return null;
            }

            if (REFUSED_BEFORE_PROCESSING.contains(status) && attempt < retryAttempts) {
                sleep(retryDelay.multipliedBy(1L << attempt));
                attempt++;
                continue;
            }

            if (GATEWAY_OUTCOME_UNKNOWN.contains(status)) {
                if (!repeatable) {
                    throw new FlowCatalystException(new SdkError.OutcomeUnknown(
                            verb + " " + fullUrl + " answered " + status
                                    + ": the request may have been applied and was not repeated",
                            verb, fullUrl, status, response.body(), null));
                }
                if (attempt < retryAttempts) {
                    earlierOutcomeUnknown = true;
                    sleep(retryDelay.multipliedBy(1L << attempt));
                    attempt++;
                    continue;
                }
            }

            throw new FlowCatalystException(
                    SdkError.fromHttpStatus(status, parseQuietly(response.body()), response.body()));
        }
    }

    /**
     * Whether {@code e} happened before any byte of the request left: DNS, a
     * refused or unroutable connection, a connect timeout, a TLS handshake
     * failure. The JDK client rethrows the failure of its exchange as a new
     * exception of the same kind with the original as its cause, so the whole
     * chain is searched. Anything else (a reset, EOF, "header parser received
     * no bytes", the request timeout) may have come after the request was sent.
     */
    static boolean nothingSent(Throwable e) {
        Set<Throwable> seen = Collections.newSetFromMap(new IdentityHashMap<>());
        for (Throwable t = e; t != null && seen.add(t); t = t.getCause()) {
            if (t instanceof HttpConnectTimeoutException
                    || t instanceof ConnectException
                    || t instanceof NoRouteToHostException
                    || t instanceof UnknownHostException
                    || t instanceof UnresolvedAddressException
                    || t instanceof SSLHandshakeException) {
                return true;
            }
        }
        return false;
    }

    private FlowCatalystException transportError(IOException e) {
        if (e instanceof HttpTimeoutException) {
            return new FlowCatalystException(new SdkError.Timeout(
                    "Request timed out after " + timeout.toMillis() + "ms", timeout));
        }
        return new FlowCatalystException(new SdkError.Network(
                e.getMessage() != null ? e.getMessage() : "Network error", e));
    }

    private static String describe(Throwable e) {
        return e.getMessage() == null
                ? e.getClass().getSimpleName()
                : e.getClass().getSimpleName() + ": " + e.getMessage();
    }

    private String acquireToken() {
        try {
            return tokenProvider.getToken();
        } catch (FlowCatalystException e) {
            throw e;
        } catch (RuntimeException e) {
            throw new FlowCatalystException(new SdkError.TokenExpired(
                    e.getMessage() != null ? e.getMessage() : "Failed to get access token"));
        }
    }

    private Reply send(
            String method, String url, Object body, String token, Map<String, String> headers)
            throws IOException, InterruptedException {
        HttpRequest.Builder builder = HttpRequest.newBuilder(URI.create(url))
                .timeout(timeout)
                .header("Accept", headers != null && headers.containsKey("Accept") ? headers.get("Accept") : "application/json");
        if (token != null) {
            builder.header("Authorization", "Bearer " + token);
        }
        if (headers != null) {
            headers.forEach((name, value) -> {
                if (value != null && !name.equalsIgnoreCase("Accept")) builder.header(name, value);
            });
        }
        if (body instanceof Bytes raw) {
            builder.header("Content-Type", raw.contentType());
            builder.method(method, HttpRequest.BodyPublishers.ofByteArray(raw.data()));
        } else if (body != null) {
            builder.header("Content-Type", "application/json");
            builder.method(method, HttpRequest.BodyPublishers.ofString(serialize(body)));
        } else {
            builder.method(method, HttpRequest.BodyPublishers.noBody());
        }
        HttpResponse<byte[]> res = http.send(builder.build(), HttpResponse.BodyHandlers.ofByteArray());
        return new Reply(res.statusCode(), res.body(), res.headers());
    }

    private String serialize(Object body) {
        try {
            return mapper.writeValueAsString(body);
        } catch (JacksonException e) {
            throw new FlowCatalystException(
                    new SdkError.Network("Failed to serialize request body: " + e.getMessage(), e));
        }
    }

    private Object parseResponse(String body, int status, JavaType responseType) {
        if (responseType == null || status == 204 || body == null || body.isEmpty()) {
            return null;
        }
        // A String response is the body as it came (a YAML file), not a JSON string.
        if (responseType.getRawClass() == String.class) {
            return body;
        }
        try {
            return mapper.readValue(body, responseType);
        } catch (JacksonException e) {
            throw new FlowCatalystException(
                    new SdkError.Network("Failed to parse response: " + e.getMessage(), e));
        }
    }

    private JsonNode parseQuietly(String body) {
        try {
            return body == null || body.isEmpty() ? null : mapper.readTree(body);
        } catch (JacksonException e) {
            return null;
        }
    }

    private static String queryString(Map<String, Object> query) {
        if (query == null || query.isEmpty()) return "";
        StringBuilder sb = new StringBuilder();
        Map<String, Object> ordered = new LinkedHashMap<>(query);
        ordered.forEach((key, value) -> {
            if (value == null) return;
            if (value instanceof Collection<?> values) {
                for (Object v : values) appendParam(sb, key, v);
            } else {
                appendParam(sb, key, value);
            }
        });
        return sb.isEmpty() ? "" : "?" + sb;
    }

    private static void appendParam(StringBuilder sb, String key, Object value) {
        if (!sb.isEmpty()) sb.append('&');
        sb.append(URLEncoder.encode(key, StandardCharsets.UTF_8))
                .append('=')
                .append(URLEncoder.encode(String.valueOf(value), StandardCharsets.UTF_8));
    }

    private static void sleep(Duration duration) {
        try {
            Thread.sleep(duration.toMillis());
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
            throw new FlowCatalystException(new SdkError.Network("Retry interrupted", e));
        }
    }
}
