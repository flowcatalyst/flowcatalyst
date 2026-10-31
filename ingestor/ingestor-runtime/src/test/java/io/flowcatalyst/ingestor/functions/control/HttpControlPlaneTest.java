package io.flowcatalyst.ingestor.functions.control;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

import com.sun.net.httpserver.HttpExchange;
import com.sun.net.httpserver.HttpServer;
import io.flowcatalyst.framework.http.Json;
import io.flowcatalyst.framework.httpclient.Auth;
import io.flowcatalyst.framework.httpclient.HttpClients;
import io.flowcatalyst.framework.httpclient.RetryPolicy;
import io.flowcatalyst.framework.httpclient.ServiceOptions;
import io.flowcatalyst.function.DispatchException;
import io.flowcatalyst.function.EmitResult;
import java.io.IOException;
import java.net.InetSocketAddress;
import java.net.URI;
import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.util.ArrayList;
import java.util.Base64;
import java.util.List;
import java.util.Map;
import java.util.concurrent.CopyOnWriteArrayList;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import tools.jackson.databind.JsonNode;

/// [HttpControlPlane] against an in-process platform: the requests it sends are the Go client's
/// (`internal/functions/control/client.go`) and the answers are read and classified as that client does.
class HttpControlPlaneTest {

    /// One request the fake platform saw.
    record Seen(String method, String path, String query, Map<String, List<String>> headers, byte[] body) {
        String header(String name) {
            List<String> v = headers.get(name.toLowerCase());
            return v == null || v.isEmpty() ? null : v.get(0);
        }

        JsonNode json() throws Exception {
            return Json.mapper().readTree(body);
        }
    }

    interface Handler {
        void handle(HttpExchange exchange, Seen seen) throws IOException;
    }

    HttpServer platform;
    HttpServer storage;
    HttpClients clients;
    HttpControlPlane plane;
    final List<Seen> platformSeen = new CopyOnWriteArrayList<>();
    final List<Seen> storageSeen = new CopyOnWriteArrayList<>();
    volatile Handler platformHandler = (e, s) -> reply(e, 404, "{}");
    volatile Handler storageHandler = (e, s) -> reply(e, 404, "{}");

    @BeforeEach
    void start() throws IOException {
        platform = serve(platformSeen, () -> platformHandler);
        storage = serve(storageSeen, () -> storageHandler);
        clients = HttpClients.create();
        plane = new HttpControlPlane(clients.service(ServiceOptions.builder("platform", URI.create("http://127.0.0.1:" + platform.getAddress().getPort()))
                .auth(Auth.bearer(() -> "tok-1"))
                .maxResponseBytes(HttpControlPlane.MAX_ARTIFACT_BYTES + 1)
                .retry(RetryPolicy.none())
                .build()));
    }

    @AfterEach
    void stop() {
        platform.stop(0);
        storage.stop(0);
        clients.close();
    }

    private static HttpServer serve(List<Seen> seen, java.util.function.Supplier<Handler> handler) throws IOException {
        HttpServer server = HttpServer.create(new InetSocketAddress("127.0.0.1", 0), 0);
        server.createContext("/", exchange -> {
            Seen s = new Seen(exchange.getRequestMethod(), exchange.getRequestURI().getPath(),
                    exchange.getRequestURI().getRawQuery() == null ? "" : exchange.getRequestURI().getRawQuery(),
                    lower(exchange.getRequestHeaders()), exchange.getRequestBody().readAllBytes());
            seen.add(s);
            try {
                handler.get().handle(exchange, s);
            } finally {
                exchange.close();
            }
        });
        server.setExecutor(java.util.concurrent.Executors.newVirtualThreadPerTaskExecutor());
        server.start();
        return server;
    }

    private static Map<String, List<String>> lower(com.sun.net.httpserver.Headers headers) {
        Map<String, List<String>> out = new java.util.LinkedHashMap<>();
        headers.forEach((k, v) -> out.put(k.toLowerCase(), v));
        return out;
    }

    static void reply(HttpExchange e, int status, String body) throws IOException {
        byte[] bytes = body.getBytes(StandardCharsets.UTF_8);
        e.getResponseHeaders().add("Content-Type", "application/json");
        e.sendResponseHeaders(status, bytes.length == 0 ? -1 : bytes.length);
        if (bytes.length > 0) {
            e.getResponseBody().write(bytes);
        }
    }

