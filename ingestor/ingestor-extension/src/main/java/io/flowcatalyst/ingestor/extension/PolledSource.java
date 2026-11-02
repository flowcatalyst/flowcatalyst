package io.flowcatalyst.ingestor.extension;

import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.RuntimeReport.Failure;
import io.flowcatalyst.framework.httpclient.HttpClientException;
import io.flowcatalyst.framework.httpclient.UnexpectedStatusException;
import java.time.Duration;
import java.time.Instant;
import java.time.temporal.ChronoUnit;
import java.util.concurrent.ScheduledExecutorService;
import java.util.concurrent.TimeUnit;
import org.apache.camel.Consumer;
import org.apache.camel.Exchange;
import org.apache.camel.Processor;
import org.apache.camel.Producer;
import org.apache.camel.support.DefaultConsumer;
import org.apache.camel.support.DefaultEndpoint;
import org.jspecify.annotations.Nullable;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/**
 * A source polled with code of its own: each poll may hand the route any number of payloads, each
 * split and dispatched like any other, and says when to poll again. A poll that throws is a {@code
 * SOURCE} failure, or {@code AUTH} when the source refused the login ({@link
 * #refusedLogin(Throwable)}); the ingestion is {@code FAILING} until a poll gets through. Polls run
 * one at a time, on one thread. The runtime reports the failures ({@link #reportTo(Reporter)}).
 *
 * <p>Failures are paced and logged for the extension (docs/ingestor-failure-pacing.md, R5):
 * <ul>
 *   <li>After a poll that succeeded the wait it returned is used, but never less than 250 ms.
 *   <li>A poll that threw, failed in part ({@link #partFailed}), or in which {@link
 *       Payloads#dispatch} returned false at least once is a failure. The next poll waits {@link
 *       Backoff#FAILURES} for the failures in a row, from the larger of {@link #retryDelay()} and
 *       one second, and not less than the wait the poll returned. A poll whose dispatch failed is
 *       not an exception: it has no stack trace in the log.
 *   <li>A refused login waits {@link Backoff#REFUSED_LOGIN} (5 minutes, doubling to an hour).
 *   <li>The first failure of a streak is logged in full, then a line a minute with a count
 *       ({@link FailureLog}).
 * </ul>
 */
public abstract class PolledSource extends DefaultEndpoint {

  /** How long the first refused login waits before it is tried again; each further one doubles it, up to an hour. */
  public static final Duration AUTH_RETRY = Duration.ofMinutes(5);

  private static final Logger LOG = LoggerFactory.getLogger(PolledSource.class);

  private static final Duration FIRST_POLL = Duration.ofSeconds(1);

  /** The ingestion polled. */
  protected final Ingestion ingestion;

  private final String kind;
  private @Nullable Reporter reporter;
  private @Nullable Duration authRetry;
  private boolean failedThisPoll;
  private boolean dispatchRefused;
  private PollPacing pacing = new PollPacing();
  private FailureLog failureLog = FailureLog.warn(LOG);

  /**
   * A source of one ingestion.
   *
   * @param context the ingestion and the runtime's Camel context
   * @param kind the kind's type, which names the endpoint and its thread
   */
  protected PolledSource(ExtensionContext context, String kind) {
    this.ingestion = context.ingestion();
    this.kind = kind;
    setCamelContext(context.camel());
  }

  /** Hands one payload to the route. */
  @FunctionalInterface
  public interface Payloads {
    /**
     * Splits and dispatches a payload.
     *
     * @param body the payload
     * @return whether its jobs were created; when not, the route has reported why, and the poll
     *     should not move on past it
     * @throws Exception when the route cannot be run at all
     */
    boolean dispatch(String body) throws Exception;

    /**
     * Splits and dispatches a payload, saying where it came from ({@link PayloadIdentity}), so its
     * jobs carry idempotency keys and read again it makes no new jobs.
     *
     * @param body the payload
     * @param identity where it came from, the same each time it is read; null or blank for none
     * @return whether its jobs were created, as {@link #dispatch(String)}
     * @throws Exception when the route cannot be run at all
     */
    default boolean dispatch(String body, @Nullable String identity) throws Exception {
      return dispatch(body);
    }
  }

  /** Where the runtime is told of failures; set before the route starts. */
  public interface Reporter {
    /**
     * A failure: the ingestion is {@code FAILING}.
     *
     * @param kind {@code SOURCE} or {@code AUTH}
     * @param message what failed; the runtime takes out what looks like a secret
     */
    void failed(Failure.Kind kind, String message);

