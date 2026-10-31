package io.flowcatalyst.ingestor.functions.http;

import io.flowcatalyst.ingestor.functions.control.Describe;
import java.util.LinkedHashMap;
import java.util.Map;
import org.jspecify.annotations.Nullable;

/// CORS for an endpoint that declares `cors` (`handleCORS` in `internal/functions/runner/invoke.go`); nothing is
/// done for an endpoint that does not. The host is the SOLE authority for a `cors`-declaring endpoint's
/// `Access-Control-*` response headers: it answers a genuine preflight (`OPTIONS` with `Origin` and
/// `Access-Control-Request-Method`) itself, without invoking, taking a permit or authenticating, and on an
/// actual request it sets its own headers, which the function cannot add to or override.
///
/// - an origin is allowed when it equals a declared origin (case-insensitive) or the declared origins include
///   `*` and credentials are not allowed (a wildcard never applies with credentials);
/// - allowed: `Vary: Origin`; `Access-Control-Allow-Origin` is `*` only when `*` is the sole origin (and no
///   credentials), else the request's origin; `Access-Control-Allow-Credentials: true` when declared;
/// - a preflight is then answered `204` with `Access-Control-Allow-Methods` (the declared methods, else the
///   requested one), `Access-Control-Allow-Headers` (the declared headers, else the requested ones) and
///   `Access-Control-Max-Age: 600`; a preflight from a disallowed origin is `403` with no headers.
public final class CorsPolicy {

    private CorsPolicy() {
    }

    /// What the policy decided. `answered` is true for a preflight: send `status` and `headers` and stop.
    /// Otherwise `headers` (possibly empty) are added to the function's answer.
    public record Outcome(boolean answered, int status, Map<String, String> headers) {
    }

    static boolean isPreflight(String method, @Nullable String requestMethodHeader) {
        return "OPTIONS".equals(method) && requestMethodHeader != null && !requestMethodHeader.isEmpty();
    }

    public static Outcome evaluate(
            Describe.Cors cors,
            String method,
            @Nullable String origin,
            @Nullable String requestMethod,
            @Nullable String requestHeaders) {
        if (origin == null || origin.isEmpty()) {
            return new Outcome(false, 0, Map.of());
        }
        boolean preflight = isPreflight(method, requestMethod);
        boolean allowed = false;
        for (String o : cors.origins()) {
            if ((o.equals("*") && !cors.allowCredentials()) || o.equalsIgnoreCase(origin)) {
                allowed = true;
                break;
            }
        }
        if (!allowed) {
            return preflight ? new Outcome(true, 403, Map.of()) : new Outcome(false, 0, Map.of());
        }
        Map<String, String> headers = new LinkedHashMap<>();
        headers.put("Vary", "Origin");
        if (cors.origins().size() == 1 && cors.origins().get(0).equals("*") && !cors.allowCredentials()) {
            headers.put("Access-Control-Allow-Origin", "*");
        } else {
            headers.put("Access-Control-Allow-Origin", origin);
        }
        if (cors.allowCredentials()) {
            headers.put("Access-Control-Allow-Credentials", "true");
        }
        if (!preflight) {
            return new Outcome(false, 0, headers);
        }
        headers.put("Access-Control-Allow-Methods", cors.methods().isEmpty() ? String.valueOf(requestMethod) : String.join(", ", cors.methods()));
        if (!cors.headers().isEmpty()) {
            headers.put("Access-Control-Allow-Headers", String.join(", ", cors.headers()));
        } else if (requestHeaders != null && !requestHeaders.isEmpty()) {
            headers.put("Access-Control-Allow-Headers", requestHeaders);
        }
        headers.put("Access-Control-Max-Age", "600");
        return new Outcome(true, 204, headers);
    }
}
