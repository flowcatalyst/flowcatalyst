package io.flowcatalyst.ingestor.functions.context;

import java.time.Duration;
import java.util.Map;
import java.util.Objects;
import java.util.concurrent.ConcurrentHashMap;
import java.util.function.Function;
import java.util.function.LongSupplier;
import org.jspecify.annotations.Nullable;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/// The host's log budgets, one per function address ([LogBudget]), and the one timer that writes their "N log
/// lines dropped" summaries (docs/ingestor-failure-pacing.md R6; docs/ingestor-gap-fixes.md B3).
///
/// Once [#start]ed, a single virtual thread for the whole host (not one a function) wakes once a minute and writes,
/// for every function that dropped lines since its last summary, one WARN through the function's logger
/// (`fn.<address>`). So a function that floods and then falls silent still has its last minute reported. While the
/// timer runs a function's log calls write no summary of their own, so there is at most one a minute. Closing a
/// function writes what it has pending ([HostLogger#flushDropped]); [#close] stops the timer and writes what every
/// function has pending.
public final class LogBudgets implements AutoCloseable {

    private static final Logger LOG = LoggerFactory.getLogger(LogBudgets.class);

    /// The timer thread's name.
    static final String THREAD = "function-log-summaries";

    private final LogBudget.Limits limits;
    private final LongSupplier nanos;
    private final Function<String, Logger> loggers;
    private final Map<String, LogBudget> budgets = new ConcurrentHashMap<>();
    private final Object lock = new Object();
    private @Nullable Thread timer;
    private volatile boolean running;

    /// @param limits how many lines a second each function may log, and its burst
    public LogBudgets(LogBudget.Limits limits) {
        this(limits, System::nanoTime, LoggerFactory::getLogger);
    }

    /// @param nanos   a monotonic clock in nanoseconds for the budgets (tests move it)
    /// @param loggers the logger of a name (`fn.<address>`)
    LogBudgets(LogBudget.Limits limits, LongSupplier nanos, Function<String, Logger> loggers) {
        this.limits = Objects.requireNonNull(limits, "limits");
        this.nanos = Objects.requireNonNull(nanos, "nanos");
        this.loggers = Objects.requireNonNull(loggers, "loggers");
    }

    /// The budget of a function address, shared by all its versions.
    public LogBudget budget(String address) {
        return budgets.computeIfAbsent(address, a -> new LogBudget(limits, nanos, () -> running));
    }

    /// Starts the timer: once a minute it writes every pending summary. Starting it twice starts one.
    public void start() {
        start(Duration.ofNanos(LogBudget.REPORT_EVERY_NANOS));
    }

    /// @param every how often the timer writes (tests shorten it)
    void start(Duration every) {
        synchronized (lock) {
            if (timer != null) {
                return;
            }
            running = true;
            timer = Thread.ofVirtual().name(THREAD).start(() -> {
                while (running) {
                    try {
                        Thread.sleep(every);
                    } catch (InterruptedException e) {
                        return;
                    }
                    flush();
                }
            });
        }
    }

    /// Writes the summary of every function that dropped lines since its last one.
    public void flush() {
        for (Map.Entry<String, LogBudget> e : budgets.entrySet()) {
            try {
                HostLogger.summarise(loggers.apply("fn." + e.getKey()), e.getKey(), e.getValue().takeAll(), limits.rate());
            } catch (RuntimeException failure) {
                // A logger that fails must not end the timer for every other function.
                LOG.debug("the log summary of {} could not be written", e.getKey(), failure);
            }
        }
    }

    /// Stops the timer and writes what every function has pending.
    @Override
    public void close() {
        Thread stopping;
        synchronized (lock) {
            running = false;
            stopping = timer;
            timer = null;
        }
        if (stopping != null) {
            stopping.interrupt();
            try {
                stopping.join(Duration.ofSeconds(1));
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
            }
        }
        flush();
    }
}
