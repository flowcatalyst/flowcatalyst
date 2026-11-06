package io.flowcatalyst.ingestor.functions.trace;

import static org.assertj.core.api.Assertions.assertThat;

import com.sun.net.httpserver.HttpExchange;
import com.sun.net.httpserver.HttpServer;
import io.flowcatalyst.framework.http.Json;
import io.flowcatalyst.framework.http.TestResponse;
import io.flowcatalyst.ingestor.functions.Fx;
import io.flowcatalyst.ingestor.functions.Harness;
import io.flowcatalyst.ingestor.functions.control.Control;
import java.io.IOException;
import java.net.InetSocketAddress;
import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.util.List;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;
import java.util.function.BooleanSupplier;
import java.util.function.Consumer;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.Timeout;
import tools.jackson.databind.JsonNode;

/// A JVM function traced end to end through the listener: the window in the desired document, the gate, the `http`, `log`
/// and `outbound` records, the values replaced in them, and what the shipper handed the control plane
/// (docs/jvm-function-trace-plan.md section 4).
@Timeout(value = 60, threadMode = Timeout.ThreadMode.SEPARATE_THREAD)
class FunctionTraceTest {

    static final String BASE = "/fn/app.trace";
    static final String PASSWORD = "pw-for-vendor-9";
    static final String SESSION = "sess-abcdefghijklmnop-1234";
    static final String SECRET = "s3cr3t-value-123";

    Harness h;
    HttpServer vendor;
    String vendorBase;
    final List<String> vendorSeen = new CopyOnWriteArrayList<>();

    @BeforeEach
    void start() throws IOException {
        h = new Harness();
        vendor = HttpServer.create(new InetSocketAddress("127.0.0.1", 0), 0);
        vendor.createContext("/", this::vendorAnswer);
        vendor.setExecutor(Executors.newVirtualThreadPerTaskExecutor());
        vendor.start();
        vendorBase = "http://127.0.0.1:" + vendor.getAddress().getPort();
    }

    @AfterEach
    void stop() {
        vendor.stop(0);
        h.close();
    }

    /// The vendor: a login that hands out a session, a call that wants it in the path, and a plain listing.
    private void vendorAnswer(HttpExchange e) throws IOException {
        String path = e.getRequestURI().getRawPath();
        vendorSeen.add(path + (e.getRequestURI().getRawQuery() == null ? "" : "?" + e.getRequestURI().getRawQuery()));
        String body;
        if (path.equals("/Membership/Login")) {
            body = "{\"SessionToken\":\"" + SESSION + "\",\"User\":\"u\"}";
        } else if (path.equals("/Vehicles/" + SESSION + "/GetVehicles")) {
            body = "{\"vehicles\":[1,2]}";
        } else if (path.startsWith("/REST/Items")) {
            body = "{\"items\":[\"a\",\"b\"]}";
        } else {
            body = "{\"error\":\"unknown\"}";
        }
        byte[] bytes = body.getBytes(StandardCharsets.UTF_8);
        e.getResponseHeaders().add("Content-Type", "application/json");
        e.sendResponseHeaders(path.startsWith("/Vehicles/") && !path.contains(SESSION) ? 404 : 200, bytes.length);
        e.getResponseBody().write(bytes);
        e.close();
    }

    /// Deploys the trace function; `window` adds a trace window (or several) and `more` changes anything else.
    private void deploy(Consumer<Fx.FunctionBuilder> window) {
        deploy(window, b -> {
        });
    }

    private void deploy(Consumer<Fx.FunctionBuilder> window, Consumer<Fx.FunctionBuilder> more) {
        Fx.Jar jar = TraceFn.jar();
        h.plane.addArtifact(jar);
        Fx.FunctionBuilder b = Fx.function("fn_t", "app.trace")
                .config("base", vendorBase)
                .config("password", PASSWORD)
                .secret("key", SECRET)
                .version(Fx.version(1, jar, TraceFn.describe(), "live"));
        window.accept(b);
        more.accept(b);
        h.plane.push(Fx.desired(1, b.build()));
        h.reconcile();
    }

    private long in(long ms) {
        return h.clock.millis() + ms;
    }

    private TestResponse get(String path, String... headers) {
        var call = h.http.get(BASE + path);
        for (int i = 0; i < headers.length; i += 2) {
            call = call.header(headers[i], headers[i + 1]);
        }
        return call.send();
    }

