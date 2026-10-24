package io.flowcatalyst.ingestor.contract;

/**
 * One configured ingestion as the platform serves it to a runtime: where the data comes from, how it
 * is split into records, and the dispatch job each record becomes. It belongs to one application and
 * one client, which is also the client of every job it creates.
 *
 * @param id the ingestion's id ({@code ing_...}); {@code ing_test} for a definition under test that is not saved
 * @param version the live version; a runtime rebuilds the route when it differs
 * @param clientId the platform id of the ingestion's client ({@code clt_...})
 * @param clientIdentifier the client's identifier, a slug such as {@code acme}
 * @param applicationCode the code of the application that owns the ingestion
 * @param code a stable code, unique within the application and client
 * @param name a name for people
 * @param source where the data comes from
 * @param split how one inbound payload becomes records
 * @param target the dispatch job created for each record
 * @param keepFailedPayloads whether a failure is reported with the start of the payload or record
 *     it failed on; off unless the client has agreed, since it may hold personal data
 */
public record Ingestion(
    String id,
    long version,
    String clientId,
    String clientIdentifier,
    String applicationCode,
    String code,
    String name,
    Source source,
    Split split,
    Target target,
    boolean keepFailedPayloads) {}
