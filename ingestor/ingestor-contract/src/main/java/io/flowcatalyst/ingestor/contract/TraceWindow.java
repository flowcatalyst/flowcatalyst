package io.flowcatalyst.ingestor.contract;

import java.time.Instant;

/**
 * A switched-on trace window the platform lists in a poll answer for an ingestion the answer lists to
 * this runtime. The runtime starts its own capture when it first sees a window id; the platform
 * refuses records past {@code hardStop}, so the runtime never captures past it.
 *
 * @param ingestionId the ingestion traced
 * @param windowId the window, new each time tracing is switched on
 * @param windowSecs how long the runtime captures, counted from when it first sees the window
 * @param sampleEverySecs the least time between the starts of two captures of the ingestion
 * @param hardStop the instant after which nothing may be captured
 */
public record TraceWindow(String ingestionId, String windowId, int windowSecs, int sampleEverySecs, Instant hardStop) {}
