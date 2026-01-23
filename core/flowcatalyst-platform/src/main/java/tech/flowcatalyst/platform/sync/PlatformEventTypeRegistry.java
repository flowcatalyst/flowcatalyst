package tech.flowcatalyst.platform.sync;

import jakarta.enterprise.context.ApplicationScoped;

import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.util.ArrayList;
import java.util.Collections;
import java.util.HexFormat;
import java.util.List;

/**
 * Registry of platform event types defined in code.
 *
 * <p>This is the source of truth for platform event types. Event types defined here
 * are automatically synced to the database on startup using {@link PlatformEventTypeSyncService}.
 *
 * <p>Event types are synced with source=CODE to distinguish them from API-sourced or UI-created types.
 *
 * <p>Code format: {application}:{subdomain}:{aggregate}:{event}
 */
@ApplicationScoped
public class PlatformEventTypeRegistry {

    private final List<PlatformEventTypeDefinition> definitions;
    private final String contentHash;

    public PlatformEventTypeRegistry() {
        List<PlatformEventTypeDefinition> defs = new ArrayList<>();

        // ========================================================================
        // Control Plane - EventType aggregate
        // ========================================================================
        defs.add(def("platform:control-plane:eventtype:created", "Event Type Created",
            "A new event type has been registered in the platform"));
        defs.add(def("platform:control-plane:eventtype:updated", "Event Type Updated",
            "Event type metadata has been updated"));
        defs.add(def("platform:control-plane:eventtype:archived", "Event Type Archived",
            "Event type has been archived"));
        defs.add(def("platform:control-plane:eventtype:deleted", "Event Type Deleted",
            "Event type has been deleted from the platform"));
        defs.add(def("platform:control-plane:eventtype:schema-added", "Event Type Schema Added",
            "A new schema version has been added to an event type"));
        defs.add(def("platform:control-plane:eventtype:schema-deprecated", "Event Type Schema Deprecated",
            "A schema version has been marked as deprecated"));
        defs.add(def("platform:control-plane:eventtype:schema-finalised", "Event Type Schema Finalised",
            "A schema version has been finalised as the current version"));

        // ========================================================================
        // Control Plane - Application aggregate
        // ========================================================================
        defs.add(def("platform:control-plane:application:created", "Application Created",
            "A new application has been registered in the platform"));
        defs.add(def("platform:control-plane:application:updated", "Application Updated",
            "Application details have been updated"));
        defs.add(def("platform:control-plane:application:activated", "Application Activated",
            "Application has been activated"));
        defs.add(def("platform:control-plane:application:deactivated", "Application Deactivated",
            "Application has been deactivated"));
        defs.add(def("platform:control-plane:application:deleted", "Application Deleted",
            "Application has been deleted from the platform"));
        defs.add(def("platform:control-plane:application:service-account-provisioned", "Service Account Provisioned",
            "A service account has been provisioned for the application"));

        // ========================================================================
        // Control Plane - Application Client Config aggregate
        // ========================================================================
        defs.add(def("platform:control-plane:application-client-config:enabled", "Application Enabled for Client",
            "An application has been enabled for a specific client"));
        defs.add(def("platform:control-plane:application-client-config:disabled", "Application Disabled for Client",
            "An application has been disabled for a specific client"));

        // ========================================================================
        // Control Plane - Role aggregate
        // ========================================================================
        defs.add(def("platform:control-plane:role:created", "Role Created",
            "A new role has been created"));
        defs.add(def("platform:control-plane:role:updated", "Role Updated",
            "Role details or permissions have been updated"));
        defs.add(def("platform:control-plane:role:deleted", "Role Deleted",
            "Role has been deleted"));
        defs.add(def("platform:control-plane:role:synced", "Roles Synced",
            "Roles have been bulk synced from an external application"));

        // ========================================================================
        // Control Plane - Subscription aggregate
        // ========================================================================
        defs.add(def("platform:control-plane:subscription:created", "Subscription Created",
            "A new subscription has been created"));
        defs.add(def("platform:control-plane:subscription:updated", "Subscription Updated",
            "Subscription configuration has been updated"));
        defs.add(def("platform:control-plane:subscription:deleted", "Subscription Deleted",
            "Subscription has been deleted"));

        // ========================================================================
        // Control Plane - Dispatch Pool aggregate
        // ========================================================================
        defs.add(def("platform:control-plane:dispatch-pool:created", "Dispatch Pool Created",
            "A new dispatch pool has been created"));
        defs.add(def("platform:control-plane:dispatch-pool:updated", "Dispatch Pool Updated",
            "Dispatch pool configuration has been updated"));
        defs.add(def("platform:control-plane:dispatch-pool:deleted", "Dispatch Pool Deleted",
            "Dispatch pool has been deleted"));

        // ========================================================================
        // Control Plane - CORS Origin aggregate
        // ========================================================================
        defs.add(def("platform:control-plane:cors-origin:added", "CORS Origin Added",
            "A new CORS origin has been added"));
        defs.add(def("platform:control-plane:cors-origin:deleted", "CORS Origin Deleted",
            "A CORS origin has been removed"));

        // ========================================================================
        // IAM - User aggregate
        // ========================================================================
        defs.add(def("platform:iam:user:created", "User Created",
            "A new user has been created"));
        defs.add(def("platform:iam:user:updated", "User Updated",
            "User details have been updated"));
        defs.add(def("platform:iam:user:deleted", "User Deleted",
            "User has been deleted"));
        defs.add(def("platform:iam:user:activated", "User Activated",
            "User has been activated"));
        defs.add(def("platform:iam:user:deactivated", "User Deactivated",
            "User has been deactivated"));
        defs.add(def("platform:iam:user:roles-assigned", "User Roles Assigned",
            "Roles have been assigned to a user"));
        defs.add(def("platform:iam:user:client-access-granted", "User Client Access Granted",
            "A user has been granted access to a client"));
        defs.add(def("platform:iam:user:client-access-revoked", "User Client Access Revoked",
            "A user's access to a client has been revoked"));

        // ========================================================================
        // IAM - Service Account aggregate
        // ========================================================================
        defs.add(def("platform:iam:service-account:created", "Service Account Created",
            "A new service account has been created"));
        defs.add(def("platform:iam:service-account:updated", "Service Account Updated",
            "Service account details have been updated"));
        defs.add(def("platform:iam:service-account:deleted", "Service Account Deleted",
            "Service account has been deleted"));
        defs.add(def("platform:iam:service-account:roles-assigned", "Service Account Roles Assigned",
            "Roles have been assigned to a service account"));
        defs.add(def("platform:iam:service-account:auth-token-regenerated", "Service Account Auth Token Regenerated",
            "Service account authentication token has been regenerated"));
        defs.add(def("platform:iam:service-account:signing-secret-regenerated", "Service Account Signing Secret Regenerated",
            "Service account signing secret has been regenerated"));

        // ========================================================================
        // IAM - Auth Config aggregate
        // ========================================================================
        defs.add(def("platform:iam:auth-config:type-updated", "Auth Config Type Updated",
            "Auth config type has been changed"));
        defs.add(def("platform:iam:auth-config:granted-clients-updated", "Auth Config Granted Clients Updated",
            "The clients granted access via auth config have been updated"));
        defs.add(def("platform:iam:auth-config:additional-clients-updated", "Auth Config Additional Clients Updated",
            "The additional clients for auth config have been updated"));

        // ========================================================================
        // Messaging - Sync Events
        // ========================================================================
        defs.add(def("platform:messaging:event-type:synced", "Event Types Synced",
            "Event types have been bulk synced from an external application"));
        defs.add(def("platform:messaging:subscription:synced", "Subscriptions Synced",
            "Subscriptions have been bulk synced from an external application"));

        this.definitions = Collections.unmodifiableList(defs);
        this.contentHash = computeHash(defs);
    }

