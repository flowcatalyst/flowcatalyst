package tech.flowcatalyst.eventtype;


import lombok.Builder;
import lombok.With;
import tech.flowcatalyst.platform.shared.TsidGenerator;

import java.time.Instant;
import java.util.ArrayList;
import java.util.List;

/**
 * Represents an event type in the FlowCatalyst platform.
 *
 * <p>Event types define the structure and schema for events in the system.
 * Each event type has a globally unique code and can have multiple
 * schema versions for backwards compatibility.
 *
 * <p>Code format: {APPLICATION}:{SUBDOMAIN}:{AGGREGATE}:{EVENT}
 * Example: operant:execution:trip:started
 *
 * <p>Use {@link #create(String, String)} for safe construction with defaults.
 */

@Builder(toBuilder = true)
@With
public record EventType(
    String id,

    /**
     * Unique event type code (globally unique, not tenant-scoped).
     * Format: {application}:{subdomain}:{aggregate}:{event}
     * Example: operant:execution:trip:started
     */
    String code,

    /**
     * Human-friendly name for the event type.
     */
    String name,

    /**
     * Description of the event type.
     */
    String description,

    /**
     * Schema versions for this event type.
     */
    List<SpecVersion> specVersions,

    /**
     * Current status of the event type.
     */
    EventTypeStatus status,

    /**
     * Source of the event type - how it was created (CODE, API/SDK, or UI).
     */
    EventTypeSource source,

    /**
     * Whether events of this type are scoped to a specific client.
     *
     * <p>Client-scoped event types represent events that occur within a client context
     * (e.g., spar:orders:order:created). Non-client-scoped event types are platform-wide
     * (e.g., platform:iam:user:created).
     *
     * <p>This field is immutable after creation. Subscriptions to client-scoped event types
     * must also be client-scoped, and vice versa.
     */
    boolean clientScoped,

    Instant createdAt,

    Instant updatedAt
) {

    /**
     * Find a spec version by version string.
     */
    public SpecVersion findSpecVersion(String version) {
        if (specVersions == null) return null;
        return specVersions.stream()
            .filter(sv -> sv.version().equals(version))
            .findFirst()
            .orElse(null);
    }

    /**
     * Check if all spec versions are in DEPRECATED status.
     */
    public boolean allVersionsDeprecated() {
        if (specVersions == null || specVersions.isEmpty()) {
            return true;
        }
        return specVersions.stream()
            .allMatch(sv -> sv.status() == SpecVersionStatus.DEPRECATED);
    }

    /**
     * Check if all spec versions are in FINALISING status (never finalized).
     */
    public boolean allVersionsFinalising() {
        if (specVersions == null || specVersions.isEmpty()) {
            return true;
        }
        return specVersions.stream()
            .allMatch(sv -> sv.status() == SpecVersionStatus.FINALISING);
    }

    /**
     * Check if a version string already exists.
     */
    public boolean hasVersion(String version) {
        return findSpecVersion(version) != null;
    }

    // ========================================================================
    // Factory Methods
    // ========================================================================

    /**
     * Create a new event type with required fields and sensible defaults.
     *
     * @param code Unique event type code
     * @param name Human-readable name
     * @param clientScoped Whether events of this type are scoped to a client
     * @return A pre-configured builder with defaults set
     */
    public static EventTypeBuilder create(String code, String name, boolean clientScoped) {
        var now = Instant.now();
        return EventType.builder()
            .id(TsidGenerator.generate())
            .code(code)
            .name(name)
            .specVersions(new ArrayList<>())
            .status(EventTypeStatus.CURRENT)
            .source(EventTypeSource.UI)
            .clientScoped(clientScoped)
            .createdAt(now)
            .updatedAt(now);
    }

    /**
     * Create a new event type from SDK/API with required fields and sensible defaults.
     *
     * @param code Unique event type code
     * @param name Human-readable name
     * @param clientScoped Whether events of this type are scoped to a client
     * @return A pre-configured builder with defaults set
     */
    public static EventTypeBuilder createFromApi(String code, String name, boolean clientScoped) {
        var now = Instant.now();
        return EventType.builder()
            .id(TsidGenerator.generate())
            .code(code)
            .name(name)
            .specVersions(new ArrayList<>())
            .status(EventTypeStatus.CURRENT)
            .source(EventTypeSource.API)
            .clientScoped(clientScoped)
            .createdAt(now)
            .updatedAt(now);
    }

    /**
     * Create a new event type from code (platform event types) with required fields and sensible defaults.
     *
     * @param code Unique event type code
     * @param name Human-readable name
     * @param clientScoped Whether events of this type are scoped to a client
     * @return A pre-configured builder with defaults set
     */
    public static EventTypeBuilder createFromCode(String code, String name, boolean clientScoped) {
        var now = Instant.now();
        return EventType.builder()
            .id(TsidGenerator.generate())
            .code(code)
            .name(name)
            .specVersions(new ArrayList<>())
            .status(EventTypeStatus.CURRENT)
            .source(EventTypeSource.CODE)
            .clientScoped(clientScoped)
            .createdAt(now)
            .updatedAt(now);
    }

    /**
     * Add a spec version to this event type.
     *
     * @param specVersion The spec version to add
     * @return A new EventType with the spec version added
     */
    public EventType addSpecVersion(SpecVersion specVersion) {
        var newVersions = new ArrayList<>(specVersions != null ? specVersions : List.of());
        newVersions.add(specVersion);
        return this.toBuilder()
            .specVersions(newVersions)
            .updatedAt(Instant.now())
            .build();
    }
}
