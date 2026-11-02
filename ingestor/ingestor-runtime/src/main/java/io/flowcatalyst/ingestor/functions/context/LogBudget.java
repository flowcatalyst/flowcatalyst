package io.flowcatalyst.ingestor.functions.context;

import java.util.concurrent.atomic.AtomicLong;
import java.util.concurrent.atomic.LongAdder;
import java.util.function.LongSupplier;

/// How many log lines one function address may write: a token bucket of `burst` lines refilled at `rate` a second
/// (docs/ingestor-failure-pacing.md R6). One is shared by every version of an address and by every virtual thread
/// of every call, on each log call, so it is one atomic and no lock.
///
/// Mechanism: the generic cell rate algorithm, which is a token bucket kept as one number. `tat` is the time (in
/// nanoseconds) by which the bucket would be empty again if no more lines came; a line costs one interval
/// (`1/rate`) and is allowed while `tat` stays no more than `burst` intervals ahead of now. One compare-and-set
/// per line, no allocation.
///
/// Lines over the budget are dropped and counted. The count is reported by [#takeDropped], which the logger calls
/// on its log calls once a minute has passed since the last report (no timer thread: a function that floods and
/// then falls silent has its last minute reported at its next line).
public final class LogBudget {

    /// The most lines a second, and the most in a burst.
    public record Limits(int rate, int burst) {
        public static final Limits DEFAULTS = new Limits(100, 1000);

        public Limits {
            if (rate < 1) {
                throw new IllegalArgumentException("rate must be at least 1");
            }
            if (burst < 1) {
                throw new IllegalArgumentException("burst must be at least 1");
            }
        }
    }

    /// How often dropped lines are reported.
    static final long REPORT_EVERY_NANOS = 60_000_000_000L;

    private final Limits limits;
    private final long intervalNanos;
    private final long windowNanos;
    private final LongSupplier nanos;
    private final AtomicLong tat;
    private final AtomicLong nextReport;
    private final LongAdder dropped = new LongAdder();

    public LogBudget(Limits limits) {
        this(limits, System::nanoTime);
    }

    /// @param nanos a monotonic clock in nanoseconds (tests move it)
    LogBudget(Limits limits, LongSupplier nanos) {
        this.limits = limits;
        this.nanos = nanos;
        this.intervalNanos = Math.max(1, 1_000_000_000L / limits.rate());
        this.windowNanos = intervalNanos * limits.burst();
        long now = nanos.getAsLong();
        this.tat = new AtomicLong(now);
        this.nextReport = new AtomicLong(now + REPORT_EVERY_NANOS);
    }

    public Limits limits() {
        return limits;
    }

    /// Takes one line from the budget. `false`: over budget, the line is dropped and counted.
    public boolean tryAcquire() {
        long now = nanos.getAsLong();
        while (true) {
            long current = tat.get();
            long next = Math.max(current, now) + intervalNanos;
            if (next - now > windowNanos) {
                dropped.increment();
                return false;
            }
            if (tat.compareAndSet(current, next)) {
                return true;
            }
        }
    }

    /// The lines dropped since the last report, when a minute has passed since it; otherwise 0. Resets the count.
    public long takeDropped() {
        long now = nanos.getAsLong();
        long due = nextReport.get();
        if (now - due < 0 || !nextReport.compareAndSet(due, now + REPORT_EVERY_NANOS)) {
            return 0;
        }
        return dropped.sumThenReset();
    }
}
