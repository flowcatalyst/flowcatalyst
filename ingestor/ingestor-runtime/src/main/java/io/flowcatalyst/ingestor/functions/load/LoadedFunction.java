package io.flowcatalyst.ingestor.functions.load;

import io.flowcatalyst.function.Function;
import io.flowcatalyst.function.FunctionAddress;
import io.flowcatalyst.function.FunctionContext;
import io.flowcatalyst.function.Request;
import io.flowcatalyst.function.Result;
import java.time.Duration;
import java.util.Objects;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.ExecutionException;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.TimeoutException;
import java.util.concurrent.atomic.AtomicInteger;
import org.jspecify.annotations.Nullable;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/// One loaded version of one function: its instance, the runtime resource
/// behind it, and enough bookkeeping to unload it safely. This is the one
/// object that keeps a reference to anything the runtime defined for this
/// version — closing it is what makes that collectable.
///
/// `resource` is whatever the version's loader must release on unload — a JVM
/// function's own `URLClassLoader`. When the resource IS a [ClassLoader], it is
/// set as the thread's context class loader for every call into the function.
///
/// **Concurrent (R3):** one instance serves every call at once. Nothing here is
/// locked on the invoke path: [#tryRetain]/[#release] are atomic increments and
/// decrements, and the drain that [#close] waits on is a future completed by
/// whichever call releases last. (javalin's version parked callers on a monitor;
/// the monitor was only entered by the last release, but "no `synchronized` on
/// the invoke path" is a requirement here, so it is not used.)
public final class LoadedFunction implements AutoCloseable {

    private static final Logger LOG = LoggerFactory.getLogger(LoadedFunction.class);
    private static final Duration DEFAULT_DRAIN_TIMEOUT = Duration.ofSeconds(30);

    private final Function function;
    private final AutoCloseable resource;
    private final FunctionAddress address;
    private final int version;
    private final AtomicInteger inFlight = new AtomicInteger();

    /// Completed when no call is in flight and the version is closing.
    private final CompletableFuture<Void> drained = new CompletableFuture<>();

    /// [#close] has begun: no new call may be retained. Calls already retained drain.
    private volatile boolean closing;

    /// [#close] has finished draining and stopped the function: [#invoke] is refused.
    private volatile boolean stopped;

    /// This version's [FunctionContext]: built and attached exactly once, by
    /// whoever loaded this instance (the reconciler), before [#init] runs — the
    /// SAME instance for every [#invoke] over this object's whole lifetime.
    /// `null` until [#attachContext] runs.
    private volatile @Nullable FunctionContext context;

    /// What the loader of this version wants to find again with it (the reconciler's record of the describe
    /// and settings it was loaded with); opaque here.
    private volatile @Nullable Object attachment;

    LoadedFunction(Function function, AutoCloseable resource, FunctionAddress address, int version) {
        this.function = Objects.requireNonNull(function, "function");
        this.resource = Objects.requireNonNull(resource, "resource");
        this.address = Objects.requireNonNull(address, "address");
        this.version = version;
    }

    public FunctionAddress address() {
        return address;
    }

    public int version() {
        return version;
    }

    /// Attaches this version's [FunctionContext] — called exactly once by the
    /// loader (the reconciler), before [#init]. [#close] releases it (via
    /// [AutoCloseable] if the context implements it) exactly once.
    ///
    /// @throws IllegalStateException a context is already attached
    public void attachContext(FunctionContext ctx) {
        Objects.requireNonNull(ctx, "ctx");
        if (context != null) {
            throw new IllegalStateException("a FunctionContext is already attached to " + address + "@" + version);
        }
        context = ctx;
    }

    /// This version's attached [FunctionContext], or `null` if [#attachContext] was never called.
    public @Nullable FunctionContext context() {
        return context;
    }

    /// Attaches the loader's own bookkeeping to this version.
    public void attach(Object attachment) {
        this.attachment = Objects.requireNonNull(attachment, "attachment");
    }

    /// What [#attach] stored, or `null`.
    public @Nullable Object attachment() {
        return attachment;
    }

    /// Whether [#close()] has finished: the loader's classes are then collectible
    /// and [#invoke] no longer accepted.
    public boolean isClosed() {
        return stopped;
    }

    /// Whether [#close()] has begun (new calls are refused from this moment).
    public boolean isClosing() {
        return closing;
    }

    /// The function's own class loader (JVM functions; `null` for any other
    /// resource) — package-private, for the leak test to build a `WeakReference` against.
    @Nullable ClassLoader loaderForTest() {
        return resource instanceof ClassLoader loader ? loader : null;
    }

    /// Calls in flight right now — test/diagnostic seam.
    public int inFlight() {
        return inFlight.get();
    }

    /// Marks one invocation as in flight — [#close()] waits for every
    /// retained call to [#release()] before it tears the loader down.
    public void retain() {
        inFlight.incrementAndGet();
    }

