package tech.flowcatalyst.sdk.client.resources;

import com.fasterxml.jackson.core.type.TypeReference;
import tech.flowcatalyst.sdk.client.FlowCatalystClient;
import tech.flowcatalyst.sdk.dto.ListResult;
import tech.flowcatalyst.sdk.dto.Role;

import java.util.List;
import java.util.Map;
import java.util.Set;

/**
 * Resource for managing roles.
 */
public class Roles {

    private final FlowCatalystClient client;

    public Roles(FlowCatalystClient client) {
        this.client = client;
    }

    /**
     * List all roles.
     */
    public ListResult<Role> list() {
        return list(Map.of());
    }

    /**
     * List roles with filters.
     */
    public ListResult<Role> list(Map<String, String> filters) {
        String query = filters.isEmpty() ? "" : "?" + buildQuery(filters);
        var response = client.request("GET", "/api/roles" + query, null,
            new TypeReference<Map<String, Object>>() {});

        @SuppressWarnings("unchecked")
        var items = (List<Map<String, Object>>) response.getOrDefault("items", List.of());
        var roles = items.stream()
            .map(this::mapRole)
            .toList();

        return ListResult.of(roles);
    }

    /**
     * Get a role by ID.
     */
    public Role get(String id) {
        var response = client.request("GET", "/api/roles/" + id, null,
            new TypeReference<Map<String, Object>>() {});
        return mapRole(response);
    }

    /**
     * Create a new role.
     */
    public Role create(CreateRoleRequest request) {
        var response = client.request("POST", "/api/roles", request,
            new TypeReference<Map<String, Object>>() {});
        return mapRole(response);
    }

    /**
     * Update a role.
     */
    public Role update(String id, UpdateRoleRequest request) {
        var response = client.request("PATCH", "/api/roles/" + id, request,
            new TypeReference<Map<String, Object>>() {});
        return mapRole(response);
    }

    /**
     * Delete a role.
     */
    public void delete(String id) {
        client.requestVoid("DELETE", "/api/roles/" + id, null);
    }

    /**
     * Sync roles for an application (SDK-managed roles).
     *
     * <p>This will create/update roles to match the provided list and optionally
     * remove roles not in the list.
     */
    public SyncResult sync(String applicationCode, List<SyncRoleDefinition> roles, boolean removeUnlisted) {
        var request = new SyncRolesRequest(applicationCode, roles, removeUnlisted);
        var response = client.request("POST", "/api/roles/sync", request,
            new TypeReference<Map<String, Object>>() {});

        @SuppressWarnings("unchecked")
        var created = (List<String>) response.getOrDefault("created", List.of());
        @SuppressWarnings("unchecked")
        var updated = (List<String>) response.getOrDefault("updated", List.of());
        @SuppressWarnings("unchecked")
        var deleted = (List<String>) response.getOrDefault("deleted", List.of());

        return new SyncResult(created, updated, deleted);
    }

    private Role mapRole(Map<String, Object> data) {
        return client.getObjectMapper().convertValue(data, Role.class);
    }

    private String buildQuery(Map<String, String> params) {
        return params.entrySet().stream()
            .map(e -> e.getKey() + "=" + e.getValue())
            .reduce((a, b) -> a + "&" + b)
            .orElse("");
    }

    // Request/Response DTOs

    public record CreateRoleRequest(
        String applicationId,
        String name,
        String displayName,
        String description,
        Set<String> permissions,
        boolean clientManaged
    ) {}

    public record UpdateRoleRequest(
        String displayName,
        String description,
        Set<String> permissions
    ) {}

    public record SyncRolesRequest(
        String applicationCode,
        List<SyncRoleDefinition> roles,
        boolean removeUnlisted
    ) {}

    public record SyncRoleDefinition(
        String name,
        String displayName,
        String description,
        Set<String> permissions
    ) {}

    public record SyncResult(
        List<String> created,
        List<String> updated,
        List<String> deleted
    ) {}
}
