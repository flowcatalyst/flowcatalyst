package tech.flowcatalyst.dispatchpool;

import lombok.Builder;
import lombok.With;
import tech.flowcatalyst.platform.shared.TsidGenerator;

import java.time.Instant;

/**
 * A dispatch pool controls the rate at which dispatch jobs can be processed.
 *
 * <p>Pools define rate limits (per minute) and concurrency limits for message dispatching.
 * Pools can optionally be scoped to a client:
 * <ul>
 *   <li>Client-specific pools: clientId is set, pool is scoped to that client</li>
 *   <li>Anchor-level pools: clientId is null, pool is for non-client-scoped dispatch jobs</li>
 * </ul>
 *
 * <p>Code uniqueness is enforced per clientId combination.
 *
 * <p>Use {@link #builder()} for safe construction with named parameters.
 * Use {@link #toBuilder()} to create a modified copy of an existing instance.
 */
@Builder(toBuilder = true)
@With
public record DispatchPool(
    String id,

    String code,
    String name,
    String description,

    /** Maximum dispatches per minute */
    int rateLimit,

    /** Maximum concurrent dispatches (must be >= 1) */
    int concurrency,

    /** Client this pool belongs to (nullable - null means anchor-level pool) */
    String clientId,

    /** Denormalized client identifier for queries (nullable) */
    String clientIdentifier,

    DispatchPoolStatus status,

    Instant createdAt,
    Instant updatedAt
) {
    // ========================================================================
    // Factory Methods
    // ========================================================================

    /**
     * Create a new dispatch pool with required fields and sensible defaults.
     * Use this instead of the raw builder for new entity creation.
     *
     * @param code Unique code for the pool (will be lowercased)
     * @param name Human-readable name
     * @return A pre-configured builder with defaults set
     */
    public static DispatchPoolBuilder create(String code, String name) {
        var now = Instant.now();
        return DispatchPool.builder()
            .id(TsidGenerator.generate())
            .code(code.toLowerCase())
            .name(name)
            .rateLimit(100)      // default
            .concurrency(10)     // default
            .status(DispatchPoolStatus.ACTIVE)
            .createdAt(now)
            .updatedAt(now);
    }

    // ========================================================================
    // Domain logic
    // ========================================================================

    /**
     * Check if this is an anchor-level pool (not client-specific).
     */
    public boolean isAnchorLevel() {
        return clientId == null;
    }

    /**
     * Check if this pool is active and can process jobs.
     */
    public boolean isActive() {
        return status == DispatchPoolStatus.ACTIVE;
    }

    /**
     * Check if this pool is archived.
     */
    public boolean isArchived() {
        return status == DispatchPoolStatus.ARCHIVED;
    }
}
