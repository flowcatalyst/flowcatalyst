package io.flowcatalyst.ingestor.functions;

import io.flowcatalyst.framework.http.RawRequest;
import io.flowcatalyst.framework.http.Request;
import io.flowcatalyst.framework.httpclient.ClientResponse;
import java.lang.reflect.Field;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import org.jspecify.annotations.Nullable;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/// The two things a function host needs from the framework that its public API does not give: every header of
/// an inbound [Request] (it offers `header(name)` only) and every header of a [ClientResponse] (it offers
/// `header(name)` only). Both read the framework's own private field by reflection — the framework is a
/// plain class-path library, not a module — and fall back, if the field is ever renamed, to the headers
/// named in `known`. A function's `Request.headers` and an outbound `HttpReply.headers` are whole maps, so a
/// partial answer is a degraded one, and the fallback logs once.
///
/// This is a stand-in for `Request.headers()` and `ClientResponse.headers()` in the framework.
public final class FrameworkAccess {

    private static final Logger LOG = LoggerFactory.getLogger(FrameworkAccess.class);

    private static final @Nullable Field REQUEST_RAW = field(Request.class, "raw");
    private static final @Nullable Field RESPONSE_HEADERS = field(ClientResponse.class, "headers");

    /// The inbound headers read if reflection is unavailable.
    private static final List<String> REQUEST_FALLBACK = List.of(
            "content-type", "content-length", "authorization", "accept", "accept-encoding", "accept-language", "user-agent",
            "origin", "host", "referer", "cookie", "x-correlation-id", "x-forwarded-for", "x-forwarded-proto", "x-forwarded-host",
            "x-flowcatalyst-signature", "x-flowcatalyst-timestamp", "x-flowcatalyst-client", "x-dispatch-job-id", "x-event-type",
            "idempotency-key", "access-control-request-method", "access-control-request-headers");

    /// The response headers read if reflection is unavailable.
    private static final List<String> RESPONSE_FALLBACK = List.of(
            "content-type", "content-length", "content-encoding", "location", "etag", "last-modified", "cache-control",
            "retry-after", "set-cookie", "www-authenticate", "link", "date", "vary", "x-request-id");

    private FrameworkAccess() {
    }

    private static @Nullable Field field(Class<?> owner, String name) {
        try {
            Field f = owner.getDeclaredField(name);
            f.setAccessible(true);
            return f;
        } catch (ReflectiveOperationException | RuntimeException e) {
            LOG.warn("cannot read {}.{} by reflection; falling back to a fixed list of headers", owner.getSimpleName(), name);
            return null;
        }
    }

    /// Every header of the request, names lower-cased (the framework lower-cases them), values in order.
    @SuppressWarnings("unchecked")
    public static Map<String, List<String>> requestHeaders(Request request) {
        Field f = REQUEST_RAW;
        if (f != null) {
            try {
                Object raw = f.get(request);
                if (raw instanceof RawRequest r) {
                    return r.headers();
                }
            } catch (IllegalAccessException | RuntimeException e) {
                // fall through
            }
        }
        Map<String, List<String>> out = new LinkedHashMap<>();
        for (String name : REQUEST_FALLBACK) {
            List<String> values = request.headers(name);
            if (!values.isEmpty()) {
                out.put(name, values);
            }
        }
        return out;
    }

    /// Every header of the response, names lower-cased, values in order.
    @SuppressWarnings("unchecked")
    public static Map<String, List<String>> responseHeaders(ClientResponse response) {
        Field f = RESPONSE_HEADERS;
        if (f != null) {
            try {
                Object headers = f.get(response);
                if (headers instanceof Map<?, ?> m) {
                    return (Map<String, List<String>>) m;
                }
            } catch (IllegalAccessException | RuntimeException e) {
                // fall through
            }
        }
        Map<String, List<String>> out = new LinkedHashMap<>();
        for (String name : RESPONSE_FALLBACK) {
            List<String> values = response.headers(name);
            if (!values.isEmpty()) {
                out.put(name, values);
            }
        }
        return out;
    }
}
