package io.flowcatalyst.ingestor.functions.trace;

import static org.assertj.core.api.Assertions.assertThat;

import io.flowcatalyst.framework.http.Json;
import io.flowcatalyst.framework.httpclient.HttpClients;
import io.flowcatalyst.function.TraceOptions;
import io.flowcatalyst.ingestor.functions.FakeControlPlane;
import io.flowcatalyst.ingestor.functions.context.ContextFactory;
import io.flowcatalyst.ingestor.functions.context.DbPools;
import io.flowcatalyst.ingestor.functions.context.HostFunctionContext;
import io.flowcatalyst.ingestor.functions.context.LogBudget;
import io.flowcatalyst.ingestor.functions.context.OutboundClients;
import io.flowcatalyst.ingestor.functions.control.Control;
import io.flowcatalyst.ingestor.functions.control.Describe;
import java.time.Clock;
import java.time.Instant;
import java.time.ZoneOffset;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.List;
import java.util.Map;
import java.util.concurrent.CopyOnWriteArrayList;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import tools.jackson.databind.JsonNode;

/// The gate ([FunctionTraces#begin]) and the author API on a real host context ([HostFunctionContext]) bound to an
/// invocation's trace, with the records in a list.
class FunctionTracesTest {

    static final Instant NOW = Instant.parse("2026-11-06T12:00:00Z");
    static final long NOW_MS = NOW.toEpochMilli();

    final Clock clock = Clock.fixed(NOW, ZoneOffset.UTC);
    final List<Control.TraceRecord> sink = new CopyOnWriteArrayList<>();
    FunctionTraces traces;
    HttpClients clients;
    DbPools pools;
    ContextFactory factory;

    @BeforeEach
    void start() {
        traces = new FunctionTraces(clock, sink::add, FunctionTraces.DEFAULT_BODY_BYTES);
        clients = HttpClients.create();
        pools = new DbPools(DbPools.Limits.DEFAULTS, clock);
        factory = new ContextFactory(new OutboundClients(clients), clock, new FakeControlPlane(), pools, LogBudget.Limits.DEFAULTS, traces);
    }

    @AfterEach
    void stop() {
        pools.close();
        clients.close();
    }

    private static Control.FunctionDoc doc(Control.TraceState... windows) {
        return new Control.FunctionDoc("fn_1", "app.hello", "app_1", null, null, false, null, Map.of(), Map.of("s", "hunter2-secret"), Map.of(),
                List.of(windows), List.of());
    }

    private HostFunctionContext context(Control.FunctionDoc doc) {
        Describe describe = new Describe(1, List.of(), List.of(), List.of("s"), List.of(), List.of(), List.of(), List.of(), false,
                new Describe.Jvm("a.B"), List.of(new Describe.TraceContext("sync", null), new Describe.TraceContext("other", null)));
        return factory.build(doc, 2, describe);
    }

    private static Control.TraceState window(String context, long untilMs, int rate) {
        return new Control.TraceState(context, untilMs, rate);
    }

    /// Runs `body` as an invocation of `doc` that the gate admitted; `null` trace when it did not.
    private @org.jspecify.annotations.Nullable InvocationTrace invoke(Control.FunctionDoc doc, Runnable body) {
        InvocationTrace trace = traces.begin(doc, 2, "inv-1", "corr-1");
        if (trace == null) {
            body.run();
        } else {
            ScopedValue.where(InvocationTrace.CURRENT, trace).run(body);
        }
        return trace;
    }

    private static JsonNode data(Control.TraceRecord r) {
        return Json.mapper().valueToTree(r.data());
    }

    // ── the gate ──────────────────────────────────────────────────────────────────────────────────

    @Test
    void aFunctionWithNoWindowGetsNoTraceAndNothingIsAllocatedOrKept() {
        assertThat(traces.begin(doc(), 1, "i", null)).isNull();
        assertThat(sink).isEmpty();
    }

    @Test
    void anExpiredWindowIsNotLiveAndALiveOneIs() {
        assertThat(traces.begin(doc(window("http", NOW_MS, 1)), 1, "i", null)).as("until == now is over").isNull();
        assertThat(traces.begin(doc(window("http", NOW_MS - 1, 1)), 1, "i", null)).isNull();
        InvocationTrace live = traces.begin(doc(window("http", NOW_MS + 1, 1)), 1, "i", null);
        assertThat(live).isNotNull();
        assertThat(live.httpLive()).isTrue();
        assertThat(live.isLive("http")).isTrue();
        assertThat(live.isLive("sync")).isFalse();
    }

