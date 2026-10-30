package io.flowcatalyst.ingestor.contract;

import java.util.Map;

/**
 * One line of a trace, as shipped to {@code POST /api/ingestion-runtime/v1/traces}. Records of one
 * capture share the {@code captureId}.
 *
 * @param id unique, so a batch sent again is stored once
 * @param ingestionId the ingestion
 * @param version the ingestion version that was running
 * @param windowId the platform's trace window the capture belongs to
 * @param captureId names the capture (one poll cycle, or one exchange)
 * @param at when it happened, epoch milliseconds
 * @param kind {@code poll}, {@code exchange}, {@code http} or {@code error}
 * @param level {@code info}, {@code warn} or {@code error}
 * @param message one line, at most {@value #MESSAGE} characters
 * @param data the kind's detail, already redacted
 */
public record TraceRecord(
    String id,
    String ingestionId,
    long version,
    String windowId,
    String captureId,
    long at,
    String kind,
    String level,
    String message,
    Map<String, Object> data) {

  /** The longest message the platform takes. */
  public static final int MESSAGE = 1000;

  /** Cuts the message to the platform's limit and copies the data. */
  public TraceRecord {
    message = Text.cut(Text.withoutNul(message), MESSAGE);
    data = java.util.Collections.unmodifiableMap(new java.util.LinkedHashMap<>(data));
  }
}
