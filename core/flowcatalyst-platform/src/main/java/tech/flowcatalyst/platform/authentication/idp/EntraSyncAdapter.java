package tech.flowcatalyst.platform.authentication.idp;

import io.quarkus.logging.Log;
import jakarta.ws.rs.client.Client;
import jakarta.ws.rs.client.ClientBuilder;
import jakarta.ws.rs.client.Entity;
import jakarta.ws.rs.core.MediaType;
import jakarta.ws.rs.core.Response;
import tech.flowcatalyst.platform.authorization.PermissionDefinition;
import tech.flowcatalyst.platform.authorization.RoleDefinition;

import java.util.*;

/**
 * Microsoft Entra (formerly Azure AD) IDP sync adapter.
 *
 * Syncs roles and permissions to Microsoft Entra via the Microsoft Graph API.
 * Uses client credentials flow for authentication.
 *
 * Entra role mapping:
 * - Each RoleDefinition becomes an App Role in the Entra app registration
 * - Role name (value): "{subdomain}:{role-name}" (e.g., "platform:tenant-admin")
 * - Display name: Human-readable role name
 * - Description: From RoleDefinition.description
 * - Permissions are stored in the role description (Entra doesn't have separate permissions)
 *
 * Configuration required:
 * - entra.tenant-id: Azure tenant ID
 * - entra.client-id: App registration client ID
 * - entra.client-secret: App registration client secret
 * - entra.application-object-id: The object ID of the app registration
 *
 * Note: App Roles in Entra must be configured at the application level and require
 * the Application.ReadWrite.All permission in Microsoft Graph.
 */
public class EntraSyncAdapter implements IdpSyncAdapter {

    private static final String GRAPH_API_BASE = "https://graph.microsoft.com/v1.0";
    private static final String LOGIN_BASE = "https://login.microsoftonline.com";

    private final String tenantId;
    private final String clientId;
    private final String clientSecret;
    private final String applicationObjectId;
    private final Client httpClient;

    public EntraSyncAdapter(String tenantId, String clientId, String clientSecret, String applicationObjectId) {
        this.tenantId = tenantId;
        this.clientId = clientId;
        this.clientSecret = clientSecret;
        this.applicationObjectId = applicationObjectId;
        this.httpClient = ClientBuilder.newClient();
    }

    @Override
    public void syncRolesToIdp(Set<RoleDefinition> roles) throws IdpSyncException {
        Log.info("Syncing " + roles.size() + " roles to Microsoft Entra tenant: " + tenantId);

        try {
            String accessToken = getAccessToken();

            // Get current app roles
            List<Map<String, Object>> currentAppRoles = getCurrentAppRoles(accessToken);

            // Build new app roles list (preserve existing roles not in our definition)
            List<Map<String, Object>> updatedAppRoles = new ArrayList<>(currentAppRoles);

            // Update or add our roles
            for (RoleDefinition role : roles) {
                Map<String, Object> appRole = buildAppRoleRepresentation(role);
                String roleValue = role.toRoleString();

                // Find and replace existing role, or add new one
                boolean found = false;
                for (int i = 0; i < updatedAppRoles.size(); i++) {
                    Map<String, Object> existing = updatedAppRoles.get(i);
                    if (roleValue.equals(existing.get("value"))) {
                        // Keep the same ID to avoid breaking existing assignments
                        appRole.put("id", existing.get("id"));
                        updatedAppRoles.set(i, appRole);
                        found = true;
                        break;
                    }
                }

                if (!found) {
                    updatedAppRoles.add(appRole);
                }
            }

            // Update the application with new app roles
            updateApplicationAppRoles(updatedAppRoles, accessToken);

            Log.info("Successfully synced " + roles.size() + " roles to Microsoft Entra");
        } catch (Exception e) {
            throw new IdpSyncException("Failed to sync roles to Microsoft Entra: " + e.getMessage(), e);
        }
    }

    @Override
    public void syncPermissionsToIdp(Set<PermissionDefinition> permissions) throws IdpSyncException {
        // Microsoft Entra doesn't have standalone permissions at the app role level
        // Permissions are embedded in roles as part of the role description
        Log.debug("Microsoft Entra doesn't support standalone app-level permissions - they are embedded in roles");
    }

    @Override
    public String getIdpType() {
        return "ENTRA";
    }

