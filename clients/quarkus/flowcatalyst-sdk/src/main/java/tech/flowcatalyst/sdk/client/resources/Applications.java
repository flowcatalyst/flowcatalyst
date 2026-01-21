package tech.flowcatalyst.sdk.client.resources;

import com.fasterxml.jackson.core.type.TypeReference;
import tech.flowcatalyst.sdk.client.FlowCatalystClient;
import tech.flowcatalyst.sdk.dto.Application;
import tech.flowcatalyst.sdk.dto.ListResult;

import java.util.List;
import java.util.Map;

/**
 * Resource for managing applications.
 */
public class Applications {

    private final FlowCatalystClient client;

    public Applications(FlowCatalystClient client) {
        this.client = client;
    }

    /**
     * List all applications.
     */
    public ListResult<Application> list() {
        var response = client.request("GET", "/api/applications", null,
            new TypeReference<Map<String, Object>>() {});

        @SuppressWarnings("unchecked")
        var items = (List<Map<String, Object>>) response.getOrDefault("items", List.of());
        var apps = items.stream()
            .map(this::mapApplication)
            .toList();

        return ListResult.of(apps);
    }

    /**
     * Get an application by ID.
     */
    public Application get(String id) {
        var response = client.request("GET", "/api/applications/" + id, null,
            new TypeReference<Map<String, Object>>() {});
        return mapApplication(response);
    }

    /**
     * Get an application by code.
     */
    public Application getByCode(String code) {
        var response = client.request("GET", "/api/applications/by-code/" + code, null,
            new TypeReference<Map<String, Object>>() {});
        return mapApplication(response);
    }

    /**
     * Create a new application.
     */
    public Application create(CreateApplicationRequest request) {
        var response = client.request("POST", "/api/applications", request,
            new TypeReference<Map<String, Object>>() {});
        return mapApplication(response);
    }

    /**
     * Update an application.
     */
    public Application update(String id, UpdateApplicationRequest request) {
        var response = client.request("PATCH", "/api/applications/" + id, request,
            new TypeReference<Map<String, Object>>() {});
        return mapApplication(response);
    }

    /**
     * Activate an application.
     */
    public Application activate(String id) {
        var response = client.request("POST", "/api/applications/" + id + "/activate", null,
            new TypeReference<Map<String, Object>>() {});
        return mapApplication(response);
    }

    /**
     * Deactivate an application.
     */
    public Application deactivate(String id) {
        var response = client.request("POST", "/api/applications/" + id + "/deactivate", null,
            new TypeReference<Map<String, Object>>() {});
        return mapApplication(response);
    }

    /**
     * Delete an application.
     */
    public void delete(String id) {
        client.requestVoid("DELETE", "/api/applications/" + id, null);
    }

    private Application mapApplication(Map<String, Object> data) {
        return client.getObjectMapper().convertValue(data, Application.class);
    }

    // Request DTOs

    public record CreateApplicationRequest(
        String code,
        String name,
        String description,
        String defaultBaseUrl,
        String iconUrl
    ) {}

    public record UpdateApplicationRequest(
        String name,
        String description,
        String defaultBaseUrl,
        String iconUrl
    ) {}
}
