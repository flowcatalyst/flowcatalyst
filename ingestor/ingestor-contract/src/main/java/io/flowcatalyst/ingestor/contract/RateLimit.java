package io.flowcatalyst.ingestor.contract;

import java.time.Duration;

/**
 * A limit on how fast an ingestion accepts requests. A caller over the limit is told to slow down.
 *
 * @param requests how many requests are accepted
 * @param per in this much time
 */
public record RateLimit(int requests, Duration per) {}
