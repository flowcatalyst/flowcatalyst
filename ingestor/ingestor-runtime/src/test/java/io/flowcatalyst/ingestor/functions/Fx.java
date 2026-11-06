package io.flowcatalyst.ingestor.functions;

import io.flowcatalyst.framework.http.Json;
import io.flowcatalyst.ingestor.functions.control.Control;
import io.flowcatalyst.ingestor.functions.control.HttpControlPlane;
import io.flowcatalyst.ingestor.functions.load.FixtureJars;
import java.io.IOException;
import java.io.UncheckedIOException;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import tools.jackson.core.JacksonException;
import tools.jackson.databind.JsonNode;

/// Builders for the host's tests: function jars built at test time, describe documents and desired-state documents.
public final class Fx {

    private Fx() {
    }

    /// A built jar and its digest.
    public record Jar(byte[] bytes, String digest) {
    }

    /// A jar of one source file: `className` implements `Function`.
    public static Jar jar(String className, String source) {
        return jar(Map.of(className, source));
    }

    public static Jar jar(Map<String, String> sources) {
        try {
            Path dir = Files.createTempDirectory("fx-jar-");
            FixtureJars.Builder b = FixtureJars.builder();
            sources.forEach(b::source);
            Path file = b.build(dir.resolve("fixture.jar"));
            byte[] bytes = Files.readAllBytes(file);
            return new Jar(bytes, HttpControlPlane.sha256Hex(bytes));
        } catch (IOException e) {
            throw new UncheckedIOException(e);
        }
    }

    /// A function whose answer is `{"name":..,"v":<tag>}` for `GET /hello/{name}`; `tag` tells two versions apart.
    public static Jar hello(String tag) {
        return jar("fixture.app.Hello", """
                package fixture.app;
                import io.flowcatalyst.function.*;
                public final class Hello implements Function {
                    public Result handle(Request in, FunctionContext ctx) {
                        return Result.json(200, "{\\"name\\":\\"" + in.pathParams().getOrDefault("name", "-") + "\\",\\"v\\":\\"%s\\"}");
                    }
                }
                """.formatted(tag));
    }

    /// A describe document for `entrypoint` with `endpoints` (each a JSON object) and `extra` members (e.g. `"emits":[...]`).
    public static String describe(String entrypoint, List<String> endpoints, String... extra) {
        StringBuilder sb = new StringBuilder("{\"abi\":1,\"jvm\":{\"entrypoint\":\"").append(entrypoint).append("\"},\"endpoints\":[");
        sb.append(String.join(",", endpoints)).append(']');
        for (String e : extra) {
            sb.append(',').append(e);
        }
        return sb.append('}').toString();
    }

    public static String endpoint(String method, String path, String auth) {
        return "{" + (method.isEmpty() ? "" : "\"method\":\"" + method + "\",") + "\"path\":\"" + path + "\",\"auth\":\"" + auth + "\"}";
    }

    public static JsonNode json(String text) {
        try {
            return Json.mapper().readTree(text);
        } catch (JacksonException e) {
            throw new IllegalArgumentException(e);
        }
    }

    public static Control.VersionDoc version(int number, Jar jar, String describeJson, String... roles) {
        return new Control.VersionDoc(number, jar.digest(), Control.RUNTIME_JVM, 1, json(describeJson), List.of(roles));
    }

    /// A function with default limits (16 permits, 30 s, 1 MiB), not warm, no webhook secret, no settings.
    public static FunctionBuilder function(String id, String address) {
        return new FunctionBuilder(id, address);
    }

    public static final class FunctionBuilder {
        private final String id;
        private final String address;
        private Control.Limits limits = Control.Limits.DEFAULTS;
        private boolean warm;
        private String webhookSecret;
        private String clientId;
        private final Map<String, String> config = new java.util.LinkedHashMap<>();
        private final Map<String, String> secrets = new java.util.LinkedHashMap<>();
        private final List<Control.TraceState> trace = new ArrayList<>();
        private final List<Control.VersionDoc> versions = new ArrayList<>();

        private FunctionBuilder(String id, String address) {
            this.id = id;
            this.address = address;
        }

        public FunctionBuilder limits(int maxConcurrency, long timeoutMs, long maxBodyBytes) {
            this.limits = new Control.Limits(64, maxConcurrency, timeoutMs, maxBodyBytes);
            return this;
        }

        public FunctionBuilder warm(boolean warm) {
            this.warm = warm;
            return this;
        }

        public FunctionBuilder webhookSecret(String secret) {
            this.webhookSecret = secret;
            return this;
        }

        public FunctionBuilder clientId(String clientId) {
            this.clientId = clientId;
            return this;
        }

        public FunctionBuilder config(String key, String value) {
            config.put(key, value);
            return this;
        }

        public FunctionBuilder secret(String key, String value) {
            secrets.put(key, value);
            return this;
        }

        /// A trace window of `context` that ends at `enabledUntilMs` (unix milliseconds), sampling one in `sampleRate`.
        public FunctionBuilder trace(String context, long enabledUntilMs, int sampleRate) {
            trace.add(new Control.TraceState(context, enabledUntilMs, sampleRate));
            return this;
        }

        public FunctionBuilder version(Control.VersionDoc v) {
            versions.add(v);
            return this;
        }

        public Control.FunctionDoc build() {
            return new Control.FunctionDoc(id, address, "app_1", clientId, limits, warm, webhookSecret, config, secrets, Map.of(), trace, versions);
        }
    }

    public static Control.Desired desired(long revision, Control.FunctionDoc... functions) {
        return new Control.Desired("default", revision, null, List.of(functions));
    }
}
