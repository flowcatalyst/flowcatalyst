package io.flowcatalyst.ingestor.functions.context;

import io.flowcatalyst.function.Config;
import io.flowcatalyst.function.Dispatcher;
import io.flowcatalyst.function.Events;
import io.flowcatalyst.function.FunctionAddress;
import io.flowcatalyst.function.FunctionContext;
import io.flowcatalyst.function.HttpCaller;
import io.flowcatalyst.function.Secrets;
import io.flowcatalyst.function.TraceOptions;
import io.flowcatalyst.ingestor.functions.trace.FunctionTraces;
import io.flowcatalyst.ingestor.functions.trace.InvocationTrace;
import java.time.Clock;
import java.util.List;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;
import java.util.Objects;
import java.util.Set;
import javax.sql.DataSource;
import org.jspecify.annotations.Nullable;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/// The real [FunctionContext], built once per loaded version by [ContextFactory#build] and passed to `init` and
/// every `handle` on that [io.flowcatalyst.ingestor.functions.load.LoadedFunction] — the SAME instance for the
/// version's whole lifetime, shared by every concurrent call, so every service on it is thread-safe and
/// nothing on it is per-invocation state (a settings change loads a brand-new
/// [io.flowcatalyst.ingestor.functions.load.LoadedFunction] with a brand-new context instead of mutating
/// this one in place).
///
/// `dataSource(name)` borrows from the host's shared pool for the DSN bound to the name in the desired document
/// (see [DbPools]); the DSNs are the ones in the document this version was loaded with, and a changed DSN loads a
/// new version of the context.
public final class HostFunctionContext implements FunctionContext {

    private static final Logger LOG = LoggerFactory.getLogger(HostFunctionContext.class);

    private final FunctionAddress address;
    private final int version;
    private final Config config;
    private final Secrets secrets;
    private final HttpCaller http;
    private final Events events;
    private final Dispatcher dispatcher;
    private final PlatformTokenSource platformTokens;
    private final Clock clock;
    private final System.Logger logger;
    private final List<String> declaredDatabases;
    private final Map<String, String> databaseDsns;
    private final DbPools dbPools;
    private final FunctionTraces traces;
    private final String traceAddress;
    private final Set<String> traceContexts;
    private final Map<String, DataSource> dataSources = new ConcurrentHashMap<>();

    HostFunctionContext(FunctionAddress address, int version, Config config, Secrets secrets, HttpCaller http, Events events,
            Dispatcher dispatcher, PlatformTokenSource platformTokens, Clock clock, List<String> declaredDatabases,
            Map<String, String> databaseDsns, DbPools dbPools) {
        this(address, version, config, secrets, http, events, dispatcher, platformTokens, clock, declaredDatabases, databaseDsns, dbPools,
                new LogBudget(LogBudget.Limits.DEFAULTS));
    }

    HostFunctionContext(FunctionAddress address, int version, Config config, Secrets secrets, HttpCaller http, Events events,
            Dispatcher dispatcher, PlatformTokenSource platformTokens, Clock clock, List<String> declaredDatabases,
            Map<String, String> databaseDsns, DbPools dbPools, LogBudget logBudget) {
        this(address, version, config, secrets, http, events, dispatcher, platformTokens, clock, declaredDatabases, databaseDsns, dbPools, logBudget,
                FunctionTraces.disabled(clock), address.render(), Set.of());
    }

