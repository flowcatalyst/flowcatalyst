package io.flowcatalyst.ingestor.functions.control;

import io.flowcatalyst.framework.http.Json;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.HashSet;
import java.util.List;
import java.util.Set;
import java.util.regex.Pattern;
import org.jspecify.annotations.Nullable;
import tools.jackson.core.JacksonException;
import tools.jackson.databind.DeserializationFeature;
import tools.jackson.databind.JsonNode;
import tools.jackson.databind.ObjectMapper;

/// A version's describe document as the platform stores it (`internal/functions/abi/describe.go`): what the
/// function declares it needs (including `platformToken`, a boolean), plus `jvm.entrypoint`, the class implementing `Function`. The platform
/// validated it at publish; the host re-checks only what it routes and loads by, and the names of the trace
/// contexts the function may write to. Subscriptions and schedules are the platform's business and are not read here.
public record Describe(
        int abi,
        List<Endpoint> endpoints,
        List<String> config,
        List<String> secrets,
        List<String> db,
        List<String> httpAllow,
        List<String> emits,
        List<DispatchTarget> dispatches,
        boolean platformToken,
        @Nullable Jvm jvm,
        List<TraceContext> traceContexts) {

    public static final String AUTH_WEBHOOK = "webhook";
    public static final String AUTH_PLATFORM = "platform";
    public static final String AUTH_NONE = "none";

    /// Reads the platform's JSON tolerantly: a field a later platform adds must not stop a load.
    private static final ObjectMapper MAPPER =
            Json.mapper().rebuild().disable(DeserializationFeature.FAIL_ON_UNKNOWN_PROPERTIES).build();

    public Describe {
        endpoints = endpoints == null ? List.of() : List.copyOf(endpoints);
        config = config == null ? List.of() : List.copyOf(config);
        secrets = secrets == null ? List.of() : List.copyOf(secrets);
        db = db == null ? List.of() : List.copyOf(db);
        httpAllow = httpAllow == null ? List.of() : List.copyOf(httpAllow);
        emits = emits == null ? List.of() : List.copyOf(emits);
        dispatches = dispatches == null ? List.of() : List.copyOf(dispatches);
        traceContexts = traceContexts == null ? List.of() : List.copyOf(traceContexts);
    }

    /// A document that declares no trace contexts.
    public Describe(int abi, List<Endpoint> endpoints, List<String> config, List<String> secrets, List<String> db,
            List<String> httpAllow, List<String> emits, List<DispatchTarget> dispatches, boolean platformToken,
            @Nullable Jvm jvm) {
        this(abi, endpoints, config, secrets, db, httpAllow, emits, dispatches, platformToken, jvm, List.of());
    }

    /// A document that does not declare `platformToken`.
    public Describe(int abi, List<Endpoint> endpoints, List<String> config, List<String> secrets, List<String> db,
            List<String> httpAllow, List<String> emits, List<DispatchTarget> dispatches, @Nullable Jvm jvm) {
        this(abi, endpoints, config, secrets, db, httpAllow, emits, dispatches, false, jvm, List.of());
    }

    /// One path the function answers, and how its callers authenticate (`abi.Endpoint`). An empty
    /// `method` matches every method.
    public record Endpoint(
            @Nullable String method,
            String path,
            @Nullable String auth,
            @Nullable Long maxBodyBytes,
            @Nullable Long timeoutMs,
            @Nullable Cors cors) {

        /// The `net/http` ServeMux pattern: `[METHOD ]PATH`.
        public String pattern() {
            return method == null || method.isEmpty() ? path : method + " " + path;
        }
    }

    /// An endpoint's cross-origin policy (`abi.CORS`).
    public record Cors(List<String> origins, List<String> methods, List<String> headers, boolean allowCredentials) {
        public Cors {
            origins = origins == null ? List.of() : List.copyOf(origins);
            methods = methods == null ? List.of() : List.copyOf(methods);
            headers = headers == null ? List.of() : List.copyOf(headers);
        }
    }

    /// One declared dispatch destination (`abi.DispatchTarget`).
    public record DispatchTarget(String target, String path, @Nullable String pool, @Nullable String connection) {
    }

    /// A trace context the function declares (`abi.TraceContext`): the name it writes with `ctx.trace`, and what it is for.
    public record TraceContext(@Nullable String name, @Nullable String description) {
    }

    /// The longest trace context name, the most contexts, and the longest description in bytes (`abi.Describe.Validate`).
    public static final Pattern TRACE_CONTEXT_NAME = Pattern.compile("^[a-z][a-z0-9-]{0,39}$");
    public static final int MAX_TRACE_CONTEXTS = 20;
    public static final int MAX_TRACE_DESCRIPTION_BYTES = 200;

    /// The built-in context every function has, readable but not writable by the function.
    public static final String TRACE_CONTEXT_HTTP = "http";

    /// The jvm-runtime block: `entrypoint` is the binary name of the class implementing `Function`.
    public record Jvm(@Nullable String entrypoint) {
    }

    /// The describe document is not one this host can serve.
    public static final class DescribeException extends Exception {
        private static final long serialVersionUID = 1L;

        public DescribeException(String message) {
            super(message);
        }
    }

    /// Reads and checks `node`.
    ///
    /// @throws DescribeException it is not a describe document this host can run
    public static Describe parse(@Nullable JsonNode node) throws DescribeException {
        if (node == null || node.isNull() || !node.isObject()) {
            throw new DescribeException("the describe document is missing");
        }
        Describe d;
        try {
            d = MAPPER.treeToValue(node, Describe.class);
        } catch (JacksonException e) {
            throw new DescribeException("not a describe document");
        }
        d.check();
        return d;
    }

    private void check() throws DescribeException {
        List<String> problems = new ArrayList<>();
        if (abi != 1) {
            problems.add("abi " + abi + " is not supported (want 1)");
        }
        if (endpoints.isEmpty()) {
            problems.add("endpoints: at least one endpoint is required");
        }
        for (int i = 0; i < endpoints.size(); i++) {
            Endpoint e = endpoints.get(i);
            String auth = e.auth();
            if (!AUTH_WEBHOOK.equals(auth) && !AUTH_PLATFORM.equals(auth) && !AUTH_NONE.equals(auth)) {
                problems.add("endpoints[" + i + "]: auth is not webhook, platform or none");
            }
            if (e.path() == null || !e.path().startsWith("/")) {
                problems.add("endpoints[" + i + "]: path must start with /");
            }
        }
        if (traceContexts.size() > MAX_TRACE_CONTEXTS) {
            problems.add("traceContexts: at most " + MAX_TRACE_CONTEXTS + " are allowed");
        }
        Set<String> seen = new HashSet<>();
        for (int i = 0; i < traceContexts.size(); i++) {
            TraceContext c = traceContexts.get(i);
            String name = c == null ? null : c.name();
            if (name == null || !TRACE_CONTEXT_NAME.matcher(name).matches()) {
                problems.add("traceContexts[" + i + "]: name must match " + TRACE_CONTEXT_NAME.pattern());
            } else if (name.equals(TRACE_CONTEXT_HTTP)) {
                problems.add("traceContexts[" + i + "]: " + TRACE_CONTEXT_HTTP + " is built in and cannot be declared");
            } else if (!seen.add(name)) {
                problems.add("traceContexts[" + i + "]: " + name + " is declared twice");
            }
            String description = c == null ? null : c.description();
            if (description != null && description.getBytes(StandardCharsets.UTF_8).length > MAX_TRACE_DESCRIPTION_BYTES) {
                problems.add("traceContexts[" + i + "]: description is over " + MAX_TRACE_DESCRIPTION_BYTES + " bytes");
            }
        }
        Jvm j = jvm;
        if (j == null || j.entrypoint() == null || j.entrypoint().isBlank()) {
            problems.add("jvm.entrypoint is required");
        }
        if (!problems.isEmpty()) {
            throw new DescribeException(String.join("; ", problems));
        }
    }

    /// The names of the declared trace contexts (not `http`, which is built in).
    public Set<String> traceContextNames() {
        Set<String> names = new HashSet<>();
        for (TraceContext c : traceContexts) {
            if (c != null && c.name() != null) {
                names.add(c.name());
            }
        }
        return Set.copyOf(names);
    }

    /// The entrypoint class name; only valid on a checked document.
    public String entrypoint() {
        Jvm j = jvm;
        String name = j == null ? null : j.entrypoint();
        if (name == null) {
            throw new IllegalStateException("no jvm.entrypoint");
        }
        return name;
    }
}
