package io.flowcatalyst.ingestor.functions;

import io.flowcatalyst.framework.auth.PlatformTokens;
import io.flowcatalyst.framework.http.Routes;
import io.flowcatalyst.framework.metrics.Metrics;
import io.flowcatalyst.framework.httpclient.HttpClients;
import io.flowcatalyst.ingestor.functions.context.ContextFactory;
import io.flowcatalyst.ingestor.functions.context.DbPools;
import io.flowcatalyst.ingestor.functions.context.LogBudget;
import io.flowcatalyst.ingestor.functions.context.OutboundClients;
import io.flowcatalyst.ingestor.functions.control.ControlPlane;
import io.flowcatalyst.ingestor.functions.http.FunctionListener;
import io.flowcatalyst.ingestor.functions.load.FunctionRegistry;
import io.flowcatalyst.ingestor.functions.load.JvmFunctionLoader;
import io.flowcatalyst.ingestor.functions.load.MetaspaceGuard;
import io.flowcatalyst.ingestor.functions.reconcile.ArtifactCache;
import io.flowcatalyst.ingestor.functions.reconcile.ReconcileLoop;
import io.flowcatalyst.ingestor.functions.reconcile.Reconciler;
import io.flowcatalyst.ingestor.functions.trace.FunctionTraceOptions;
import io.flowcatalyst.ingestor.functions.trace.FunctionTraceShipper;
import io.flowcatalyst.ingestor.functions.trace.FunctionTraces;
import java.time.Clock;
import java.time.Duration;
import java.util.Objects;
import java.util.concurrent.Executors;
import java.util.concurrent.ScheduledExecutorService;
import java.util.concurrent.TimeUnit;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/// The function role of the ingestor: everything this package builds, wired once. [#start] registers the listener on
/// the server's routes and starts the reconcile loop; [#close] stops the loop and unloads every function, waiting
/// for calls in flight.
public final class FunctionHost implements AutoCloseable {

    private static final Logger LOG = LoggerFactory.getLogger(FunctionHost.class);

    /// The most addresses kept loaded at once; lazy ones beyond it are closed least-recently-used first
    /// and load again on their next call.
    static final int MAX_LOADED = 4096;

    /// The longest the last trace records are given to be shipped when the host stops.
    static final Duration TRACE_FLUSH = Duration.ofSeconds(3);

    private final Reconciler reconciler;
    private final ReconcileLoop loop;
    private final FunctionListener listener;
    private final DbPools dbPools;
    private final ScheduledExecutorService dbSweeper;
    private final FunctionTraceShipper traceShipper;

    private FunctionHost(Reconciler reconciler, ReconcileLoop loop, FunctionListener listener, DbPools dbPools,
            ScheduledExecutorService dbSweeper, FunctionTraceShipper traceShipper) {
        this.traceShipper = traceShipper;
        this.reconciler = reconciler;
        this.loop = loop;
        this.listener = listener;
        this.dbPools = dbPools;
        this.dbSweeper = dbSweeper;
    }

    /// Builds the host and starts it.
    ///
    /// @param pool          the function pool this host serves (the `pool` of the desired-state request)
    /// @param runnerId      names this host in its heartbeats
    /// @param controlPlane  the platform's control plane, as the ingestor's service account
    /// @param clients       the engine the functions' outbound calls go through
    /// @param tokens        the verifier of platform access tokens presented to `auth: platform` endpoints
    /// @param routes        the server's routes, which the listener is installed on
    public static FunctionHost start(
            String pool,
            String runnerId,
            String version,
            ControlPlane controlPlane,
            HttpClients clients,
            PlatformTokens tokens,
            Routes routes,
            Clock clock) {
        return start(pool, runnerId, version, controlPlane, clients, tokens, routes, clock, DbPools.Limits.DEFAULTS);
    }

    /// As above, with the limits of the shared database pools (`ingest.functions-db-*`).
    public static FunctionHost start(
            String pool,
            String runnerId,
            String version,
            ControlPlane controlPlane,
            HttpClients clients,
            PlatformTokens tokens,
            Routes routes,
            Clock clock,
            DbPools.Limits dbLimits) {
        return start(pool, runnerId, version, controlPlane, clients, tokens, routes, clock, dbLimits, LogBudget.Limits.DEFAULTS);
    }

    /// As above, with how many log lines a second one function may write (`ingest.functions-log-rate`, `-burst`).
    public static FunctionHost start(
            String pool,
            String runnerId,
            String version,
            ControlPlane controlPlane,
            HttpClients clients,
            PlatformTokens tokens,
            Routes routes,
            Clock clock,
            DbPools.Limits dbLimits,
            LogBudget.Limits logLimits) {
        return start(pool, runnerId, version, controlPlane, clients, tokens, routes, clock, dbLimits, logLimits,
                FunctionTraceOptions.fromEnvironment(System::getenv), Metrics.noop());
    }

    /// As above, with how function traces are queued and shipped (`FC_FN_TRACE_*`) and where their metrics
    /// (`fc_fn_trace_*`) are counted. Traces are off for a function until the platform's desired document carries a
    /// window for it.
    public static FunctionHost start(
            String pool,
            String runnerId,
            String version,
            ControlPlane controlPlane,
            HttpClients clients,
            PlatformTokens tokens,
            Routes routes,
            Clock clock,
            DbPools.Limits dbLimits,
            LogBudget.Limits logLimits,
            FunctionTraceOptions traceOptions,
            Metrics metrics) {
        Objects.requireNonNull(pool, "pool");
        FunctionTraceShipper traceShipper = new FunctionTraceShipper(controlPlane, runnerId, pool, traceOptions, metrics);
        traceShipper.start();
        FunctionTraces traces = new FunctionTraces(clock, traceShipper, traceOptions.bodyBytes());
        DbPools dbPools = new DbPools(dbLimits, clock);
        ArtifactCache artifacts = new ArtifactCache(controlPlane, null);
        FunctionRegistry registry = new FunctionRegistry(MAX_LOADED, clock);
        ContextFactory contexts = new ContextFactory(new OutboundClients(clients), clock, controlPlane, dbPools, logLimits, traces);
        Reconciler reconciler = new Reconciler(
                pool, runnerId, version, controlPlane, artifacts, new JvmFunctionLoader(), registry, contexts,
                MetaspaceGuard.system(), clock, Reconciler.Timing.DEFAULTS);
        FunctionListener listener = new FunctionListener(reconciler, tokens, clock);
        listener.install(routes);
        ReconcileLoop loop = new ReconcileLoop(reconciler);
        loop.start();
        ScheduledExecutorService sweeper = Executors.newSingleThreadScheduledExecutor(r -> {
            Thread t = new Thread(r, "function-db-sweeper");
            t.setDaemon(true);
            return t;
        });
        long everySeconds = Math.clamp(dbLimits.idleTtl().toSeconds() / 2, 1, 60);
        sweeper.scheduleWithFixedDelay(dbPools::sweep, everySeconds, everySeconds, TimeUnit.SECONDS);
        LOG.info("function host started: pool={} runner={}", pool, runnerId);
        return new FunctionHost(reconciler, loop, listener, dbPools, sweeper, traceShipper);
    }

    public Reconciler reconciler() {
        return reconciler;
    }

    public FunctionListener listener() {
        return listener;
    }

    @Override
    public void close() {
        reconciler.drain();
        loop.close();
        reconciler.close();
        dbSweeper.shutdownNow();
        dbPools.close();
        // After the calls in flight have ended: what they recorded is shipped, within a bound.
        traceShipper.close(TRACE_FLUSH);
        LOG.info("function host stopped");
    }
}
