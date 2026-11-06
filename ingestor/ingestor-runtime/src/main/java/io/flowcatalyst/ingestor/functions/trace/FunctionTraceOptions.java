package io.flowcatalyst.ingestor.functions.trace;

import java.time.Duration;
import java.util.function.Function;
import org.jspecify.annotations.Nullable;

/// How function traces are queued and shipped. The three settings an operator may change are environment variables, as
/// the Go runner's are: `FC_FN_TRACE_QUEUE` (records queued), `FC_FN_TRACE_QUEUE_BYTES` (bytes of JSON queued) and
/// `FC_FN_TRACE_BODY_BYTES` (the most bytes of a body an `http` record keeps).
///
/// @param queueRecords the most records queued; when full the oldest of the function with the most queued is dropped
/// @param queueBytes   the most bytes of JSON queued
/// @param bodyBytes    the most bytes of each body an `http` record keeps
/// @param flushRecords queued records that make a batch go at once
/// @param flushEvery   the longest the oldest record waits for a batch to go
/// @param firstBackoff the first pause before a batch the platform did not take is tried again
/// @param maxBackoff   the longest such pause
public record FunctionTraceOptions(
        int queueRecords,
        long queueBytes,
        int bodyBytes,
        int flushRecords,
        Duration flushEvery,
        Duration firstBackoff,
        Duration maxBackoff) {

    public static final int DEFAULT_QUEUE = 5000;
    public static final long DEFAULT_QUEUE_BYTES = 32L << 20;
    public static final long MIN_QUEUE_BYTES = 1L << 20;

    public static final String ENV_QUEUE = "FC_FN_TRACE_QUEUE";
    public static final String ENV_QUEUE_BYTES = "FC_FN_TRACE_QUEUE_BYTES";
    public static final String ENV_BODY_BYTES = "FC_FN_TRACE_BODY_BYTES";

    /// The defaults: 5000 records, 32 MiB, bodies of 64 KiB, a batch at 200 records or every 2 seconds.
    public static final FunctionTraceOptions DEFAULTS = new FunctionTraceOptions(
            DEFAULT_QUEUE, DEFAULT_QUEUE_BYTES, FunctionTraces.DEFAULT_BODY_BYTES, 200, Duration.ofSeconds(2),
            Duration.ofMillis(500), Duration.ofSeconds(15));

    /// The defaults with the three environment settings applied; a value that is not a number, or is out of range, is
    /// ignored.
    ///
    /// @param env the environment, by variable name; `null` for a variable that is not set
    public static FunctionTraceOptions fromEnvironment(Function<String, @Nullable String> env) {
        int queue = (int) number(env.apply(ENV_QUEUE), 1, Integer.MAX_VALUE, DEFAULTS.queueRecords());
        long bytes = number(env.apply(ENV_QUEUE_BYTES), MIN_QUEUE_BYTES, Long.MAX_VALUE, DEFAULTS.queueBytes());
        int body = (int) number(env.apply(ENV_BODY_BYTES), 1, 1 << 20, DEFAULTS.bodyBytes());
        return new FunctionTraceOptions(queue, bytes, body, DEFAULTS.flushRecords(), DEFAULTS.flushEvery(), DEFAULTS.firstBackoff(),
                DEFAULTS.maxBackoff());
    }

    private static long number(@Nullable String text, long min, long max, long fallback) {
        if (text == null || text.isBlank()) {
            return fallback;
        }
        try {
            long value = Long.parseLong(text.strip());
            return value < min || value > max ? fallback : value;
        } catch (NumberFormatException e) {
            return fallback;
        }
    }
}
