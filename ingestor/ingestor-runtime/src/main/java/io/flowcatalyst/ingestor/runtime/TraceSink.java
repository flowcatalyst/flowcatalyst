package io.flowcatalyst.ingestor.runtime;

import io.flowcatalyst.ingestor.contract.TraceRecord;

/** Where a capture's records go. Taking one never blocks and never throws. */
@FunctionalInterface
interface TraceSink {

  /** Takes records and drops them: tracing is not wired. */
  TraceSink DISCARD = record -> {};

  /**
   * Takes a record to ship.
   *
   * @param record a record, already redacted
   */
  void offer(TraceRecord record);
}
