package io.flowcatalyst.ingestor.functions.http;

import static org.assertj.core.api.Assertions.assertThat;

import io.flowcatalyst.framework.http.TestResponse;
import io.flowcatalyst.function.FunctionAddress;
import io.flowcatalyst.ingestor.functions.Fx;
import io.flowcatalyst.ingestor.functions.Harness;
import io.flowcatalyst.ingestor.functions.load.LoadedFunction;
import java.time.Duration;
import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.concurrent.atomic.AtomicInteger;
import org.junit.jupiter.api.AfterEach;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.Timeout;

/// R3, the owner's requirement: one loaded instance per function version serves every call at once, each call on its own
/// virtual thread; only the per-function permit count limits concurrency; nothing serialises calls of one
/// function. Each of these fails if the invoke path queues, locks or serialises.
@Timeout(value = 60, threadMode = Timeout.ThreadMode.SEPARATE_THREAD) // a call that queues instead of answering 429 must fail, not hang
class ConcurrencyTest {

    static final FunctionAddress ADDRESS = FunctionAddress.parse("app.track");
    static final String BASE = "/fn/app.track";

    Harness h;
    ExecutorService callers = Executors.newVirtualThreadPerTaskExecutor();

    @BeforeEach
    void start() {
        System.getProperties().remove("fx.inflight");
        System.getProperties().remove("fx.max");
        System.getProperties().remove("fx.stubborn.returned");
        h = new Harness();
    }

    @AfterEach
    void stop() {
        callers.shutdownNow();
        h.close();
    }

    private void deploy(int version, String tag, int maxConcurrency, long timeoutMs, String... roles) {
        Fx.Jar jar = TrackFunction.jar(tag);
        h.plane.addArtifact(jar);
        h.plane.push(Fx.desired(version, Fx.function("fn_t", "app.track").limits(maxConcurrency, timeoutMs, 1 << 20)
                .version(Fx.version(version, jar, TrackFunction.describe(), roles)).build()));
        h.reconcile();
    }

    private Future<TestResponse> async(String path) {
        return callers.submit(() -> h.http.get(path).send());
    }

    private static int counter(String name) {
        return ((AtomicInteger) System.getProperties().getOrDefault(name, new AtomicInteger())).get();
    }

    private void awaitUntil(java.util.function.BooleanSupplier condition, String what) throws InterruptedException {
        long end = System.nanoTime() + Duration.ofSeconds(10).toNanos();
        while (!condition.getAsBoolean()) {
            if (System.nanoTime() > end) {
                throw new AssertionError("timed out waiting for " + what);
            }
            Thread.sleep(5);
        }
    }

    @Test
    void fiftyConcurrentSlowCallsToOneFunctionTakeAboutOneCallsTime() throws Exception {
        deploy(1, "v1", 100, 30_000, "live");
        int n = 50;
        CountDownLatch go = new CountDownLatch(1);
        List<Future<TestResponse>> calls = new ArrayList<>();
        for (int i = 0; i < n; i++) {
            calls.add(callers.submit(() -> {
                go.await();
                return h.http.get(BASE + "/track/200").send();
            }));
        }
        long start = System.nanoTime();
        go.countDown();
        for (Future<TestResponse> f : calls) {
            assertThat(f.get().status()).isEqualTo(200);
        }
        Duration took = Duration.ofNanos(System.nanoTime() - start);

        // 50 serialised calls would take 10 s; "about one call's time" is 200 ms plus scheduling.
        assertThat(took).as("N calls of one function run at once").isLessThan(Duration.ofMillis(1500));
        assertThat(counter("fx.max")).as("every call was inside the function at the same time").isEqualTo(n);
    }