    /// [#retain()], unless [#close()] has begun: then nothing is retained and
    /// `false` is returned, so a call racing an unload is refused rather than lost.
    /// The increment comes first and the check second: [#close] sets its flag and
    /// then reads the count, so either it sees this call (and drains it) or this
    /// call sees the flag (and backs out).
    public boolean tryRetain() {
        inFlight.incrementAndGet();
        if (closing) {
            release();
            return false;
        }
        return true;
    }

    /// Matches a prior [#retain()].
    ///
    /// @throws IllegalStateException if called without a matching `retain()`
    public void release() {
        int remaining = inFlight.decrementAndGet();
        if (remaining < 0) {
            inFlight.incrementAndGet();
            throw new IllegalStateException("release() without a matching retain() on " + address + "@" + version);
        }
        if (remaining == 0 && closing) {
            drained.complete(null);
        }
    }

    /// Runs `ctx`'s function through one invocation with the calling
    /// thread's context class loader set to this function's loader for the
    /// duration of the call — restored in `finally`, so a throw restores it
    /// too.
    ///
    /// @throws IllegalStateException if this function has been [#close()]d
    public Result invoke(Request in, FunctionContext ctx) throws Exception {
        requireOpen();
        ClassLoader previous = enterFunctionLoader();
        try {
            return function.handle(in, ctx);
        } finally {
            Thread.currentThread().setContextClassLoader(previous);
        }
    }

    /// Runs `Function#init` with the same context-class-loader swap as
    /// [#invoke]. Called once, before this function is registered for
    /// invocation.
    public void init(FunctionContext ctx) throws Exception {
        requireOpen();
        ClassLoader previous = enterFunctionLoader();
        try {
            function.init(ctx);
        } finally {
            Thread.currentThread().setContextClassLoader(previous);
        }
    }

    /// Waits (bounded) for every in-flight [#invoke] call to [#release()],
    /// then runs `Function#stop` (with the same context-class-loader swap;
    /// exceptions are logged, never propagated) and closes the underlying
    /// runtime resource. Idempotent. After this returns, [#invoke] throws
    /// `IllegalStateException`.
    @Override
    public void close() {
        close(DEFAULT_DRAIN_TIMEOUT);
    }

    /// [#close()] with an explicit drain timeout — package-private, for
    /// tests that need the bound tighter than 30s.
    void close(Duration drainTimeout) {
        if (closing) {
            awaitStopped();
            return;
        }
        closing = true;
        if (inFlight.get() == 0) {
            drained.complete(null);
        }

        boolean ok = awaitDrain(drainTimeout);
        if (!ok) {
            LOG.atWarn()
                    .setMessage("closing loaded function before every invocation released: forcing close")
                    .addKeyValue("address", address.render())
                    .addKeyValue("version", version)
                    .addKeyValue("count", inFlight.get())
                    .log();
        }

        ClassLoader previous = enterFunctionLoader();
        try {
            function.stop();
        } catch (Exception e) {
            LOG.atWarn()
                    .setMessage("function stop() threw")
                    .addKeyValue("address", address.render())
                    .addKeyValue("version", version)
                    .setCause(e)
                    .log();
        }
        finally {
            Thread.currentThread().setContextClassLoader(previous);
        }
        stopped = true;

        try {
            resource.close();
        } catch (Exception e) {
            LOG.atWarn()
                    .setMessage("closing function runtime resource failed")
                    .addKeyValue("address", address.render())
                    .addKeyValue("version", version)
                    .setCause(e)
                    .log();
        }

        // Releases whatever this version's context holds (database pools) — exactly once,
        // regardless of how this close() was reached, and regardless of whether a context
        // was ever attached at all.
        if (context instanceof AutoCloseable closeable) {
            try {
                closeable.close();
            } catch (Exception e) {
                LOG.atWarn()
                        .setMessage("closing function context failed")
                        .addKeyValue("address", address.render())
                        .addKeyValue("version", version)
                        .setCause(e)
                        .log();
            }
        }
    }

    /// A second close() while the first is draining waits for it, so "close returned" always
    /// means "closed".
    private void awaitStopped() {
        while (!stopped) {
            try {
                Thread.sleep(5);
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                return;
            }
        }
    }

    private boolean awaitDrain(Duration timeout) {
        try {
            drained.get(Math.max(1, timeout.toMillis()), TimeUnit.MILLISECONDS);
            return true;
        } catch (TimeoutException e) {
            return false;
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
            return false;
        } catch (ExecutionException e) {
            return false;
        }
    }

    private void requireOpen() {
        if (stopped) {
            throw new IllegalStateException("LoadedFunction is closed: " + address + "@" + version);
        }
    }

    /// Sets the calling thread's context class loader to the function's loader (when the resource is
    /// one) and returns the one to restore.
    private ClassLoader enterFunctionLoader() {
        Thread thread = Thread.currentThread();
        ClassLoader previous = thread.getContextClassLoader();
        if (resource instanceof ClassLoader loader) {
            thread.setContextClassLoader(loader);
        }
        return previous;
    }
}