    static void replyBytes(HttpExchange e, int status, byte[] bytes) throws IOException {
        e.sendResponseHeaders(status, bytes.length == 0 ? -1 : bytes.length);
        if (bytes.length > 0) {
            e.getResponseBody().write(bytes);
        }
    }

    // ── desired ────────────────────────────────────────────────────────────────────────────────

    static final String DESIRED = """
            {"pool":"default","revision":42,"auth":{"issuer":"http://p","audience":"http://p"},
             "functions":[{"id":"fn_1","address":"app.hello","applicationId":"app_1","clientId":"clt_a",
               "limits":{"memoryMb":64,"maxConcurrency":8,"timeoutMs":5000,"maxBodyBytes":2048},
               "warm":true,"webhookSecret":"whsec","config":{"k":"v"},"secrets":{"s":"x"},"db":{"main":"postgres://x"},
               "trace":[{"context":"c","enabledUntilMs":1,"sampleRate":1}],
               "versions":[{"number":3,"digest":"%s","runtime":"jvm","abi":1,
                 "describe":{"abi":1,"jvm":{"entrypoint":"a.B"},"endpoints":[{"method":"GET","path":"/x","auth":"none"}]},
                 "roles":["live","alias:blue"]}]}],
             "routes":[{"hostname":"h","pathPrefix":"/","address":"app.hello"}]}
            """.formatted("a".repeat(64));

    @Test
    void desiredSendsThePoolRuntimesWaitAndEtagAndParsesTheDocument() throws Exception {
        platformHandler = (e, s) -> {
            e.getResponseHeaders().add("ETag", "\"etag-1\"");
            reply(e, 200, DESIRED);
        };

        ControlPlane.Fetched fetched = plane.desired("default", "\"old\"", Duration.ofSeconds(30));

        Seen seen = platformSeen.get(0);
        assertThat(seen.method()).isEqualTo("GET");
        assertThat(seen.path()).isEqualTo("/control/functions/desired");
        assertThat(seen.query()).contains("pool=default").contains("runtimes=jvm").contains("wait=30");
        assertThat(seen.header("If-None-Match")).isEqualTo("\"old\"");
        assertThat(seen.header("Authorization")).isEqualTo("Bearer tok-1");

        ControlPlane.Fetched.Changed changed = (ControlPlane.Fetched.Changed) fetched;
        assertThat(changed.etag()).isEqualTo("\"etag-1\"");
        Control.Desired d = changed.document();
        assertThat(d.revision()).isEqualTo(42);
        Control.FunctionDoc f = d.functions().get(0);
        assertThat(f.id()).isEqualTo("fn_1");
        assertThat(f.address()).isEqualTo("app.hello");
        assertThat(f.clientId()).isEqualTo("clt_a");
        assertThat(f.limits()).isEqualTo(new Control.Limits(64, 8, 5000, 2048));
        assertThat(f.warm()).isTrue();
        assertThat(f.webhookSecret()).isEqualTo("whsec");
        assertThat(f.config()).containsEntry("k", "v");
        assertThat(f.secrets()).containsEntry("s", "x");
        assertThat(f.db()).containsEntry("main", "postgres://x");
        Control.VersionDoc v = f.versions().get(0);
        assertThat(v.number()).isEqualTo(3);
        assertThat(v.runtime()).isEqualTo("jvm");
        assertThat(v.roles()).containsExactly("live", "alias:blue");
        Describe describe = Describe.parse(v.describe());
        assertThat(describe.entrypoint()).isEqualTo("a.B");
        assertThat(describe.endpoints()).extracting(Describe.Endpoint::pattern).containsExactly("GET /x");
    }

    @Test
    void desiredWithoutAnEtagOrWaitSendsNeither() throws Exception {
        platformHandler = (e, s) -> reply(e, 200, DESIRED);

        plane.desired("p2", null, Duration.ZERO);

        Seen seen = platformSeen.get(0);
        assertThat(seen.query()).contains("pool=p2").contains("runtimes=jvm").doesNotContain("wait=");
        assertThat(seen.header("If-None-Match")).isNull();
    }

