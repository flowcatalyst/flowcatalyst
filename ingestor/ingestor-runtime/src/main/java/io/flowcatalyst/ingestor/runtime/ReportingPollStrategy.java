package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.RuntimeReport.Failure;
import org.apache.camel.Consumer;
import org.apache.camel.Endpoint;
import org.apache.camel.spi.PollingConsumerPollStrategy;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/**
 * A polled source's failures, kept for the platform: a poll that throws (no connection, a
 * login refused, an error answer) is a {@code SOURCE} failure and makes the route {@code FAILING}
 * until a poll succeeds. The next poll comes at the usual interval.
 */
final class ReportingPollStrategy implements PollingConsumerPollStrategy {

  private static final Logger LOG = LoggerFactory.getLogger(ReportingPollStrategy.class);

  private final Ingestion ingestion;
  private final ExchangeCounters counters;
  private final SourceHealth health;

  ReportingPollStrategy(Ingestion ingestion, ExchangeCounters counters, SourceHealth health) {
    this.ingestion = ingestion;
    this.counters = counters;
    this.health = health;
  }

  @Override
  public boolean begin(Consumer consumer, Endpoint endpoint) {
    return true;
  }

  @Override
  public void commit(Consumer consumer, Endpoint endpoint, int polledMessages) {
    health.recovered(ingestion);
  }

  @Override
  public boolean rollback(Consumer consumer, Endpoint endpoint, int retryCounter, Exception cause) {
    String message = ExchangeCounters.clean(cause.getMessage() == null ? cause.getClass().getSimpleName() : cause.getMessage());
    LOG.warn("ingestion {} could not poll its source: {}", ingestion.id(), message);
    counters.failure(ingestion, Failure.Kind.SOURCE, message, null, null);
    health.failing(ingestion, message);
    return false;
  }
}
