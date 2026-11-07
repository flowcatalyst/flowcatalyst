package io.flowcatalyst.ingestor.extension;

import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.RuntimeReport.Failure;
import io.flowcatalyst.framework.httpclient.HttpClientException;
import io.flowcatalyst.framework.httpclient.UnexpectedStatusException;
import java.time.Duration;
import java.time.Instant;
import java.time.temporal.ChronoUnit;
import java.util.concurrent.ScheduledExecutorService;
import java.util.concurrent.ScheduledFuture;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.locks.Condition;
import java.util.concurrent.locks.ReentrantLock;
import org.apache.camel.Consumer;
import org.apache.camel.Exchange;
import org.apache.camel.Processor;
import org.apache.camel.Producer;
import org.apache.camel.Suspendable;
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
 *
 * <p>When its route stops, or is suspended (the lease is ending, the ingestion moved, the runtime is
 * shutting down), no further poll is scheduled and the poll in progress is not interrupted: it is
 * asked to stop. {@link Payloads#stopping()} is then true and {@link Payloads#dispatch} returns
 * false at once without running the route, which is not a failure, so a poll that does not move past
 * a payload not taken ends cleanly at its next dispatch; a payload already taken away from the
 * source by reading it goes through {@link Payloads#dispatchTaken}, which runs the route all the
 * same. The runtime waits for it ({@link
 * #polling()}) within the stop's bound; a poll still running when the route is stopped is given
 * half a second more, then interrupted.
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
   * How long a poll still running when the route is stopped is waited for before it is interrupted.
   * The runtime has already waited for it, within the stop's bound, before it stops the route.
   */
  static final Duration STOP_WAIT = Duration.ofMillis(500);

  /** Set while a poll runs ({@link #polling()}). */
  private volatile boolean polling;

  /** Set while the source is asked to stop: suspended, or its route stopping ({@link Payloads#stopping()}). */
  private volatile boolean stopping;

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

    /**
     * Splits and dispatches a payload the source will not serve again: one it has already taken
     * away by reading it (a queue that empties as it serves). Unlike {@link #dispatch(String,
     * String)} it runs the route even while the source is {@linkplain #stopping() stopping}, so
     * what was read before the stop came is not lost; the runtime waits for it, within the stop's
     * bound, before it counts the route as stopped. A false is the platform's answer (its jobs were
     * not created; the route has reported why), never the stop's. Check {@link #stopping()} before
     * each such read, and once it is true, dispatch what has been read this way and return.
     *
     * <p>A default method that calls {@link #dispatch(String, String)}, so a {@code Payloads}
     * written before it keeps compiling. An extension that calls it needs a runtime that has it.
     *
     * @param body the payload
     * @param identity where it came from ({@link PayloadIdentity}); null or blank for none
     * @return whether its jobs were created
     * @throws Exception when the route cannot be run at all
     */
    default boolean dispatchTaken(String body, @Nullable String identity) throws Exception {
      return dispatch(body, identity);
    }

    /**
     * Whether the source has been asked to stop: its route is being stopped, or suspended (the lease
     * is ending, the ingestion moved, the runtime is shutting down). From then on {@link
     * #dispatch} returns false at once without running the route, and that is not a failure; the
     * poll should end at its next dispatch, its cursor not moved past the payload not taken. A poll
     * with costly work before its first dispatch (many calls to make) can ask this first and skip
     * it. False by default, so a {@code Payloads} written before it keeps compiling.
     *
     * @return true once the source is asked to stop
     */
    default boolean stopping() {
      return false;
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

  /**
   * Whether a poll is running: from just before {@link #poll} is called until it has returned or
   * thrown. The runtime reads it, without any lock, to know whether anything of the source is still
   * running before it gives the ingestion's lease back.
   *
   * @return true while a poll is running
   */
  public final boolean polling() {
    return polling;
  }

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
    if (stopping) {
      // Most likely the stop itself (a payload not taken because the source is stopping): not a failure.
      LOG.debug("ingestion {}: a part of the poll failed while the source is stopping: {}", ingestion.id(), message);
      return;
    }
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

  /**
   * Polls on a thread of its own. Suspended, it schedules no further poll and asks the poll in
   * progress to stop ({@link Payloads#stopping()}) without interrupting it; resumed, it polls again.
   * Stopped, it does the same, waits {@link #STOP_WAIT} for the poll in progress to end, then
   * interrupts it.
   */
  private final class Poller extends DefaultConsumer implements Suspendable {

    private @Nullable ScheduledExecutorService executor;
    private @Nullable ScheduledFuture<?> scheduled;
    /** Orders a poll's start and end against suspend, resume and stop. */
    private final ReentrantLock turn = new ReentrantLock();
    private final Condition pollEnded = turn.newCondition();

    Poller(Processor processor) {
      super(PolledSource.this, processor);
    }

    @Override
    protected void doStart() throws Exception {
      super.doStart();
      ScheduledExecutorService started =
          getEndpoint().getCamelContext().getExecutorServiceManager().newSingleThreadScheduledExecutor(this, kind + "-" + ingestion.id());
      turn.lock();
      try {
        executor = started;
        stopping = false;
        schedule(FIRST_POLL);
      } finally {
        turn.unlock();
      }
    }

    @Override
    protected void doSuspend() {
      turn.lock();
      try {
        stopping = true;
        cancelScheduled();
      } finally {
        turn.unlock();
      }
    }

    @Override
    protected void doResume() {
      turn.lock();
      try {
        stopping = false;
        if (!polling) {
          // A poll still running schedules the next when it ends.
          schedule(Duration.ZERO);
        }
      } finally {
        turn.unlock();
      }
    }

    @Override
    protected void doStop() throws Exception {
      ScheduledExecutorService running;
      turn.lock();
      try {
        stopping = true;
        cancelScheduled();
        long left = STOP_WAIT.toNanos();
        while (polling && left > 0) {
          left = pollEnded.awaitNanos(left);
        }
        if (polling) {
          LOG.debug("ingestion {}: the poll in progress has not ended within {}; interrupting it", ingestion.id(), STOP_WAIT);
        }
        running = executor;
        executor = null;
      } finally {
        turn.unlock();
      }
      if (running != null) {
        getEndpoint().getCamelContext().getExecutorServiceManager().shutdownNow(running);
      }
      release();
      super.doStop();
    }

    /** Begins a poll, unless the source is asked to stop. */
    private boolean begin() {
      turn.lock();
      try {
        if (stopping || !isRunAllowed()) {
          return false;
        }
        polling = true;
        return true;
      } finally {
        turn.unlock();
      }
    }

    /** Ends a poll and, unless the source is asked to stop, schedules the next. */
    private void end(@Nullable Duration next) {
      turn.lock();
      try {
        polling = false;
        pollEnded.signalAll();
        if (next != null && !stopping) {
          schedule(next);
        }
      } finally {
        turn.unlock();
      }
    }

    /**
     * One poll, and the next one scheduled whatever it threw: an extension's {@code Error} (a
     * missing class, a stack overflow) is a failure of that poll, not the end of the source. While
     * the source is asked to stop, a dispatch not taken, a part that failed and a throw are no
     * failure, and no poll is scheduled.
     */
    private void run() {
      if (!begin()) {
        return;
      }
      Duration next = null;
      try {
        next = once();
      } finally {
        end(next);
      }
    }

    /** @return when to poll next; null for no next poll */
    private @Nullable Duration once() {
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
        return null;
      } catch (Throwable e) {
        String message = e.getMessage() == null ? e.getClass().getSimpleName() : e.getMessage();
        ended(trace, false, message, null);
        if (stopping && !(e instanceof OutOfMemoryError)) {
          // Most likely the stop itself (a dispatch not taken, an interrupted call): not a failure.
          LOG.debug("ingestion {}: the poll ended with {} while the source is stopping", ingestion.id(), message, e);
          return null;
        }
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
          end(next);
          throw oom;
        }
      }
      return next;
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

    /** Schedules the next poll; called holding {@link #turn}. */
    private void schedule(Duration next) {
      ScheduledExecutorService running = executor;
      if (running != null && isRunAllowed() && !running.isShutdown()) {
        try {
          scheduled = running.schedule(this::run, Math.max(next.toMillis(), 1), TimeUnit.MILLISECONDS);
        } catch (java.util.concurrent.RejectedExecutionException e) {
          // Stopping.
        }
      }
    }

    /** Cancels the poll scheduled and not yet begun; called holding {@link #turn}. */
    private void cancelScheduled() {
      ScheduledFuture<?> pending = scheduled;
      if (pending != null) {
        pending.cancel(false);
        scheduled = null;
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
            if (stopping) {
              // Not taken, and not refused: the route is not run, and the poll should end here.
              return false;
            }
            return route(body, identity);
          }

          @Override
          public boolean dispatchTaken(String body, @Nullable String identity) throws Exception {
            // Read already, and not to be served again: run the route even while stopping.
            return route(body, identity);
          }

          /** Runs the route with the payload; whether its jobs were created. */
          private boolean route(String body, @Nullable String identity) throws Exception {
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

          @Override
          public boolean stopping() {
            return stopping;
          }
        };
  }
}
