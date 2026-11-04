package io.flowcatalyst.ingestor.functions.reconcile;

import static org.assertj.core.api.Assertions.assertThat;

import io.flowcatalyst.framework.http.TestResponse;
import io.flowcatalyst.function.FunctionAddress;
import io.flowcatalyst.ingestor.functions.Fx;
import io.flowcatalyst.ingestor.functions.Harness;
import io.flowcatalyst.ingestor.functions.control.Control;
import io.flowcatalyst.ingestor.functions.control.ControlPlaneException;
import io.flowcatalyst.ingestor.functions.http.PinnedVersions;
import io.flowcatalyst.ingestor.functions.load.LoadedFunction;
import java.time.Duration;
import java.util.ArrayList;
import java.util.List;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

/// Desired state → fetch → load → heartbeat, against a fake control plane and real jars built in the test: the
/// swap (new before old, drain), lazy and warm, the heartbeat states with the Go runner's rule (only a permanent
/// failure is FAILED), retries, and the outage rule (nothing is unloaded).
class ReconcilerTest {

    static final FunctionAddress ADDRESS = FunctionAddress.parse("app.hello");
    static final String HELLO_ENDPOINTS = Fx.endpoint("GET", "/hello/{name}", "none");

    Harness h;

    @BeforeEach
    void start() {
        h = new Harness();
    }

    @AfterEach
    void stop() {
        h.close();
    }

    static String helloDescribe() {
        return Fx.describe("fixture.app.Hello", List.of(HELLO_ENDPOINTS));
    }

    Control.FunctionDoc fn(Control.VersionDoc... versions) {
        Fx.FunctionBuilder b = Fx.function("fn_1", "app.hello");
        for (Control.VersionDoc v : versions) {
            b.version(v);
        }
        return b.build();
    }

    private TestResponseView call(String path) {
        TestResponse r = h.http.get(path).send();
        return new TestResponseView(r);
    }

    record TestResponseView(TestResponse r) {
        int status() {
            return r.status();
        }

        String v() {
            return r.tree().path("v").asString("");
        }
    }

    private Control.VersionReport report(int number) {
        return h.plane.report("fn_1", number);
    }

    private List<String> statesEverReported(int number) {
        List<String> out = new ArrayList<>();
        for (Control.Heartbeat hb : h.plane.heartbeats) {
            for (Control.VersionReport r : hb.versions()) {
                if (r.number() == number) {
                    out.add(r.state());
                }
            }
        }
        return out;
    }

    // ── loading and reporting ────────────────────────────────────────────────────────────────

    @Test
    void aLiveVersionIsFetchedLoadedServedAndReportedLoaded() {
        Fx.Jar v1 = Fx.hello("1");
        h.plane.addArtifact(v1);
        h.plane.push(Fx.desired(7, fn(Fx.version(1, v1, helloDescribe(), "live"))));

        assertThat(h.reconcile()).isEqualTo(Reconciler.Outcome.CHANGED);

        assertThat(report(1).state()).isEqualTo("LOADED");
        assertThat(call("/fn/app.hello/hello/ann").v()).isEqualTo("1");
        assertThat(h.plane.lastRequestedPool).isEqualTo("default");
        Control.Heartbeat hb = h.plane.heartbeats.get(h.plane.heartbeats.size() - 1);
        assertThat(hb.runnerId()).isEqualTo("runner-1");
        assertThat(hb.pool()).isEqualTo("default");
        assertThat(hb.revision()).as("the revision of the document applied").isEqualTo(7);
        assertThat(hb.capabilities().protocol()).isEqualTo(1);
        assertThat(hb.capabilities().runtimes()).containsExactly("jvm");
        assertThat(hb.capabilities().runnerKind()).isEqualTo("ingestor-jvm");
        assertThat(hb.capabilities().version()).isEqualTo("test");
        assertThat(hb.budget().limitBytes()).isPositive();
        assertThat(hb.startedAt()).isNotBlank();
    }

