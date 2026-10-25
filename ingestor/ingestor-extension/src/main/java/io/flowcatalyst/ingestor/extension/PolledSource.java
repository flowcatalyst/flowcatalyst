package io.flowcatalyst.ingestor.extension;

import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.RuntimeReport.Failure;
import io.flowcatalyst.framework.httpclient.HttpClientException;
import io.flowcatalyst.framework.httpclient.UnexpectedStatusException;
import java.time.Duration;
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
 * #refusedLogin(Throwable)}), which is tried again every {@link #AUTH_RETRY}; the ingestion is
 * {@code FAILING} until a poll gets through. Polls run one at a time, on one thread. The runtime
 * reports the failures ({@link #reportTo(Reporter)}).
 */
public abstract class PolledSource extends DefaultEndpoint {

  /** How long a refused login waits before it is tried again. */
  public static final Duration AUTH_RETRY = Duration.ofMinutes(5);

  private static final Logger LOG = LoggerFactory.getLogger(PolledSource.class);

  private static final Duration FIRST_POLL = Duration.ofSeconds(1);

  /** The ingestion polled. */
  protected final Ingestion ingestion;

  private final String kind;
  private @Nullable Reporter reporter;
  private Duration authRetry = AUTH_RETRY;
  private boolean failedThisPoll;

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
   * How long a refused login waits; tests shorten it.
   *
   * @param wait the wait
   */
  public final void authRetry(Duration wait) {
    this.authRetry = wait;
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
      try {
        next = poll(payloads);
        Reporter r = reporter;
        if (r != null && !failedThisPoll) {
          r.recovered();
        }
      } catch (InterruptedException e) {
        Thread.currentThread().interrupt();
        return;
      } catch (Throwable e) {
        String message = e.getMessage() == null ? e.getClass().getSimpleName() : e.getMessage();
        if (refusedLogin(e)) {
          LOG.warn("ingestion {}: the {} login was refused; trying again in {}", ingestion.id(), kind, authRetry);
          failure(Failure.Kind.AUTH, message + "; tried again every " + authRetry.toMinutes() + " minutes, or at once when the ingestion is saved");
          next = authRetry;
        } else {
          LOG.warn("ingestion {} could not poll {}", ingestion.id(), kind, e);
          failure(Failure.Kind.SOURCE, message);
          next = safeRetryDelay();
        }
        if (e instanceof OutOfMemoryError oom) {
          // Nothing is safe after this; the process should end (the image runs with -XX:+ExitOnOutOfMemoryError).
          schedule(next);
          throw oom;
        }
      }
      schedule(next);
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
              return exchange.getException() == null;
            } finally {
              releaseExchange(exchange, false);
            }
          }
        };
  }
}