    @Test
    void theWaitIsCappedAtTheControlPlanesMaximum() throws Exception {
        platformHandler = (e, s) -> reply(e, 304, "");

        assertThat(plane.desired("default", "\"e\"", Duration.ofMinutes(10))).isInstanceOf(ControlPlane.Fetched.NotModified.class);
        assertThat(platformSeen.get(0).query()).contains("wait=60");
    }

    @Test
    void desiredAnswers304AsNotModified() throws Exception {
        platformHandler = (e, s) -> reply(e, 304, "");
        assertThat(plane.desired("default", "\"e\"", Duration.ofSeconds(1))).isInstanceOf(ControlPlane.Fetched.NotModified.class);
    }

    @Test
    void desiredErrorsAreClassifiedTransientOrNot() {
        for (int status : new int[] {500, 502, 503, 429, 408, 425, 401, 403}) {
            platformHandler = (e, s) -> reply(e, status, "{\"error\":\"X\",\"message\":\"m\"}");
            assertThatThrownBy(() -> plane.desired("default", null, Duration.ZERO))
                    .as("status " + status)
                    .isInstanceOfSatisfying(ControlPlaneException.class, ex -> {
                        assertThat(ex.status()).isEqualTo(status);
                        assertThat(ex.isTransient()).as("status " + status).isTrue();
                    });
        }
        for (int status : new int[] {400, 404, 410, 422}) {
            platformHandler = (e, s) -> reply(e, status, "{\"error\":\"GONE\",\"message\":\"m\"}");
            assertThatThrownBy(() -> plane.desired("default", null, Duration.ZERO))
                    .as("status " + status)
                    .isInstanceOfSatisfying(ControlPlaneException.class, ex -> {
                        assertThat(ex.isTransient()).as("status " + status).isFalse();
                        assertThat(ex.code()).isEqualTo("GONE");
                    });
        }
    }

    @Test
    void anUnreachablePlatformIsATransientFailure() {
        platform.stop(0);
        assertThatThrownBy(() -> plane.desired("default", null, Duration.ZERO))
                .isInstanceOfSatisfying(ControlPlaneException.class, ex -> {
                    assertThat(ex.status()).isZero();
                    assertThat(ex.isTransient()).isTrue();
                });
    }

    // ── heartbeat ──────────────────────────────────────────────────────────────────────────────

    @Test
    void aHeartbeatIsPostedInThePlatformsShape() throws Exception {
        platformHandler = (e, s) -> reply(e, 204, "");
        Control.Heartbeat hb = new Control.Heartbeat(
                "runner-1", "default", "2026-10-31T12:00:00Z", 42,
                new Control.BudgetReport(1000, 0, 1000, 10, 0, 0, 10, 2, 0),
                List.of(
                        new Control.VersionReport("fn_1", 1, "LOADED", null),
                        new Control.VersionReport("fn_1", 2, "FAILED", "LOAD:ENTRYPOINT_NOT_FOUND"),
                        new Control.VersionReport("fn_2", 1, "RETRYING", "attempt 1 failed")),
                new Control.RunnerCapabilities(1, List.of("jvm"), "ingestor-jvm", "0.1.0", Map.of("jvm", "25")));

        plane.heartbeat(hb);

        Seen seen = platformSeen.get(0);
        assertThat(seen.method()).isEqualTo("POST");
        assertThat(seen.path()).isEqualTo("/control/functions/heartbeat");
        assertThat(seen.header("Content-Type")).startsWith("application/json");
        JsonNode j = seen.json();
        assertThat(j.path("runnerId").asString()).isEqualTo("runner-1");
        assertThat(j.path("pool").asString()).isEqualTo("default");
        assertThat(j.path("startedAt").asString()).isEqualTo("2026-10-31T12:00:00Z");
        assertThat(j.path("revision").asLong()).isEqualTo(42);
        assertThat(j.path("budget").path("limitBytes").asLong()).isEqualTo(1000);
        assertThat(j.path("budget").path("linearBytes").asLong()).isEqualTo(10);
        assertThat(j.path("budget").path("loaded").asInt()).isEqualTo(2);
        assertThat(j.path("versions")).hasSize(3);
        assertThat(j.path("versions").get(0).path("functionId").asString()).isEqualTo("fn_1");
        assertThat(j.path("versions").get(0).path("number").asInt()).isEqualTo(1);
        assertThat(j.path("versions").get(0).path("state").asString()).isEqualTo("LOADED");
        assertThat(j.path("versions").get(1).path("state").asString()).isEqualTo("FAILED");
        assertThat(j.path("versions").get(1).path("reason").asString()).isEqualTo("LOAD:ENTRYPOINT_NOT_FOUND");
        assertThat(j.path("versions").get(2).path("state").asString()).isEqualTo("RETRYING");
        assertThat(j.path("capabilities").path("protocol").asInt()).isEqualTo(1);
        assertThat(j.path("capabilities").path("runtimes").get(0).asString()).isEqualTo("jvm");
        assertThat(j.path("capabilities").path("runnerKind").asString()).isEqualTo("ingestor-jvm");
        assertThat(j.path("capabilities").path("version").asString()).isEqualTo("0.1.0");
    }

