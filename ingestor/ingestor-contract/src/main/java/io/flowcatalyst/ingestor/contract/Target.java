package io.flowcatalyst.ingestor.contract;

import org.jspecify.annotations.Nullable;

/**
 * The dispatch job created for each record of an ingestion, by name. The runtime never creates a job
 * itself: it sends the records, and the platform builds every job from the ingestion's own target,
 * resolved as at save, so a runtime can neither choose nor alter any of it.
 *
 * @param code the job's code, {@code application:subdomain:aggregate:action}
 * @param targetUrl where the platform delivers the job
 * @param pool the code of the dispatch pool that limits its rate and concurrency
 * @param connection the code of the connection whose service account signs each delivery and whose
 *     pause holds the jobs; null for none
 * @param serviceAccount the code of the service account that signs each delivery, when there is no
 *     connection; null (with no connection) to deliver unsigned
 * @param mode ordering within one inbound payload's records
 * @param batchSize how many records are sent in one call when a payload is split
 */
public record Target(
    String code,
    String targetUrl,
    String pool,
    @Nullable String connection,
    @Nullable String serviceAccount,
    Mode mode,
    int batchSize) {

  /** The platform's dispatch modes. */
  public enum Mode {
    /** No ordering. */
    IMMEDIATE,
    /** In order; a failed job is retried later and the group moves on. */
    NEXT_ON_ERROR,
    /** In order; a failed job blocks the group. */
    BLOCK_ON_ERROR
  }
}
