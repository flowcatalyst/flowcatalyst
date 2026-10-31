package io.flowcatalyst.ingestor.contract;

import java.util.List;
import org.jspecify.annotations.Nullable;

/**
 * The platform's answer to a jobs request, one result per record in order.
 *
 * @param kind what the ids in the results are: {@code DISPATCH_JOB} or {@code EVENT}; null (an
 *     older platform) is {@code DISPATCH_JOB}
 * @param created jobs made by this request; records answered with an existing job are not counted
 * @param results the outcome of each record
 */
public record JobsAnswer(@Nullable String kind, int created, List<JobResult> results) {

  /** What the ids are when the platform does not say. */
  public static final String DISPATCH_JOB = "DISPATCH_JOB";

  /** The ids are those of events. */
  public static final String EVENT = "EVENT";

  /** Copies the list. */
  public JobsAnswer {
    results = List.copyOf(results);
  }

  /** An answer without a kind, as an older platform sends. */
  public JobsAnswer(int created, List<JobResult> results) {
    this(null, created, results);
  }

  /**
   * Whether the ids are events'.
   *
   * @return true for {@code EVENT}
   */
  public boolean events() {
    return EVENT.equals(kind);
  }
}
