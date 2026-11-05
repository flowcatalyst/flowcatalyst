package io.flowcatalyst.ingestor.contract;

/**
 * The platform's answer to a {@link LeaseRelease} (200).
 *
 * @param released the leases the runtime held among those listed, now ended
 */
public record LeaseReleased(int released) {}