    @Test
    void aRefusedHeartbeatIsAnError() {
        platformHandler = (e, s) -> reply(e, 403, "{\"error\":\"FORBIDDEN\",\"message\":\"needs platform:function-runner\"}");
        assertThatThrownBy(() -> plane.heartbeat(new Control.Heartbeat("r", "p", "t", 1,
                new Control.BudgetReport(0, 0, 0, 0, 0, 0, 0, 0, 0), List.of(), new Control.RunnerCapabilities(1, List.of("jvm"), "k", "v", Map.of()))))
                .isInstanceOfSatisfying(ControlPlaneException.class, ex -> assertThat(ex.code()).isEqualTo("FORBIDDEN"));
    }

    // ── artifacts ──────────────────────────────────────────────────────────────────────────────

    private static String sha(byte[] b) {
        return HttpControlPlane.sha256Hex(b);
    }

    @Test
    void anArtifactStreamedByThePlatformIsVerified() throws Exception {
        byte[] jar = "PK-fake-jar-bytes".getBytes(StandardCharsets.UTF_8);
        platformHandler = (e, s) -> replyBytes(e, 200, jar);

        assertThat(plane.artifact(sha(jar))).isEqualTo(jar);
        assertThat(platformSeen.get(0).path()).isEqualTo("/control/functions/artifacts/" + sha(jar));
        assertThat(platformSeen.get(0).header("Authorization")).isEqualTo("Bearer tok-1");
    }

    @Test
    void aRedirectToStorageIsFollowedWithoutTheCredential() throws Exception {
        byte[] jar = "PK-presigned".getBytes(StandardCharsets.UTF_8);
        platformHandler = (e, s) -> {
            e.getResponseHeaders().add("Location", "http://127.0.0.1:" + storage.getAddress().getPort() + "/bucket/obj?X-Sig=abc%2Fdef");
            e.sendResponseHeaders(302, -1);
        };
        storageHandler = (e, s) -> replyBytes(e, 200, jar);

        assertThat(plane.artifact(sha(jar))).isEqualTo(jar);

        assertThat(storageSeen).hasSize(1);
        assertThat(storageSeen.get(0).path()).isEqualTo("/bucket/obj");
        assertThat(storageSeen.get(0).query()).isEqualTo("X-Sig=abc%2Fdef");
        assertThat(storageSeen.get(0).header("Authorization")).as("storage never sees the host's token").isNull();
    }

    @Test
    void aRedirectToThePlatformsOwnOriginKeepsTheCredential() throws Exception {
        byte[] jar = "PK-same-origin".getBytes(StandardCharsets.UTF_8);
        platformHandler = (e, s) -> {
            if (s.path().startsWith("/control/functions/artifacts/")) {
                e.getResponseHeaders().add("Location", "/blob/1?x=1");
                e.sendResponseHeaders(302, -1);
            } else {
                replyBytes(e, 200, jar);
            }
        };

        assertThat(plane.artifact(sha(jar))).isEqualTo(jar);
        assertThat(platformSeen).hasSize(2);
        assertThat(platformSeen.get(1).path()).isEqualTo("/blob/1");
        assertThat(platformSeen.get(1).header("Authorization")).isEqualTo("Bearer tok-1");
    }

