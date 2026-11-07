package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.RuntimeReport.Failure;
import io.flowcatalyst.ingestor.contract.Source;
import io.flowcatalyst.ingestor.extension.FailureLog;
import java.time.Duration;
import java.util.Locale;
import org.apache.camel.Consumer;
import org.apache.camel.Endpoint;
import org.apache.camel.Exchange;
import org.apache.camel.spi.ExceptionHandler;
import org.apache.camel.spi.PollingConsumerPollStrategy;
import org.jspecify.annotations.Nullable;

/**
 * A polled source's failures, kept for the platform: a poll that throws (no connection, a
 * login refused, an error answer) is a {@code SOURCE} failure, or {@code AUTH} for a refused login,
 * and makes the route {@code FAILING} until a poll succeeds. A payload that was read and dispatched
 * but could not be marked done (a file not moved, an object not deleted) is a {@code SOURCE} failure
 * too, and the route is {@code FAILING} (R4). Before each poll the source's hosts are checked again
 * ({@link OutboundGuard}); a refused one skips the poll and is reported the same way.
 *
 * <p>Failures are paced by the ingestion's {@link PollPacer}: while the wait after failures in a row
 * is running, {@link #begin} skips the poll. They are logged through a {@link FailureLog}, the first
 * of a streak in full and then a line a minute (R3).
 *
 * <p>It is also the endpoint's exception handler (and a file endpoint's on-completion handler), so
 * that Camel does not log a second stack trace for what is reported here (F12), and so that the
 * failure of a move, a delete or a mark after a good read reaches {@link #commitFailed}.
 */
final class ReportingPollStrategy implements PollingConsumerPollStrategy, ExceptionHandler {

  private final Ingestion ingestion;
  private final ExchangeCounters counters;
  private final SourceHealth health;
  private final Runnable guard;
  private final PollPacer pacer;
  private final FailureLog failures;
  private final Duration interval;
  private final String pollKey;
  private final String commitKey;
  /** The failure {@link #rollback} reported last, so the consumer's exception handler does not report it again. */
  private volatile @Nullable Throwable reported;

  ReportingPollStrategy(Ingestion ingestion, ExchangeCounters counters, SourceHealth health) {
    this(ingestion, counters, health, () -> {});
  }

  ReportingPollStrategy(Ingestion ingestion, ExchangeCounters counters, SourceHealth health, Runnable guard) {
    this(ingestion, counters, health, guard, Pacing.system());
  }

  ReportingPollStrategy(Ingestion ingestion, ExchangeCounters counters, SourceHealth health, Runnable guard, Pacing pacing) {
    this(ingestion, counters, health, guard, pacing.pacer(), pacing.failures());
  }

  ReportingPollStrategy(
      Ingestion ingestion, ExchangeCounters counters, SourceHealth health, Runnable guard, PollPacer pacer, FailureLog failures) {
    this.ingestion = ingestion;
    this.counters = counters;
    this.health = health;
    this.guard = guard;
    this.pacer = pacer;
    this.failures = failures;
    this.interval = PollPacer.interval(ingestion);
    this.pollKey = ingestion.id() + ":poll";
    this.commitKey = ingestion.id() + ":commit";
  }

  @Override
  public boolean begin(Consumer consumer, Endpoint endpoint) {
    if (!pacer.due()) {
      // Failures in a row: the wait is not over. The consumer's own timer goes on and this costs a call.
      return false;
    }
    pacer.started();
    try {
      guard.run();
      return true;
    } catch (UnsupportedIngestionException e) {
      String message = ExchangeCounters.clean(e.getMessage());
      failures.failed(pollKey, "ingestion " + ingestion.id() + " did not poll its source: " + message, null);
      counters.failure(ingestion, Failure.Kind.SOURCE, message, null, null);
      health.failing(ingestion, message);
      pacer.sourceFailed();
      pacer.ended(interval);
      return false;
    }
  }

  @Override
  public void commit(Consumer consumer, Endpoint endpoint, int polledMessages) {
    counters.polled(ingestion);
    pacer.ended(interval);
    if (!pacer.lastFailed()) {
      health.recovered(ingestion);
      failures.recovered(pollKey, "ingestion " + ingestion.id() + " polls its source again");
      failures.recovered(commitKey, "ingestion " + ingestion.id() + " marks what it has read again");
      failures.recovered(ingestion.id() + ":dispatch", "ingestion " + ingestion.id() + " dispatches what it reads again");
    } else if (!pacer.lastSourceFailed()) {
      // Only the platform did not take a payload: the source answered, and the route has reported it.
      health.recovered(ingestion);
    }
  }

