package io.flowcatalyst.ingestor.functions.context;

import io.flowcatalyst.function.FunctionAddress;
import io.flowcatalyst.ingestor.functions.control.Control;
import io.flowcatalyst.ingestor.functions.control.ControlPlane;
import io.flowcatalyst.ingestor.functions.control.Describe;
import io.flowcatalyst.ingestor.functions.trace.FunctionTraces;
import java.time.Clock;
import java.util.Objects;
import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;

/// Builds a [HostFunctionContext] from a desired-state function and one of its versions' describe documents:
/// config and secrets restricted to what the describe declares, the allowlisting HTTP caller, and events, dispatch
/// and platform tokens through the one [ControlPlane] the reconciler itself polls and heartbeats through.
public final class ContextFactory {

    private final OutboundClients outbound;
    private final Clock clock;
    private final ControlPlane controlPlane;
    private final DbPools dbPools;
    private final PlatformTokenCache platformTokens;
    private final LogBudget.Limits logLimits;
    private final FunctionTraces traces;

    /// One log budget per function address, shared by all its versions (docs/ingestor-failure-pacing.md R6).
    private final ConcurrentHashMap<String, LogBudget> logBudgets = new ConcurrentHashMap<>();

    /// @param dbPools the host's shared database pools (the host owns and closes them)
    public ContextFactory(OutboundClients outbound, Clock clock, ControlPlane controlPlane, DbPools dbPools) {
        this(outbound, clock, controlPlane, dbPools, LogBudget.Limits.DEFAULTS);
    }

    /// @param logLimits how many lines a second a function may log, and its burst
    public ContextFactory(OutboundClients outbound, Clock clock, ControlPlane controlPlane, DbPools dbPools, LogBudget.Limits logLimits) {
        this(outbound, clock, controlPlane, dbPools, logLimits, FunctionTraces.disabled(clock));
    }

    /// @param traces the host's function-trace state: the contexts built here conceal and trace through it, and the
    ///               listener opens each invocation's trace with it
    public ContextFactory(OutboundClients outbound, Clock clock, ControlPlane controlPlane, DbPools dbPools, LogBudget.Limits logLimits,
            FunctionTraces traces) {
        this.traces = Objects.requireNonNull(traces, "traces");
        this.logLimits = Objects.requireNonNull(logLimits, "logLimits");
        this.dbPools = Objects.requireNonNull(dbPools, "dbPools");
        this.outbound = Objects.requireNonNull(outbound, "outbound");
        this.clock = Objects.requireNonNull(clock, "clock");
        this.controlPlane = Objects.requireNonNull(controlPlane, "controlPlane");
        this.platformTokens = new PlatformTokenCache(controlPlane, clock);
    }

    /// Drops the cached platform tokens of every function not in `functionIds` (the ids in the desired document).
    public void retainFunctions(Set<String> functionIds) {
        platformTokens.retain(functionIds);
    }

    /// The host's function-trace state.
    public FunctionTraces traces() {
        return traces;
    }

    /// The host's platform-token cache.
    public PlatformTokenCache platformTokens() {
        return platformTokens;
    }

    /// @throws ContextLoadException the context cannot be built: the address is not one the function API can represent
    ///                               (a permanent load failure)
    public HostFunctionContext build(Control.FunctionDoc function, int version, Describe describe) {
        FunctionAddress address;
        try {
            address = FunctionAddress.parse(function.address());
        } catch (IllegalArgumentException e) {
            // The platform accepts application codes the function API's address type does not (upper case,
            // underscores): such a function cannot be served by this host.
            throw new ContextLoadException("ADDRESS_UNSUPPORTED", "the address " + function.address() + " is not one the function API can represent");
        }
        return new HostFunctionContext(
                address,
                version,
                new MapConfig(function.config(), describe.config()),
                new MapSecrets(function.secrets(), describe.secrets()),
                new AllowlistHttpCaller(outbound, describe.httpAllow(), clock),
                new ControlPlaneEvents(controlPlane, function.id(), function.address(), version, describe.emits()),
                new ControlPlaneDispatcher(controlPlane, function.id(), version, describe.dispatches()),
                new PlatformTokenSource(platformTokens, function.id(), version, describe.platformToken(), clock),
                clock,
                describe.db(),
                function.db(),
                dbPools,
                logBudgets.computeIfAbsent(address.render(), a -> new LogBudget(logLimits)),
                traces,
                function.address(),
                describe.traceContextNames());
    }
}