    static void await(BooleanSupplier condition) {
        long end = System.nanoTime() + TimeUnit.SECONDS.toNanos(10);
        while (!condition.getAsBoolean()) {
            if (System.nanoTime() > end) {
                throw new AssertionError("timed out waiting");
            }
            try {
                Thread.sleep(5);
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                throw new AssertionError(e);
            }
        }
    }

    private List<Control.TraceRecord> records() {
        return h.plane.traceRecords();
    }

    private List<Control.TraceRecord> awaitRecords(int n) {
        await(() -> records().size() >= n);
        return records();
    }

    private static List<Control.TraceRecord> ofKind(List<Control.TraceRecord> all, String kind) {
        return all.stream().filter(r -> r.kind().equals(kind)).toList();
    }

    private static JsonNode data(Control.TraceRecord r) {
        return Json.mapper().valueToTree(r.data());
    }

    /// Every record as the platform will receive it.
    private String wire() {
        return Json.mapper().writeValueAsString(records());
    }

    private void assertNothingTraced() {
        assertThat(h.traceShipper.queued() + h.traceShipper.shipped() + h.traceShipper.dropped()).isZero();
        assertThat(h.plane.traceCalls.get()).isZero();
    }

    // ── the http context ──────────────────────────────────────────────────────────────────────────

    @Test
    void withHttpLiveACallWritesAnHttpRecordItsLogLinesAndItsOutboundCall() {
        deploy(b -> b.trace("http", in(60_000), 1));

        TestResponse r = get("/ok?limit=5&token=abc123&name=ann", "X-Correlation-Id", "corr-1", "Authorization", "Bearer sekrit-bearer-value");
        assertThat(r.status()).isEqualTo(200);
        List<Control.TraceRecord> all = awaitRecords(5);

        assertThat(all).hasSize(5);
        assertThat(all).allSatisfy(rec -> {
            assertThat(rec.id()).isNotBlank();
            assertThat(rec.address()).isEqualTo("app.trace");
            assertThat(rec.version()).isEqualTo(1);
            assertThat(rec.invocationId()).isEqualTo(r.header("X-FlowCatalyst-Invocation"));
            assertThat(rec.correlationId()).isEqualTo("corr-1");
            assertThat(rec.context()).isEqualTo("http");
            assertThat(rec.at()).isEqualTo(h.clock.millis());
        });
        assertThat(h.plane.traceBatches.getFirst().runnerId()).isEqualTo("runner-1");
        assertThat(h.plane.traceBatches.getFirst().pool()).isEqualTo("default");

        Control.TraceRecord http = ofKind(all, "http").getFirst();
        assertThat(http.level()).isEqualTo("info");
        assertThat(http.message()).matches("GET /ok -> 200 \\(\\d+ ms\\)");
        JsonNode d = data(http);
        assertThat(d.path("outcome").asString()).isEqualTo("ok");
        assertThat(d.path("durationMs").asLong()).isGreaterThanOrEqualTo(0);
        JsonNode req = d.path("request");
        assertThat(req.path("method").asString()).isEqualTo("GET");
        assertThat(req.path("path").asString()).isEqualTo("/ok");
        assertThat(req.path("rawQuery").asString()).isEqualTo("limit=5&token=[redacted]&name=[redacted]");
        assertThat(req.path("headers").path("authorization").get(0).asString()).isEqualTo("[redacted]");
        assertThat(req.path("headers").path("x-correlation-id").get(0).asString()).isEqualTo("corr-1");
        assertThat(req.path("body").isNull()).isTrue();
        assertThat(req.path("bodyBytes").asLong()).isZero();
        assertThat(req.path("caller").path("kind").asString()).isEqualTo("anonymous");
        JsonNode res = d.path("response");
        assertThat(res.path("status").asInt()).isEqualTo(200);
        assertThat(res.path("body").asString()).isEqualTo("{\"ok\":true}");
        assertThat(res.path("bodyEncoding").asString()).isEqualTo("utf8");
        assertThat(res.path("bodyBytes").asLong()).isEqualTo(11);
        assertThat(res.path("headers").path("x-reply").get(0).asString()).isEqualTo("r1");
        assertThat(res.path("headers").path("content-type").get(0).asString()).isEqualTo("application/json");

        List<Control.TraceRecord> logs = ofKind(all, "log");
        assertThat(logs).extracting(Control.TraceRecord::message).containsExactlyInAnyOrder("hello from ok", "a debug line", "careful now");
        assertThat(logs).extracting(Control.TraceRecord::level).containsExactlyInAnyOrder("info", "debug", "warn");
        assertThat(logs).allSatisfy(rec -> assertThat(rec.data()).isNull());

        Control.TraceRecord outbound = ofKind(all, "outbound").getFirst();
        assertThat(outbound.level()).isEqualTo("info");
        assertThat(outbound.message()).isEqualTo("GET " + vendorBase + "/REST/Items -> 200 (" + data(outbound).path("durationMs").asLong() + " ms)");
        JsonNode o = data(outbound);
        assertThat(o.path("client").asString()).isEqualTo("fn-http");
        assertThat(o.path("request").path("method").asString()).isEqualTo("GET");
        assertThat(o.path("request").path("url").asString()).isEqualTo(vendorBase + "/REST/Items?limit=10&api_key=[redacted]");
        assertThat(o.path("response").path("status").asInt()).isEqualTo(200);
        assertThat(o.path("response").path("body").asString()).isEqualTo("{\"items\":[\"a\",\"b\"]}");
        assertThat(o.path("attempt").asInt()).isEqualTo(1);
        assertThat(o.has("failure")).isFalse();
        assertThat(wire()).doesNotContain("sekrit-bearer-value").doesNotContain("zzz-key-value").doesNotContain("abc123");
    }

