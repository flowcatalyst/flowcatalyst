package io.flowcatalyst.ingestor.contract;

import java.util.List;

/**
 * The body of {@code POST /api/ingestion-runtime/v1/traces}: at most {@value #MAX_RECORDS} records and
 * {@value #MAX_BYTES} bytes of JSON.
 *
 * @param runtimeId the shipping runtime
 * @param records the records
 */
public record TraceBatch(String runtimeId, List<TraceRecord> records) {

  /** The most records one batch carries. */
  public static final int MAX_RECORDS = 500;

  /** The largest batch body the platform takes, in bytes of JSON. */
  public static final int MAX_BYTES = 1024 * 1024;

  /** Copies the list. */
  public TraceBatch {
    records = List.copyOf(records);
  }
}
