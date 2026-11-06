package io.flowcatalyst.ingestor.functions.trace;

import io.flowcatalyst.ingestor.functions.control.Control;
import io.flowcatalyst.ingestor.runtime.TraceRedactor;
import java.time.Clock;
import java.util.ArrayDeque;
import java.util.ArrayList;
import java.util.Collection;
import java.util.List;
import java.util.Objects;
import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;
import java.util.concurrent.atomic.AtomicLong;
import org.jspecify.annotations.Nullable;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/// The host's function-trace state (docs/jvm-function-trace-plan.md): the gate that decides, at the start of an
/// invocation, which trace contexts are live for it ([#begin]), the samplers behind it, and the credentials functions
/// obtained at run time ([#conceal]), kept per function address and dropped when the address leaves the desired
/// document ([#retain]).
///
/// The windows themselves are not kept here: they are the `trace` of the function's entry in the desired document the
/// invocation resolved through, replaced as a whole when the next document is applied. A window is live iff its
/// `enabledUntilMs` is after this host's clock, compared on every use.
///
/// With no window on the function, [#begin] reads a field and returns `null`: nothing is allocated, bound or recorded.
public final class FunctionTraces {

    private static final Logger LOG = LoggerFactory.getLogger(FunctionTraces.class);

    /// The built-in context (`abi.TraceHTTP`): the call itself, the function's log lines and its outbound calls.
    public static final String HTTP = "http";

    /// The default most bytes of a body an `http` record keeps (`FC_FN_TRACE_BODY_BYTES`).
    public static final int DEFAULT_BODY_BYTES = 65536;

    /// The concealed values kept per function; the oldest go first.
    public static final int MAX_CONCEALED = 32;

    private final Clock clock;
    private final FunctionTraceSink sink;
    private final int bodyBytes;

    /// `address → context → counter`; the counter is how many invocations saw the window live.
    private final ConcurrentHashMap<String, ConcurrentHashMap<String, AtomicLong>> samplers = new ConcurrentHashMap<>();

    /// `address → values`, oldest first.
    private final ConcurrentHashMap<String, ArrayDeque<String>> concealed = new ConcurrentHashMap<>();

    private final Set<String> warned = ConcurrentHashMap.newKeySet();

    /// @param clock     the host's clock: windows are compared with it and records are stamped by it
    /// @param sink      where finished records go
    /// @param bodyBytes the most bytes of a body an `http` record keeps
    public FunctionTraces(Clock clock, FunctionTraceSink sink, int bodyBytes) {
        this.clock = Objects.requireNonNull(clock, "clock");
        this.sink = Objects.requireNonNull(sink, "sink");
        this.bodyBytes = Math.max(0, bodyBytes);
    }

    /// A host that ships no traces: windows are still honoured (so `tracing` answers truthfully) but records go
    /// nowhere.
    public static FunctionTraces disabled(Clock clock) {
        return new FunctionTraces(clock, FunctionTraceSink.DISCARD, DEFAULT_BODY_BYTES);
    }

    Clock clock() {
        return clock;
    }

    FunctionTraceSink sink() {
        return sink;
    }

    int bodyBytes() {
        return bodyBytes;
    }

    /// The gate, called once per invocation, after its concurrency permit was taken: the contexts of `doc`'s windows
    /// that are live now and whose sampler admits this invocation (one in `sampleRate`, counted per function and
    /// context). `null` when there are none, which is the case at once for a function with no window.
    ///
    /// @param doc           the function's entry in the desired document the invocation resolved through
    /// @param version       the version serving it
    /// @param invocationId  the invocation's id
    /// @param correlationId the request's `X-Correlation-Id`, if it had one
    public @Nullable InvocationTrace begin(Control.FunctionDoc doc, int version, String invocationId, @Nullable String correlationId) {
        List<Control.TraceState> windows = doc.trace();
        if (windows.isEmpty()) {
            return null;
        }
        long now = clock.millis();
        List<String> live = null;
        for (Control.TraceState window : windows) {
            String context = window.context();
            if (context == null || context.isEmpty() || window.enabledUntilMs() <= now || (live != null && live.contains(context))) {
                continue;
            }
            long rate = Math.max(1, window.sampleRate());
            AtomicLong counter = samplers.computeIfAbsent(doc.address(), a -> new ConcurrentHashMap<>())
                    .computeIfAbsent(context, c -> new AtomicLong());
            if (counter.getAndIncrement() % rate == 0) {
                if (live == null) {
                    live = new ArrayList<>(2);
                }
                live.add(context);
            }
        }
        if (live == null) {
            return null;
        }
        return new InvocationTrace(this, doc.address(), version, invocationId, correlationId == null ? "" : correlationId,
                live, doc.secrets().values());
    }

    /// Keeps `value`, a credential the function at `address` obtained at run time (a session, an access token), out of
    /// every record it writes from now on. Ignored when under [TraceRedactor#MIN_SECRET] characters. The last
    /// [#MAX_CONCEALED] are held. Called whether or not any window is on.
    public void conceal(String address, @Nullable String value) {
        if (value == null || value.length() < TraceRedactor.MIN_SECRET) {
            return;
        }
        ArrayDeque<String> held = concealed.computeIfAbsent(address, a -> new ArrayDeque<>());
        synchronized (held) {
            held.remove(value);
            held.addLast(value);
            while (held.size() > MAX_CONCEALED) {
                held.removeFirst();
            }
        }
    }

    /// The values concealed for `address`, oldest first.
    public List<String> concealed(String address) {
        ArrayDeque<String> held = concealed.get(address);
        if (held == null) {
            return List.of();
        }
        synchronized (held) {
            return List.copyOf(held);
        }
    }

    /// Forgets what was kept for every function address not in `addresses`: the concealed values and the samplers.
    public void retain(Collection<String> addresses) {
        concealed.keySet().removeIf(a -> !addresses.contains(a));
        samplers.keySet().removeIf(a -> !addresses.contains(a));
        warned.removeIf(key -> !addresses.contains(key.substring(0, Math.max(0, key.indexOf('\0')))));
    }

    /// How many distinct (function, context) pairs have been warned about by [#warnNotWritable]: for tests.
    public int warnings() {
        return warned.size();
    }

    /// Logs, once per function address and context, that a function wrote to a trace context it may not.
    ///
    /// @param builtIn the context is `http`, which a function reads but does not write
    public void warnNotWritable(String address, String context, boolean builtIn) {
        if (warned.add(address + '\0' + context)) {
            if (builtIn) {
                LOG.warn("function {} wrote to trace context '{}', which is built in and not writable; nothing is recorded", address, context);
            } else {
                LOG.warn("function {} wrote to trace context '{}', which its describe does not declare (NOT_DECLARED); nothing is recorded",
                        address, context);
            }
        }
    }
}
