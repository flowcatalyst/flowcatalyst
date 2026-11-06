package io.flowcatalyst.function.testing;

import io.flowcatalyst.function.Config;
import io.flowcatalyst.function.Dispatcher;
import io.flowcatalyst.function.Events;
import io.flowcatalyst.function.FunctionAddress;
import io.flowcatalyst.function.FunctionContext;
import io.flowcatalyst.function.HttpCaller;
import io.flowcatalyst.function.PlatformTokenException;
import io.flowcatalyst.function.Secrets;
import io.flowcatalyst.function.TraceOptions;

import javax.sql.DataSource;
import java.lang.System.Logger;
import java.time.Clock;
import java.time.Instant;
import java.time.ZoneOffset;
import java.util.LinkedHashMap;
import java.util.LinkedHashSet;
import java.util.List;
import java.util.Map;
import java.util.NoSuchElementException;
import java.util.Objects;
import java.util.Optional;
import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.CopyOnWriteArrayList;
import java.util.concurrent.atomic.AtomicInteger;
import org.jspecify.annotations.Nullable;

/// A [FunctionContext] for unit-testing a function without a host. It lives in
/// the API jar (package `io.flowcatalyst.function.testing`) so a function's
/// tests need no second artifact.
///
/// ```java
/// var ctx = FakeFunctionContext.builder(FunctionAddress.parse("hello.greet"))
///         .config("greeting", "Hi").secret("api-key", "s3cr3t").build();
/// Result r = new GreetFunction().handle(FakeRequest.get("/greet/ann").build(), ctx);
/// assertThat(ctx.events().emitted()).hasSize(1);
/// ```
///
/// The clock is fixed (2026-09-20T00:00:00Z) unless set. Config and secret
/// lookups behave as the host's do: an undeclared key is empty, `require`
/// throws [NoSuchElementException] naming the key, and secret values never
/// appear in `toString`. `http()` and `dataSource(name)` throw
/// [UnsupportedOperationException] unless set. `platformToken()` answers the token set with
/// [Builder#platformToken], or throws the failure set with [Builder#platformTokenFailure]; with neither it
/// throws `NOT_DECLARED`, as a host does for a function that did not declare it. Every call is counted
/// ([#platformTokenCalls], [#platformTokenRefreshes]).
///
/// Traces: every `trace` call is recorded ([#traces]) whether or not its context is live, since a unit test wants
/// to see what the function wrote; `tracing(context)` answers `true` for the contexts given to [Builder#live] (or
/// [#setLive]); `setReference` keeps the last reference ([#reference]) and `conceal` the values ([#concealed]).
public final class FakeFunctionContext implements FunctionContext {

    private final FunctionAddress address;
    private final int version;
    private final Config config;
    private final Secrets secrets;
    private final RecordingEvents events;
    private final RecordingDispatcher dispatcher;
    private final CapturingLogger logger;
    private final Clock clock;
    private final HttpCaller http;
    private final Map<String, DataSource> dataSources;
    private final String platformToken;
    private final PlatformTokenException platformTokenFailure;
    private final AtomicInteger platformTokenCalls = new AtomicInteger();
    private final AtomicInteger platformTokenRefreshes = new AtomicInteger();
    private final Set<String> live = ConcurrentHashMap.newKeySet();
    private final List<TraceCall> traces = new CopyOnWriteArrayList<>();
    private final List<String> concealed = new CopyOnWriteArrayList<>();
    private volatile String reference = "";

    /// One call to `trace`, as the function made it.
    ///
    /// @param context the context named
    /// @param message the message
    /// @param data    the data, or `null`
    /// @param options the options, or `null`
    public record TraceCall(String context, String message, @Nullable Map<String, ?> data, @Nullable TraceOptions options) {
    }

    private FakeFunctionContext(Builder b) {
        this.address = b.address;
        this.version = b.version;
        Map<String, String> configValues = Map.copyOf(b.config);
        Map<String, String> secretValues = Map.copyOf(b.secrets);
        this.config = key -> Optional.ofNullable(configValues.get(key));
        this.secrets = new MapSecrets(secretValues);
        this.events = b.events;
        this.dispatcher = b.dispatcher;
        this.clock = b.clock;
        this.http = b.http;
        this.dataSources = Map.copyOf(b.dataSources);
        this.platformToken = b.platformToken;
        this.platformTokenFailure = b.platformTokenFailure;
        this.logger = new CapturingLogger("fn." + address.render());
        this.live.addAll(b.live);
    }

    /// Starts a context for the function at `address`, version 1.
    public static Builder builder(FunctionAddress address) {
        return new Builder(address);
    }

