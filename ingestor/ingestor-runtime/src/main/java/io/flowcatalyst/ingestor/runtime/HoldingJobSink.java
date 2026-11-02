package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.JobRecord;
import io.flowcatalyst.ingestor.contract.JobsAnswer;
import io.flowcatalyst.ingestor.extension.Backoff;
import io.flowcatalyst.ingestor.extension.FailureLog;
import java.time.Duration;
import java.time.Instant;
import java.time.InstantSource;
import java.util.List;
import java.util.Objects;
import java.util.function.Consumer;
import org.jspecify.annotations.Nullable;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/**
 * A runtime-wide breaker in front of the platform's jobs endpoint (docs/ingestor-failure-pacing.md,
 * R2). When a batch fails after its attempts with the platform's failure ({@link
 * PlatformUnavailableException}: no answer, 5xx, 429, 401 or 403 after the token was renewed), every
 * ingestion's submit is held until {@code now + Backoff.FAILURES.delay(n)} for {@code n} failures in
 * a row, or until the platform's {@code Retry-After} when it sent a longer one (at most 15
 * minutes). While held a submit fails at once with {@link PlatformHeldException}, which the route
 * turns into a {@link DispatchFailedException}, so the source keeps its payload and R1 paces the
 * source. The first submit after the hold is a probe: the others stay held until it ends. A probe
 * that succeeds, or that is answered with a failure that is the ingestion's (a 409, a refusal),
 * closes the breaker: the platform answered. A failure that is the ingestion's never opens it.
 *
 * <p>Mechanism: the route asks {@link #admit()} before each attempt and tells {@link #settled} how
 * the batch ended, because the breaker opens after the route's own attempts (which stay, for a single
 * blip), and the sink alone cannot tell attempts from batches.
 *
 * <p>Logging: the first failure that opens it is a WARN ("holding every ingestion until ..."), then
 * a line a minute through the {@link FailureLog} while it stays open, and one INFO when it closes.
 */
final class HoldingJobSink implements JobSink {

  private static final Logger LOG = LoggerFactory.getLogger(HoldingJobSink.class);

  /** The longest hold a {@code Retry-After} may ask for. */
  static final Duration LONGEST_RETRY_AFTER = Duration.ofMinutes(15);

  /** How long a probe may take before another caller is let through in its place. */
  private static final Duration PROBE_LIMIT = Duration.ofMinutes(2);

  private static final String KEY = "platform:jobs";

  private final JobSink delegate;
  private final InstantSource clock;
  private final Backoff backoff;
  private final FailureLog failures;
  private final Consumer<String> closed;

  // All guarded by this.
  private boolean open;
  private Instant until = Instant.MIN;
  private boolean probing;
  private Instant probeStarted = Instant.MIN;
  private int streak;

  HoldingJobSink(JobSink delegate) {
    this(delegate, InstantSource.system(), Backoff.FAILURES, Pacing.system().failures(), LOG::info);
  }

  /**
   * Creates the breaker.
   *
   * @param delegate the sink that calls the platform
   * @param clock the time
   * @param backoff the hold after failures in a row
   * @param failures where the open and still-open lines go
   * @param closed told the line to log when the breaker closes (INFO)
   */
  HoldingJobSink(JobSink delegate, InstantSource clock, Backoff backoff, FailureLog failures, Consumer<String> closed) {
    this.delegate = Objects.requireNonNull(delegate, "delegate");
    this.clock = Objects.requireNonNull(clock, "clock");
    this.backoff = Objects.requireNonNull(backoff, "backoff");
    this.failures = Objects.requireNonNull(failures, "failures");
    this.closed = Objects.requireNonNull(closed, "closed");
  }

  @Override
  public void create(Ingestion ingestion, List<JobRecord> records) throws Exception {
    delegate.create(ingestion, records);
  }

  @Override
  public JobsAnswer submit(Ingestion ingestion, List<JobRecord> records) throws Exception {
    return delegate.submit(ingestion, records);
  }

  @Override
  public void admit() throws java.io.IOException {
    Instant now = clock.instant();
    String held;
    synchronized (this) {
      if (!open) {
        return;
      }
      if (!now.isBefore(until) && (!probing || now.isAfter(probeStarted.plus(PROBE_LIMIT)))) {
        probing = true;
        probeStarted = now;
        return;
      }
      held = "the platform is not taking jobs; held until " + (now.isBefore(until) ? until : "the probe ends");
    }
    failures.failed(KEY, "the platform is still not taking jobs; holding every ingestion: " + held, null);
    throw new PlatformHeldException(held);
  }

  @Override
  public void settled(@Nullable Exception failure) {
    Instant now = clock.instant();
    String openedLine = null;
    String closedLine = null;
    synchronized (this) {
      if (failure instanceof PlatformUnavailableException down) {
        if (open && !probing) {
          // A batch that began before it opened has failed too: the hold stands and is not lengthened.
          return;
        }
        streak++;
        Duration hold = backoff.delay(streak);
        Duration asked = down.retryAfter();
        if (asked != null && asked.compareTo(hold) > 0) {
          hold = asked.compareTo(LONGEST_RETRY_AFTER) > 0 ? LONGEST_RETRY_AFTER : asked;
        }
        open = true;
        probing = false;
        until = now.plus(hold);
        openedLine = "the platform is not taking jobs (" + down.getMessage() + "); holding every ingestion until " + until;
      } else if (open) {
        // The platform answered, or took the batch: it is up.
        open = false;
        probing = false;
        streak = 0;
        closedLine = "the platform takes jobs again; ingestions are no longer held";
      } else {
        streak = 0;
      }
    }
    if (openedLine != null) {
      failures.failed(KEY, openedLine, null);
    }
    if (closedLine != null) {
      failures.recovered(KEY);
      closed.accept(closedLine);
    }
  }
}