    @Test
    void anUnchangedDocumentKeepsEverythingAndSendsAHeartbeatAgain() {
        Fx.Jar v1 = Fx.hello("1");
        h.plane.addArtifact(v1);
        h.plane.push(Fx.desired(1, fn(Fx.version(1, v1, helloDescribe(), "live"))));
        h.reconcile();
        LoadedFunction first = h.registry.peek(ADDRESS);
        int fetches = h.plane.artifactFetches.get();
        int beats = h.plane.heartbeats.size();

        assertThat(h.reconcile()).isEqualTo(Reconciler.Outcome.NOT_MODIFIED);

        assertThat(h.registry.peek(ADDRESS)).isSameAs(first);
        assertThat(h.plane.artifactFetches.get()).isEqualTo(fetches);
        assertThat(h.plane.heartbeats.size()).isGreaterThan(beats);
    }

    @Test
    void aVersionOfAnotherRuntimeIsNeitherHeldNorReported() {
        Control.VersionDoc wasm = new Control.VersionDoc(1, "0".repeat(64), "wasm", 1, Fx.json(helloDescribe()), List.of("live"));
        h.plane.push(Fx.desired(1, fn(wasm)));

        h.reconcile();

        assertThat(report(1)).isNull();
        assertThat(h.plane.artifactFetches.get()).isZero();
        assertThat(call("/fn/app.hello/hello/ann").status()).as("the function has no jvm version here").isEqualTo(503);
    }

    // ── the swap ────────────────────────────────────────────────────────────────────────────

    @Test
    void aNewLiveVersionReplacesTheOldNewBeforeOldAndTheOldIsClosed() throws Exception {
        Fx.Jar v1 = Fx.hello("1");
        Fx.Jar v2 = Fx.hello("2");
        h.plane.addArtifact(v1);
        h.plane.addArtifact(v2);
        h.plane.push(Fx.desired(1, fn(Fx.version(1, v1, helloDescribe(), "live"))));
        h.reconcile();
        LoadedFunction old = h.registry.peek(ADDRESS);
        assertThat(call("/fn/app.hello/hello/ann").v()).isEqualTo("1");

        h.plane.push(Fx.desired(2, fn(Fx.version(2, v2, helloDescribe(), "live"))));
        h.reconcile();

        assertThat(call("/fn/app.hello/hello/ann").v()).isEqualTo("2");
        assertThat(h.registry.peek(ADDRESS)).isNotSameAs(old);
        awaitClosed(old);
        assertThat(report(2).state()).isEqualTo("LOADED");
        assertThat(report(1)).as("a version out of the document is not reported").isNull();
    }

    @Test
    void aNewVersionThatFailsToLoadLeavesTheOldOneServing() {
        Fx.Jar v1 = Fx.hello("1");
        Fx.Jar broken = Fx.jar("fixture.app.Other", "package fixture.app; public final class Other {}");
        h.plane.addArtifact(v1);
        h.plane.addArtifact(broken);
        h.plane.push(Fx.desired(1, fn(Fx.version(1, v1, helloDescribe(), "live"))));
        h.reconcile();

        h.plane.push(Fx.desired(2, fn(Fx.version(2, broken, Fx.describe("fixture.app.Hello", List.of(HELLO_ENDPOINTS)), "live"))));
        h.reconcile();

        assertThat(call("/fn/app.hello/hello/ann").v()).as("the old version keeps serving").isEqualTo("1");
        assertThat(report(2).state()).isEqualTo("FAILED");
        assertThat(report(2).reason()).isEqualTo("LOAD:ENTRYPOINT_NOT_FOUND");
    }

    @Test
    void aCandidateIsProvenThenPromotedAsTheSameInstance() {
        Fx.Jar v1 = Fx.hello("1");
        Fx.Jar v2 = Fx.hello("2");
        h.plane.addArtifact(v1);
        h.plane.addArtifact(v2);
        h.plane.push(Fx.desired(1, fn(
                Fx.version(1, v1, helloDescribe(), "live"),
                Fx.version(2, v2, helloDescribe(), "candidate"))));
        h.reconcile();

        // proven without being served: the platform promotes a candidate only after a LOADED report
        assertThat(report(2).state()).isEqualTo("LOADED");
        assertThat(call("/fn/app.hello/hello/ann").v()).isEqualTo("1");
        LoadedFunction proven = h.reconciler.pinned().peek(new PinnedVersions.Key("app.hello", 2));
        assertThat(proven).isNotNull();

        h.plane.push(Fx.desired(2, fn(
                Fx.version(1, v1, helloDescribe(), "candidate"),
                Fx.version(2, v2, helloDescribe(), "live"))));
        h.reconcile();

        assertThat(h.registry.peek(ADDRESS)).as("the instance that was proven now serves").isSameAs(proven);
        assertThat(call("/fn/app.hello/hello/ann").v()).isEqualTo("2");
    }