    @Test
    void theRequestAndResponseBodiesAreRecordedAndABinaryOneIsBase64() {
        deploy(b -> b.trace("http", in(60_000), 1));

        TestResponse text = h.http.post(BASE + "/echo").header("Content-Type", "application/json").body("{\"a\":1,\"password\":\"hunter2hunter2\"}").send();
        assertThat(text.status()).isEqualTo(201);
        JsonNode d = data(awaitRecords(1).getFirst());

        assertThat(d.path("request").path("body").asString()).isEqualTo("{\"a\":1,\"password\":\"[redacted]\"}");
        assertThat(d.path("request").path("bodyBytes").asLong()).isEqualTo(35);
        assertThat(d.path("response").path("status").asInt()).isEqualTo(201);
        assertThat(d.path("response").path("bodyEncoding").asString()).isEqualTo("utf8");
    }

    @Test
    void aBodyWithANulIsBase64AndABigOneIsCutAtTheConfiguredLimit() {
        deploy(b -> b.trace("http", in(60_000), 1));
        byte[] binary = {'a', 0, 'b'};
        h.http.post(BASE + "/echo").header("Content-Type", "application/octet-stream").body(new String(binary, StandardCharsets.UTF_8)).send();
        JsonNode bin = data(awaitRecords(1).getFirst());
        assertThat(bin.path("request").path("bodyEncoding").asString()).isEqualTo("base64");
        assertThat(bin.path("request").path("body").asString()).isEqualTo(java.util.Base64.getEncoder().encodeToString(binary));
        assertThat(bin.path("request").path("bodyBytes").asLong()).isEqualTo(3);

        String big = "x".repeat(200_000);
        h.http.post(BASE + "/echo").header("Content-Type", "text/plain").body(big).send();
        await(() -> records().size() >= 2);
        JsonNode d = data(records().get(1));
        assertThat(d.path("request").path("bodyTruncated").asBoolean()).isTrue();
        assertThat(d.path("request").path("bodyBytes").asLong()).isEqualTo(200_000);
        assertThat(d.path("request").path("body").asString().length()).isLessThanOrEqualTo(65536);
        assertThat(Json.mapper().writeValueAsBytes(d).length).as("the platform's cap for an http record").isLessThanOrEqualTo(Control.MAX_TRACE_HTTP_DATA);
        assertThat(d.path("response").path("bodyTruncated").asBoolean()).isTrue();
    }

    @Test
    void aFunctionWithoutAWindowWritesNothing() {
        deploy(b -> {
        });

        assertThat(get("/ok").status()).isEqualTo(200);
        assertThat(get("/boom").status()).isEqualTo(500);

        assertNothingTraced();
    }