    @Test
    void aRateBelowOneIsTreatedAsOne() {
        Control.FunctionDoc d = doc(window("http", NOW_MS + 1000, 0), window("sync", NOW_MS + 1000, -4));
        for (int i = 0; i < 5; i++) {
            InvocationTrace t = traces.begin(d, 1, "i", null);
            assertThat(t).isNotNull();
            assertThat(t.isLive("http")).isTrue();
            assertThat(t.isLive("sync")).isTrue();
        }
    }

    @Test
    void aRateOfOneAdmitsEveryInvocation() {
        Control.FunctionDoc d = doc(window("http", NOW_MS + 1000, 1));
        int admitted = 0;
        for (int i = 0; i < 10; i++) {
            if (traces.begin(d, 1, "i" + i, null) != null) {
                admitted++;
            }
        }
        assertThat(admitted).isEqualTo(10);
    }

    @Test
    void aRateOfThreeAdmitsOneInThreeStartingWithTheFirst() {
        Control.FunctionDoc d = doc(window("http", NOW_MS + 1000, 3));
        List<Boolean> admitted = new ArrayList<>();
        for (int i = 0; i < 9; i++) {
            admitted.add(traces.begin(d, 1, "i" + i, null) != null);
        }
        assertThat(admitted).containsExactly(true, false, false, true, false, false, true, false, false);
    }

    @Test
    void eachContextHasItsOwnSamplerAndEachFunctionToo() {
        Control.FunctionDoc d = doc(window("http", NOW_MS + 1000, 2), window("sync", NOW_MS + 1000, 3));
        List<String> live = new ArrayList<>();
        for (int i = 0; i < 6; i++) {
            InvocationTrace t = traces.begin(d, 1, "i" + i, null);
            live.add(t == null ? "-" : (t.isLive("http") ? "h" : "") + (t.isLive("sync") ? "s" : ""));
        }
        // http: 0,2,4 ; sync: 0,3
        assertThat(live).containsExactly("hs", "-", "h", "s", "h", "-");

        Control.FunctionDoc other = new Control.FunctionDoc("fn_2", "app.other", "app_1", null, null, false, null, Map.of(), Map.of(), Map.of(),
                List.of(window("http", NOW_MS + 1000, 2)), List.of());
        assertThat(traces.begin(other, 1, "x", null)).as("its own counter starts at 0").isNotNull();
    }

    @Test
    void theSamplingTurnIsTakenOncePerInvocationNotPerQuestion() {
        Control.FunctionDoc d = doc(window("sync", NOW_MS + 1000, 2));
        HostFunctionContext ctx = context(d);
        InvocationTrace t = traces.begin(d, 1, "i", null);
        assertThat(t).isNotNull();
        ScopedValue.where(InvocationTrace.CURRENT, t).run(() -> {
            for (int i = 0; i < 5; i++) {
                assertThat(ctx.tracing("sync")).isTrue();
            }
        });
        assertThat(traces.begin(d, 1, "j", null)).as("the next turn is the second one").isNull();
    }

    // ── the author API ─────────────────────────────────────────────────────────────────────────────

    @Test
    void tracingIsTrueOnlyForADeclaredContextThatIsLiveAndSampledIn() {
        Control.FunctionDoc d = doc(window("sync", NOW_MS + 1000, 2), window("ghost", NOW_MS + 1000, 1));
        HostFunctionContext ctx = context(d);
        List<Boolean> seen = new ArrayList<>();

        invoke(d, () -> seen.addAll(List.of(ctx.tracing("sync"), ctx.tracing("other"), ctx.tracing("ghost"), ctx.tracing("http"))));
        // second invocation: sync not sampled in, ghost still live but undeclared
        invoke(d, () -> seen.addAll(List.of(ctx.tracing("sync"), ctx.tracing("ghost"))));

        assertThat(seen).containsExactly(true, false, false, false, false, false);
        assertThat(ctx.tracing("sync")).as("outside an invocation").isFalse();
    }

    @Test
    void tracingHttpIsTrueWhenItIsLive() {
        Control.FunctionDoc d = doc(window("http", NOW_MS + 1000, 1));
        HostFunctionContext ctx = context(d);
        List<Boolean> seen = new ArrayList<>();

        invoke(d, () -> seen.add(ctx.tracing("http")));

        assertThat(seen).containsExactly(true);
    }

