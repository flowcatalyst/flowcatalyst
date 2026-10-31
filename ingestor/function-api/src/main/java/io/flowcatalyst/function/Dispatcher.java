package io.flowcatalyst.function;

/// Creates dispatch jobs on the platform on a function's behalf, through the
/// host (`describe.dispatches`; `fn.dispatch` in the other SDKs). The
/// function names a target and a path, never a URL or a credential: the
/// platform builds the delivery URL and signs the delivery as the
/// application. This is an interface only; the host implements it.
///
/// A dispatch is **not idempotent**: the platform does not de-duplicate
/// jobs, so retrying after an ambiguous failure can create a second job.
public interface Dispatcher {

    /// Creates the job and returns its id. `(target, path)` must be declared in
    /// the function's `dispatches`.
    ///
    /// @throws DispatchException when the platform or host refuses or cannot
    ///                           be reached; [DispatchException#retryable()]
    ///                           is true only for platform trouble
    ///                           (`UNAVAILABLE`)
    String dispatch(DispatchJob job);
}