    @Test
    void anExpiredWindowWritesNothingAndTheClockIsCheckedOnEveryCall() {
        deploy(b -> b.trace("http", in(1_000), 1));

        get("/ok");
        awaitRecords(5);
        int before = records().size();
        h.clock.advance(Duration.ofSeconds(2));
        assertThat(get("/ok").status()).isEqualTo(200);

        // everything the first call wrote was queued before it answered; the second queued nothing
        assertThat(h.traceShipper.queued()).isZero();
        assertThat(records()).hasSize(before);
    }

    @Test
    void aWindowOfAnotherContextDoesNotTurnTheHttpRecordsOn() {
        deploy(b -> b.trace("sync", in(60_000), 1));

        assertThat(get("/ok").status()).isEqualTo(200);

        assertNothingTraced();
    }

    @Test
    void aTimeoutIsOutcomeTimeoutWithA504() {
        deploy(b -> b.trace("http", in(60_000), 1), b -> b.limits(16, 300, 1 << 20));

        assertThat(get("/sleep").status()).isEqualTo(504);

        Control.TraceRecord http = ofKind(awaitRecords(1), "http").getFirst();
        assertThat(http.level()).isEqualTo("error");
        assertThat(http.message()).matches("GET /sleep -> 504 \\(\\d+ ms\\)");
        assertThat(data(http).path("outcome").asString()).isEqualTo("timeout");
        assertThat(data(http).path("response").path("status").asInt()).isEqualTo(504);
        assertThat(data(http).path("response").path("body").asString()).contains("FUNCTION_TIMEOUT");
    }

    @Test
    void aThrownExceptionIsOutcomeErrorWithA500AndTheExceptionIsNotInTheRecord() {
        deploy(b -> b.trace("http", in(60_000), 1));

        assertThat(get("/boom").status()).isEqualTo(500);

        List<Control.TraceRecord> all = awaitRecords(2);
        Control.TraceRecord http = ofKind(all, "http").getFirst();
        assertThat(http.level()).isEqualTo("error");
        assertThat(data(http).path("outcome").asString()).isEqualTo("error");
        assertThat(data(http).path("response").path("status").asInt()).isEqualTo(500);
        assertThat(ofKind(all, "log")).extracting(Control.TraceRecord::message).containsExactly("about to fail");
        assertThat(wire()).doesNotContain("kaput");
    }

    @Test
    void aHeaderTheServerRefusesIsBadResponseWith502() {
        deploy(b -> b.trace("http", in(60_000), 1));

        TestResponse r = get("/badheader");

        assertThat(r.status()).isEqualTo(502);
        Control.TraceRecord http = ofKind(awaitRecords(1), "http").getFirst();
        assertThat(data(http).path("outcome").asString()).isEqualTo("bad_response");
        assertThat(http.level()).isEqualTo("error");
    }

    @Test
    void a4xxIsWarnAndARefusedCallWritesNoRecord() {
        deploy(b -> b.trace("http", in(60_000), 1));

        assertThat(get("/nothing-here").status()).isEqualTo(404);
        assertThat(h.http.post(BASE + "/ok").send().status()).isEqualTo(405);

        // refused before a permit was held: no invocation, no record
        assertNothingTraced();
    }

    // ── the sampler ───────────────────────────────────────────────────────────────────────────────

    @Test
    void aRateOfOneTracesEveryCallAndARateOfThreeOneInThree() {
        deploy(b -> b.trace("http", in(60_000), 1));
        for (int i = 0; i < 4; i++) {
            get("/pause/1");
        }
        assertThat(ofKind(awaitRecords(4), "http")).hasSize(4);

        h.plane.traceBatches.clear();
        deploy(b -> b.trace("http", in(60_000), 3));
        for (int i = 0; i < 9; i++) {
            get("/ok");
        }
        // 3 admitted invocations, each: 1 http + 3 logs + 1 outbound
        List<Control.TraceRecord> all = awaitRecords(15);

        assertThat(all.stream().map(Control.TraceRecord::invocationId).distinct()).hasSize(3);
        assertThat(ofKind(all, "http")).hasSize(3);
        for (String invocation : all.stream().map(Control.TraceRecord::invocationId).distinct().toList()) {
            assertThat(all.stream().filter(r -> r.invocationId().equals(invocation)).map(Control.TraceRecord::kind).sorted())
                    .as("a context is decided once per invocation, so an admitted call is whole")
                    .containsExactly("http", "log", "log", "log", "outbound");
        }
    }