    /**
     * Get all platform event type definitions.
     */
    public List<PlatformEventTypeDefinition> getDefinitions() {
        return definitions;
    }

    /**
     * Get the content hash representing the current state of definitions.
     * Used to detect when definitions have changed and need re-syncing.
     */
    public String getContentHash() {
        return contentHash;
    }

    /**
     * Compute a SHA-256 hash of all definitions.
     * Any change to codes, names, descriptions, or clientScoped will change the hash.
     */
    private static String computeHash(List<PlatformEventTypeDefinition> defs) {
        try {
            MessageDigest digest = MessageDigest.getInstance("SHA-256");
            for (PlatformEventTypeDefinition def : defs) {
                digest.update(def.code().getBytes(StandardCharsets.UTF_8));
                digest.update((byte) 0);
                digest.update(def.name().getBytes(StandardCharsets.UTF_8));
                digest.update((byte) 0);
                if (def.description() != null) {
                    digest.update(def.description().getBytes(StandardCharsets.UTF_8));
                }
                digest.update((byte) 0);
                digest.update((byte) (def.clientScoped() ? 1 : 0));
                digest.update((byte) 0);
            }
            return HexFormat.of().formatHex(digest.digest());
        } catch (NoSuchAlgorithmException e) {
            throw new RuntimeException("SHA-256 not available", e);
        }
    }

    private static PlatformEventTypeDefinition def(String code, String name, String description) {
        return new PlatformEventTypeDefinition(code, name, description, false);
    }

    private static PlatformEventTypeDefinition def(String code, String name, String description, boolean clientScoped) {
        return new PlatformEventTypeDefinition(code, name, description, clientScoped);
    }

    /**
     * Platform event type definition.
     */
    public record PlatformEventTypeDefinition(
        String code,
        String name,
        String description,
        boolean clientScoped
    ) {}
}