    @Test
    void functionsAndVersionsLeavingTheDocumentAreClosed() throws Exception {
        Fx.Jar v1 = Fx.hello("1");
        h.plane.addArtifact(v1);
        h.plane.push(Fx.desired(1, fn(Fx.version(1, v1, helloDescribe(), "live", "alias:blue"))));
        h.reconcile();
        LoadedFunction fnV1 = h.registry.peek(ADDRESS);

        h.plane.push(Fx.desired(2));
        h.reconcile();

        assertThat(h.registry.peek(ADDRESS)).isNull();
        awaitClosed(fnV1);
        assertThat(call("/fn/app.hello/hello/ann").status()).isEqualTo(404);
        assertThat(h.plane.heartbeats.get(h.plane.heartbeats.size() - 1).versions()).isEmpty();
    }

    @Test
    void aFunctionLeavingTheDocumentLosesItsCachedPlatformToken() {
        Fx.Jar v1 = Fx.hello("1");
        h.plane.addArtifact(v1);
        h.plane.push(Fx.desired(1, fn(Fx.version(1, v1, helloDescribe(), "live"))));
        h.reconcile();
        assertThat(h.contexts.platformTokens().token("fn_1", 1, false)).isEqualTo("fake-platform-token");
        assertThat(h.contexts.platformTokens().size()).isEqualTo(1);

        h.plane.push(Fx.desired(2));
        h.reconcile();

        assertThat(h.contexts.platformTokens().size()).isZero();
    }

    @Test
    void aFunctionWithNoLiveVersionIsClosedButItsCandidateIsStillProven() {
        Fx.Jar v1 = Fx.hello("1");
        h.plane.addArtifact(v1);
        h.plane.push(Fx.desired(1, fn(Fx.version(1, v1, helloDescribe(), "live"))));
        h.reconcile();

        h.plane.push(Fx.desired(2, fn(Fx.version(1, v1, helloDescribe(), "candidate"))));
        h.reconcile();

        assertThat(h.registry.peek(ADDRESS)).isNull();
        assertThat(call("/fn/app.hello/hello/ann").status()).isEqualTo(503);
        assertThat(report(1).state()).isEqualTo("LOADED");
    }

    // ── lazy and warm ─────────────────────────────────────────────────────────────────────────

    @Test
    void aLazyFunctionIsClosedWhenIdleAndLoadedAgainByItsNextCall() throws Exception {
        Fx.Jar v1 = Fx.hello("1");
        h.plane.addArtifact(v1);
        h.plane.push(Fx.desired(1, fn(Fx.version(1, v1, helloDescribe(), "live"))));
        h.reconcile();
        LoadedFunction first = h.registry.peek(ADDRESS);

        h.clock.advance(Duration.ofHours(2));
        h.reconcile();

        assertThat(h.registry.peek(ADDRESS)).as("idle: closed, route kept").isNull();
        awaitClosed(first);
        assertThat(report(1).state()).as("still proven").isEqualTo("LOADED");
        assertThat(call("/fn/app.hello/hello/ann").v()).as("the next call loads it again").isEqualTo("1");
        assertThat(h.registry.peek(ADDRESS)).isNotNull().isNotSameAs(first);
    }

    @Test
    void aWarmFunctionIsNeverEvictedForIdleness() {
        Fx.Jar v1 = Fx.hello("1");
        h.plane.addArtifact(v1);
        h.plane.push(Fx.desired(1, Fx.function("fn_1", "app.hello").warm(true).version(Fx.version(1, v1, helloDescribe(), "live")).build()));
        h.reconcile();
        LoadedFunction first = h.registry.peek(ADDRESS);

        h.clock.advance(Duration.ofHours(5));
        h.reconcile();

        assertThat(h.registry.peek(ADDRESS)).isSameAs(first);
    }

