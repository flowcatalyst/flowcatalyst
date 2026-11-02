package io.flowcatalyst.ingestor.functions.context;

import io.flowcatalyst.function.FunctionAddress;
import io.flowcatalyst.ingestor.functions.control.Control;
import io.flowcatalyst.ingestor.functions.control.ControlPlane;
import io.flowcatalyst.ingestor.functions.control.Describe;
import java.time.Clock;
import java.util.Objects;
import java.util.concurrent.ConcurrentHashMap;

/// Builds a [HostFunctionContext] from a desired-state function and one of its versions' describe documents:
/// config and secrets restricted to what the describe declares, the allowlisting HTTP caller, and events and
/// dispatch through the one [ControlPlane] the reconciler itself polls and heartbeats through.
public final class ContextFactory {

    private final OutboundClients outbound;
    private final Clock clock;
    private final ControlPlane controlPlane;
    private final DbPools dbPools;
    private final LogBudget.Limits logLimits;

    /// One log budget per function address, shared by all its versions (docs/ingestor-failure-pacing.md R6).
    private final ConcurrentHashMap<String, LogBudget> logBudgets = new ConcurrentHashMap<>();

    /// @param dbPools the host's shared database pools (the host owns and closes them)
    public ContextFactory(OutboundClients outbound, Clock clock, ControlPlane controlPlane, DbPools dbPools) {
        this(outbound, clock, controlPlane, dbPools, LogBudget.Limits.DEFAULTS);
    }

    /// @param logLimits how many lines a second a function may log, and its burst
    public ContextFactory(OutboundClients outbound, Clock clock, ControlPlane controlPlane, DbPools dbPools, LogBudget.Limits logLimits) {
        this.logLimits = Objects.requireNonNull(logLimits, "logLimits");
        this.dbPools = Objects.requireNonNull(dbPools, "dbPools");
        this.outbound = Objects.requireNonNull(outbound, "outbound");
        this.clock = Objects.requireNonNull(clock, "clock");
        this.controlPlane = Objects.requireNonNull(controlPlane, "controlPlane");
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
                clock,
                describe.db(),
                function.db(),
                dbPools,
                logBudgets.computeIfAbsent(address.render(), a -> new LogBudget(logLimits)));
    }
}
