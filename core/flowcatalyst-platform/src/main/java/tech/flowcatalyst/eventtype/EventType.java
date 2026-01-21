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
     * Format: {app}:{subdomain}:{aggregate}:{event}
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
     * @return A pre-configured builder with defaults set
     */
    public static EventTypeBuilder create(String code, String name) {
        var now = Instant.now();
        return EventType.builder()
            .id(TsidGenerator.generate())
            .code(code)
            .name(name)
            .specVersions(new ArrayList<>())
            .status(EventTypeStatus.CURRENT)
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
