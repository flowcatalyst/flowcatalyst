package io.flowcatalyst.ingestor.functions;

import io.flowcatalyst.framework.auth.PlatformTokens;
import io.flowcatalyst.framework.http.Routes;
import io.flowcatalyst.framework.httpclient.HttpClients;
import io.flowcatalyst.ingestor.functions.context.ContextFactory;
import io.flowcatalyst.ingestor.functions.context.OutboundClients;
import io.flowcatalyst.ingestor.functions.control.ControlPlane;
import io.flowcatalyst.ingestor.functions.http.FunctionListener;
import io.flowcatalyst.ingestor.functions.load.FunctionRegistry;
import io.flowcatalyst.ingestor.functions.load.JvmFunctionLoader;
import io.flowcatalyst.ingestor.functions.load.MetaspaceGuard;
import io.flowcatalyst.ingestor.functions.reconcile.ArtifactCache;
import io.flowcatalyst.ingestor.functions.reconcile.ReconcileLoop;
import io.flowcatalyst.ingestor.functions.reconcile.Reconciler;
import java.time.Clock;
import java.util.Objects;
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

    private final Reconciler reconciler;
    private final ReconcileLoop loop;
    private final FunctionListener listener;

    private FunctionHost(Reconciler reconciler, ReconcileLoop loop, FunctionListener listener) {
        this.reconciler = reconciler;
        this.loop = loop;
        this.listener = listener;
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
        Objects.requireNonNull(pool, "pool");
        ArtifactCache artifacts = new ArtifactCache(controlPlane, null);
        FunctionRegistry registry = new FunctionRegistry(MAX_LOADED, clock);
        ContextFactory contexts = new ContextFactory(new OutboundClients(clients), clock, controlPlane);
        Reconciler reconciler = new Reconciler(
                pool, runnerId, version, controlPlane, artifacts, new JvmFunctionLoader(), registry, contexts,
                MetaspaceGuard.system(), clock, Reconciler.Timing.DEFAULTS);
        FunctionListener listener = new FunctionListener(reconciler, tokens, clock);
        listener.install(routes);
        ReconcileLoop loop = new ReconcileLoop(reconciler);
        loop.start();
        LOG.info("function host started: pool={} runner={}", pool, runnerId);
        return new FunctionHost(reconciler, loop, listener);
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
        LOG.info("function host stopped");
    }
}
