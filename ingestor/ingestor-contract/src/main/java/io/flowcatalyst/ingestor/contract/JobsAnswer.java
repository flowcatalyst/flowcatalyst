package io.flowcatalyst.ingestor.contract;

import java.util.List;

/**
 * The platform's answer to a jobs request, one result per record in order.
 *
 * @param created jobs made by this request; records answered with an existing job are not counted
 * @param results the outcome of each record
 */
public record JobsAnswer(int created, List<JobResult> results) {

  /** Copies the list. */
  public JobsAnswer {
    results = List.copyOf(results);
  }
}