    @Test
    void aSettingsChangeReloadsTheFunctionNewBeforeOld() throws Exception {
        Fx.Jar v1 = Fx.jar("fixture.app.Cfg", """
                package fixture.app;
                import io.flowcatalyst.function.*;
                public final class Cfg implements Function {
                    public Result handle(Request in, FunctionContext ctx) {
                        return Result.json(200, "{\\"v\\":\\"" + ctx.config().get("k").orElse("-") + "\\"}");
                    }
                }
                """);
        String describe = Fx.describe("fixture.app.Cfg", List.of(HELLO_ENDPOINTS), "\"config\":[\"k\"]");
        h.plane.addArtifact(v1);
        h.plane.push(Fx.desired(1, Fx.function("fn_1", "app.hello").config("k", "one").version(Fx.version(1, v1, describe, "live")).build()));
        h.reconcile();
        LoadedFunction first = h.registry.peek(ADDRESS);
        assertThat(call("/fn/app.hello/hello/x").v()).isEqualTo("one");

        h.plane.push(Fx.desired(2, Fx.function("fn_1", "app.hello").config("k", "two").version(Fx.version(1, v1, describe, "live")).build()));
        h.reconcile();

        assertThat(call("/fn/app.hello/hello/x").v()).isEqualTo("two");
        awaitClosed(first);
    }

    @Test
    void aliasVersionsBeyondThePinnedCapacityStayProvenAndLoadAgainOnACall() {
        Fx.Jar v = Fx.hello("a");
        h.plane.addArtifact(v);
        Fx.FunctionBuilder b = Fx.function("fn_1", "app.hello").version(Fx.version(1, v, helloDescribe(), "live"));
        int extra = PinnedVersions.CAPACITY + 4;
        for (int i = 0; i < extra; i++) {
            b.version(Fx.version(10 + i, v, helloDescribe(), "alias:a" + i));
        }
        h.plane.push(Fx.desired(1, b.build()));

        h.reconcile();

        assertThat(h.reconciler.pinned().size()).isLessThanOrEqualTo(PinnedVersions.CAPACITY);
        for (int i = 0; i < extra; i++) {
            assertThat(report(10 + i).state()).as("alias a" + i).isEqualTo("LOADED");
            assertThat(call("/fn/app.hello@a" + i + "/hello/ann").status()).as("alias a" + i).isEqualTo(200);
        }
        assertThat(h.reconciler.pinned().size()).isLessThanOrEqualTo(PinnedVersions.CAPACITY);
    }

    // ── failures: FAILED is permanent only ──────────────────────────────────────────────────────

    @Test
    void aBadJarIsFailedAndNeverRetriedWithoutANewDocument() {
        Fx.Jar bad = Fx.jar("fixture.app.Hello", "package fixture.app; public final class Hello {}"); // not a Function
        h.plane.addArtifact(bad);
        h.plane.push(Fx.desired(1, fn(Fx.version(1, bad, helloDescribe(), "live"))));

        h.reconcile();
        int fetches = h.plane.artifactFetches.get();
        h.clock.advance(Duration.ofMinutes(10));
        h.reconcile();

        assertThat(report(1).state()).isEqualTo("FAILED");
        assertThat(report(1).reason()).isEqualTo("LOAD:ENTRYPOINT_NOT_A_FUNCTION");
        assertThat(h.plane.artifactFetches.get()).isEqualTo(fetches);
    }

    @Test
    void anInitThatThrowsIsFailedWithoutTheExceptionsText() {
        Fx.Jar v = Fx.jar("fixture.app.Hello", """
                package fixture.app;
                import io.flowcatalyst.function.*;
                public final class Hello implements Function {
                    public void init(FunctionContext ctx) { throw new IllegalStateException("password=hunter2"); }
                    public Result handle(Request in, FunctionContext ctx) { return Result.ack(); }
                }
                """);
        h.plane.addArtifact(v);
        h.plane.push(Fx.desired(1, fn(Fx.version(1, v, helloDescribe(), "live"))));

        h.reconcile();

        assertThat(report(1).state()).isEqualTo("FAILED");
        assertThat(report(1).reason()).isEqualTo("LOAD:INIT_FAILED").doesNotContain("hunter2");
        assertThat(h.registry.peek(ADDRESS)).isNull();
    }

