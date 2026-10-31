package io.flowcatalyst.function;

import java.lang.System.Logger;
import java.time.Clock;
import javax.sql.DataSource;

/// Everything a function may reach outside its own invocation. The host
/// builds one instance per loaded version and passes it to [Function#init],
/// [Function#handle] and [Function#stop]; it is shared by every concurrent
/// call, so its services must be, and are, thread-safe.
///
/// [HttpCaller], [Events] and [Dispatcher] are interfaces with value types
/// here; the host implements them. `config`, `secrets`, `dataSource`, `http`,
/// `events` and `dispatcher` are limited to what the function declared in its
/// describe document (`config`, `secrets`, `db`, `httpAllow`, `emits`,
/// `dispatches`). For a unit test without a host see
/// [io.flowcatalyst.function.testing.FakeFunctionContext].
///
/// Function traces (`traceContexts`) are a later slice and have no API here yet.
public interface FunctionContext {

    /// A logger carrying the host's MDC keys (function address, version,
    /// invocation id).
    Logger logger();

    /// The declared config keys, resolved by the host.
    Config config();

    /// The declared secrets, resolved by the host. Values
    /// never appear in a `toString`.
    Secrets secrets();

    /// The pool for the declared database named `name`.
    DataSource dataSource(String name);

    /// The host-mediated outbound HTTP client.
    HttpCaller http();

    /// Emits platform events on this function's behalf, through the host.
    Events events();

    /// Creates dispatch jobs to declared targets on this function's behalf,
    /// through the host.
    Dispatcher dispatcher();

    /// The host's clock — never `Clock.systemUTC()` read directly, so a test
    /// can fix time.
    Clock clock();

    /// The address of the function this context was built for.
    FunctionAddress address();

    /// The loaded version of the function this context was built for.
    int version();
}