    /** A poll got through without a failure. */
    void recovered();

    /**
     * A poll is about to start. The runtime counts it and, when an operator has switched tracing on
     * for the ingestion, records what the poll does. A default method, so a reporter written before
     * it keeps compiling.
     *
     * @return the poll's trace; never null, and {@link PollTrace#NONE} by default
     */
    default PollTrace pollStarting() {
      return PollTrace.NONE;
    }
  }

  /**
   * Polls the source once.
   *
   * @param payloads where its payloads go
   * @return how soon to poll again
   * @throws Exception when the source cannot be read; reported, and polled again after {@link
   *     #retryDelay()}
   */
  protected abstract Duration poll(Payloads payloads) throws Exception;

  /**
   * How soon to poll again after a poll that failed.
   *
   * @return the delay
   */
  protected abstract Duration retryDelay();

  /** Gives back what the source holds open; called when the route stops. */
  protected void release() {}

  /**
   * Where failures are reported; set by the runtime before the route starts.
   *
   * @param to the reporter
   */
  public final void reportTo(Reporter to) {
    this.reporter = to;
  }

  /**
   * How long a refused login waits, every time instead of {@link Backoff#REFUSED_LOGIN}; tests
   * shorten it.
   *
   * @param wait the wait
   */
  public final void authRetry(Duration wait) {
    this.authRetry = wait;
  }

  /**
   * The waits after failures and refused logins; tests shorten them. The one second a failed poll
   * waits at least and the 250 ms a poll that succeeded waits at least stay.
   *
   * @param failures the wait after failures in a row
   * @param refusedLogin the wait after refused logins in a row
   */
  public final void pacing(Backoff failures, Backoff refusedLogin) {
    this.pacing = new PollPacing(failures, refusedLogin);
  }

  /**
   * Where repeated failures are logged; tests capture them.
   *
   * @param log the failure log
   */
  public final void failureLog(FailureLog log) {
    this.failureLog = log;
  }

  /**
   * Reports a failure that does not end the poll, such as one asset of many that could not be
   * read. The ingestion is {@code FAILING} until a poll has none.
   *
   * @param message what failed, without a secret
   */
  protected final void partFailed(String message) {
    failedThisPoll = true;
    failure(Failure.Kind.SOURCE, message);
  }

  /**
   * Whether a failure is the source refusing the login rather than failing to answer: the framework
   * HTTP client's {@code AUTH} failure, or a 401 or 403.
   *
   * @param t the failure
   * @return true for a refused login
   */
  public static boolean refusedLogin(Throwable t) {
    return (t instanceof HttpClientException failed && failed.kind() == HttpClientException.Kind.AUTH)
        || (t instanceof UnexpectedStatusException status && (status.status() == 401 || status.status() == 403));
  }

  @Override
  protected String createEndpointUri() {
    return "ingest-" + kind + ":" + ingestion.id();
  }

  @Override
  public Producer createProducer() {
    throw new UnsupportedOperationException("an ingestion's source is only consumed from");
  }

  @Override
  public Consumer createConsumer(Processor processor) throws Exception {
    Poller consumer = new Poller(processor);
    configureConsumer(consumer);
    return consumer;
  }

  private void failure(Failure.Kind failureKind, String message) {
    Reporter r = reporter;
    if (r != null) {
      r.failed(failureKind, message);
    }
  }

  private final class Poller extends DefaultConsumer {

    private @Nullable ScheduledExecutorService executor;

    Poller(Processor processor) {
      super(PolledSource.this, processor);
    }

    @Override
    protected void doStart() throws Exception {
      super.doStart();
      ScheduledExecutorService started =
          getEndpoint().getCamelContext().getExecutorServiceManager().newSingleThreadScheduledExecutor(this, kind + "-" + ingestion.id());
      executor = started;
      started.schedule(this::run, FIRST_POLL.toMillis(), TimeUnit.MILLISECONDS);
    }

    @Override
    protected void doStop() throws Exception {
      ScheduledExecutorService running = executor;
      if (running != null) {
        getEndpoint().getCamelContext().getExecutorServiceManager().shutdownNow(running);
      }
      executor = null;
      release();
      super.doStop();
    }

