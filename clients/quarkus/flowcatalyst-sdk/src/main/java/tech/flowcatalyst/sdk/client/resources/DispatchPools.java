package tech.flowcatalyst.sdk.client.resources;

import com.fasterxml.jackson.core.type.TypeReference;
import tech.flowcatalyst.sdk.client.FlowCatalystClient;
import tech.flowcatalyst.sdk.dto.DispatchPool;
import tech.flowcatalyst.sdk.dto.ListResult;

import java.util.List;
import java.util.Map;

/**
 * Resource for managing dispatch pools.
 */
public class DispatchPools {

    private final FlowCatalystClient client;

    public DispatchPools(FlowCatalystClient client) {
        this.client = client;
    }

    /**
     * List all dispatch pools.
     */
    public ListResult<DispatchPool> list() {
        var response = client.request("GET", "/api/dispatch-pools", null,
            new TypeReference<Map<String, Object>>() {});

        @SuppressWarnings("unchecked")
        var items = (List<Map<String, Object>>) response.getOrDefault("items", List.of());
        var pools = items.stream()
            .map(this::mapDispatchPool)
            .toList();

        return ListResult.of(pools);
    }

    /**
     * Get a dispatch pool by ID.
     */
    public DispatchPool get(String id) {
        var response = client.request("GET", "/api/dispatch-pools/" + id, null,
            new TypeReference<Map<String, Object>>() {});
        return mapDispatchPool(response);
    }

    /**
     * Create a new dispatch pool.
     */
    public DispatchPool create(CreateDispatchPoolRequest request) {
        var response = client.request("POST", "/api/dispatch-pools", request,
            new TypeReference<Map<String, Object>>() {});
        return mapDispatchPool(response);
    }

    /**
     * Update a dispatch pool.
     */
    public DispatchPool update(String id, UpdateDispatchPoolRequest request) {
        var response = client.request("PATCH", "/api/dispatch-pools/" + id, request,
            new TypeReference<Map<String, Object>>() {});
        return mapDispatchPool(response);
    }

    /**
     * Suspend a dispatch pool.
     */
    public DispatchPool suspend(String id) {
        var response = client.request("POST", "/api/dispatch-pools/" + id + "/suspend", null,
            new TypeReference<Map<String, Object>>() {});
        return mapDispatchPool(response);
    }

    /**
     * Activate a suspended dispatch pool.
     */
    public DispatchPool activate(String id) {
        var response = client.request("POST", "/api/dispatch-pools/" + id + "/activate", null,
            new TypeReference<Map<String, Object>>() {});
        return mapDispatchPool(response);
    }

    /**
     * Delete a dispatch pool.
     */
    public void delete(String id) {
        client.requestVoid("DELETE", "/api/dispatch-pools/" + id, null);
    }

    private DispatchPool mapDispatchPool(Map<String, Object> data) {
        return client.getObjectMapper().convertValue(data, DispatchPool.class);
    }

    // Request DTOs

    public record CreateDispatchPoolRequest(
        String code,
        String name,
        String description,
        int rateLimit,
        int concurrency
    ) {
        public static Builder builder() {
            return new Builder();
        }

        public static class Builder {
            private String code;
            private String name;
            private String description;
            private int rateLimit = 100;
            private int concurrency = 10;

            public Builder code(String code) { this.code = code; return this; }
            public Builder name(String name) { this.name = name; return this; }
            public Builder description(String description) { this.description = description; return this; }
            public Builder rateLimit(int rateLimit) { this.rateLimit = rateLimit; return this; }
            public Builder concurrency(int concurrency) { this.concurrency = concurrency; return this; }

            public CreateDispatchPoolRequest build() {
                return new CreateDispatchPoolRequest(code, name, description, rateLimit, concurrency);
            }
        }
    }

    public record UpdateDispatchPoolRequest(
        String name,
        String description,
        Integer rateLimit,
        Integer concurrency
    ) {}
}
