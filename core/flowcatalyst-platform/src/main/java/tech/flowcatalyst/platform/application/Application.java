package tech.flowcatalyst.platform.application;

import java.time.Instant;

/**
 * Represents an application or integration in the FlowCatalyst platform ecosystem.
 *
 * <p>Two types are supported:
 * <ul>
 *   <li>{@link ApplicationType#APPLICATION} - User-facing applications (TMS, WMS, etc.)
 *       that users log into. Can be assigned to clients and users.</li>
 *   <li>{@link ApplicationType#INTEGRATION} - Third-party adapters and connectors
 *       (Salesforce, SAP, etc.). Used for event/subscription scoping but not user access.</li>
 * </ul>
 *
 * <p>Each application/integration has a unique code used as the prefix for:
 * <ul>
 *   <li>Roles (e.g., "tms:admin", "sf:sync")</li>
 *   <li>Event types (e.g., "tms:shipment-created", "sf:contact-updated")</li>
 *   <li>Permissions</li>
 * </ul>
 *
 * <p>Every application/integration automatically gets a service account that can
 * manage its own resources (roles, permissions, event types, subscriptions).
 *
 * @see ApplicationType
 */
public class Application {

    public String id;

    /**
     * Type of this entity.
     * Defaults to APPLICATION for backwards compatibility.
     */
    public ApplicationType type = ApplicationType.APPLICATION;

    /**
     * Unique application code used in role prefixes.
     * Examples: "tms", "wms", "platform", "sf" (for Salesforce integration)
     */
    public String code;

    public String name;

    public String description;

    public String iconUrl;

    /**
     * Public website URL for this application/integration.
     * Example: https://www.yardmanagement.com
     * Can be overridden per client via ApplicationClientConfig.websiteOverride.
     */
    public String website;

    /**
     * Embedded logo content (SVG/vector format).
     * Stored directly in the database.
     * Use logoMimeType to determine the format.
     */
    public String logo;

    /**
     * MIME type of the logo content.
     * Example: "image/svg+xml" for SVG logos.
     */
    public String logoMimeType;

    /**
     * Default base URL for the application.
     * Can be overridden per tenant via ApplicationTenantConfig.
     * Primarily used for APPLICATION type.
     */
    public String defaultBaseUrl;

    /**
     * The service account ID for this application/integration.
     * References the standalone ServiceAccount entity in service_accounts collection.
     * Contains webhook credentials (auth token, signing secret) for dispatching.
     */
    public String serviceAccountId;

    /**
     * @deprecated Use {@link #serviceAccountId} instead.
     * The service account principal ID for this application/integration.
     * Auto-created when the application is created.
     * Used for machine-to-machine authentication (client_credentials grant).
     */
    @Deprecated
    public String serviceAccountPrincipalId;

    public boolean active = true;

    public Instant createdAt = Instant.now();

    public Instant updatedAt = Instant.now();

    public Application() {
    }

    public Application(String code, String name) {
        this.code = code;
        this.name = name;
    }

    public Application(String code, String name, ApplicationType type) {
        this.code = code;
        this.name = name;
        this.type = type;
    }

    /**
     * Check if this is a user-facing application.
     */
    public boolean isApplication() {
        return type == ApplicationType.APPLICATION;
    }

    /**
     * Check if this is a third-party integration.
     */
    public boolean isIntegration() {
        return type == ApplicationType.INTEGRATION;
    }

    /**
     * Application or Integration type.
     */
    public enum ApplicationType {
        /**
         * User-facing application that users log into.
         * Can be assigned to clients and users.
         */
        APPLICATION,

        /**
         * Third-party adapter or connector.
         * Used for event/subscription scoping but not user access.
         */
        INTEGRATION
    }
}
