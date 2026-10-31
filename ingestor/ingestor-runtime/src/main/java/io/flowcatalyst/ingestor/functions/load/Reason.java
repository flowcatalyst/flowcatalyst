package io.flowcatalyst.ingestor.functions.load;

/// Why a load was refused. A closed set — a refusal is routine (it becomes a
/// heartbeat entry), never a reason to invent a new case without updating every
/// switch over it. [#transientReason()] says which of them the host retries on its
/// own (heartbeat `RETRYING`) and which are permanent (heartbeat `FAILED`): the platform
/// latches the first `FAILED` report for a version, so a condition that can pass must
/// never be reported as one.
public enum Reason {

    /// The jar contains a `.so`/`.dll`/`.dylib`/`.jnilib` entry: one class
    /// loader per JVM may load a given native library, so a second function
    /// bundling the same one would get `UnsatisfiedLinkError`.
    NATIVE_LIBRARY(false),

    /// The jar registers a `java.security.Provider` service: a second
    /// provider under one JVM-wide name is silently ignored, so one
    /// function would run another's provider.
    SECURITY_PROVIDER(false),

    /// The jar contains a class in package `io.flowcatalyst.function` — a
    /// shaded copy of the API. It would never be loaded (the parent loader
    /// wins that package unconditionally), but it signals a build that
    /// forgot to mark the API `provided`.
    BUNDLES_API(false),

    /// The describe's entrypoint class was not found in the jar.
    ENTRYPOINT_NOT_FOUND(false),

    /// The entrypoint class does not implement `io.flowcatalyst.function.Function`
    /// as the host's own copy of that interface.
    ENTRYPOINT_NOT_A_FUNCTION(false),

    /// The entrypoint class has no public no-arg constructor, or
    /// constructing it threw.
    ENTRYPOINT_NOT_INSTANTIABLE(false),

    /// The jar file itself could not be opened or read.
    UNREADABLE_JAR(false),

    /// Class loading, entrypoint construction, or `init` hit the metaspace
    /// fence: a catchable `OutOfMemoryError` naming Metaspace or Compressed
    /// class space. This ONE version fails to load; the fence itself, and every
    /// other function, is untouched — a Java-heap `OutOfMemoryError` is never
    /// this reason (it is not caught at all). This is the BACKSTOP:
    /// [MetaspaceGuard] is meant to refuse a load before it ever gets this
    /// close, so a healthy host should see [#METASPACE_HEADROOM] far more often.
    /// Retried: unloading other functions frees space.
    OUT_OF_METASPACE(true),

    /// [MetaspaceGuard] refused the load WITHOUT ATTEMPTING IT: free
    /// metaspace was already below the reserve before class loading even
    /// began — an ordinary, routine load failure, distinct from
    /// [#OUT_OF_METASPACE] (which means an actual `OutOfMemoryError` was
    /// caught). The old version (if any) keeps serving; the next reconcile
    /// cycle re-checks.
    METASPACE_HEADROOM(true);

    private final boolean transientReason;

    Reason(boolean transientReason) {
        this.transientReason = transientReason;
    }

    /// Whether the condition can pass without the jar changing, so the load is retried and reported `RETRYING`.
    public boolean transientReason() {
        return transientReason;
    }
}
