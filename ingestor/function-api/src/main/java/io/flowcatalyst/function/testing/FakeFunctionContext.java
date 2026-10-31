package io.flowcatalyst.function.testing;

import io.flowcatalyst.function.Config;
import io.flowcatalyst.function.Dispatcher;
import io.flowcatalyst.function.Events;
import io.flowcatalyst.function.FunctionAddress;
import io.flowcatalyst.function.FunctionContext;
import io.flowcatalyst.function.HttpCaller;
import io.flowcatalyst.function.Secrets;

import javax.sql.DataSource;
import java.lang.System.Logger;
import java.time.Clock;
import java.time.Instant;
import java.time.ZoneOffset;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.NoSuchElementException;
import java.util.Objects;
import java.util.Optional;

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
/// [UnsupportedOperationException] unless set.
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
        this.logger = new CapturingLogger("fn." + address.render());
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

        public Builder clock(Clock clock) {
            this.clock = Objects.requireNonNull(clock, "clock");
            return this;
        }

        public FakeFunctionContext build() {
            return new FakeFunctionContext(this);
        }
    }
}
