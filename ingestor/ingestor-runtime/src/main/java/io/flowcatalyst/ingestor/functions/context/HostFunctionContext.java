package io.flowcatalyst.ingestor.functions.context;

import io.flowcatalyst.function.Config;
import io.flowcatalyst.function.Dispatcher;
import io.flowcatalyst.function.Events;
import io.flowcatalyst.function.FunctionAddress;
import io.flowcatalyst.function.FunctionContext;
import io.flowcatalyst.function.HttpCaller;
import io.flowcatalyst.function.Secrets;
import java.lang.System.Logger;
import java.time.Clock;
import java.util.List;
import java.util.Objects;
import javax.sql.DataSource;

/// The real [FunctionContext], built once per loaded version by [ContextFactory#build] and passed to `init` and
/// every `handle` on that [io.flowcatalyst.ingestor.functions.load.LoadedFunction] — the SAME instance for the
/// version's whole lifetime, shared by every concurrent call, so every service on it is thread-safe and
/// nothing on it is per-invocation state (a settings change loads a brand-new
/// [io.flowcatalyst.ingestor.functions.load.LoadedFunction] with a brand-new context instead of mutating
/// this one in place).
///
/// `dataSource(name)` is not available in the ingestor's function host yet: the ingestor build carries no JDBC
/// connection pool (see [#dataSource]).
public final class HostFunctionContext implements FunctionContext {

    private final FunctionAddress address;
    private final int version;
    private final Config config;
    private final Secrets secrets;
    private final HttpCaller http;
    private final Events events;
    private final Dispatcher dispatcher;
    private final Clock clock;
    private final Logger logger;
    private final List<String> declaredDatabases;

    HostFunctionContext(FunctionAddress address, int version, Config config, Secrets secrets, HttpCaller http, Events events,
            Dispatcher dispatcher, Clock clock, List<String> declaredDatabases) {
        this.address = Objects.requireNonNull(address, "address");
        this.version = version;
        this.config = Objects.requireNonNull(config, "config");
        this.secrets = Objects.requireNonNull(secrets, "secrets");
        this.http = Objects.requireNonNull(http, "http");
        this.events = Objects.requireNonNull(events, "events");
        this.dispatcher = Objects.requireNonNull(dispatcher, "dispatcher");
        this.clock = Objects.requireNonNull(clock, "clock");
        this.declaredDatabases = List.copyOf(declaredDatabases);
        this.logger = new HostLogger(address.render());
    }

    @Override
    public Logger logger() {
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
    /// @throws UnsupportedOperationException always, for a declared name: the ingestor's function host has no
    ///                                        database connection pool yet
    @Override
    public DataSource dataSource(String name) {
        if (!declaredDatabases.contains(name)) {
            throw new IllegalArgumentException("no database named '" + name + "' declared by this function's describe");
        }
        throw new UnsupportedOperationException(
                "dataSource('" + name + "') is not available: the ingestor's function host has no database support yet");
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
