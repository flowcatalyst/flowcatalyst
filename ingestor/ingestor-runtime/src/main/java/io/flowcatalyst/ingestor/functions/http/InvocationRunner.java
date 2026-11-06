package io.flowcatalyst.ingestor.functions.http;

import io.flowcatalyst.framework.httpclient.HttpCapture;
import io.flowcatalyst.function.FunctionContext;
import io.flowcatalyst.function.Request;
import io.flowcatalyst.function.Result;
import io.flowcatalyst.ingestor.functions.context.InvocationDeadline;
import io.flowcatalyst.ingestor.functions.context.InvocationInfo;
import io.flowcatalyst.ingestor.functions.load.LoadedFunction;
import io.flowcatalyst.ingestor.functions.trace.InvocationTrace;
import java.time.Instant;
import java.util.LinkedHashMap;
import java.util.Map;
import java.util.Objects;
import java.util.concurrent.CompletableFuture;
import org.jspecify.annotations.Nullable;
import org.slf4j.MDC;

/// Runs one invocation on its own virtual thread with a deadline: the caller has already retained `fn`
/// ([LoadedFunction#tryRetain]) and holds a permit, and the WORKER thread releases both, in its own `finally`,
/// when the handler really returns — not when the caller gives up waiting. A function that ignores the
/// interrupt past the deadline is still "in flight" for [LoadedFunction#close]'s drain, and still holds its
/// permit, exactly as long as it actually keeps running (the permit gauge shows the truth).
///
/// The MDC keys (`function`, `version`, `execution_id`, `correlation_id`) are set HERE, on the worker thread
/// that actually calls [LoadedFunction#invoke] — the thread whose SLF4J logger calls (including every one
/// [FunctionContext#logger()]'s `fn.<address>` logger makes) need them.
///
/// [InvocationDeadline#CURRENT] and [InvocationInfo#CURRENT] are bound around the call into the function
/// (never mutable fields on the shared [FunctionContext]: concurrent invocations of one loaded version must
/// never race on them). A call is one new virtual thread and nothing else: no lock, no queue, no shared
/// executor, so N calls of one function run at once (R3).
public final class InvocationRunner {

    private InvocationRunner() {
    }

    /// `future` completes when the function returns or throws; `worker` is what a caller interrupts on a deadline.
    public record Invocation(CompletableFuture<Result> future, Thread worker) {
    }

    /// @param fn         retained by the caller; released by the worker when the handler returns
    /// @param onReturned runs on the worker after the handler returned or threw, after `fn` was released — the
    ///                    permit's release
    /// @param deadline   when this invocation's [InvocationDeadline] expires — bound as a [ScopedValue] around the
    ///                    call, so an outbound call's timeout is capped to the time left
    public static Invocation start(LoadedFunction fn, Request request, FunctionContext ctx, Instant deadline, Runnable onReturned) {
        return start(fn, request, ctx, deadline, onReturned, null);
    }

    /// As above; `trace`, when not null, is bound as [InvocationTrace#CURRENT] around the call, and, when its `http`
    /// context is live, its outbound sink as [HttpCapture#CURRENT], so the function's outbound calls become `outbound`
    /// records. Bind nothing for an invocation with no live context: the off path adds no binding.
    public static Invocation start(
            LoadedFunction fn, Request request, FunctionContext ctx, Instant deadline, Runnable onReturned, @Nullable InvocationTrace trace) {
        Objects.requireNonNull(fn, "fn");
        Objects.requireNonNull(request, "request");
        Objects.requireNonNull(ctx, "ctx");
        Objects.requireNonNull(deadline, "deadline");
        Objects.requireNonNull(onReturned, "onReturned");
        CompletableFuture<Result> future = new CompletableFuture<>();
        InvocationInfo info = new InvocationInfo(request.id(), request.header("X-Correlation-Id").orElse(null));
        ScopedValue.Carrier scope = ScopedValue.where(InvocationDeadline.CURRENT, deadline).where(InvocationInfo.CURRENT, info);
        if (trace != null) {
            scope = scope.where(InvocationTrace.CURRENT, trace);
            if (trace.httpLive()) {
                scope = scope.where(HttpCapture.CURRENT, trace.outboundSink());
            }
        }
        ScopedValue.Carrier bound = scope;
        Thread worker = Thread.ofVirtual().unstarted(() -> {
            Map<String, String> mdc = mdcFor(request);
            mdc.forEach(MDC::put);
            try {
                Result result = bound.call(() -> fn.invoke(request, ctx));
                future.complete(result);
            } catch (Throwable t) {
                future.completeExceptionally(t);
            } finally {
                mdc.keySet().forEach(MDC::remove);
                try {
                    fn.release();
                } finally {
                    onReturned.run();
                }
            }
        });
        worker.start();
        return new Invocation(future, worker);
    }

    private static Map<String, String> mdcFor(Request request) {
        Map<String, String> mdc = new LinkedHashMap<>();
        mdc.put("function", request.address().render());
        mdc.put("version", String.valueOf(request.version()));
        mdc.put("execution_id", request.id());
        @Nullable String correlation = request.header("X-Correlation-Id").orElse(null);
        if (correlation != null) {
            mdc.put("correlation_id", correlation);
        }
        return mdc;
    }
}
