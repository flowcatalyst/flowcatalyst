package tech.flowcatalyst.sdk.client.resources;

import com.fasterxml.jackson.core.type.TypeReference;
import tech.flowcatalyst.sdk.client.FlowCatalystClient;
import tech.flowcatalyst.sdk.dto.ListResult;
import tech.flowcatalyst.sdk.dto.Permission;

import java.util.List;
import java.util.Map;

/**
 * Resource for managing permissions.
 */
public class Permissions {

    private final FlowCatalystClient client;

    public Permissions(FlowCatalystClient client) {
        this.client = client;
    }

    /**
     * List all permissions.
     */
    public ListResult<Permission> list() {
        return list(Map.of());
    }

    /**
     * List permissions with filters.
     */
    public ListResult<Permission> list(Map<String, String> filters) {
        String query = filters.isEmpty() ? "" : "?" + buildQuery(filters);
        var response = client.request("GET", "/api/permissions" + query, null,
            new TypeReference<Map<String, Object>>() {});

        @SuppressWarnings("unchecked")
        var items = (List<Map<String, Object>>) response.getOrDefault("items", List.of());
        var permissions = items.stream()
            .map(this::mapPermission)
            .toList();

        return ListResult.of(permissions);
    }

    private Permission mapPermission(Map<String, Object> data) {
        return client.getObjectMapper().convertValue(data, Permission.class);
    }

    private String buildQuery(Map<String, String> params) {
        return params.entrySet().stream()
            .map(e -> e.getKey() + "=" + e.getValue())
            .reduce((a, b) -> a + "&" + b)
            .orElse("");
    }
}
