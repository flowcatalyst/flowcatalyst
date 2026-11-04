package io.flowcatalyst.ingestor.functions.control;

import io.flowcatalyst.framework.http.Json;
import java.util.ArrayList;
import java.util.List;
import org.jspecify.annotations.Nullable;
import tools.jackson.core.JacksonException;
import tools.jackson.databind.DeserializationFeature;
import tools.jackson.databind.JsonNode;
import tools.jackson.databind.ObjectMapper;

/// A version's describe document as the platform stores it (`internal/functions/abi/describe.go`): what the
/// function declares it needs (including `platformToken`, a boolean), plus `jvm.entrypoint`, the class implementing `Function`. The platform
/// validated it at publish; the host re-checks only what it routes and loads by. Subscriptions, schedules
/// and trace contexts are the platform's business and are not read here.
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
        @Nullable Jvm jvm) {

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
    }

    /// A document that does not declare `platformToken`.
    public Describe(int abi, List<Endpoint> endpoints, List<String> config, List<String> secrets, List<String> db,
            List<String> httpAllow, List<String> emits, List<DispatchTarget> dispatches, @Nullable Jvm jvm) {
        this(abi, endpoints, config, secrets, db, httpAllow, emits, dispatches, false, jvm);
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
        Jvm j = jvm;
        if (j == null || j.entrypoint() == null || j.entrypoint().isBlank()) {
            problems.add("jvm.entrypoint is required");
        }
        if (!problems.isEmpty()) {
            throw new DescribeException(String.join("; ", problems));
        }
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
