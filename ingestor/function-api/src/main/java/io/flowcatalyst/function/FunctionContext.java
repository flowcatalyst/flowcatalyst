package io.flowcatalyst.function;

import java.lang.System.Logger;
import java.time.Clock;
import java.util.Map;
import javax.sql.DataSource;
import org.jspecify.annotations.Nullable;

/// Everything a function may reach outside its own invocation. The host
/// builds one instance per loaded version and passes it to [Function#init],
/// [Function#handle] and [Function#stop]; it is shared by every concurrent
/// call, so its services must be, and are, thread-safe.
///
/// [HttpCaller], [Events] and [Dispatcher] are interfaces with value types
/// here; the host implements them. `config`, `secrets`, `dataSource`, `http`,
/// `events`, `dispatcher` and `platformToken` are limited to what the function
/// declared in its describe document (`config`, `secrets`, `db`, `httpAllow`, `emits`,
/// `dispatches`, `platformToken`). For a unit test without a host see
/// [io.flowcatalyst.function.testing.FakeFunctionContext].
///
/// Function traces: an operator switches a trace context on for a function for a while (docs/function-trace-design.md),
/// and while it is on the host records what the function writes with [#trace], plus, for the built-in `http`
/// context, the request, the response, the function's log lines and its outbound calls. A function declares the
/// contexts it writes to in `traceContexts` of its describe. The trace methods are no-ops when nothing is
/// switched on and never throw; a function can call them freely.
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

    /// A FlowCatalyst access token for calling another first-party application, as the service account
    /// linked to this function's application. The function declares `platformToken: true` in its describe
    /// and holds no client id or secret. The host caches the token while it is valid and renews it shortly
    /// before it expires, so call this per outbound request and do not keep the value. The token is a
    /// credential: never log it.
    ///
    /// @throws PlatformTokenException not declared, not allowed by the platform, or the platform could not be
    ///                                reached ([PlatformTokenException#retryable()])
    default String platformToken() {
        return platformToken(false);
    }

    /// As [#platformToken()]; `refresh` skips the cache and fetches a new token, for a target that answered
    /// 401 because the account's roles changed.
    ///
    /// @throws PlatformTokenException as [#platformToken()]
    String platformToken(boolean refresh);

    /// The host's clock — never `Clock.systemUTC()` read directly, so a test
    /// can fix time.
    Clock clock();

    /// Records `message` in the trace context `context`, if the context is switched on for this invocation.
    /// Same as `trace(context, message, null, null)`.
    default void trace(String context, String message) {
        trace(context, message, null, null);
    }

    /// Records `message` and `data` in the trace context `context`. Same as `trace(context, message, data, null)`.
    default void trace(String context, String message, @Nullable Map<String, ?> data) {
        trace(context, message, data, null);
    }

    /// Records `message`, with `data` (serialized as JSON; a record whose data is over 32768 bytes is not written),
    /// in the trace context `context`. The context must be declared in the describe's `traceContexts`: `http` is
    /// built in and not writable, and an undeclared name records nothing (the host logs one warning). A record is
    /// written only when the context is switched on and this invocation was sampled in; at most 500 records an
    /// invocation. Declared secrets and values given to [#conceal] are replaced in it. Never throws.
    ///
    /// @param options the record's level and its own reference, or `null` for `info` and the invocation's reference
    default void trace(String context, String message, @Nullable Map<String, ?> data, @Nullable TraceOptions options) {
    }

    /// Whether `context` (declared, or `http`) is switched on and sampled in for the current invocation, so building
    /// an expensive [#trace] record is worth it. `false` outside an invocation.
    default boolean tracing(String context) {
        return false;
    }

    /// Sets the invocation's business key (cut to 200 characters), stamped on every record it writes from now on,
    /// the `http` record of the call included. An empty reference clears it. Does nothing outside an invocation.
    default void setReference(String reference) {
    }

    /// Keeps `value`, a credential the function obtained at run time (a session id, an access token), out of every
    /// trace record, as its declared secrets are: replaced wherever it appears, in a URL's path as well. Call it each
    /// time one is obtained, whether or not tracing is on, so a value that outlives the moment an operator switches
    /// tracing on is covered. Values under four characters are ignored; the host holds the last 32 per function.
    /// Works in [Function#init] too.
    default void conceal(String value) {
    }

    /// The address of the function this context was built for.
    FunctionAddress address();

    /// The loaded version of the function this context was built for.
    int version();
}