    @Override
    public boolean testConnection() {
        try {
            String accessToken = getAccessToken();
            return accessToken != null && !accessToken.isEmpty();
        } catch (Exception e) {
            Log.warn("Microsoft Entra connection test failed: " + e.getMessage());
            return false;
        }
    }

    @Override
    public String getAdapterName() {
        return "Microsoft Entra (Azure AD) Graph API";
    }

    /**
     * Get access token using client credentials flow.
     */
    private String getAccessToken() throws IdpSyncException {
        String tokenUrl = LOGIN_BASE + "/" + tenantId + "/oauth2/v2.0/token";

        try {
            Response response = httpClient.target(tokenUrl)
                .request(MediaType.APPLICATION_JSON)
                .post(Entity.form(
                    new jakarta.ws.rs.core.Form()
                        .param("grant_type", "client_credentials")
                        .param("client_id", clientId)
                        .param("client_secret", clientSecret)
                        .param("scope", "https://graph.microsoft.com/.default")
                ));

            if (response.getStatus() != 200) {
                throw new IdpSyncException("Failed to get access token: HTTP " + response.getStatus());
            }

            Map<String, Object> tokenResponse = response.readEntity(Map.class);
            return (String) tokenResponse.get("access_token");
        } catch (Exception e) {
            throw new IdpSyncException("Failed to authenticate with Microsoft Entra: " + e.getMessage(), e);
        }
    }

    /**
     * Get current app roles from the application.
     */
    private List<Map<String, Object>> getCurrentAppRoles(String accessToken) throws IdpSyncException {
        String appUrl = GRAPH_API_BASE + "/applications/" + applicationObjectId;

        try {
            Response response = httpClient.target(appUrl)
                .queryParam("$select", "appRoles")
                .request(MediaType.APPLICATION_JSON)
                .header("Authorization", "Bearer " + accessToken)
                .get();

            if (response.getStatus() != 200) {
                throw new IdpSyncException("Failed to get application: HTTP " + response.getStatus());
            }

            Map<String, Object> app = response.readEntity(Map.class);
            List<Map<String, Object>> appRoles = (List<Map<String, Object>>) app.get("appRoles");
            return appRoles != null ? appRoles : new ArrayList<>();
        } catch (IdpSyncException e) {
            throw e;
        } catch (Exception e) {
            throw new IdpSyncException("Failed to get current app roles: " + e.getMessage(), e);
        }
    }

    /**
     * Update the application's app roles.
     */
    private void updateApplicationAppRoles(List<Map<String, Object>> appRoles, String accessToken) throws IdpSyncException {
        String appUrl = GRAPH_API_BASE + "/applications/" + applicationObjectId;

        try {
            Map<String, Object> patch = new HashMap<>();
            patch.put("appRoles", appRoles);

            Response response = httpClient.target(appUrl)
                .request(MediaType.APPLICATION_JSON)
                .header("Authorization", "Bearer " + accessToken)
                .method("PATCH", Entity.json(patch));

            if (response.getStatus() != 204) {
                throw new IdpSyncException("Failed to update app roles: HTTP " + response.getStatus());
            }
        } catch (IdpSyncException e) {
            throw e;
        } catch (Exception e) {
            throw new IdpSyncException("Failed to update app roles: " + e.getMessage(), e);
        }
    }

    /**
     * Build Microsoft Entra App Role representation from RoleDefinition.
     */
    private Map<String, Object> buildAppRoleRepresentation(RoleDefinition role) {
        Map<String, Object> appRole = new HashMap<>();

        // Generate a stable UUID based on role name (so we can preserve ID across syncs)
        String roleId = UUID.nameUUIDFromBytes(role.toRoleString().getBytes()).toString();
        appRole.put("id", roleId);

        // Role value (used in token claims)
        appRole.put("value", role.toRoleString());

        // Display name (human-readable)
        String displayName = role.roleName().replace("-", " ");
        displayName = Character.toUpperCase(displayName.charAt(0)) + displayName.substring(1);
        appRole.put("displayName", role.application() + " - " + displayName);

        // Description (include permission count)
        String description = role.description() + " (" + role.permissions().size() + " permissions)";
        appRole.put("description", description);

        // Allowed member types (users and applications can have this role)
        appRole.put("allowedMemberTypes", List.of("User", "Application"));

        // Role is enabled
        appRole.put("isEnabled", true);

        return appRole;
    }
}
