package io.flowcatalyst.ingestor.contract;

import com.fasterxml.jackson.annotation.JsonInclude;
import java.util.List;
import org.jspecify.annotations.Nullable;

/**
 * The body of {@code POST /api/ingestion-runtime/v1/leases/release}: a runtime giving back the leases
 * of single-owner ingestions whose routes it has stopped, so the platform can hand them to another
 * runtime at once instead of when the lease runs out. Only ingestions whose routes have actually
 * stopped are listed; a route still stopping is left to its lease.
 *
 * @param runtimeId the runtime giving them back
 * @param instanceId the process giving them back (as in {@link RuntimePoll#instanceId()}); the
 *     platform refuses with {@code 409 RUNTIME_ID_IN_USE} when another process holds the runtime id.
 *     Absent, it does not check
 * @param ingestionIds the ingestions, at most {@value #MAX_IDS}; ones the runtime does not hold are ignored
 * @param goodbye whether the runtime is shutting down: it then no longer counts as live, and its
 *     process no longer holds the runtime id
 */
@JsonInclude(JsonInclude.Include.NON_NULL)
public record LeaseRelease(String runtimeId, @Nullable String instanceId, List<String> ingestionIds, boolean goodbye) {

  /** The most ingestions one release lists. */
  public static final int MAX_IDS = 1000;

  /** Copies the list. */
  public LeaseRelease {
    ingestionIds = List.copyOf(ingestionIds);
  }
}