    @Test
    void anInitThatRunsOutOfMetaspaceIsRetryingNotFailed() {
        Fx.Jar v = Fx.jar("fixture.app.Hello", """
                package fixture.app;
                import io.flowcatalyst.function.*;
                public final class Hello implements Function {
                    public void init(FunctionContext ctx) { throw new OutOfMemoryError("Metaspace"); }
                    public Result handle(Request in, FunctionContext ctx) { return Result.ack(); }
                }
                """);
        h.plane.addArtifact(v);
        h.plane.push(Fx.desired(1, fn(Fx.version(1, v, helloDescribe(), "live"))));

        h.reconcile();

        assertThat(report(1).state()).isEqualTo("RETRYING");
        assertThat(report(1).reason()).contains("LOAD:OUT_OF_METASPACE");
        assertThat(h.registry.peek(ADDRESS)).isNull();
    }

    @Test
    void aJavaHeapOutOfMemoryIsNotSwallowed() {
        Fx.Jar v = Fx.jar("fixture.app.Hello", """
                package fixture.app;
                import io.flowcatalyst.function.*;
                public final class Hello implements Function {
                    public void init(FunctionContext ctx) { throw new OutOfMemoryError("Java heap space"); }
                    public Result handle(Request in, FunctionContext ctx) { return Result.ack(); }
                }
                """);
        h.plane.addArtifact(v);
        h.plane.push(Fx.desired(1, fn(Fx.version(1, v, helloDescribe(), "live"))));

        org.assertj.core.api.Assertions.assertThatThrownBy(() -> h.reconcile()).isInstanceOf(OutOfMemoryError.class).hasMessage("Java heap space");
    }

    @Test
    void aBadDescribeIsFailedAndNothingIsFetched() {
        Fx.Jar v1 = Fx.hello("1");
        h.plane.addArtifact(v1);
        h.plane.push(Fx.desired(1, fn(Fx.version(1, v1, "{\"abi\":1,\"endpoints\":[]}", "live"))));

        h.reconcile();

        assertThat(report(1).state()).isEqualTo("FAILED");
        assertThat(report(1).reason()).startsWith("DESCRIBE:");
        assertThat(h.plane.artifactFetches.get()).isZero();
    }

    @Test
    void anAddressTheFunctionApiCannotRepresentIsFailed() {
        Fx.Jar v1 = Fx.hello("1");
        h.plane.addArtifact(v1);
        h.plane.push(Fx.desired(1, Fx.function("fn_1", "App_1.hello").version(Fx.version(1, v1, helloDescribe(), "live")).build()));

        h.reconcile();

        assertThat(report(1).state()).isEqualTo("FAILED");
        assertThat(report(1).reason()).isEqualTo("ADDRESS_UNSUPPORTED");
    }

    @Test
    void anArtifactThePlatformRefusesIsFailedAtOnce() {
        Fx.Jar v1 = Fx.hello("1"); // never added: the plane answers 404
        h.plane.push(Fx.desired(1, fn(Fx.version(1, v1, helloDescribe(), "live"))));

        h.reconcile();

        assertThat(report(1).state()).isEqualTo("FAILED");
        assertThat(report(1).reason()).startsWith("ARTIFACT:");
    }

    @Test
    void aFailedVersionIsRetriedWhenTheDocumentIsRenderedAgainAfterTheRetryBase() {
        Fx.Jar v1 = Fx.hello("1");
        h.plane.push(Fx.desired(1, fn(Fx.version(1, v1, helloDescribe(), "live"))));
        h.reconcile();
        assertThat(report(1).state()).isEqualTo("FAILED");

        h.plane.addArtifact(v1); // the platform has it now
        h.plane.push(Fx.desired(2, fn(Fx.version(1, v1, helloDescribe(), "live"))));
        h.reconcile();
        assertThat(report(1).state()).as("inside the retry base: not retried").isEqualTo("FAILED");

        h.clock.advance(Duration.ofSeconds(1));
        h.plane.push(Fx.desired(3, fn(Fx.version(1, v1, helloDescribe(), "live"))));
        h.reconcile();
        assertThat(report(1).state()).isEqualTo("LOADED");
        assertThat(call("/fn/app.hello/hello/ann").status()).isEqualTo(200);
    }

