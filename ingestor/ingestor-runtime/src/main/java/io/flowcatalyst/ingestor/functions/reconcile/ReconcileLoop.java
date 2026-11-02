package io.flowcatalyst.ingestor.functions.reconcile;

import io.flowcatalyst.ingestor.extension.Backoff;
import io.flowcatalyst.ingestor.extension.FailureLog;
import io.flowcatalyst.ingestor.functions.load.JvmFunctionLoader;
import java.time.Clock;
import java.time.Duration;
import java.util.Objects;
import java.util.concurrent.Semaphore;
import java.util.concurrent.TimeUnit;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/// Runs [Reconciler#reconcileOnce] on one virtual thread, forever, until [#close], and heartbeats on another.
/// Interruption is the stop signal: [#close] interrupts the loop's threads and joins them, bounded.
///
/// The platform's desired-state request is a long-poll (it answers as soon as the document changes, or `304` after
/// the wait), so a cycle follows the previous one at once; the wait is shortened to when a retry of a transient
/// load failure is due ([Reconciler#nextWake]). When the platform cannot be reached the loop backs off (1 s,
/// doubling to 30 s) and what is loaded keeps serving; when the platform refuses the runtime (401/403) it waits
/// [Backoff#REFUSED_LOGIN] (5 minutes doubling to an hour) instead. Whatever a cycle throws (short of a
/// `VirtualMachineError`) is logged and followed by the same backoff, never ends the loop. A heartbeat goes out every 15 s and as soon as a version's
/// state changes.
public final class ReconcileLoop implements AutoCloseable {

    private static final Logger LOG = LoggerFactory.getLogger(ReconcileLoop.class);

    /// The long-poll wait the loop asks the platform for.
    static final Duration LONG_POLL = Duration.ofSeconds(30);

    /// The heartbeat period.
    static final Duration HEARTBEAT_EVERY = Duration.ofSeconds(15);

    private static final Duration CLOSE_JOIN_TIMEOUT = Duration.ofSeconds(5);
    private static final Duration FIRST_BACKOFF = Duration.ofSeconds(1);
    private static final Duration MAX_BACKOFF = Duration.ofSeconds(30);

    /// Between two cycles that changed nothing, at least this: a platform that answers `304` at once (one that
    /// ignores `wait`) must not be polled in a tight loop.
    private static final Duration MIN_CYCLE = Duration.ofMillis(250);

    private final Reconciler reconciler;
    private final Clock clock;
    private final Duration longPoll;
    private final Duration heartbeatEvery;
    private final Duration firstBackoff;
    private final Thread reconcileThread;
    private final Thread heartbeatThread;
    private final Semaphore heartbeatNow = new Semaphore(0);
    private final FailureLog failures;
    private volatile boolean closed;

    public ReconcileLoop(Reconciler reconciler) {
        this(reconciler, Clock.systemUTC(), LONG_POLL, HEARTBEAT_EVERY, FIRST_BACKOFF);
    }

    /// Package-private: tests shorten the waits; production never calls it.
    ReconcileLoop(Reconciler reconciler, Clock clock, Duration longPoll, Duration heartbeatEvery, Duration firstBackoff) {
        this(reconciler, clock, longPoll, heartbeatEvery, firstBackoff, new FailureLog((message, cause) -> {
            if (cause == null) {
                LOG.warn(message);
            } else {
                LOG.warn(message, cause);
            }
        }, Duration.ofMinutes(1), clock));
    }

    ReconcileLoop(Reconciler reconciler, Clock clock, Duration longPoll, Duration heartbeatEvery, Duration firstBackoff,
            FailureLog failures) {
        this.failures = Objects.requireNonNull(failures, "failures");
        this.reconciler = Objects.requireNonNull(reconciler, "reconciler");
        this.clock = Objects.requireNonNull(clock, "clock");
        this.longPoll = longPoll;
        this.heartbeatEvery = heartbeatEvery;
        this.firstBackoff = firstBackoff;
        this.reconcileThread = Thread.ofVirtual().name("fn-host-reconcile-loop").unstarted(this::runReconcile);
        this.heartbeatThread = Thread.ofVirtual().name("fn-host-heartbeat-loop").unstarted(this::runHeartbeat);
        reconciler.addChangeListener(() -> heartbeatNow.release());
    }

