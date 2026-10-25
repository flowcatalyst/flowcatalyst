package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.ingestor.contract.ConnectionTest;
import io.flowcatalyst.ingestor.contract.PollAnswer;
import io.flowcatalyst.ingestor.contract.RuntimeReport;
import java.io.IOException;
import java.time.Duration;
import java.util.List;

/** The two calls the runtime makes every interval to the platform's ingestion runtime API. */
public interface RuntimeApi {

  /**
   * An ingestion the answer listed whose definition this runtime cannot read: a kind, a variant or
   * a value of a newer platform. It is reported {@code FAILED}; the others run.
   *
   * @param ingestionId the ingestion
   * @param version the version listed
   * @param reason why, for people; holds nothing from the definition but a kind's name
   */
  record Unreadable(String ingestionId, long version, String reason) {}

  /**
   * A poll's answer as this runtime read it.
   *
   * @param answer the ingestions and tests it could read
   * @param unreadable the ingestions it could not
   * @param unreadableTests the answers to the tests whose definition it could not read
   */
  record Polled(PollAnswer answer, List<Unreadable> unreadable, List<ConnectionTest> unreadableTests) {

    /** Copies the lists. */
    public Polled {
      unreadable = List.copyOf(unreadable);
      unreadableTests = List.copyOf(unreadableTests);
    }

    /**
     * An answer read in full.
     *
     * @param answer the answer
     */
    public Polled(PollAnswer answer) {
      this(answer, List.of(), List.of());
    }
  }

  /**
   * Asks what this runtime should run. The call renews the leases the runtime holds, and carries the
   * kinds the runtime adds and the built-in kinds it runs. An ingestion or test whose definition
   * cannot be read does not fail the poll.
   *
   * @param timeout how long to wait for the answer
   * @return the ingestions to run, how long the single-owner ones may run without another answer,
   *     the tests waiting for this runtime, and what could not be read
   * @throws IOException when the platform cannot be reached or answers with an error
   * @throws InterruptedException when interrupted while waiting
   */
  Polled poll(Duration timeout) throws IOException, InterruptedException;

  /**
   * Sends one report.
   *
   * @param report route states, exchange counts, failures and test answers
   * @param timeout how long to wait for the answer
   * @throws IOException when the platform cannot be reached or answers with an error; a {@link
   *     RejectedException} when it will not take this report however often it is sent
   * @throws InterruptedException when interrupted while waiting
   */
  void report(RuntimeReport report, Duration timeout) throws IOException, InterruptedException;
}