    /// The captured log lines.
    public CapturingLogger capturingLogger() {
        return logger;
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

    @Override
    public DataSource dataSource(String name) {
        DataSource ds = dataSources.get(name);
        if (ds == null) {
            throw new UnsupportedOperationException("this fake context declares no database: " + name);
        }
        return ds;
    }

    @Override
    public HttpCaller http() {
        return http;
    }

    /// The recording [Events]: `events().emitted()` lists what the function emitted.
    @Override
    public RecordingEvents events() {
        return events;
    }

    /// The recording [Dispatcher]: `dispatcher().dispatched()` lists the jobs.
    @Override
    public RecordingDispatcher dispatcher() {
        return dispatcher;
    }

    @Override
    public String platformToken(boolean refresh) {
        platformTokenCalls.incrementAndGet();
        if (refresh) {
            platformTokenRefreshes.incrementAndGet();
        }
        if (platformTokenFailure != null) {
            throw platformTokenFailure;
        }
        if (platformToken == null) {
            throw new PlatformTokenException("NOT_DECLARED", "this fake context has no platform token; set one with platformToken(...)");
        }
        return platformToken;
    }

    /// How many times `platformToken` was called, refreshes included.
    public int platformTokenCalls() {
        return platformTokenCalls.get();
    }

    /// How many of those calls asked for a refresh.
    public int platformTokenRefreshes() {
        return platformTokenRefreshes.get();
    }

    @Override
    public void trace(String context, String message, @Nullable Map<String, ?> data, @Nullable TraceOptions options) {
        traces.add(new TraceCall(context, message, data, options));
    }

    @Override
    public boolean tracing(String context) {
        return live.contains(context);
    }

    @Override
    public void setReference(String reference) {
        this.reference = reference == null ? "" : reference;
    }

    @Override
    public void conceal(String value) {
        if (value != null) {
            concealed.add(value);
        }
    }

    /// Every `trace` call so far, in order.
    public List<TraceCall> traces() {
        return traces;
    }

    /// The reference set last with `setReference`; empty when none.
    public String reference() {
        return reference;
    }

    /// The values given to `conceal`, in order.
    public List<String> concealed() {
        return concealed;
    }

    /// Makes `tracing(context)` answer `live` from now on.
    public void setLive(String context, boolean live) {
        if (live) {
            this.live.add(context);
        } else {
            this.live.remove(context);
        }
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

    private record MapSecrets(Map<String, String> values) implements Secrets {
        @Override
        public Optional<String> get(String key) {
            return Optional.ofNullable(values.get(key));
        }

        @Override
        public String toString() {
            return "Secrets" + values.keySet();
        }
    }

    /// Builds a [FakeFunctionContext].
    public static final class Builder {
        private final FunctionAddress address;
        private int version = 1;
        private final Map<String, String> config = new LinkedHashMap<>();
        private final Map<String, String> secrets = new LinkedHashMap<>();
        private final Map<String, DataSource> dataSources = new LinkedHashMap<>();
        private RecordingEvents events = new RecordingEvents();
        private RecordingDispatcher dispatcher = new RecordingDispatcher();
        private String platformToken;
        private PlatformTokenException platformTokenFailure;
        private final Set<String> live = new LinkedHashSet<>();
        private Clock clock = Clock.fixed(Instant.parse("2026-09-20T00:00:00Z"), ZoneOffset.UTC);
        private HttpCaller http = call -> {
            throw new UnsupportedOperationException("this fake context has no HTTP caller; set one with http(...)");
        };

        private Builder(FunctionAddress address) {
            this.address = Objects.requireNonNull(address, "address");
        }

        public Builder version(int version) {
            this.version = version;
            return this;
        }

        public Builder config(String key, String value) {
            config.put(key, value);
            return this;
        }

        public Builder secret(String key, String value) {
            secrets.put(key, value);
            return this;
        }

        public Builder dataSource(String name, DataSource dataSource) {
            dataSources.put(name, dataSource);
            return this;
        }

        public Builder http(HttpCaller http) {
            this.http = Objects.requireNonNull(http, "http");
            return this;
        }

        public Builder events(RecordingEvents events) {
            this.events = Objects.requireNonNull(events, "events");
            return this;
        }

        public Builder dispatcher(RecordingDispatcher dispatcher) {
            this.dispatcher = Objects.requireNonNull(dispatcher, "dispatcher");
            return this;
        }

        /// The token every `platformToken` call answers (use an obviously fake string).
        public Builder platformToken(String token) {
            this.platformToken = Objects.requireNonNull(token, "token");
            return this;
        }

        /// Makes every `platformToken` call throw `failure`.
        public Builder platformTokenFailure(PlatformTokenException failure) {
            this.platformTokenFailure = Objects.requireNonNull(failure, "failure");
            return this;
        }

        /// The trace contexts `tracing` answers `true` for.
        public Builder live(String... contexts) {
            live.addAll(List.of(contexts));
            return this;
        }

        public Builder clock(Clock clock) {
            this.clock = Objects.requireNonNull(clock, "clock");
            return this;
        }

        public FakeFunctionContext build() {
            return new FakeFunctionContext(this);
        }
    }
}
