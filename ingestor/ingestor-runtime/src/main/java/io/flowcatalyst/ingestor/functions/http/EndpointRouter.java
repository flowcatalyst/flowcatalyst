package io.flowcatalyst.ingestor.functions.http;

import io.flowcatalyst.ingestor.functions.control.Describe;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import org.jspecify.annotations.Nullable;

/// Matches a request's method and path to one of a describe document's endpoints with the semantics of Go's
/// `net/http` ServeMux (`internal/functions/abi/router.go`), which is what the platform validated the
/// document against: patterns are `[METHOD ]/path` with literal segments, `{name}` (one non-empty segment),
/// `{name...}` (the rest, possibly empty) and `{$}` (exactly the trailing slash); a pattern ending in `/` matches
/// the whole subtree. The most specific matching pattern wins (literal over `{name}` over a rest wildcard,
/// compared segment by segment; a pattern with a method beats one without when the paths are otherwise equal).
/// A path some pattern matches, but not with this method, is a 405. A `GET` pattern also serves `HEAD`.
///
/// Immutable and lock-free: built once per loaded version, read by every call. Divergence from Go: the path
/// must already be clean (no empty or dot segments), otherwise it simply matches nothing (Go would redirect).
public final class EndpointRouter {

    /// What [#match] found.
    public sealed interface Matched permits Matched.Found, Matched.NoRoute, Matched.MethodNotAllowed {

        record Found(int index, Describe.Endpoint endpoint, Map<String, String> pathParams) implements Matched {
        }

        record NoRoute() implements Matched {
        }

        record MethodNotAllowed() implements Matched {
        }
    }

    private enum Kind { LITERAL, WILDCARD, REST, END }

    private record Seg(Kind kind, String text) {
        int rank() {
            return switch (kind) {
                case LITERAL, END -> 0;
                case WILDCARD -> 1;
                case REST -> 2;
            };
        }
    }

    private record Compiled(int index, Describe.Endpoint endpoint, String method, List<Seg> segs) {
    }

    private final List<Compiled> patterns;

    /// @throws IllegalArgumentException a pattern is malformed (a wildcard that is not a whole segment, `...` or
    ///                                   `{$}` not last)
    public EndpointRouter(List<Describe.Endpoint> endpoints) {
        List<Compiled> out = new ArrayList<>(endpoints.size());
        for (int i = 0; i < endpoints.size(); i++) {
            Describe.Endpoint e = endpoints.get(i);
            String method = e.method() == null ? "" : e.method();
            out.add(new Compiled(i, e, method, parse(e.path())));
        }
        this.patterns = List.copyOf(out);
    }

    private static List<Seg> parse(String path) {
        if (!path.startsWith("/")) {
            throw new IllegalArgumentException("pattern must start with /: " + path);
        }
        String[] parts = path.substring(1).split("/", -1);
        List<Seg> segs = new ArrayList<>(parts.length);
        for (int i = 0; i < parts.length; i++) {
            String part = parts[i];
            boolean last = i == parts.length - 1;
            if (part.equals("{$}")) {
                if (!last) {
                    throw new IllegalArgumentException("{$} must be last: " + path);
                }
                segs.add(new Seg(Kind.END, ""));
            } else if (part.startsWith("{") && part.endsWith("}") && part.length() > 2) {
                String name = part.substring(1, part.length() - 1);
                if (name.endsWith("...")) {
                    if (!last) {
                        throw new IllegalArgumentException("a {name...} wildcard must be last: " + path);
                    }
                    segs.add(new Seg(Kind.REST, name.substring(0, name.length() - 3)));
                } else {
                    segs.add(new Seg(Kind.WILDCARD, name));
                }
            } else if (part.indexOf('{') >= 0 || part.indexOf('}') >= 0) {
                throw new IllegalArgumentException("a wildcard must be a whole segment: " + path);
            } else if (part.isEmpty() && last) {
                segs.add(new Seg(Kind.REST, "")); // a trailing slash: the whole subtree, unnamed
            } else {
                if (part.isEmpty()) {
                    throw new IllegalArgumentException("empty segment: " + path);
                }
                segs.add(new Seg(Kind.LITERAL, part));
            }
        }
        return List.copyOf(segs);
    }

    /// Finds the endpoint for `method` and the already percent-decoded `path`.
    public Matched match(String method, String path) {
        if (!path.startsWith("/") || !clean(path)) {
            return new Matched.NoRoute();
        }
        String[] request = path.substring(1).split("/", -1);
        boolean pathMatched = false;
        @Nullable Compiled best = null;
        Map<String, String> bestParams = Map.of();
        for (Compiled c : patterns) {
            Map<String, String> params = new LinkedHashMap<>();
            if (!matchPath(c.segs(), request, params)) {
                continue;
            }
            pathMatched = true;
            if (!methodMatches(c.method(), method)) {
                continue;
            }
            if (best == null || moreSpecific(c, best)) {
                best = c;
                bestParams = params;
            }
        }
        if (best != null) {
            return new Matched.Found(best.index(), best.endpoint(), Map.copyOf(bestParams));
        }
        return pathMatched ? new Matched.MethodNotAllowed() : new Matched.NoRoute();
    }

    private static boolean clean(String path) {
        if (path.equals("/")) {
            return true;
        }
        String[] parts = path.substring(1).split("/", -1);
        for (int i = 0; i < parts.length; i++) {
            String p = parts[i];
            if (p.equals(".") || p.equals("..") || (p.isEmpty() && i != parts.length - 1)) {
                return false;
            }
        }
        return true;
    }

    private static boolean methodMatches(String patternMethod, String requestMethod) {
        if (patternMethod.isEmpty() || patternMethod.equals(requestMethod)) {
            return true;
        }
        return patternMethod.equals("GET") && requestMethod.equals("HEAD");
    }

    private static boolean matchPath(List<Seg> segs, String[] request, Map<String, String> params) {
        int i = 0;
        for (Seg seg : segs) {
            switch (seg.kind()) {
                case LITERAL -> {
                    if (i >= request.length || !request[i].equals(seg.text())) {
                        return false;
                    }
                    i++;
                }
                case WILDCARD -> {
                    if (i >= request.length || request[i].isEmpty()) {
                        return false;
                    }
                    params.put(seg.text(), request[i]);
                    i++;
                }
                case REST -> {
                    if (!seg.text().isEmpty()) {
                        params.put(seg.text(), String.join("/", java.util.Arrays.asList(request).subList(Math.min(i, request.length), request.length)));
                    }
                    return true;
                }
                case END -> {
                    return i == request.length - 1 && request[i].isEmpty();
                }
            }
        }
        return i == request.length;
    }

    /// Whether `a` is a better match than `b` (both match the same path and method): the first segment where their
    /// kinds differ decides, a literal beating a wildcard beating a rest wildcard; otherwise the one with a method.
    private static boolean moreSpecific(Compiled a, Compiled b) {
        int n = Math.max(a.segs().size(), b.segs().size());
        for (int i = 0; i < n; i++) {
            int ra = i < a.segs().size() ? a.segs().get(i).rank() : 3;
            int rb = i < b.segs().size() ? b.segs().get(i).rank() : 3;
            if (ra != rb) {
                return ra < rb;
            }
        }
        return !a.method().isEmpty() && b.method().isEmpty();
    }

    /// The pattern of an endpoint of this router (`[METHOD ]PATH`), for the request's `route`.
    public @Nullable String pattern(int index) {
        return index >= 0 && index < patterns.size() ? patterns.get(index).endpoint().pattern() : null;
    }
}
