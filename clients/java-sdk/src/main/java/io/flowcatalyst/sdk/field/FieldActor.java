package io.flowcatalyst.sdk.field;

/**
 * Who is signed in as the worker (the RFC 8693 {@code act} claim of a field token).
 *
 * @param id the actor's id: a field worker id (a supervisor) or a platform principal id
 * @param type {@code field_worker} (a supervisor) or {@code principal} (a platform user)
 * @param name the actor's display name, or empty
 */
public record FieldActor(String id, String type, String name) {}