    @Test
    void aTraceOnAnUndeclaredContextOrHttpRecordsNothingAndWarnsOncePerContext() {
        Control.FunctionDoc d = doc(window("sync", NOW_MS + 1000, 1), window("http", NOW_MS + 1000, 1));
        HostFunctionContext ctx = context(d);

        invoke(d, () -> {
            ctx.trace("nope", "a");
            ctx.trace("nope", "b");
            ctx.trace("http", "c");
            ctx.trace("http", "d");
            ctx.trace("", "e");
        });
        invoke(d, () -> ctx.trace("nope", "again"));

        assertThat(sink).isEmpty();
        assertThat(traces.warnings()).isEqualTo(3);
    }

    @Test
    void aTraceIsRecordedOnlyWhenItsContextIsLiveForTheInvocation() {
        Control.FunctionDoc d = doc(window("sync", NOW_MS + 1000, 1));
        HostFunctionContext ctx = context(d);

        invoke(d, () -> {
            ctx.trace("sync", "yes");
            ctx.trace("other", "declared but not live");
        });
        ctx.trace("sync", "outside any invocation");
        ctx.trace("sync", "outside again", Map.of("k", "v"), new TraceOptions("warn", "r"));

        assertThat(sink).extracting(Control.TraceRecord::message).containsExactly("yes");
        Control.TraceRecord r = sink.getFirst();
        assertThat(r.kind()).isEqualTo("trace");
        assertThat(r.context()).isEqualTo("sync");
        assertThat(r.level()).isEqualTo("info");
        assertThat(r.address()).isEqualTo("app.hello");
        assertThat(r.version()).isEqualTo(2);
        assertThat(r.invocationId()).isEqualTo("inv-1");
        assertThat(r.correlationId()).isEqualTo("corr-1");
        assertThat(r.at()).isEqualTo(NOW_MS);
        assertThat(r.id()).isNotBlank();
        assertThat(r.data()).isNull();
    }

    @Test
    void aReferenceIsStampedOnLaterRecordsAndAPerCallOneOnlyOnItsOwn() {
        Control.FunctionDoc d = doc(window("sync", NOW_MS + 1000, 1));
        HostFunctionContext ctx = context(d);

        invoke(d, () -> {
            ctx.trace("sync", "before");
            ctx.setReference("order-77");
            ctx.trace("sync", "after");
            ctx.trace("sync", "own", null, new TraceOptions(null, "line-3"));
            ctx.trace("sync", "after own");
            ctx.setReference("");
            ctx.trace("sync", "cleared");
        });

        assertThat(sink).extracting(Control.TraceRecord::reference).containsExactly("", "order-77", "line-3", "order-77", "");
    }

    @Test
    void aReferenceIsCutTo200CodePointsNotChars() {
        Control.FunctionDoc d = doc(window("sync", NOW_MS + 1000, 1));
        HostFunctionContext ctx = context(d);
        String emoji = "😀";

        invoke(d, () -> {
            ctx.setReference(emoji.repeat(250));
            ctx.trace("sync", "a");
            ctx.trace("sync", "b", null, new TraceOptions(null, "x".repeat(300)));
        });

        assertThat(sink.get(0).reference()).isEqualTo(emoji.repeat(200));
        assertThat(sink.get(1).reference()).isEqualTo("x".repeat(200));
    }

    @Test
    void theLevelIsOneOfTheFourAndAnythingElseIsInfo() {
        Control.FunctionDoc d = doc(window("sync", NOW_MS + 1000, 1));
        HostFunctionContext ctx = context(d);

        invoke(d, () -> {
            for (String level : new String[] {"debug", "info", "warn", "error", "WARN", "fatal", "", null}) {
                ctx.trace("sync", "m", null, new TraceOptions(level, null));
            }
        });

        assertThat(sink).extracting(Control.TraceRecord::level).containsExactly("debug", "info", "warn", "error", "info", "info", "info", "info");
    }

    @Test
    void aMessageIsCutTo4096CharactersAndAnyNulIsTakenOut() {
        Control.FunctionDoc d = doc(window("sync", NOW_MS + 1000, 1));
        HostFunctionContext ctx = context(d);

        invoke(d, () -> {
            ctx.trace("sync", "a".repeat(5000));
            ctx.trace("sync", "x\0y");
            ctx.trace("sync", "😀".repeat(2049));
        });

        assertThat(sink.get(0).message()).hasSize(4096);
        assertThat(sink.get(1).message()).isEqualTo("xy");
        assertThat(sink.get(2).message().length()).isEqualTo(4096);
        assertThat(Character.isHighSurrogate(sink.get(2).message().charAt(4095))).isFalse();
    }