    @Test
    void aBusyRefusalTakesNoSamplingTurn() throws Exception {
        deploy(b -> b.trace("http", in(60_000), 2), b -> b.limits(1, 30_000, 1 << 20));
        // call A takes the one permit and the first sampling turn (admitted: 0 % 2 == 0)
        var slow = Thread.ofVirtual().start(() -> get("/pause/400"));
        await(() -> h.listener.permits().available("app.trace") == 0);

        TestResponse busy = get("/ok");
        slow.join();
        // call C takes the second turn (1 % 2 != 0: not admitted). Had the 429 taken a turn, C would have been admitted.
        TestResponse c = get("/ok");

        assertThat(busy.status()).isEqualTo(429);
        assertThat(c.status()).isEqualTo(200);
        List<Control.TraceRecord> all = awaitRecords(1);
        assertThat(all.stream().map(Control.TraceRecord::invocationId).distinct()).hasSize(1);
        assertThat(all.getFirst().message()).startsWith("GET /pause/400");
        assertThat(h.traceShipper.queued()).isZero();
    }

    // ── what is replaced ──────────────────────────────────────────────────────────────────────────

    @Test
    void aSessionLearnedFromALoginAnswerIsOutOfEveryRecordAndTheLaterUrlsPath() {
        deploy(b -> b.trace("http", in(60_000), 1));

        TestResponse r = get("/ctrack");

        assertThat(r.status()).isEqualTo(200);
        assertThat(r.tree().path("session").asString()).as("the function itself does see it").isEqualTo(SESSION);
        List<Control.TraceRecord> all = awaitRecords(3);
        List<Control.TraceRecord> outbound = ofKind(all, "outbound");
        assertThat(outbound).hasSize(2);
        assertThat(data(outbound.get(0)).path("request").path("url").asString()).isEqualTo(vendorBase + "/Membership/Login?username=[redacted]&password=[redacted]");
        assertThat(data(outbound.get(0)).path("response").path("body").asString()).isEqualTo("{\"SessionToken\":\"[redacted]\",\"User\":\"u\"}");
        assertThat(data(outbound.get(1)).path("request").path("url").asString()).isEqualTo(vendorBase + "/Vehicles/[redacted]/GetVehicles");
        assertThat(outbound.get(1).message()).isEqualTo("GET " + vendorBase + "/Vehicles/[redacted]/GetVehicles -> 200 ("
                + data(outbound.get(1)).path("durationMs").asLong() + " ms)");
        assertThat(data(ofKind(all, "http").getFirst()).path("response").path("body").asString()).contains("\"session\":\"[redacted]\"");
        assertThat(wire()).doesNotContain(PASSWORD).doesNotContain(SESSION);
        assertThat(vendorSeen).as("the vendor was really called with them").anyMatch(p -> p.contains(PASSWORD)).anyMatch(p -> p.contains(SESSION));
    }

    @Test
    void aSessionObtainedInInitAndConcealedIsRedactedInTheLaterUrlsPath() {
        deploy(b -> b.trace("http", in(60_000), 1), b -> b.config("loginInInit", "yes"));
        assertThat(vendorSeen).as("init logged in, outside any invocation").anyMatch(p -> p.startsWith("/Membership/Login"));

        assertThat(get("/ctrack-init").status()).isEqualTo(200);

        List<Control.TraceRecord> all = awaitRecords(2);
        assertThat(ofKind(all, "outbound")).hasSize(1);
        assertThat(data(ofKind(all, "outbound").getFirst()).path("request").path("url").asString()).isEqualTo(vendorBase + "/Vehicles/[redacted]/GetVehicles");
        assertThat(wire()).doesNotContain(SESSION).doesNotContain(PASSWORD);
        assertThat(vendorSeen).anyMatch(p -> p.contains("/Vehicles/" + SESSION + "/GetVehicles"));
    }