    // ── transient failures: RETRYING, never FAILED ──────────────────────────────────────────

    @Test
    void aTransientFetchFailureIsRetryingThenLoadedAndFailedIsNeverReported() {
        Fx.Jar v1 = Fx.hello("1");
        h.plane.addArtifact(v1);
        java.util.concurrent.atomic.AtomicInteger failures = new java.util.concurrent.atomic.AtomicInteger(2);
        h.plane.artifactFailure = d -> failures.getAndDecrement() > 0 ? new ControlPlaneException(503, "", "storage down", null) : null;
        h.plane.push(Fx.desired(1, fn(Fx.version(1, v1, helloDescribe(), "live"))));

        h.reconcile();
        assertThat(report(1).state()).isEqualTo("RETRYING");
        assertThat(report(1).reason()).contains("attempt 1 failed").contains("storage down");
        assertThat(call("/fn/app.hello/hello/ann").status()).isEqualTo(503);

        h.clock.advance(Duration.ofSeconds(1));
        h.reconcile();
        assertThat(report(1).state()).isEqualTo("RETRYING");
        assertThat(report(1).reason()).contains("attempt 2 failed");

        h.clock.advance(Duration.ofSeconds(1));
        h.reconcile();
        assertThat(report(1).state()).isEqualTo("LOADED");
        assertThat(call("/fn/app.hello/hello/ann").status()).isEqualTo(200);
        assertThat(statesEverReported(1)).doesNotContain("FAILED");
    }

    @Test
    void aTransientFailureIsOnlyReportedFailedOnceTheRetryBudgetIsSpentAndKeepsRetrying() {
        Harness short_ = new Harness(new Reconciler.Timing(Duration.ofMillis(10), Duration.ofMillis(50), Duration.ofSeconds(30), Duration.ofMillis(100)));
        try {
            Fx.Jar v1 = Fx.hello("1");
            short_.plane.addArtifact(v1);
            short_.plane.artifactFailure = d -> new ControlPlaneException(0, "IO", "connection reset", null);
            short_.plane.push(Fx.desired(1, fn(Fx.version(1, v1, helloDescribe(), "live"))));

            short_.reconcile();
            assertThat(short_.plane.report("fn_1", 1).state()).isEqualTo("RETRYING");

            short_.clock.advance(Duration.ofMinutes(1));
            short_.reconcile();
            assertThat(short_.plane.report("fn_1", 1).state()).as("the budget is spent").isEqualTo("FAILED");
            assertThat(short_.plane.report("fn_1", 1).reason()).contains("gave up after");

            short_.plane.artifactFailure = null;
            short_.clock.advance(Duration.ofMinutes(10));
            short_.reconcile();
            assertThat(short_.plane.report("fn_1", 1).state()).as("still retried, and loads").isEqualTo("LOADED");
        } finally {
            short_.close();
        }
    }

    @Test
    void noMetaspaceHeadroomIsRetryingNotFailed() {
        long mib = 1024L * 1024;
        h.gauge.max(512 * mib).used(512 * mib - 10 * mib); // 10 MiB free, 64 MiB reserve
        Fx.Jar v1 = Fx.hello("1");
        h.plane.addArtifact(v1);
        h.plane.push(Fx.desired(1, fn(Fx.version(1, v1, helloDescribe(), "live"))));

        h.reconcile();

        assertThat(report(1).state()).isEqualTo("RETRYING");
        assertThat(report(1).reason()).contains("LOAD:METASPACE_HEADROOM");
        assertThat(h.registry.peek(ADDRESS)).isNull();

        h.gauge.used(0);
        h.clock.advance(Duration.ofSeconds(1));
        h.reconcile();
        assertThat(report(1).state()).isEqualTo("LOADED");
    }

