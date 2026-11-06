package io.flowcatalyst.ingestor.functions;

import io.flowcatalyst.framework.auth.PlatformTokens;
import io.flowcatalyst.framework.auth.TestOidcProvider;
import io.flowcatalyst.framework.http.Routes;
import io.flowcatalyst.framework.http.TestClient;
import io.flowcatalyst.framework.httpclient.HttpClients;
import io.flowcatalyst.framework.metrics.Metrics;
import io.flowcatalyst.ingestor.extension.FailureLog;
import io.flowcatalyst.ingestor.functions.context.ContextFactory;
import io.flowcatalyst.ingestor.functions.context.LogBudget;
import io.flowcatalyst.ingestor.functions.context.OutboundClients;
import io.flowcatalyst.ingestor.functions.http.FunctionListener;
import io.flowcatalyst.ingestor.functions.load.FakeMetaspaceGauge;
import io.flowcatalyst.ingestor.functions.load.FunctionRegistry;
import io.flowcatalyst.ingestor.functions.load.JvmFunctionLoader;
import io.flowcatalyst.ingestor.functions.load.MetaspaceGuard;
import io.flowcatalyst.ingestor.functions.reconcile.ArtifactCache;
import io.flowcatalyst.ingestor.functions.reconcile.Reconciler;
import io.flowcatalyst.ingestor.functions.trace.FunctionTraceOptions;
import io.flowcatalyst.ingestor.functions.trace.FunctionTraceShipper;
import io.flowcatalyst.ingestor.functions.trace.FunctionTraces;
import java.time.Clock;
import java.time.Duration;
import java.time.Instant;
import java.time.InstantSource;
import java.time.ZoneOffset;
import java.util.List;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.atomic.AtomicReference;

/// A real host (reconciler, registry, listener, loader) over a [FakeControlPlane], with a movable clock and a fake
/// metaspace gauge, served through the framework's [TestClient].
public final class Harness implements AutoCloseable {

    /// A clock the test moves.
    public static final class MovableClock extends Clock {
        private final AtomicReference<Instant> now = new AtomicReference<>(Instant.parse("2026-10-31T12:00:00Z"));
        private final boolean real;

        public MovableClock() {
            this(false);
        }

        /// @param real follows the wall clock (plus whatever was advanced) instead of standing still
        public MovableClock(boolean real) {
            this.real = real;
            if (real) {
                now.set(Instant.EPOCH);
            }
        }

        public void advance(Duration d) {
            now.updateAndGet(i -> i.plus(d));
        }

        @Override
        public java.time.ZoneId getZone() {
            return ZoneOffset.UTC;
        }

        @Override
        public Clock withZone(java.time.ZoneId zone) {
            return this;
        }

        @Override
        public Instant instant() {
            return real ? Instant.now().plus(Duration.between(Instant.EPOCH, now.get())) : now.get();
        }
    }

    public final FakeControlPlane plane = new FakeControlPlane();
    public final MovableClock clock;
    public final FakeMetaspaceGauge gauge = new FakeMetaspaceGauge();
    public final TestOidcProvider platform = TestOidcProvider.start(InstantSource.system());
    public final HttpClients clients = HttpClients.create();
    public final io.flowcatalyst.ingestor.functions.context.DbPools dbPools;
    public final FunctionRegistry registry;
    public final ContextFactory contexts;
    public final FunctionTraceShipper traceShipper;
    public final Reconciler reconciler;
    public final FunctionListener listener;
    public final TestClient http;

    /// What the host logged through its [FailureLog]s (the reconciler's, the loop's and the listener's share it).
    public record Logged(String message, boolean withStack) {
    }

    public final List<Logged> logged = new CopyOnWriteArrayList<>();
    public final FailureLog failureLog;

    public Harness() {
        this(new Reconciler.Timing(Duration.ofMillis(10), Duration.ofMillis(50), Duration.ofMinutes(5), Duration.ofMillis(100)));
    }

    public Harness(Reconciler.Timing timing) {
        this(timing, false);
    }

    /// @param realClock the clock follows the wall clock (for tests that run the reconcile loop on its own thread)
    public Harness(Reconciler.Timing timing, boolean realClock) {
        this.clock = new MovableClock(realClock);
        this.registry = new FunctionRegistry(64, clock);
        this.dbPools = new io.flowcatalyst.ingestor.functions.context.DbPools(io.flowcatalyst.ingestor.functions.context.DbPools.Limits.DEFAULTS, clock);
        ArtifactCache artifacts = new ArtifactCache(plane, null);
        FunctionTraceOptions traceOptions = new FunctionTraceOptions(
                5000, 32L << 20, 65536, 200, Duration.ofMillis(50), Duration.ofMillis(5), Duration.ofMillis(20));
        this.traceShipper = new FunctionTraceShipper(plane, "runner-1", "default", traceOptions, Metrics.noop());
        this.traceShipper.start();
        this.contexts = new ContextFactory(new OutboundClients(clients), clock, plane, dbPools, LogBudget.Limits.DEFAULTS,
                new FunctionTraces(clock, traceShipper, traceOptions.bodyBytes()));
        this.failureLog = new FailureLog((message, cause) -> logged.add(new Logged(message, cause != null)), Duration.ofMinutes(1), clock);
        reconciler = new Reconciler(
                "default", "runner-1", "test", plane, artifacts, new JvmFunctionLoader(), registry, contexts,
                new MetaspaceGuard(gauge), clock, timing, failureLog);
        PlatformTokens tokens = PlatformTokens.createAllowingPlainHttp(platform.issuer(), Duration.ofSeconds(5), InstantSource.system());
        listener = new FunctionListener(reconciler, tokens, clock, failureLog);
        Routes routes = new Routes();
        listener.install(routes);
        http = TestClient.of(routes).withMaxBodyBytes(64L << 20);
    }

    /// One reconcile cycle at the current clock.
    public Reconciler.Outcome reconcile() {
        return reconciler.reconcileOnce(clock.instant());
    }

    @Override
    public void close() {
        reconciler.close();
        traceShipper.close(Duration.ofSeconds(1));
        dbPools.close();
        platform.close();
        clients.close();
    }
}
