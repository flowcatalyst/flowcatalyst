package tech.flowcatalyst.sdk.client.resources;

import com.fasterxml.jackson.core.type.TypeReference;
import tech.flowcatalyst.sdk.client.FlowCatalystClient;
import tech.flowcatalyst.sdk.dto.EventType;
import tech.flowcatalyst.sdk.dto.ListResult;
import tech.flowcatalyst.sdk.enums.SchemaType;

import java.util.List;
import java.util.Map;

/**
 * Resource for managing event types.
 */
public class EventTypes {

    private final FlowCatalystClient client;

    public EventTypes(FlowCatalystClient client) {
        this.client = client;
    }

    /**
     * List all event types with optional filters.
     */
    public ListResult<EventType> list() {
        return list(Map.of());
    }

    /**
     * List event types with filters.
     *
     * @param filters Optional filters: status, application, subdomain, aggregate
     */
    public ListResult<EventType> list(Map<String, String> filters) {
        String query = filters.isEmpty() ? "" : "?" + buildQuery(filters);
        var response = client.request("GET", "/api/event-types" + query, null,
            new TypeReference<Map<String, Object>>() {});

        @SuppressWarnings("unchecked")
        var items = (List<Map<String, Object>>) response.getOrDefault("items", List.of());
        var eventTypes = items.stream()
            .map(this::mapEventType)
            .toList();

        return ListResult.of(eventTypes);
    }

    /**
     * Get an event type by ID.
     */
    public EventType get(String id) {
        var response = client.request("GET", "/api/event-types/" + id, null,
            new TypeReference<Map<String, Object>>() {});
        return mapEventType(response);
    }

    /**
     * Create a new event type.
     */
    public EventType create(CreateEventTypeRequest request) {
        var response = client.request("POST", "/api/event-types", request,
            new TypeReference<Map<String, Object>>() {});
        return mapEventType(response);
    }

    /**
     * Update an event type.
     */
    public EventType update(String id, UpdateEventTypeRequest request) {
        var response = client.request("PATCH", "/api/event-types/" + id, request,
            new TypeReference<Map<String, Object>>() {});
        return mapEventType(response);
    }

    /**
     * Add a schema version to an event type.
     */
    public EventType addSchema(String id, AddSchemaRequest request) {
        var response = client.request("POST", "/api/event-types/" + id + "/schemas", request,
            new TypeReference<Map<String, Object>>() {});
        return mapEventType(response);
    }

    /**
     * Finalise a schema version (FINALISING -> CURRENT).
     */
    public EventType finaliseSchema(String id, String version) {
        var response = client.request("POST",
            "/api/event-types/" + id + "/schemas/" + version + "/finalise", null,
            new TypeReference<Map<String, Object>>() {});
        return mapEventType(response);
    }

    /**
     * Deprecate a schema version (CURRENT -> DEPRECATED).
     */
    public EventType deprecateSchema(String id, String version) {
        var response = client.request("POST",
            "/api/event-types/" + id + "/schemas/" + version + "/deprecate", null,
            new TypeReference<Map<String, Object>>() {});
        return mapEventType(response);
    }

    /**
     * Archive an event type.
     */
    public EventType archive(String id) {
        var response = client.request("POST", "/api/event-types/" + id + "/archive", null,
            new TypeReference<Map<String, Object>>() {});
        return mapEventType(response);
    }

    /**
     * Delete an event type.
     */
    public void delete(String id) {
        client.requestVoid("DELETE", "/api/event-types/" + id, null);
    }

    /**
     * Get distinct application names for filtering.
     */
    public List<String> filterApplications() {
        var response = client.request("GET", "/api/event-types/filters/applications", null,
            new TypeReference<Map<String, Object>>() {});

        @SuppressWarnings("unchecked")
        var apps = (List<String>) response.getOrDefault("applications", List.of());
        return apps;
    }

    /**
     * Get distinct subdomains for filtering.
     */
    public List<String> filterSubdomains(String application) {
        String query = application != null ? "?application=" + application : "";
        var response = client.request("GET", "/api/event-types/filters/subdomains" + query, null,
            new TypeReference<Map<String, Object>>() {});

        @SuppressWarnings("unchecked")
        var subdomains = (List<String>) response.getOrDefault("subdomains", List.of());
        return subdomains;
    }

    private EventType mapEventType(Map<String, Object> data) {
        return client.getObjectMapper().convertValue(data, EventType.class);
    }

    private String buildQuery(Map<String, String> params) {
        return params.entrySet().stream()
            .map(e -> e.getKey() + "=" + e.getValue())
            .reduce((a, b) -> a + "&" + b)
            .orElse("");
    }

    // Request DTOs

    public record CreateEventTypeRequest(
        String code,
        String name,
        String description
    ) {}

    public record UpdateEventTypeRequest(
        String name,
        String description
    ) {}

    public record AddSchemaRequest(
        String version,
        String mimeType,
        String schema,
        SchemaType schemaType
    ) {}
}
