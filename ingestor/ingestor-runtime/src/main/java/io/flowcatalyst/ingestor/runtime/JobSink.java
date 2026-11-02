package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.JobRecord;
import io.flowcatalyst.ingestor.contract.JobsAnswer;
import java.io.IOException;
import java.util.List;
import org.jspecify.annotations.Nullable;

/** Creates dispatch jobs on the platform, a batch of records at a time, under the ingestion's own target. */
public interface JobSink {

  /**
   * Creates a job for each record. The platform builds every job from the ingestion's target; the
   * records carry only themselves.
   *
   * @param ingestion the ingestion the records came from
   * @param records the records, in order; never empty, at most {@link JobRecord#MAX_PER_REQUEST}
   * @throws Exception when the jobs could not be created; the exchange that produced them fails
   */
  void create(Ingestion ingestion, List<JobRecord> records) throws Exception;

  /**
   * Creates a job for each record and answers what the platform made of them, so that an HTTP caller
   * can be told the ids. A sink that cannot say answers without results, and no ids are reported.
   *
   * @param ingestion the ingestion the records came from
   * @param records the records, in order; never empty, at most {@link JobRecord#MAX_PER_REQUEST}
   * @return the platform's answer, its results in the order of {@code records} when it gave them
   * @throws Exception when the jobs could not be created; the exchange that produced them fails
   */
  default JobsAnswer submit(Ingestion ingestion, List<JobRecord> records) throws Exception {
    create(ingestion, records);
    return new JobsAnswer(null, records.size(), List.of());
  }

  /**
   * Asked before each attempt to send a batch. A sink that holds sending while the platform is not
   * taking jobs ({@link HoldingJobSink}) throws {@link PlatformHeldException} here, at once and
   * without calling the platform; the first caller after the hold is let through as a probe.
   *
   * @throws IOException when sending is held
   */
  default void admit() throws IOException {}

  /**
   * Told how a batch ended, after its attempts: with the failure of the last attempt, or null when it
   * was sent. Not called when {@link #admit()} refused.
   *
   * @param failure the last attempt's failure, or null for success
   */
  default void settled(@Nullable Exception failure) {}
}