    @Test
    void aDeclaredSecretIsReplacedInALogLineATraceMessageAndDataAndAResponseBody() {
        deploy(b -> b.trace("http", in(60_000), 1).trace("sync", in(60_000), 1));

        TestResponse r = get("/secret");

        assertThat(r.tree().path("echo").asString()).as("the caller gets the real answer").isEqualTo(SECRET);
        List<Control.TraceRecord> all = awaitRecords(3);
        assertThat(wire()).doesNotContain(SECRET).contains("[redacted]");
        assertThat(ofKind(all, "log").getFirst().message()).isEqualTo("using [redacted]");
        Control.TraceRecord trace = ofKind(all, "trace").getFirst();
        assertThat(trace.message()).isEqualTo("msg [redacted]");
        assertThat(data(trace).path("k").asString()).isEqualTo("[redacted]");
        assertThat(data(trace).path("list").get(0).asString()).isEqualTo("[redacted]");
        assertThat(data(trace).path("list").get(1).asString()).isEqualTo("x");
        assertThat(data(ofKind(all, "http").getFirst()).path("response").path("body").asString()).isEqualTo("{\"echo\":\"[redacted]\"}");
    }

    @Test
    void whatWasConcealedForAFunctionIsForgottenWhenItLeavesTheDocument() {
        deploy(b -> {
        });
        h.reconciler.traces().conceal("app.trace", "a-concealed-value");
        assertThat(h.reconciler.traces().concealed("app.trace")).containsExactly("a-concealed-value");

        h.plane.push(Fx.desired(2));
        h.reconcile();

        assertThat(h.reconciler.traces().concealed("app.trace")).isEmpty();
    }

    // ── the author API, through a real host ────────────────────────────────────────────────────────

    @Test
    void theAuthorApiOnADeclaredContext() {
        deploy(b -> b.trace("sync", in(60_000), 1));

        TestResponse r = get("/author");

        assertThat(r.tree().path("syncBefore").asBoolean()).as("live and sampled in").isTrue();
        assertThat(r.tree().path("http").asBoolean()).as("http is not live").isFalse();
        assertThat(r.tree().path("nope").asBoolean()).as("undeclared").isFalse();
        List<Control.TraceRecord> all = awaitRecords(4);
        assertThat(all).extracting(Control.TraceRecord::message).containsExactly("first", "second", "third", "fourth");
        assertThat(all).extracting(Control.TraceRecord::context).containsOnly("sync");
        assertThat(all).extracting(Control.TraceRecord::kind).containsOnly("trace");
        assertThat(all).extracting(Control.TraceRecord::level).containsExactly("info", "info", "warn", "info");
        assertThat(all).extracting(Control.TraceRecord::reference).containsExactly("", "order-77", "own-ref", "order-77");
        assertThat(data(all.get(1)).path("n").asInt()).isEqualTo(1);
        assertThat(all.get(0).data()).isNull();
        assertThat(h.reconciler.traces().warnings()).as("one warning for 'nope' and one for 'http', each once").isEqualTo(2);
    }

    @Test
    void aReferenceIsStampedOnTheHttpAndLogRecordsToo() {
        deploy(b -> b.trace("http", in(60_000), 1).trace("sync", in(60_000), 1));

        get("/author");

        List<Control.TraceRecord> all = awaitRecords(5);
        assertThat(ofKind(all, "http").getFirst().reference()).isEqualTo("order-77");
    }

    @Test
    void traceDataOverTheLimitIsNotWrittenButTheNextRecordIs() {
        deploy(b -> b.trace("sync", in(60_000), 1));

        get("/big");

        List<Control.TraceRecord> all = awaitRecords(1);
        assertThat(all).extracting(Control.TraceRecord::message).containsExactly("small");
    }

    @Test
    void anInvocationWritesAtMost500LogRecords500TraceRecordsAnd100OutboundRecords() {
        deploy(b -> b.trace("http", in(60_000), 1).trace("sync", in(60_000), 1));

        assertThat(get("/flood").status()).isEqualTo(200);

        await(() -> h.traceShipper.queued() == 0 && h.traceShipper.shipped() + h.traceShipper.dropped() >= 1101);
        List<Control.TraceRecord> all = records();
        assertThat(ofKind(all, "log")).hasSize(500);
        assertThat(ofKind(all, "trace")).hasSize(500);
        assertThat(ofKind(all, "outbound")).hasSize(100);
        assertThat(ofKind(all, "http")).hasSize(1);
    }

    @Test
    void shippedIdsAreUnique() {
        deploy(b -> b.trace("http", in(60_000), 1));
        for (int i = 0; i < 3; i++) {
            get("/ok");
        }

        List<Control.TraceRecord> all = awaitRecords(15);

        assertThat(all.stream().map(Control.TraceRecord::id).distinct()).hasSize(all.size());
        assertThat(ofKind(all, "http")).hasSize(3);
    }
}