    @Test
    void dataOver32768BytesIsNotWrittenAndDataUnderIsKept() {
        Control.FunctionDoc d = doc(window("sync", NOW_MS + 1000, 1));
        HostFunctionContext ctx = context(d);

        invoke(d, () -> {
            ctx.trace("sync", "over", Map.of("blob", "x".repeat(32768)));
            ctx.trace("sync", "under", Map.of("blob", "x".repeat(32000), "n", 5, "nested", Map.of("a", List.of(1, 2))));
        });

        assertThat(sink).extracting(Control.TraceRecord::message).containsExactly("under");
        assertThat(data(sink.getFirst()).path("n").asInt()).isEqualTo(5);
        assertThat(data(sink.getFirst()).path("nested").path("a").get(1).asInt()).isEqualTo(2);
    }

    @Test
    void anInvocationWritesAtMost500TraceRecords() {
        Control.FunctionDoc d = doc(window("sync", NOW_MS + 1000, 1));
        HostFunctionContext ctx = context(d);

        invoke(d, () -> {
            for (int i = 0; i < 700; i++) {
                ctx.trace("sync", "n" + i);
            }
        });
        invoke(d, () -> ctx.trace("sync", "next invocation"));

        assertThat(sink).hasSize(501);
        assertThat(sink.get(499).message()).isEqualTo("n499");
        assertThat(sink.get(500).message()).isEqualTo("next invocation");
    }

    @Test
    void aTraceCallNeverThrows() {
        Control.FunctionDoc d = doc(window("sync", NOW_MS + 1000, 1));
        HostFunctionContext ctx = context(d);
        Map<String, Object> cyclic = new HashMap<>();
        cyclic.put("self", cyclic);

        invoke(d, () -> {
            ctx.trace("sync", "cyclic", cyclic);
            ctx.trace("sync", null);
            ctx.trace(null, "no context");
            ctx.trace("sync", "no options", null, null);
            ctx.setReference(null);
            ctx.conceal(null);
            ctx.tracing(null);
        });
        ctx.setReference("outside");
        ctx.conceal("a-value-outside");

        assertThat(sink).extracting(Control.TraceRecord::message).containsExactly("", "no options");
    }

    @Test
    void aDeclaredSecretAndAConcealedValueAreReplacedInTheMessageAndTheData() {
        Control.FunctionDoc d = doc(window("sync", NOW_MS + 1000, 1));
        HostFunctionContext ctx = context(d);
        ctx.conceal("session-value-1");

        invoke(d, () -> {
            ctx.trace("sync", "token session-value-1 and hunter2-secret", Map.of("hunter2-secret", "ok", "a", List.of("session-value-1")));
            ctx.setReference("ref hunter2-secret");
            ctx.trace("sync", "second");
            ctx.trace("sync", "third", null, new TraceOptions(null, "own session-value-1"));
        });

        assertThat(sink.get(0).message()).isEqualTo("token [redacted] and [redacted]");
        assertThat(data(sink.get(0)).path("[redacted]").asString()).as("keys too").isEqualTo("ok");
        assertThat(data(sink.get(0)).path("a").get(0).asString()).isEqualTo("[redacted]");
        assertThat(sink.get(2).reference()).isEqualTo("own [redacted]");
    }

    @Test
    void concealIgnoresShortValuesAndHoldsTheLast32() {
        Control.FunctionDoc d = doc(window("sync", NOW_MS + 1000, 1));
        HostFunctionContext ctx = context(d);
        ctx.conceal("abc");
        for (int i = 0; i < 33; i++) {
            ctx.conceal("value-" + String.format("%02d", i));
        }

        invoke(d, () -> ctx.trace("sync", "abc value-00 value-01 value-32"));

        assertThat(traces.concealed("app.hello")).hasSize(32).doesNotContain("abc", "value-00");
        assertThat(sink.getFirst().message()).as("the oldest went out of the deque; the short one was never kept")
                .isEqualTo("abc value-00 [redacted] [redacted]");
    }

    @Test
    void retainDropsTheValuesOfAFunctionThatLeft() {
        traces.conceal("app.hello", "value-one");
        traces.conceal("app.other", "value-two");

        traces.retain(List.of("app.other"));

        assertThat(traces.concealed("app.hello")).isEmpty();
        assertThat(traces.concealed("app.other")).containsExactly("value-two");
    }
}
