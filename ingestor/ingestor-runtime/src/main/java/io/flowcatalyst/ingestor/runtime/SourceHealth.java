package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.ingestor.contract.Ingestion;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;
import org.jspecify.annotations.Nullable;

/**
 * Which running ingestions have a source that keeps failing, and why. A route stays started while
 * its source fails (it is tried again), and the report says {@code FAILING} with the reason. A
 * problem belongs to one version: a changed ingestion is a new route and starts healthy.
 */
final class SourceHealth {

  private record Problem(long version, String message) {}

  private final Map<String, Problem> problems = new ConcurrentHashMap<>();

  /** The ingestion's source failed; the message must hold no secret. */
  void failing(Ingestion ingestion, String message) {
    problems.put(ingestion.id(), new Problem(ingestion.version(), ExchangeCounters.clean(message)));
  }

  /** The ingestion's source answered again. */
  void recovered(Ingestion ingestion) {
    problems.remove(ingestion.id());
  }

  /** Why this version of the ingestion's source is failing, or null when it is not. */
  @Nullable String problem(String ingestionId, long version) {
    Problem problem = problems.get(ingestionId);
    return problem != null && problem.version() == version ? problem.message() : null;
  }

  /** The route is gone. */
  void forget(String ingestionId) {
    problems.remove(ingestionId);
  }
}
