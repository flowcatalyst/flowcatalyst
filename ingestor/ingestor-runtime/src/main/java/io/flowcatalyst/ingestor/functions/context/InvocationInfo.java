package io.flowcatalyst.ingestor.functions.context;

import org.jspecify.annotations.Nullable;

/// What a function's host calls need to know about the invocation making them: its id, and the inbound
/// request's `X-Correlation-Id` when it carried one (the platform makes it the correlation id of a dispatch
/// job unless the function set its own). Bound as a [ScopedValue] around the worker thread's call into the
/// function ([io.flowcatalyst.ingestor.functions.http.InvocationRunner]), beside [InvocationDeadline] —
/// never a mutable field on the shared [io.flowcatalyst.function.FunctionContext], which concurrent
/// invocations of one loaded version share.
public final class InvocationInfo {

    /// Bound for the duration of one invocation's `Function#handle` call; unbound outside one (`init`).
    public static final ScopedValue<InvocationInfo> CURRENT = ScopedValue.newInstance();

    /// What applies when no invocation is bound.
    public static final InvocationInfo NONE = new InvocationInfo(null, null);

    private final @Nullable String invocationId;
    private final @Nullable String requestCorrelationId;

    public InvocationInfo(@Nullable String invocationId, @Nullable String requestCorrelationId) {
        this.invocationId = invocationId;
        this.requestCorrelationId = requestCorrelationId;
    }

    public @Nullable String invocationId() {
        return invocationId;
    }

    public @Nullable String requestCorrelationId() {
        return requestCorrelationId;
    }

    /// [#CURRENT] if bound, else [#NONE].
    public static InvocationInfo current() {
        return CURRENT.isBound() ? CURRENT.get() : NONE;
    }
}
