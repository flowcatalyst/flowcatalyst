/**
 * The wire contract between the ingestor runtime and the platform's ingestion runtime API
 * ({@code /api/ingestion-runtime/v1}): what an ingestion is, what a runtime asks and what it reports
 * back. Values only; both sides encode them as JSON.
 */
@NullMarked
package io.flowcatalyst.ingestor.contract;

import org.jspecify.annotations.NullMarked;