    @Test
    void theGuardRefusesAtMostOneGcPerCycle() {
        long mib = 1024L * 1024;
        h.gauge.max(512 * mib).used(512 * mib - 10 * mib);
        Fx.Jar v1 = Fx.hello("1");
        h.plane.addArtifact(v1);
        h.plane.push(Fx.desired(1,
                fn(Fx.version(1, v1, helloDescribe(), "live")),
                Fx.function("fn_2", "app.second").version(Fx.version(1, v1, helloDescribe(), "live")).build()));

        h.reconcile();

        assertThat(h.gauge.gcRequests()).isEqualTo(1);
    }

    // ── outage ─────────────────────────────────────────────────────────────────────────────────

    @Test
    void aControlPlaneOutageUnloadsNothingAndLoadsNothing() {
        Fx.Jar v1 = Fx.hello("1");
        h.plane.addArtifact(v1);
        h.plane.push(Fx.desired(1, fn(Fx.version(1, v1, helloDescribe(), "live"))));
        h.reconcile();
        h.plane.desiredFailure = new ControlPlaneException(503, "", "down", null);
        int beats = h.plane.heartbeats.size();

        assertThat(h.reconcile()).isEqualTo(Reconciler.Outcome.OUTAGE);
        h.clock.advance(Duration.ofHours(3)); // even an idle lazy function is not evicted during an outage

        assertThat(h.reconcile()).isEqualTo(Reconciler.Outcome.OUTAGE);
        assertThat(h.registry.peek(ADDRESS)).isNotNull();
        assertThat(call("/fn/app.hello/hello/ann").status()).isEqualTo(200);
        assertThat(h.plane.heartbeats.size()).as("still heartbeating").isGreaterThan(beats);
    }

    // ── the loop's pacing ──────────────────────────────────────────────────────────────────────

    @Test
    void theWaitShortensToWhenARetryIsDue() {
        Fx.Jar v1 = Fx.hello("1");
        h.plane.addArtifact(v1);
        h.plane.artifactFailure = d -> new ControlPlaneException(503, "", "down", null);
        h.plane.push(Fx.desired(1, fn(Fx.version(1, v1, helloDescribe(), "live"))));

        assertThat(h.reconciler.nextWake(h.clock.instant(), Duration.ofSeconds(30))).isEqualTo(Duration.ofSeconds(30));
        h.reconcile();

        Duration wait = h.reconciler.nextWake(h.clock.instant(), Duration.ofSeconds(30));
        assertThat(wait).as("a retry is due in a few ms: the floor of 1 s, not the 30 s long-poll").isEqualTo(Duration.ofSeconds(1));
    }

    // ── failure pacing (docs/ingestor-failure-pacing.md R6) ─────────────────────────────────────

    @Test
    void anUnexpectedExceptionFromTheArtifactFetchFailsTheVersionTransientAndIsNotAskedAgainAtOnce() {
        Fx.Jar v1 = Fx.hello("1");
        h.plane.addArtifact(v1);
        h.plane.artifactFailure = d -> {
            throw new IllegalArgumentException("not a digest");
        };
        h.plane.push(Fx.desired(1, fn(Fx.version(1, v1, helloDescribe(), "live"))));

        h.reconcile();
        assertThat(report(1).state()).as("transient: RETRYING, never FAILED").isEqualTo("RETRYING");
        assertThat(h.plane.artifactFetches.get()).isEqualTo(1);
        h.reconcile();
        assertThat(h.plane.artifactFetches.get()).as("not due again in the same instant").isEqualTo(1);
        assertThat(h.reconciler.nextWake(h.clock.instant(), Duration.ofSeconds(30))).isGreaterThanOrEqualTo(Duration.ofSeconds(1));

        h.clock.advance(Duration.ofSeconds(1));
        h.reconcile();
        assertThat(h.plane.artifactFetches.get()).as("retried once the delay has passed").isEqualTo(2);
        assertThat(h.logged).filteredOn(l -> l.message().contains("could not be fetched")).hasSize(1);
        assertThat(h.logged.get(0).withStack()).isTrue();

        h.plane.artifactFailure = null;
        h.clock.advance(Duration.ofMinutes(1));
        h.reconcile();
        assertThat(report(1).state()).isEqualTo("LOADED");
    }