    @Test
    void aWrongDigestIsAPermanentFailure() {
        platformHandler = (e, s) -> replyBytes(e, 200, "something else".getBytes(StandardCharsets.UTF_8));

        assertThatThrownBy(() -> plane.artifact("b".repeat(64)))
                .isInstanceOfSatisfying(ControlPlaneException.class, ex -> {
                    assertThat(ex.getMessage()).contains("does not match its digest");
                    assertThat(ex.isTransient()).isFalse();
                });
    }

    @Test
    void aWrongDigestFromStorageIsAPermanentFailureToo() {
        platformHandler = (e, s) -> {
            e.getResponseHeaders().add("Location", "http://127.0.0.1:" + storage.getAddress().getPort() + "/o");
            e.sendResponseHeaders(302, -1);
        };
        storageHandler = (e, s) -> replyBytes(e, 200, "tampered".getBytes(StandardCharsets.UTF_8));

        assertThatThrownBy(() -> plane.artifact("c".repeat(64)))
                .isInstanceOfSatisfying(ControlPlaneException.class, ex -> assertThat(ex.isTransient()).isFalse());
    }

    @Test
    void aMissingArtifactIsPermanentAndAStorageOutageIsTransient() {
        platformHandler = (e, s) -> reply(e, 404, "{\"error\":\"NOT_FOUND\",\"message\":\"no artifact\"}");
        assertThatThrownBy(() -> plane.artifact("d".repeat(64)))
                .isInstanceOfSatisfying(ControlPlaneException.class, ex -> {
                    assertThat(ex.status()).isEqualTo(404);
                    assertThat(ex.isTransient()).isFalse();
                });

        platformHandler = (e, s) -> {
            e.getResponseHeaders().add("Location", "http://127.0.0.1:" + storage.getAddress().getPort() + "/o");
            e.sendResponseHeaders(302, -1);
        };
        storageHandler = (e, s) -> replyBytes(e, 503, "slow down".getBytes(StandardCharsets.UTF_8));
        assertThatThrownBy(() -> plane.artifact("d".repeat(64)))
                .isInstanceOfSatisfying(ControlPlaneException.class, ex -> {
                    assertThat(ex.status()).isEqualTo(503);
                    assertThat(ex.isTransient()).isTrue();
                });
    }

    @Test
    void aMalformedDigestIsRefusedWithoutACall() {
        assertThatThrownBy(() -> plane.artifact("../etc/passwd"))
                .isInstanceOfSatisfying(ControlPlaneException.class, ex -> assertThat(ex.isTransient()).isFalse());
        assertThat(platformSeen).isEmpty();
    }

    // ── emit and dispatch ──────────────────────────────────────────────────────────────────────

    private static Control.EmitRequest emitRequest() {
        return new Control.EmitRequest("fn_1", 3,
                new Control.EmitEvent("app:orders:order:created", "function:app.hello", "order-1", "dedup-1", "corr-1", "cause-1", "g1", "application/json"),
                Base64.getEncoder().encodeToString("{\"n\":1}".getBytes(StandardCharsets.UTF_8)));
    }

    @Test
    void anEmitIsPostedInThePlatformsShapeAndAnswersTheEventId() throws Exception {
        platformHandler = (e, s) -> reply(e, 200, "{\"eventId\":\"evt_9\"}");

        EmitResult r = plane.emit(emitRequest());

        assertThat(r).isEqualTo(new EmitResult.Emitted("evt_9"));
        Seen seen = platformSeen.get(0);
        assertThat(seen.method()).isEqualTo("POST");
        assertThat(seen.path()).isEqualTo("/control/functions/events");
        JsonNode j = seen.json();
        assertThat(j.path("functionId").asString()).isEqualTo("fn_1");
        assertThat(j.path("version").asInt()).isEqualTo(3);
        assertThat(j.path("event").path("type").asString()).isEqualTo("app:orders:order:created");
        assertThat(j.path("event").path("source").asString()).isEqualTo("function:app.hello");
        assertThat(j.path("event").path("subject").asString()).isEqualTo("order-1");
        assertThat(j.path("event").path("dedupId").asString()).isEqualTo("dedup-1");
        assertThat(j.path("event").path("correlationId").asString()).isEqualTo("corr-1");
        assertThat(j.path("event").path("causationId").asString()).isEqualTo("cause-1");
        assertThat(j.path("event").path("messageGroup").asString()).isEqualTo("g1");
        assertThat(j.path("event").path("contentType").asString()).isEqualTo("application/json");
        assertThat(new String(Base64.getDecoder().decode(j.path("data").asString()), StandardCharsets.UTF_8)).isEqualTo("{\"n\":1}");
    }

