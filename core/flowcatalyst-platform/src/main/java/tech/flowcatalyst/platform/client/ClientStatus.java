package tech.flowcatalyst.platform.client;

/**
 * Status of a client organization.
 * Stored as VARCHAR in database for future extensibility.
 */
public enum ClientStatus {
    /**
     * Client is active and operational
     */
    ACTIVE,

    /**
     * Client is inactive (see statusReason for details)
     */
    INACTIVE,

    /**
     * Client is suspended (temporarily disabled)
     */
    SUSPENDED
}
