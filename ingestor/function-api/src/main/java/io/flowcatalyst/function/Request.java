package io.flowcatalyst.function;

import java.net.URLDecoder;
import java.nio.charset.StandardCharsets;
import java.time.Instant;
import java.util.Arrays;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.Optional;

/// The one shape every invocation takes: "a function is an HTTP application".
/// A subscription delivery, a direct dispatch job, a scheduled firing, a
/// browser call and a platform API client all arrive here, distinguished
/// only by [#caller] and by parsing [#body] with [Webhook#event] /
/// [Webhook#schedule] when the endpoint's `auth` is `webhook`.
///
/// The fields are those of the platform's invocation request (`abi.Request`)
/// plus the body bytes. Every map is deep-copied and unmodifiable; `body` is
/// cloned in the constructor and again by [#body()]. `headers` keeps the
/// original key spelling: look a header up case-insensitively with
/// [#header(String)] / [#headers(String)] rather than indexing the map.
///
/// @param id          the invocation id: unique per call, also in the logs
/// @param address     the function this call targets
/// @param version     the loaded version handling this call
/// @param method      the HTTP method
/// @param path        the request path after `/fn/{address}[@selector]`
/// @param rawQuery    the raw query string, without the `?`; empty when none
/// @param headers     request headers, in original spelling
/// @param route       the matched endpoint's pattern, `[METHOD ]PATH` as the describe declares it (e.g.
///                    `GET /hello/{name}`, or `/hooks` for an endpoint of any method), empty when none
/// @param pathParams  the path parameters the matched pattern bound
/// @param caller      who the platform proved the caller to be ([Caller])
/// @param deadline    when this call must be finished; the host cuts it off after
/// @param host        the `Host` the caller used, empty when not known
/// @param remoteAddr  the caller's address (through trusted proxies on the public entry), empty when not known
/// @param isPublic    whether the call came in through the public entry
/// @param body        the request body
public record Request(
        String id,
        FunctionAddress address,
        int version,
        String method,
        String path,
        String rawQuery,
        Map<String, List<String>> headers,
        String route,
        Map<String, String> pathParams,
        Caller caller,
        Instant deadline,
        String host,
        String remoteAddr,
        boolean isPublic,
        byte[] body) {

    public Request {
        Objects.requireNonNull(id, "id");
        Objects.requireNonNull(address, "address");
        Objects.requireNonNull(method, "method");
        Objects.requireNonNull(path, "path");
        Objects.requireNonNull(caller, "caller");
        Objects.requireNonNull(deadline, "deadline");
        rawQuery = rawQuery == null ? "" : rawQuery;
        route = route == null ? "" : route;
        host = host == null ? "" : host;
        remoteAddr = remoteAddr == null ? "" : remoteAddr;
        headers = Copies.multiMap(headers);
        pathParams = Copies.stringMap(pathParams);
        body = body == null ? new byte[0] : Copies.bytes(body);
    }

    @Override
    public byte[] body() {
        return Copies.bytes(body);
    }

    /// The body decoded as UTF-8.
    public String bodyText() {
        return new String(body, StandardCharsets.UTF_8);
    }

    /// The first value of the header named `name`, matched case-insensitively.
    public Optional<String> header(String name) {
        return headers(name).stream().findFirst();
    }

    /// Every value of the header named `name`, matched case-insensitively:
    /// the values of the first matching key, in header order.
    public List<String> headers(String name) {
        for (Map.Entry<String, List<String>> entry : headers.entrySet()) {
            if (entry.getKey().equalsIgnoreCase(name)) {
                return entry.getValue();
            }
        }
        return List.of();
    }

    /// [#rawQuery] parsed into `name -> values` (URL-decoded, in order of first
    /// appearance; a name with no `=` has the value `""`). Unmodifiable.
    public Map<String, List<String>> query() {
        Map<String, List<String>> out = new LinkedHashMap<>();
        if (!rawQuery.isEmpty()) {
            for (String pair : rawQuery.split("&")) {
                if (pair.isEmpty()) continue;
                int eq = pair.indexOf('=');
                String name = decode(eq < 0 ? pair : pair.substring(0, eq));
                String value = eq < 0 ? "" : decode(pair.substring(eq + 1));
                out.computeIfAbsent(name, k -> new java.util.ArrayList<>()).add(value);
            }
        }
        return Copies.multiMap(out);
    }

    /// The first value of the query parameter `name` (case-sensitive).
    public Optional<String> queryParam(String name) {
        List<String> values = query().get(name);
        return values == null || values.isEmpty() ? Optional.empty() : Optional.of(values.get(0));
    }

    private static String decode(String s) {
        try {
            return URLDecoder.decode(s, StandardCharsets.UTF_8);
        } catch (IllegalArgumentException e) {
            return s;
        }
    }

    @Override
    public boolean equals(Object o) {
        if (this == o) return true;
        if (!(o instanceof Request other)) return false;
        return version == other.version
                && isPublic == other.isPublic
                && Objects.equals(id, other.id)
                && Objects.equals(address, other.address)
                && Objects.equals(method, other.method)
                && Objects.equals(path, other.path)
                && Objects.equals(rawQuery, other.rawQuery)
                && Objects.equals(headers, other.headers)
                && Objects.equals(route, other.route)
                && Objects.equals(pathParams, other.pathParams)
                && Objects.equals(caller, other.caller)
                && Objects.equals(deadline, other.deadline)
                && Objects.equals(host, other.host)
                && Objects.equals(remoteAddr, other.remoteAddr)
                && Arrays.equals(body, other.body);
    }

    @Override
    public int hashCode() {
        int result = Objects.hash(id, address, version, method, path, rawQuery, headers, route, pathParams,
                caller, deadline, host, remoteAddr, isPublic);
        return 31 * result + Arrays.hashCode(body);
    }

    @Override
    public String toString() {
        return "Request[id=" + id + ", address=" + address + ", version=" + version + ", method=" + method
                + ", path=" + path + ", rawQuery=" + rawQuery + ", route=" + route + ", pathParams=" + pathParams
                + ", headers=" + headers + ", caller=" + caller + ", deadline=" + deadline + ", host=" + host
                + ", remoteAddr=" + remoteAddr + ", isPublic=" + isPublic + ", body.length=" + body.length + "]";
    }
}