    /// @param traces        the host's trace state
    /// @param traceAddress  the function's address as the desired document spells it (what [FunctionTraces] keys by)
    /// @param traceContexts the trace contexts the version's describe declares
    HostFunctionContext(FunctionAddress address, int version, Config config, Secrets secrets, HttpCaller http, Events events,
            Dispatcher dispatcher, PlatformTokenSource platformTokens, Clock clock, List<String> declaredDatabases,
            Map<String, String> databaseDsns, DbPools dbPools, LogBudget logBudget, FunctionTraces traces, String traceAddress,
            Set<String> traceContexts) {
        this.traces = Objects.requireNonNull(traces, "traces");
        this.traceAddress = Objects.requireNonNull(traceAddress, "traceAddress");
        this.traceContexts = Set.copyOf(traceContexts);
        this.address = Objects.requireNonNull(address, "address");
        this.version = version;
        this.config = Objects.requireNonNull(config, "config");
        this.secrets = Objects.requireNonNull(secrets, "secrets");
        this.http = Objects.requireNonNull(http, "http");
        this.events = Objects.requireNonNull(events, "events");
        this.dispatcher = Objects.requireNonNull(dispatcher, "dispatcher");
        this.platformTokens = Objects.requireNonNull(platformTokens, "platformTokens");
        this.clock = Objects.requireNonNull(clock, "clock");
        this.declaredDatabases = List.copyOf(declaredDatabases);
        this.databaseDsns = Map.copyOf(databaseDsns);
        this.dbPools = Objects.requireNonNull(dbPools, "dbPools");
        this.logger = new HostLogger(address.render(), logBudget);
    }

    @Override
    public System.Logger logger() {
        return logger;
    }

    @Override
    public Config config() {
        return config;
    }

    @Override
    public Secrets secrets() {
        return secrets;
    }

    /// @throws IllegalArgumentException `name` is not one of this version's declared `db` entries
    /// @throws IllegalStateException    `name` is declared but the function has no DSN bound to it, or the DSN
    ///                                   cannot be used (the message never carries any of it)
    @Override
    public DataSource dataSource(String name) {
        if (!declaredDatabases.contains(name)) {
            throw new IllegalArgumentException("no database named '" + name + "' declared by this function's describe");
        }
        DataSource known = dataSources.get(name);
        if (known != null) {
            return known;
        }
        String dsn = databaseDsns.get(name);
        if (dsn == null || dsn.isBlank()) {
            throw new IllegalStateException("database '" + name + "' has no binding on this function");
        }
        PostgresDsn.Target target;
        try {
            target = PostgresDsn.parse(dsn);
        } catch (PostgresDsn.DsnException e) {
            throw new IllegalStateException("database '" + name + "' has a DSN that cannot be used: " + e.getMessage());
        }
        return dataSources.computeIfAbsent(name, n -> dbPools.shared(dsn, target));
    }

    @Override
    public HttpCaller http() {
        return http;
    }

    @Override
    public Events events() {
        return events;
    }

    @Override
    public Dispatcher dispatcher() {
        return dispatcher;
    }

    /// Never logged: the token is a credential.
    @Override
    public String platformToken(boolean refresh) {
        return platformTokens.token(refresh);
    }

    /// Writes a record to a declared trace context while it is live for the current invocation; see
    /// [FunctionContext#trace]. Never throws.
    @Override
    public void trace(String context, String message, @Nullable Map<String, ?> data, @Nullable TraceOptions options) {
        try {
            if (context == null) {
                return;
            }
            boolean builtIn = FunctionTraces.HTTP.equals(context);
            if (builtIn || !traceContexts.contains(context)) {
                traces.warnNotWritable(traceAddress, context, builtIn);
                return;
            }
            InvocationTrace trace = InvocationTrace.current();
            if (trace != null) {
                trace.trace(context, message, data, options);
            }
        } catch (RuntimeException | StackOverflowError e) {
            LOG.debug("a trace call of {} failed", traceAddress, e);
        }
    }

    /// True iff `context` (`http` or one the describe declares) is live and sampled in for the current invocation.
    @Override
    public boolean tracing(String context) {
        if (context == null || !(FunctionTraces.HTTP.equals(context) || traceContexts.contains(context))) {
            return false;
        }
        InvocationTrace trace = InvocationTrace.current();
        return trace != null && trace.isLive(context);
    }

    @Override
    public void setReference(String reference) {
        InvocationTrace trace = InvocationTrace.current();
        if (trace != null) {
            trace.setReference(reference);
        }
    }

    /// Works outside an invocation too (`init`): the value is kept for the function, not the invocation.
    @Override
    public void conceal(String value) {
        traces.conceal(traceAddress, value);
    }

    @Override
    public Clock clock() {
        return clock;
    }

    @Override
    public FunctionAddress address() {
        return address;
    }

    @Override
    public int version() {
        return version;
    }
}