    @Test
    void aRefusedEmitKeepsThePlatformsCodeAndStatusAndRetryability() {
        platformHandler = (e, s) -> reply(e, 403, "{\"error\":\"EVENT_TYPE_NOT_OWNED\",\"message\":\"not yours\"}");
        EmitResult.Refused refused = (EmitResult.Refused) plane.emit(emitRequest());
        assertThat(refused.code()).isEqualTo("EVENT_TYPE_NOT_OWNED");
        assertThat(refused.status()).isEqualTo(403);
        assertThat(refused.message()).isEqualTo("not yours");
        assertThat(refused.retryable()).isFalse();

        platformHandler = (e, s) -> reply(e, 400, "plain text");
        EmitResult.Refused bad = (EmitResult.Refused) plane.emit(emitRequest());
        assertThat(bad.code()).isEqualTo("BAD_REQUEST");

        platformHandler = (e, s) -> reply(e, 500, "{\"error\":\"INTERNAL\",\"message\":\"boom\"}");
        EmitResult.Refused down = (EmitResult.Refused) plane.emit(emitRequest());
        assertThat(down.code()).isEqualTo("UNAVAILABLE");
        assertThat(down.retryable()).isTrue();
    }

    @Test
    void anEmitToAnUnreachablePlatformIsUnavailable() {
        platform.stop(0);
        EmitResult.Refused r = (EmitResult.Refused) plane.emit(emitRequest());
        assertThat(r.code()).isEqualTo("UNAVAILABLE");
        assertThat(r.status()).isEqualTo(503);
        assertThat(r.retryable()).isTrue();
    }

    private static Control.DispatchRequest dispatchRequest() {
        return new Control.DispatchRequest("fn_1", 3, "inv-1", "corr-9",
                new Control.DispatchBody("app.other", "/hook", null, "subj", "g1", "IMMEDIATE", null, null, "application/json", "desc", 3, 60, true,
                        List.of(new Control.Pair("k", "v"))),
                Base64.getEncoder().encodeToString("{}".getBytes(StandardCharsets.UTF_8)));
    }

    @Test
    void aDispatchIsPostedInThePlatformsShapeAndAnswersTheJobId() throws Exception {
        platformHandler = (e, s) -> reply(e, 200, "{\"jobId\":\"djb_7\"}");

        assertThat(plane.dispatch(dispatchRequest())).isEqualTo("djb_7");

        Seen seen = platformSeen.get(0);
        assertThat(seen.path()).isEqualTo("/control/functions/dispatch-jobs");
        JsonNode j = seen.json();
        assertThat(j.path("functionId").asString()).isEqualTo("fn_1");
        assertThat(j.path("version").asInt()).isEqualTo(3);
        assertThat(j.path("invocationId").asString()).isEqualTo("inv-1");
        assertThat(j.path("requestCorrelationId").asString()).isEqualTo("corr-9");
        JsonNode d = j.path("dispatch");
        assertThat(d.path("target").asString()).isEqualTo("app.other");
        assertThat(d.path("path").asString()).isEqualTo("/hook");
        assertThat(d.path("subject").asString()).isEqualTo("subj");
        assertThat(d.path("messageGroup").asString()).isEqualTo("g1");
        assertThat(d.path("mode").asString()).isEqualTo("IMMEDIATE");
        assertThat(d.path("contentType").asString()).isEqualTo("application/json");
        assertThat(d.path("descriptor").asString()).isEqualTo("desc");
        assertThat(d.path("maxRetries").asInt()).isEqualTo(3);
        assertThat(d.path("timeoutSeconds").asInt()).isEqualTo(60);
        assertThat(d.path("dataOnly").asBoolean()).isTrue();
        assertThat(d.path("metadata").get(0).path("key").asString()).isEqualTo("k");
        assertThat(d.path("metadata").get(0).path("value").asString()).isEqualTo("v");
        assertThat(j.path("payload").asString()).isEqualTo(Base64.getEncoder().encodeToString("{}".getBytes(StandardCharsets.UTF_8)));
    }