    private Fx.Jar initThrowing(String statement) {
        return Fx.jar("fixture.app.Hello", """
                package fixture.app;
                import io.flowcatalyst.function.*;
                public final class Hello implements Function {
                    public void init(FunctionContext ctx) { %s }
                    public Result handle(Request in, FunctionContext ctx) { return Result.ack(); }
                }
                """.formatted(statement));
    }

    @Test
    void aLinkageErrorFromInitIsAPermanentInitFailedAndTheCycleSurvives() {
        for (String statement : List.of("throw new NoClassDefFoundError(\"missing/Thing\");", "throw new ExceptionInInitializerError(\"static block\");")) {
            Harness own = new Harness();
            try {
                Fx.Jar v = initThrowing(statement);
                own.plane.addArtifact(v);
                own.plane.push(Fx.desired(1, fn(Fx.version(1, v, helloDescribe(), "live"))));

                own.reconcile(); // must return, not throw

                assertThat(own.plane.report("fn_1", 1).state()).as(statement).isEqualTo("FAILED");
                assertThat(own.plane.report("fn_1", 1).reason()).isEqualTo("LOAD:INIT_FAILED");
                int fetches = own.plane.artifactFetches.get();
                own.clock.advance(Duration.ofMinutes(10));
                own.reconcile();
                assertThat(own.plane.artifactFetches.get()).as("permanent: not tried again").isEqualTo(fetches);
            } finally {
                own.close();
            }
        }
    }

    @Test
    void aRefusedDesiredFetchIsRefusedNotAPlainOutageAndNamesTheCause() {
        h.plane.desiredFailure = new ControlPlaneException(403, "FORBIDDEN", "no", null);
        assertThat(h.reconcile()).isEqualTo(Reconciler.Outcome.REFUSED);
        assertThat(h.logged).anyMatch(l -> l.message().contains("refused this runtime as a function runner (403)")
                && l.message().contains("platform:function-runner"));

        h.plane.desiredFailure = new ControlPlaneException(503, "", "down", null);
        assertThat(h.reconcile()).isEqualTo(Reconciler.Outcome.OUTAGE);
    }

    @Test
    void aRefusedHeartbeatIsHeldBackByMinutesDoublingToAnHour() {
        h.plane.heartbeatFailure = new ControlPlaneException(403, "FORBIDDEN", "no", null);
        h.reconciler.sendHeartbeat();
        assertThat(h.plane.heartbeatCalls.get()).isEqualTo(1);

        // Backoff.REFUSED_LOGIN is 5 minutes doubling; with equal jitter a hold is between half of that and all of it.
        h.clock.advance(Duration.ofMinutes(2).plusSeconds(29));
        h.reconciler.sendHeartbeat();
        assertThat(h.plane.heartbeatCalls.get()).as("first hold is at least 2.5 minutes").isEqualTo(1);

        h.clock.advance(Duration.ofMinutes(2).plusSeconds(32)); // 5 minutes in: past the longest first hold
        h.reconciler.sendHeartbeat();
        assertThat(h.plane.heartbeatCalls.get()).isEqualTo(2);

        h.clock.advance(Duration.ofMinutes(4).plusSeconds(59));
        h.reconciler.sendHeartbeat();
        assertThat(h.plane.heartbeatCalls.get()).as("second hold is at least 5 minutes").isEqualTo(2);

        h.clock.advance(Duration.ofMinutes(5).plusSeconds(2)); // past the longest second hold (10 minutes)
        h.plane.heartbeatFailure = null;
        h.reconciler.sendHeartbeat();
        assertThat(h.plane.heartbeatCalls.get()).isEqualTo(3);
        assertThat(h.plane.heartbeats).hasSize(1);
        assertThat(h.logged).anyMatch(l -> l.message().contains("heartbeat refused") && l.message().contains("platform:function-runner"));
        assertThat(h.logged).anyMatch(l -> l.message().contains("heartbeats are accepted again"));
    }

    // ── helpers ─────────────────────────────────────────────────────────────────────────────────

    private static void awaitClosed(LoadedFunction fn) throws InterruptedException {
        long end = System.nanoTime() + Duration.ofSeconds(10).toNanos();
        while (!fn.isClosed()) {
            if (System.nanoTime() > end) {
                throw new AssertionError("the version never closed");
            }
            Thread.sleep(5);
        }
    }
}
