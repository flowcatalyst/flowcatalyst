package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.ingestor.contract.PollAnswer;
import io.flowcatalyst.ingestor.contract.RuntimeReport;
import java.io.IOException;

/** The two calls the runtime makes every interval to the platform's ingestion runtime API. */
public interface RuntimeApi {

  /**
   * Asks what this runtime should run. The call renews the leases the runtime holds, and carries the
   * kinds the runtime adds.
   *
   * @return the ingestions to run, how long the single-owner ones may run without another answer,
   *     and the tests waiting for this runtime
   * @throws IOException when the platform cannot be reached or answers with an error
   * @throws InterruptedException when interrupted while waiting
   */
  PollAnswer poll() throws IOException, InterruptedException;

  /**
   * Sends one report.
   *
   * @param report route states, exchange counts, failures and test answers
   * @throws IOException when the platform cannot be reached or answers with an error; a {@link
   *     RejectedException} when it will not take this report however often it is sent
   * @throws InterruptedException when interrupted while waiting
   */
  void report(RuntimeReport report) throws IOException, InterruptedException;
}
