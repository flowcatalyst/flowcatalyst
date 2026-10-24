package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.RuntimeReport.Failure;
import io.flowcatalyst.ingestor.contract.Source;
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
  private Duration authRetry = ApiPollClient.AUTH_RETRY;

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

    private void run() {
      Duration next;
      try {
        next = pollOnce();
      } catch (InterruptedException e) {
        Thread.currentThread().interrupt();
        return;
      } catch (Exception e) {
        LOG.warn("ingestion {} could not poll its API", ingestion.id(), e);
        String message = e.getMessage() == null ? e.getClass().getSimpleName() : e.getMessage();
        failure(Failure.Kind.SOURCE, message, null);
        next = source.idleDelay();
      }
      ScheduledExecutorService running = executor;
      if (running != null && isRunAllowed() && !running.isShutdown()) {
        running.schedule(this::run, Math.max(next.toMillis(), 1), TimeUnit.MILLISECONDS);
      }
    }

    private Duration pollOnce() throws Exception {
      String cursor = source.cursor() == null ? null : state.get(ingestion.id(), CURSOR).orElse(null);
      ApiPollClient.Outcome outcome = client.poll(cursor);
      return switch (outcome) {
        case ApiPollClient.Refused refused -> {
          LOG.warn("ingestion {}: {}; trying again in {}", ingestion.id(), refused.message(), authRetry);
          failure(Failure.Kind.AUTH, refused.message() + "; tried again every " + authRetry.toMinutes() + " minutes, or at once when the ingestion is saved", null);
          yield authRetry;
        }
        case ApiPollClient.Failed failed -> {
          LOG.warn("ingestion {}: {}", ingestion.id(), failed.message());
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
        getProcessor().process(exchange);
        taken = exchange.getException() == null;
        Long counted = exchange.getProperty(IngestionRoute.RECORDS, Long.class);
        records = counted == null ? 0 : counted;
      } finally {
        releaseExchange(exchange, false);
      }
      if (!taken) {
        // The route has reported it; the same answer is asked for again.
        return source.idleDelay();
      }
      String next = answer.cursor();
      if (next != null && !next.equals(cursor)) {
        state.put(ingestion.id(), CURSOR, next);
      }
      boolean results = answer.hasResults() != null ? answer.hasResults() : records > 0;
      return results ? source.every() : source.idleDelay();
    }

    private void failure(Failure.Kind kind, String message, @Nullable String body) {
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