    @Test
    void exhaustedPermitsAnswer429AtOnceWithoutQueueing() throws Exception {
        deploy(1, "v1", 2, 30_000, "live");
        Future<TestResponse> a = async(BASE + "/track/1500");
        Future<TestResponse> b = async(BASE + "/track/1500");
        awaitUntil(() -> h.listener.permits().available("app.track") == 0, "both permits to be taken");

        long start = System.nanoTime();
        TestResponse busy = h.http.get(BASE + "/track/1500").send();
        Duration took = Duration.ofNanos(System.nanoTime() - start);

        assertThat(busy.status()).isEqualTo(429);
        assertThat(busy.header("Retry-After")).isEqualTo("1");
        assertThat(busy.tree().path("error").asString()).isEqualTo("FUNCTION_BUSY");
        assertThat(took).as("refused at once, not queued behind the running calls").isLessThan(Duration.ofMillis(300));
        assertThat(a.get().status()).isEqualTo(200);
        assertThat(b.get().status()).isEqualTo(200);
        assertThat(counter("fx.max")).as("never more than the permits").isEqualTo(2);
        // the permits are back
        awaitUntil(() -> h.listener.permits().available("app.track") == 2, "the permits to be released");
        assertThat(h.http.get(BASE + "/track/1").send().status()).isEqualTo(200);
    }

    @Test
    void aCallPastItsDeadlineAnswers504ButItsPermitIsReleasedOnlyWhenTheHandlerReturns() throws Exception {
        deploy(1, "v1", 1, 200, "live");

        long start = System.nanoTime();
        TestResponse late = h.http.get(BASE + "/stubborn/1200").send();
        Duration took = Duration.ofNanos(System.nanoTime() - start);

        assertThat(late.status()).isEqualTo(504);
        assertThat(took).isLessThan(Duration.ofMillis(900));
        // the handler ignores the interrupt and is still running: it still holds the one permit
        assertThat(System.getProperties().get("fx.stubborn.returned")).isNull();
        assertThat(h.listener.permits().available("app.track")).as("the permit is held until the handler returns, not until the 504").isZero();
        TestResponse busy = h.http.get(BASE + "/track/1").send();
        assertThat(busy.status()).isEqualTo(429);

        awaitUntil(() -> System.getProperties().get("fx.stubborn.returned") != null, "the handler to return");
        awaitUntil(() -> h.listener.permits().available("app.track") == 1, "the permit to come back");
        assertThat(h.http.get(BASE + "/track/1").send().status()).isEqualTo(200);
    }

    @Test
    void aSwapDrainsTheOldVersionOnlyAfterItsInFlightCallsReturn() throws Exception {
        deploy(1, "v1", 16, 30_000, "live");
        LoadedFunction v1 = h.registry.peek(ADDRESS);
        assertThat(v1).isNotNull();
        Future<TestResponse> inFlight = async(BASE + "/track/1200");
        awaitUntil(() -> v1.inFlight() == 1, "the call to be inside v1");

        deploy(2, "v2", 16, 30_000, "live");

        // new before old: v2 serves at once, v1 is closing but not closed while its call is running
        assertThat(h.registry.peek(ADDRESS)).isNotSameAs(v1);
        assertThat(h.http.get(BASE + "/track/1").send().tree().path("tag").asString()).isEqualTo("v2");
        awaitUntil(v1::isClosing, "v1 to begin closing");
        assertThat(v1.isClosed()).as("closing waits for the call in flight").isFalse();
        assertThat(v1.tryRetain()).as("a closing version takes no new call").isFalse();

        TestResponse answer = inFlight.get();
        assertThat(answer.status()).as("the call in flight finishes on the version it started on").isEqualTo(200);
        assertThat(answer.tree().path("tag").asString()).isEqualTo("v1");
        awaitUntil(v1::isClosed, "v1 to close once drained");
    }

    @Test
    void closeWaitsForEveryRetainedCallAndRefusesNewOnes() throws Exception {
        deploy(1, "v1", 16, 30_000, "live");
        LoadedFunction fn = h.registry.peek(ADDRESS);
        assertThat(fn.tryRetain()).isTrue();
        assertThat(fn.tryRetain()).isTrue();

        Thread closer = Thread.ofVirtual().start(fn::close);
        awaitUntil(fn::isClosing, "close to begin");
        Thread.sleep(150);
        assertThat(closer.isAlive()).as("close() waits while two calls are retained").isTrue();
        assertThat(fn.tryRetain()).isFalse();

        fn.release();
        Thread.sleep(100);
        assertThat(closer.isAlive()).as("one call is still in flight").isTrue();
        fn.release();
        closer.join(Duration.ofSeconds(5));
        assertThat(closer.isAlive()).isFalse();
        assertThat(fn.isClosed()).isTrue();
        assertThat(fn.inFlight()).isZero();
    }
}