    /// Starts the loop's virtual threads. Not idempotent — call once.
    public void start() {
        reconcileThread.start();
        heartbeatThread.start();
    }

    /// Whether the reconcile thread is running (`/readyz` reads this: a loop that died stops desired state moving).
    public boolean isAlive() {
        return reconcileThread.isAlive();
    }

    private void runReconcile() {
        Duration backoff = firstBackoff;
        int refused = 0;
        while (!closed) {
            Duration pause = MIN_CYCLE;
            try {
                java.time.Instant now = clock.instant();
                Reconciler.Outcome outcome = reconciler.reconcileOnce(now, reconciler.nextWake(now, longPoll));
                switch (outcome) {
                    case CHANGED -> {
                        backoff = firstBackoff;
                        refused = 0;
                        pause = Duration.ZERO;
                    }
                    case NOT_MODIFIED -> {
                        backoff = firstBackoff;
                        refused = 0;
                    }
                    case OUTAGE -> {
                        refused = 0;
                        pause = backoff;
                        backoff = next(backoff);
                    }
                    case REFUSED -> {
                        pause = Backoff.REFUSED_LOGIN.delay(++refused);
                        backoff = firstBackoff;
                    }
                }
                failures.recovered("reconcile:cycle");
            } catch (VirtualMachineError e) {
                // A metaspace-family OutOfMemoryError — found by the cause-chain walker the per-load fence uses
                // — outside any one function's own try/catch (parsing a response, say): the loop continues.
                // Any OTHER VirtualMachineError (heap OutOfMemoryError, StackOverflowError, InternalError) is a
                // real emergency: rethrown, which ends the loop, and isAlive() says so.
                if (JvmFunctionLoader.findMetaspaceOom(e) == null) {
                    throw e;
                }
                failures.failed("reconcile:cycle", "function reconcile run hit a metaspace OutOfMemoryError; continuing", null);
                pause = backoff;
                backoff = next(backoff);
            } catch (Throwable e) {
                // Anything else a cycle throws (a LinkageError out of a function's code, a bug) is logged — in full
                // once, then a line a minute — and followed by the outage backoff, then the next cycle.
                failures.failed("reconcile:cycle", "reconcile run failed; continuing: " + e.getClass().getName(), e);
                pause = backoff;
                backoff = next(backoff);
            }
            if (!pause.isZero() && !sleep(pause)) {
                return;
            }
        }
    }

    private static Duration next(Duration backoff) {
        Duration doubled = backoff.multipliedBy(2);
        return doubled.compareTo(MAX_BACKOFF) > 0 ? MAX_BACKOFF : doubled;
    }

    private void runHeartbeat() {
        while (!closed) {
            try {
                heartbeatNow.tryAcquire(heartbeatEvery.toMillis(), TimeUnit.MILLISECONDS);
                heartbeatNow.drainPermits();
                if (closed) {
                    return;
                }
                reconciler.sendHeartbeat();
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                return;
            } catch (RuntimeException e) {
                failures.failed("reconcile:heartbeat", "heartbeat run failed; continuing: " + e.getClass().getName(), e);
            }
        }
    }

    private boolean sleep(Duration d) {
        try {
            Thread.sleep(d);
            return !closed;
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
            return false;
        }
    }

    /// Interrupts the loop's threads (unblocking a cycle stuck in the control plane) and joins them, bounded.
    @Override
    public void close() {
        closed = true;
        reconcileThread.interrupt();
        heartbeatThread.interrupt();
        try {
            reconcileThread.join(CLOSE_JOIN_TIMEOUT);
            heartbeatThread.join(CLOSE_JOIN_TIMEOUT);
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
        }
    }
}
