package io.flowcatalyst.ingestor.contract;

import java.time.Instant;

/**
 * A "test connection" request the runtime runs once and answers in its next report. The platform
 * hands it to one runtime that carries the kind, so a platform behind a firewall never needs to
 * call the runtime.
 *
 * @param id answer it in the next report under tests
 * @param expiresAt after this the platform no longer wants the answer
 * @param ingestion the source to try, saved or not; its id is {@code ing_test} for a definition not saved yet
 */
public record TestRequest(String id, Instant expiresAt, Ingestion ingestion) {}
