package io.flowcatalyst.ingestor.contract;

/**
 * One value a runtime keeps with the platform for an ingestion, such as the watermark of a
 * database poll. It is kept there, not in the runtime, so that it survives the ingestion moving to
 * another runtime.
 *
 * @param value the value, as text
 */
public record StateValue(String value) {}