  @Override
  public boolean rollback(Consumer consumer, Endpoint endpoint, int retryCounter, Exception cause) {
    counters.polled(ingestion);
    reported = cause;
    pacer.sourceFailed();
    if (cause instanceof CommitFailedException) {
      commitFailed(cause.getMessage(), cause);
    } else {
      String message = ExchangeCounters.clean(cause.getMessage() == null ? cause.getClass().getSimpleName() : cause.getMessage());
      boolean refused = ingestion.source() instanceof Source.Sftp && refusedLogin(cause);
      failures.failed(pollKey, "ingestion " + ingestion.id() + " could not poll its source: " + message, cause);
      counters.failure(ingestion, refused ? Failure.Kind.AUTH : Failure.Kind.SOURCE, message, null, null);
      health.failing(ingestion, message);
    }
    pacer.ended(interval);
    return false;
  }

  /**
   * A payload was read and dispatched, but the source could not be told so: a failure of kind
   * {@code SOURCE}, the ingestion is {@code FAILING}, and the poll counts as failed for pacing.
   */
  void commitFailed(@Nullable String message, Throwable cause) {
    String clean = ExchangeCounters.clean(message == null ? CommitFailedException.message(done(), cause) : message);
    failures.failed(commitKey, "ingestion " + ingestion.id() + ": " + clean, cause);
    counters.failure(ingestion, Failure.Kind.SOURCE, clean, null, null);
    health.failing(ingestion, clean);
    pacer.sourceFailed();
  }

  /** What the source does to a payload once it is dispatched, for the text of a failure. */
  private String done() {
    return switch (ingestion.source()) {
      case Source.Sftp _, Source.S3 _ -> "moved or deleted";
      case Source.DatabasePoll _ -> "marked as read";
      case Source.Sqs _ -> "deleted from the queue";
      default -> "marked as done";
    };
  }

  /** Whether a failure is the server refusing the login (the SSH library says "Auth fail" or "Auth cancel"). */
  static boolean refusedLogin(Throwable cause) {
    for (Throwable t = cause; t != null; t = t.getCause()) {
      String m = t.getMessage();
      if (m != null) {
        String lower = m.toLowerCase(Locale.ROOT);
        if (lower.contains("auth fail") || lower.contains("auth cancel") || lower.contains("authentication failed")) {
          return true;
        }
      }
    }
    return false;
  }

  // The consumer's exception handler. A poll that threw was reported by rollback() before Camel calls
  // this, an exchange the route failed was reported by the route, and what is left is a step after a
  // good exchange (a commit), an exchange that failed before the route began it, or something outside
  // all of them.

  @Override
  public void handleException(Throwable exception) {
    handleException("", null, exception);
  }

  @Override
  public void handleException(@Nullable String message, Throwable exception) {
    handleException(message, null, exception);
  }

  @Override
  @SuppressWarnings("ReferenceEquality") // the very exception rollback() reported, not an equal one
  public void handleException(@Nullable String message, @Nullable Exchange exchange, Throwable exception) {
    if (exception == reported) {
      return;
    }
    if (exchange != null && exchange.isFailed()) {
      if (exchange.getProperty(IngestionRoute.PENDING) == null) {
        // It failed before the route began it, so the route did not report it; the component's own line
        // for a failed exchange is turned down (simplelogger.properties), so it is reported here.
        reported = exception;
        String text = ExchangeCounters.clean(exception.getMessage() == null ? exception.getClass().getSimpleName() : exception.getMessage());
        failures.failed(pollKey, "ingestion " + ingestion.id() + ": a payload failed and the source keeps it: " + text, exception);
        counters.failure(ingestion, Failure.Kind.SOURCE, text, null, null);
        pacer.sourceFailed();
      }
      // Else the route reported the exchange; a failing rollback after it adds nothing.
      return;
    }
    if (exchange != null) {
      commitFailed(null, exception);
      return;
    }
    String text = ExchangeCounters.clean(exception.getMessage() == null ? exception.getClass().getSimpleName() : exception.getMessage());
    failures.failed(pollKey, "ingestion " + ingestion.id() + ": " + text, exception);
    counters.failure(ingestion, Failure.Kind.SOURCE, text, null, null);
    health.failing(ingestion, text);
    pacer.sourceFailed();
  }
}