    @Test
    void aRefusedDispatchIsClassifiedAsTheGoClientDoes() {
        platformHandler = (e, s) -> reply(e, 403, "{\"error\":\"DISPATCH_TARGET_NOT_OWNED\",\"message\":\"no\"}");
        assertThatThrownBy(() -> plane.dispatch(dispatchRequest()))
                .isInstanceOfSatisfying(DispatchException.class, ex -> {
                    assertThat(ex.code()).isEqualTo("NOT_ALLOWED");
                    assertThat(ex.retryable()).isFalse();
                });

        platformHandler = (e, s) -> reply(e, 403, "{\"error\":\"DISPATCH_NOT_DECLARED\",\"message\":\"no\"}");
        assertThatThrownBy(() -> plane.dispatch(dispatchRequest()))
                .isInstanceOfSatisfying(DispatchException.class, ex -> assertThat(ex.code()).isEqualTo("NOT_DECLARED"));

        platformHandler = (e, s) -> reply(e, 404, "{\"error\":\"DISPATCH_TARGET_NOT_FOUND\",\"message\":\"no\"}");
        assertThatThrownBy(() -> plane.dispatch(dispatchRequest()))
                .isInstanceOfSatisfying(DispatchException.class, ex -> assertThat(ex.code()).isEqualTo("BAD_REQUEST"));

        platformHandler = (e, s) -> reply(e, 502, "bad gateway");
        assertThatThrownBy(() -> plane.dispatch(dispatchRequest()))
                .isInstanceOfSatisfying(DispatchException.class, ex -> {
                    assertThat(ex.code()).isEqualTo("UNAVAILABLE");
                    assertThat(ex.retryable()).isTrue();
                });
    }

    @Test
    void aDispatchToAnUnreachablePlatformIsUnavailable() {
        platform.stop(0);
        assertThatThrownBy(() -> plane.dispatch(dispatchRequest()))
                .isInstanceOfSatisfying(DispatchException.class, ex -> assertThat(ex.retryable()).isTrue());
    }

    @Test
    void theDescribeIsReadAndCheckedByTheHost() throws Exception {
        assertThat(Describe.parse(Json.mapper().readTree("{\"abi\":1,\"jvm\":{\"entrypoint\":\"a.B\"},\"endpoints\":[{\"path\":\"/x\",\"auth\":\"none\"}],\"subscriptions\":[{\"eventType\":\"a:b:c:d\",\"path\":\"/x\"}],\"traceContexts\":[]}")).entrypoint())
                .as("members the host does not read are ignored").isEqualTo("a.B");
        List<String> bad = new ArrayList<>(List.of(
                "{\"abi\":2,\"jvm\":{\"entrypoint\":\"a.B\"},\"endpoints\":[{\"path\":\"/x\",\"auth\":\"none\"}]}",
                "{\"abi\":1,\"jvm\":{\"entrypoint\":\"a.B\"},\"endpoints\":[]}",
                "{\"abi\":1,\"endpoints\":[{\"path\":\"/x\",\"auth\":\"none\"}]}",
                "{\"abi\":1,\"jvm\":{\"entrypoint\":\" \"},\"endpoints\":[{\"path\":\"/x\",\"auth\":\"none\"}]}",
                "{\"abi\":1,\"jvm\":{\"entrypoint\":\"a.B\"},\"endpoints\":[{\"path\":\"/x\",\"auth\":\"basic\"}]}",
                "{\"abi\":1,\"jvm\":{\"entrypoint\":\"a.B\"},\"endpoints\":[{\"path\":\"x\",\"auth\":\"none\"}]}"));
        for (String d : bad) {
            assertThatThrownBy(() -> Describe.parse(Json.mapper().readTree(d))).as(d).isInstanceOf(Describe.DescribeException.class);
        }
    }

}
