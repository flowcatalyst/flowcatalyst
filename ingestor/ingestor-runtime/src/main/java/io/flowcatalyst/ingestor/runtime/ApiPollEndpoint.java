package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.RuntimeReport.Failure;
import io.flowcatalyst.ingestor.contract.Source;
import io.flowcatalyst.ingestor.extension.FailureLog;
import io.flowcatalyst.ingestor.extension.PollTrace;
import java.time.Duration;
import java.util.concurrent.ScheduledExecutorService;
import java.util.concurrent.TimeUnit;
import org.apache.camel.CamelContext;
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
 * An HTTP API polled by {@link ApiPollClient}, one request at a time. An answer with results is one
 * exchange whose body is the answer, split like any payload; the next poll comes after {@code
 * every}, or after {@code idleDelay} when there were none. The cursor is stored only once the
 * answer's jobs exist, so an answer the platform did not take is asked for again.
 *
 * <p>A refused login (also with a fresh token) is an {@code AUTH} failure, the route is {@code
 * FAILING}, and the API is tried every {@link ApiPollClient#AUTH_RETRY} until it answers. Saving the
 * ingestion makes a new route, which starts at once.
 */
final class ApiPollEndpoint extends DefaultEndpoint {

  private static final Logger LOG = LoggerFactory.getLogger(ApiPollEndpoint.class);

  /** The key the cursor is stored under. */
  static final String CURSOR = "cursor";

  private static final Duration FIRST_POLL = Duration.ofSeconds(1);

  private final Ingestion ingestion;
  private final Source.ApiPoll source;
  private final ApiPollClient client;
  private final StateStore state;
  private @Nullable ExchangeCounters counters;
  private @Nullable SourceHealth health;
  private TraceWindows traces = TraceWindows.disabled();
  private Duration authRetry = ApiPollClient.AUTH_RETRY;
  private PollPacer pacer = Pacing.system().pacer();
  private FailureLog failures = Pacing.system().failures();

  ApiPollEndpoint(CamelContext camel, Ingestion ingestion, Source.ApiPoll source, ApiPollClient client, StateStore state) {
    this.ingestion = ingestion;
    this.source = source;
    this.client = client;
    this.state = state;
    setCamelContext(camel);
  }

  /** Where failures and the source's health are reported; set before the route starts. */
  void reportTo(ExchangeCounters counters, SourceHealth health) {
    this.counters = counters;
    this.health = health;
  }

  /** Where polls are counted and, while tracing is on, captured; set before the route starts. */
  void traceTo(TraceWindows windows) {
    this.traces = windows;
  }

  /**
   * The pacer and the failure log of the ingestion's route (R1, R3); set before the route starts. The
   * next poll after failures in a row waits at least {@code Backoff.FAILURES.delay(n, idleDelay)}.
   */
  void pacedBy(PollPacer pacer, FailureLog failures) {
    this.pacer = pacer;
    this.failures = failures;
  }

  /** How long a refused login waits; tests shorten it. */
  void authRetry(Duration wait) {
    this.authRetry = wait;
  }

  @Override
  protected String createEndpointUri() {
    return "ingest-api:" + ingestion.id();
  }

  @Override
  public Producer createProducer() {
    throw new UnsupportedOperationException("an ingestion's source is only consumed from");
  }

  @Override
  public Consumer createConsumer(Processor processor) throws Exception {
    ApiPoller consumer = new ApiPoller(processor);
    configureConsumer(consumer);
    return consumer;
  }

  private final class ApiPoller extends DefaultConsumer {

    private @Nullable ScheduledExecutorService executor;
    /** Whether the poll in progress reported a failure, and the last one's text; used by one thread. */
    private boolean failedThisPoll;
    private @Nullable String lastFailure;

    ApiPoller(Processor processor) {
      super(ApiPollEndpoint.this, processor);
    }

    @Override
    protected void doStart() throws Exception {
      super.doStart();
      ScheduledExecutorService started =
          getEndpoint().getCamelContext().getExecutorServiceManager().newSingleThreadScheduledExecutor(this, "api-poll-" + ingestion.id());
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
      // Waits for a poll in flight; a restarted route opens its clients again.
      client.close();
      super.doStop();
    }

    /** One poll, and the next one scheduled whatever it threw: an {@code Error} fails that poll, not the source. */
    private void run() {
      Duration next = source.idleDelay();
      failedThisPoll = false;
      lastFailure = null;
      pacer.started();
      ExchangeCounters c = counters;
      PollTrace trace = c == null ? PollTrace.NONE : traces.pollStarting(ingestion, c);
      try {
        next = trace.run(this::pollOnce);
        next = paced(next);
        ended(trace, next);
      } catch (InterruptedException e) {
        Thread.currentThread().interrupt();
        return;
      } catch (Throwable e) {
        String message = e.getMessage() == null ? e.getClass().getSimpleName() : e.getMessage();
        boolean commit = e instanceof CommitFailedException;
        failures.failed(
            ingestion.id() + (commit ? ":commit" : ":poll"),
            "ingestion " + ingestion.id() + (commit ? ": " : " could not poll its API: ") + ExchangeCounters.clean(message),
            e);
        failure(Failure.Kind.SOURCE, message, null);
        next = paced(next);
        ended(trace, null);
        if (e instanceof OutOfMemoryError oom) {
          schedule(next);
          throw oom;
        }
      }
      schedule(next);
    }

    /**
     * The wait before the next poll: {@code next} after a poll that did not fail, else the wait for
     * failures in a row (R1). A refused login keeps its own, longer wait.
     */
    private Duration paced(Duration next) {
      Duration wait = pacer.ended(source.idleDelay());
      if (!pacer.lastFailed()) {
        failures.recovered(ingestion.id() + ":poll", "ingestion " + ingestion.id() + " polls its API again");
        failures.recovered(ingestion.id() + ":commit", "ingestion " + ingestion.id() + " stores its cursor again");
        failures.recovered(ingestion.id() + ":dispatch", "ingestion " + ingestion.id() + " dispatches what it reads again");
        return next;
      }
      return wait.compareTo(next) > 0 ? wait : next;
    }

    /** Tells the runtime how the poll ended; tracing never changes what the poll does next. */
    private void ended(PollTrace trace, @Nullable Duration next) {
      try {
        trace.finished(!failedThisPoll, lastFailure, next);
      } catch (RuntimeException e) {
        LOG.warn("ingestion {}: tracing a poll failed to finish", ingestion.id(), e);
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

    private Duration pollOnce() throws Exception {
      String cursor = source.cursor() == null ? null : state.get(ingestion.id(), CURSOR).orElse(null);
      ApiPollClient.Outcome outcome = client.poll(cursor);
      return switch (outcome) {
        case ApiPollClient.Refused refused -> {
          failures.failed(ingestion.id() + ":poll", "ingestion " + ingestion.id() + ": " + ExchangeCounters.clean(refused.message()) + "; trying again in " + authRetry, null);
          failure(Failure.Kind.AUTH, refused.message() + "; tried again every " + authRetry.toMinutes() + " minutes, or at once when the ingestion is saved", null);
          yield authRetry;
        }
        case ApiPollClient.Failed failed -> {
          failures.failed(ingestion.id() + ":poll", "ingestion " + ingestion.id() + ": " + ExchangeCounters.clean(failed.message()), null);
          failure(Failure.Kind.SOURCE, failed.message(), failed.body());
          yield source.idleDelay();
        }
        case ApiPollClient.Answer answer -> {
          SourceHealth h = health;
          if (h != null) {
            h.recovered(ingestion);
          }
          yield answered(answer, cursor);
        }
      };
    }

    private Duration answered(ApiPollClient.Answer answer, @Nullable String cursor) throws Exception {
      if (Boolean.FALSE.equals(answer.hasResults())) {
        return source.idleDelay();
      }
      Exchange exchange = createExchange(true);
      long records;
      boolean taken;
      try {
        exchange.getMessage().setBody(answer.body());
        String identity = identity(cursor);
        if (identity != null) {
          exchange.setProperty(IngestionRoute.IDENTITY, identity);
        }
        getProcessor().process(exchange);
        taken = exchange.getException() == null;
        Long counted = exchange.getProperty(IngestionRoute.RECORDS, Long.class);
        records = counted == null ? 0 : counted;
      } finally {
        releaseExchange(exchange, false);
      }
      if (!taken) {
        // The route has reported and logged it; the same answer is asked for again, after a wait.
        pacer.dispatchFailed();
        return source.idleDelay();
      }
      String next = answer.cursor();
      if (next != null && !next.equals(cursor)) {
        try {
          state.put(ingestion.id(), CURSOR, next);
        } catch (InterruptedException e) {
          throw e;
        } catch (Exception e) {
          // Read and sent, but not remembered (R4): the answer is asked for again.
          throw new CommitFailedException("stored as the cursor", e);
        }
      }
      boolean results = answer.hasResults() != null ? answer.hasResults() : records > 0;
      return results ? source.every() : source.idleDelay();
    }

    /**
     * The request an answer was given to, when it names a page: the address with the cursor sent.
     * Without a cursor every poll asks the same address for something new, which says nothing
     * about which records came back, so there is none and the jobs carry no key.
     */
    private @Nullable String identity(@Nullable String cursor) {
      Source.ApiPoll.Cursor configured = source.cursor();
      if (configured == null) {
        return null;
      }
      String sent = cursor != null ? cursor : configured.initial();
      return sent == null ? null : source.url() + (source.url().contains("?") ? "&" : "?") + configured.param() + "=" + sent;
    }

    private void failure(Failure.Kind kind, String message, @Nullable String body) {
      failedThisPoll = true;
      pacer.sourceFailed();
      lastFailure = message;
      ExchangeCounters c = counters;
      if (c != null) {
        c.failure(ingestion, kind, message, null, body);
      }
      SourceHealth h = health;
      if (h != null) {
        h.failing(ingestion, message);
      }
    }
  }
}
