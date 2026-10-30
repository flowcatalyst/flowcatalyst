package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.ingestor.contract.TraceRecord;
import java.io.IOException;
import java.time.Duration;
import java.util.List;

/** The call that carries a batch of trace records to the platform. */
@FunctionalInterface
interface TraceUplink {

  /**
   * What the platform answered.
   *
   * @param status the HTTP status
   * @param accepted records it stored, when it said
   * @param dropped records it refused, when it said
   */
  record Result(int status, int accepted, int dropped) {

    /** Whether the batch was taken. */
    boolean taken() {
      return status >= 200 && status < 300;
    }
  }

  /**
   * Sends one batch.
   *
   * @param records the records
   * @param timeout how long to wait for the answer
   * @return the answer, whatever its status
   * @throws IOException when there is no answer
   * @throws InterruptedException when interrupted while waiting
   */
  Result sendTraces(List<TraceRecord> records, Duration timeout) throws IOException, InterruptedException;
}
