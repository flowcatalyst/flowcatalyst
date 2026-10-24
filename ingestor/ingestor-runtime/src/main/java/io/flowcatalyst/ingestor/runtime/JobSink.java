package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.ingestor.contract.Ingestion;
import io.flowcatalyst.ingestor.contract.JobRecord;
import java.util.List;

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
}
