package io.flowcatalyst.ingestor.contract;

import com.fasterxml.jackson.annotation.JsonInclude;
import org.jspecify.annotations.Nullable;

/**
 * One record to become a dispatch job. It carries only the record: where the job goes, what signs it
 * and for which client are the ingestion's own {@link Target}, and the platform refuses a field for
 * any of them.
 *
 * @param payload the record, as text
 * @param payloadContentType the record's media type, at most 100 characters
 * @param messageGroup orders the jobs of a group within the target's mode, at most 200 characters
 * @param externalId the record's idempotency key within the ingestion, at most 100 characters:
 *     sending it again answers the job it made first
 * @param correlationId ties the job to the exchange that produced it, at most 100 characters
 * @param subject at most 500 characters
 */
@JsonInclude(JsonInclude.Include.NON_NULL)
public record JobRecord(
    String payload,
    String payloadContentType,
    @Nullable String messageGroup,
    @Nullable String externalId,
    @Nullable String correlationId,
    @Nullable String subject) {

  /** The most records one jobs request carries. */
  public static final int MAX_PER_REQUEST = 1000;

  /** The largest jobs request body the platform takes, in bytes of its JSON. */
  public static final int MAX_REQUEST_BYTES = 10 * 1024 * 1024;

  /**
   * The size a runtime keeps a jobs request under, in bytes of its JSON, leaving room below {@link
   * #MAX_REQUEST_BYTES}; a single record larger than this is sent on its own.
   */
  public static final int TARGET_REQUEST_BYTES = 8 * 1024 * 1024;
}
