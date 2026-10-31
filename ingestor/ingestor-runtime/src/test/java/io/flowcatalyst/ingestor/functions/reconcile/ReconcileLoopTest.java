package io.flowcatalyst.ingestor.functions.reconcile;

import static org.assertj.core.api.Assertions.assertThat;

import io.flowcatalyst.function.FunctionAddress;
import io.flowcatalyst.ingestor.functions.Fx;
import io.flowcatalyst.ingestor.functions.Harness;
import io.flowcatalyst.ingestor.functions.control.ControlPlaneException;
import java.time.Duration;
import java.util.List;
import java.util.function.BooleanSupplier;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.Timeout;

/// The loop around the reconciler: it fetches the document and loads the functions on its own thread, heartbeats on
/// another, backs off when the platform cannot be reached (and keeps what is loaded), and stops on close.
@Timeout(60)
class ReconcileLoopTest {

    Harness h;
    ReconcileLoop loop;

    @BeforeEach
    void start() {
        h = new Harness(new Reconciler.Timing(Duration.ofMillis(10), Duration.ofMillis(50), Duration.ofMinutes(5), Duration.ofMillis(100)), true);
    }

    @AfterEach
    void stop() {
        if (loop != null) {
            loop.close();
        }
        h.close();
    }

    private static void await(BooleanSupplier condition, String what) throws InterruptedException {
        long end = System.nanoTime() + Duration.ofSeconds(20).toNanos();
        while (!condition.getAsBoolean()) {
            if (System.nanoTime() > end) {
                throw new AssertionError("timed out waiting for " + what);
            }
            Thread.sleep(10);
        }
    }

    private void deploy(int revision, Fx.Jar jar, String tag) {
        h.plane.addArtifact(jar);
        h.plane.push(Fx.desired(revision, Fx.function("fn_1", "app.hello")
                .version(Fx.version(revision, jar, Fx.describe("fixture.app.Hello", List.of(Fx.endpoint("GET", "/hello/{name}", "none"))), "live")).build()));
    }

    @Test
    void theLoopLoadsWhatTheDocumentNamesAndFollowsItsChanges() throws Exception {
        FunctionAddress address = FunctionAddress.parse("app.hello");
        loop = new ReconcileLoop(h.reconciler, h.clock, Duration.ofMillis(200), Duration.ofMillis(100), Duration.ofMillis(20));
        deploy(1, Fx.hello("1"), "1");
        loop.start();

        await(() -> h.plane.report("fn_1", 1) != null && h.plane.report("fn_1", 1).state().equals("LOADED"), "version 1 loaded and reported");
        assertThat(h.registry.peek(address)).isNotNull();
        assertThat(loop.isAlive()).isTrue();

        deploy(2, Fx.hello("2"), "2");
        await(() -> h.registry.peek(address) != null && h.registry.peek(address).version() == 2, "version 2 to replace version 1");
        await(() -> h.plane.report("fn_1", 2) != null && h.plane.report("fn_1", 2).state().equals("LOADED"), "version 2 reported");
    }

    @Test
    void aHeartbeatGoesOutOnTheHeartbeatThreadAsWell() throws Exception {
        loop = new ReconcileLoop(h.reconciler, h.clock, Duration.ofMillis(200), Duration.ofMillis(50), Duration.ofMillis(20));
        deploy(1, Fx.hello("1"), "1");
        loop.start();
        await(() -> h.plane.report("fn_1", 1) != null, "a first heartbeat");

        int beats = h.plane.heartbeats.size();
        await(() -> h.plane.heartbeats.size() >= beats + 3, "three more heartbeats");
    }

    @Test
    void whenThePlatformCannotBeReachedTheLoopBacksOffAndKeepsWhatIsLoaded() throws Exception {
        loop = new ReconcileLoop(h.reconciler, h.clock, Duration.ofMillis(100), Duration.ofSeconds(5), Duration.ofMillis(100));
        deploy(1, Fx.hello("1"), "1");
        loop.start();
        await(() -> h.registry.peek(FunctionAddress.parse("app.hello")) != null, "the function to load");

        h.plane.desiredFailure = new ControlPlaneException(503, "", "down", null);
        Thread.sleep(300); // let a cycle in flight finish
        int before = h.plane.desiredCalls.get();
        Thread.sleep(1500);
        int during = h.plane.desiredCalls.get() - before;

        // backoff 100 ms doubling: at most ~5 attempts in 1.5 s, where an unthrottled loop would make hundreds
        assertThat(during).as("attempts while the platform is down").isLessThan(12);
        assertThat(h.registry.peek(FunctionAddress.parse("app.hello"))).as("what was loaded keeps serving").isNotNull();

        h.plane.desiredFailure = null;
        deploy(2, Fx.hello("2"), "2");
        await(() -> h.registry.peek(FunctionAddress.parse("app.hello")).version() == 2, "the loop to recover");
    }

    @Test
    void aPlatformThatAnswersNotModifiedAtOnceIsNotPolledInATightLoop() throws Exception {
        loop = new ReconcileLoop(h.reconciler, h.clock, Duration.ofMillis(100), Duration.ofSeconds(5), Duration.ofMillis(100));
        deploy(1, Fx.hello("1"), "1");
        loop.start();
        await(() -> h.registry.peek(FunctionAddress.parse("app.hello")) != null, "the function to load");

        int before = h.plane.desiredCalls.get();
        Thread.sleep(1000);

        assertThat(h.plane.desiredCalls.get() - before).as("calls in a second").isLessThan(10);
    }

    @Test
    void closeStopsTheThreads() throws Exception {
        loop = new ReconcileLoop(h.reconciler, h.clock, Duration.ofMillis(100), Duration.ofMillis(50), Duration.ofMillis(20));
        deploy(1, Fx.hello("1"), "1");
        loop.start();
        await(() -> h.plane.report("fn_1", 1) != null, "a first heartbeat");

        loop.close();

        assertThat(loop.isAlive()).isFalse();
        int calls = h.plane.desiredCalls.get();
        int beats = h.plane.heartbeats.size();
        Thread.sleep(400);
        assertThat(h.plane.desiredCalls.get()).isEqualTo(calls);
        assertThat(h.plane.heartbeats.size()).isEqualTo(beats);
    }

    @Test
    void aTransientFailureIsRetriedByTheLoopWithoutANewDocument() throws Exception {
        FunctionAddress address = FunctionAddress.parse("app.hello");
        loop = new ReconcileLoop(h.reconciler, h.clock, Duration.ofSeconds(30), Duration.ofSeconds(5), Duration.ofMillis(20));
        Fx.Jar jar = Fx.hello("1");
        deploy(1, jar, "1");
        java.util.concurrent.atomic.AtomicInteger failures = new java.util.concurrent.atomic.AtomicInteger(2);
        h.plane.artifactFailure = d -> failures.getAndDecrement() > 0 ? new ControlPlaneException(503, "", "down", null) : null;
        loop.start();

        // the loop shortens its wait to when the retry is due (the real clock, 10-50 ms delays in the harness timing)
        await(() -> h.registry.peek(address) != null, "the retry to load the function");
        assertThat(h.plane.heartbeats.stream().flatMap(b -> b.versions().stream()).map(r -> r.state())).doesNotContain("FAILED");
    }
}
