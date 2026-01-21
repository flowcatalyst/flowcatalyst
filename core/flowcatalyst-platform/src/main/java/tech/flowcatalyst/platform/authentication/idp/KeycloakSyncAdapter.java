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
 * Keycloak IDP sync adapter.
 *
 * Syncs roles and permissions to Keycloak via the Admin REST API.
 * Uses client credentials flow for authentication.
 *
 * Keycloak role mapping:
 * - Each RoleDefinition becomes a Keycloak realm role
 * - Role name: "{subdomain}:{role-name}" (e.g., "platform:tenant-admin")
 * - Role description: From RoleDefinition.description
 * - Permissions are stored in role attributes (Keycloak doesn't have first-class permissions)
 *
 * Configuration required:
 * - keycloak.admin.url: Keycloak admin API URL (e.g., "http://localhost:8080")
 * - keycloak.admin.realm: Realm to sync to (e.g., "flowcatalyst")
 * - keycloak.admin.client-id: Admin client ID
 * - keycloak.admin.client-secret: Admin client secret
 */
public class KeycloakSyncAdapter implements IdpSyncAdapter {

    private final String adminUrl;
    private final String realm;
    private final String clientId;
    private final String clientSecret;
    private final Client httpClient;

    public KeycloakSyncAdapter(String adminUrl, String realm, String clientId, String clientSecret) {
        this.adminUrl = adminUrl;
        this.realm = realm;
        this.clientId = clientId;
        this.clientSecret = clientSecret;
        this.httpClient = ClientBuilder.newClient();
    }

    @Override
    public void syncRolesToIdp(Set<RoleDefinition> roles) throws IdpSyncException {
        Log.info("Syncing " + roles.size() + " roles to Keycloak realm: " + realm);

        try {
            String accessToken = getAccessToken();

            for (RoleDefinition role : roles) {
                syncRole(role, accessToken);
            }

            Log.info("Successfully synced " + roles.size() + " roles to Keycloak");
        } catch (Exception e) {
            throw new IdpSyncException("Failed to sync roles to Keycloak: " + e.getMessage(), e);
        }
    }

    @Override
    public void syncPermissionsToIdp(Set<PermissionDefinition> permissions) throws IdpSyncException {
        // Keycloak doesn't have first-class permissions
        // Permissions are embedded in roles, so this is a no-op
        Log.debug("Keycloak doesn't support standalone permissions - they are embedded in roles");
    }

    @Override
    public String getIdpType() {
        return "KEYCLOAK";
    }

    @Override
    public boolean testConnection() {
        try {
            String accessToken = getAccessToken();
            return accessToken != null && !accessToken.isEmpty();
        } catch (Exception e) {
            Log.warn("Keycloak connection test failed: " + e.getMessage());
            return false;
        }
    }

    @Override
    public String getAdapterName() {
        return "Keycloak Admin API";
    }

    /**
     * Get access token using client credentials flow.
     */
    private String getAccessToken() throws IdpSyncException {
        String tokenUrl = adminUrl + "/realms/" + realm + "/protocol/openid-connect/token";

        try {
            Response response = httpClient.target(tokenUrl)
                .request(MediaType.APPLICATION_JSON)
                .post(Entity.form(
                    new jakarta.ws.rs.core.Form()
                        .param("grant_type", "client_credentials")
                        .param("client_id", clientId)
                        .param("client_secret", clientSecret)
                ));

            if (response.getStatus() != 200) {
                throw new IdpSyncException("Failed to get access token: HTTP " + response.getStatus());
            }

            Map<String, Object> tokenResponse = response.readEntity(Map.class);
            return (String) tokenResponse.get("access_token");
        } catch (Exception e) {
            throw new IdpSyncException("Failed to authenticate with Keycloak: " + e.getMessage(), e);
        }
    }

    /**
     * Sync a single role to Keycloak.
     * Creates the role if it doesn't exist, updates if it does.
     */
    private void syncRole(RoleDefinition role, String accessToken) throws IdpSyncException {
        String roleName = role.toRoleString();
        String rolesUrl = adminUrl + "/admin/realms/" + realm + "/roles";

        try {
            // Check if role exists
            Response getResponse = httpClient.target(rolesUrl + "/" + roleName)
                .request(MediaType.APPLICATION_JSON)
                .header("Authorization", "Bearer " + accessToken)
                .get();

            Map<String, Object> roleRepresentation = buildRoleRepresentation(role);

            if (getResponse.getStatus() == 404) {
                // Role doesn't exist - create it
                Response createResponse = httpClient.target(rolesUrl)
                    .request(MediaType.APPLICATION_JSON)
                    .header("Authorization", "Bearer " + accessToken)
                    .post(Entity.json(roleRepresentation));

                if (createResponse.getStatus() != 201) {
                    throw new IdpSyncException("Failed to create role " + roleName + ": HTTP " + createResponse.getStatus());
                }

                Log.debug("Created Keycloak role: " + roleName);
            } else if (getResponse.getStatus() == 200) {
                // Role exists - update it
                Response updateResponse = httpClient.target(rolesUrl + "/" + roleName)
                    .request(MediaType.APPLICATION_JSON)
                    .header("Authorization", "Bearer " + accessToken)
                    .put(Entity.json(roleRepresentation));

                if (updateResponse.getStatus() != 204) {
                    throw new IdpSyncException("Failed to update role " + roleName + ": HTTP " + updateResponse.getStatus());
                }

                Log.debug("Updated Keycloak role: " + roleName);
            } else {
                throw new IdpSyncException("Unexpected response checking role " + roleName + ": HTTP " + getResponse.getStatus());
            }
        } catch (IdpSyncException e) {
            throw e;
        } catch (Exception e) {
            throw new IdpSyncException("Failed to sync role " + roleName + ": " + e.getMessage(), e);
        }
    }

    /**
     * Build Keycloak role representation from RoleDefinition.
     */
    private Map<String, Object> buildRoleRepresentation(RoleDefinition role) {
        Map<String, Object> representation = new HashMap<>();
        representation.put("name", role.toRoleString());
        representation.put("description", role.description());

        // Store permissions as role attributes (Keycloak doesn't have first-class permissions)
        Map<String, List<String>> attributes = new HashMap<>();
        attributes.put("permissions", new ArrayList<>(role.permissionStrings()));
        attributes.put("application", List.of(role.application()));
        representation.put("attributes", attributes);

        return representation;
    }
}