    /**
     * One poll, and the next one scheduled whatever it threw: an extension's {@code Error} (a
     * missing class, a stack overflow) is a failure of that poll, not the end of the source.
     */
    private void run() {
      Duration next = FIRST_POLL;
      failedThisPoll = false;
      dispatchRefused = false;
      String pollKey = ingestion.id() + ":poll";
      PollTrace trace = starting();
      try {
        Duration returned = trace.run(() -> poll(payloads));
        Reporter r = reporter;
        if (r != null && !failedThisPoll) {
          r.recovered();
        }
        if (failedThisPoll || dispatchRefused) {
          // A payload the platform did not take is a failure for pacing, though nothing was thrown (R5).
          next = pacing.afterFailedPoll(returned, safeRetryDelay());
        } else {
          next = pacing.afterSuccess(returned);
          failureLog.recovered(pollKey, "ingestion " + ingestion.id() + " polls " + kind + " again");
          failureLog.recovered(ingestion.id() + ":login", "ingestion " + ingestion.id() + " is let in to " + kind + " again");
        }
        ended(trace, !failedThisPoll, failedThisPoll ? "a part of the poll failed" : null, next);
      } catch (InterruptedException e) {
        Thread.currentThread().interrupt();
        return;
      } catch (Throwable e) {
        String message = e.getMessage() == null ? e.getClass().getSimpleName() : e.getMessage();
        ended(trace, false, message, null);
        if (refusedLogin(e)) {
          Duration wait = pacing.afterRefusedLogin();
          Duration fixed = authRetry;
          next = fixed != null ? fixed : wait;
          Instant again = Instant.now().plus(next).truncatedTo(ChronoUnit.SECONDS);
          failureLog.failed(
              ingestion.id() + ":login", "ingestion " + ingestion.id() + ": the " + kind + " login was refused; trying again at " + again, null);
          failure(Failure.Kind.AUTH, message + "; tries again at " + again + ", or at once when the ingestion is saved");
        } else {
          next = pacing.afterError(safeRetryDelay());
          failureLog.failed(pollKey, "ingestion " + ingestion.id() + " could not poll " + kind + ": " + message, e);
          failure(Failure.Kind.SOURCE, message);
        }
        if (e instanceof OutOfMemoryError oom) {
          // Nothing is safe after this; the process should end (the image runs with -XX:+ExitOnOutOfMemoryError).
          schedule(next);
          throw oom;
        }
      }
      schedule(next);
    }

    /** The poll's trace; a runtime that cannot give one must not stop the poll. */
    private PollTrace starting() {
      Reporter r = reporter;
      if (r == null) {
        return PollTrace.NONE;
      }
      try {
        return r.pollStarting();
      } catch (RuntimeException e) {
        LOG.warn("ingestion {}: tracing a poll failed to start", ingestion.id(), e);
        return PollTrace.NONE;
      }
    }

    /** Tells the runtime how the poll ended; tracing never changes what the poll does next. */
    private void ended(PollTrace trace, boolean ok, @Nullable String error, @Nullable Duration next) {
      try {
        trace.finished(ok, error, next);
      } catch (RuntimeException e) {
        LOG.warn("ingestion {}: tracing a poll failed to finish", ingestion.id(), e);
      }
    }

    private Duration safeRetryDelay() {
      try {
        return retryDelay();
      } catch (RuntimeException e) {
        return AUTH_RETRY;
      }
    }

    private void schedule(Duration next) {
      ScheduledExecutorService running = executor;
      if (running != null && isRunAllowed() && !running.isShutdown()) {
        try {
          running.schedule(this::run, Math.max(next.toMillis(), 1), TimeUnit.MILLISECONDS);
        } catch (java.util.concurrent.RejectedExecutionException e) {
          // Stopping.
        }
      }
    }

    private final Payloads payloads =
        new Payloads() {
          @Override
          public boolean dispatch(String body) throws Exception {
            return dispatch(body, null);
          }

          @Override
          public boolean dispatch(String body, @Nullable String identity) throws Exception {
            Exchange exchange = createExchange(true);
            try {
              exchange.getMessage().setBody(body);
              if (identity != null) {
                PayloadIdentity.set(exchange, identity);
              }
              getProcessor().process(exchange);
              boolean taken = exchange.getException() == null;
              if (!taken) {
                dispatchRefused = true;
              }
              return taken;
            } finally {
              releaseExchange(exchange, false);
            }
          }
        };
  }
}
