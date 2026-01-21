package tech.flowcatalyst.sdk.client.resources;

import com.fasterxml.jackson.core.type.TypeReference;
import tech.flowcatalyst.sdk.client.FlowCatalystClient;
import tech.flowcatalyst.sdk.dto.EventTypeBinding;
import tech.flowcatalyst.sdk.dto.ListResult;
import tech.flowcatalyst.sdk.dto.Subscription;
import tech.flowcatalyst.sdk.enums.DispatchMode;

import java.util.List;
import java.util.Map;

/**
 * Resource for managing subscriptions.
 */
public class Subscriptions {

    private final FlowCatalystClient client;

    public Subscriptions(FlowCatalystClient client) {
        this.client = client;
    }

    /**
     * List all subscriptions.
     */
    public ListResult<Subscription> list() {
        return list(Map.of());
    }

    /**
     * List subscriptions with filters.
     */
    public ListResult<Subscription> list(Map<String, String> filters) {
        String query = filters.isEmpty() ? "" : "?" + buildQuery(filters);
        var response = client.request("GET", "/api/subscriptions" + query, null,
            new TypeReference<Map<String, Object>>() {});

        @SuppressWarnings("unchecked")
        var items = (List<Map<String, Object>>) response.getOrDefault("items", List.of());
        var subscriptions = items.stream()
            .map(this::mapSubscription)
            .toList();

        return ListResult.of(subscriptions);
    }

    /**
     * Get a subscription by ID.
     */
    public Subscription get(String id) {
        var response = client.request("GET", "/api/subscriptions/" + id, null,
            new TypeReference<Map<String, Object>>() {});
        return mapSubscription(response);
    }

    /**
     * Create a new subscription.
     */
    public Subscription create(CreateSubscriptionRequest request) {
        var response = client.request("POST", "/api/subscriptions", request,
            new TypeReference<Map<String, Object>>() {});
        return mapSubscription(response);
    }

    /**
     * Update a subscription.
     */
    public Subscription update(String id, UpdateSubscriptionRequest request) {
        var response = client.request("PATCH", "/api/subscriptions/" + id, request,
            new TypeReference<Map<String, Object>>() {});
        return mapSubscription(response);
    }

    /**
     * Pause a subscription.
     */
    public Subscription pause(String id) {
        var response = client.request("POST", "/api/subscriptions/" + id + "/pause", null,
            new TypeReference<Map<String, Object>>() {});
        return mapSubscription(response);
    }

    /**
     * Resume a paused subscription.
     */
    public Subscription resume(String id) {
        var response = client.request("POST", "/api/subscriptions/" + id + "/resume", null,
            new TypeReference<Map<String, Object>>() {});
        return mapSubscription(response);
    }

    /**
     * Delete a subscription.
     */
    public void delete(String id) {
        client.requestVoid("DELETE", "/api/subscriptions/" + id, null);
    }

    private Subscription mapSubscription(Map<String, Object> data) {
        return client.getObjectMapper().convertValue(data, Subscription.class);
    }

    private String buildQuery(Map<String, String> params) {
        return params.entrySet().stream()
            .map(e -> e.getKey() + "=" + e.getValue())
            .reduce((a, b) -> a + "&" + b)
            .orElse("");
    }

    // Request DTOs

    public record CreateSubscriptionRequest(
        String code,
        String name,
        String description,
        List<EventTypeBinding> eventTypes,
        String target,
        String queue,
        String dispatchPoolId,
        DispatchMode mode,
        int timeoutSeconds,
        int maxRetries,
        int delaySeconds,
        int sequence,
        int maxAgeSeconds,
        boolean dataOnly
    ) {
        public static Builder builder() {
            return new Builder();
        }

        public static class Builder {
            private String code;
            private String name;
            private String description;
            private List<EventTypeBinding> eventTypes = List.of();
            private String target;
            private String queue = "default";
            private String dispatchPoolId;
            private DispatchMode mode = DispatchMode.IMMEDIATE;
            private int timeoutSeconds = 30;
            private int maxRetries = 3;
            private int delaySeconds = 0;
            private int sequence = 99;
            private int maxAgeSeconds = 86400;
            private boolean dataOnly = true;

            public Builder code(String code) { this.code = code; return this; }
            public Builder name(String name) { this.name = name; return this; }
            public Builder description(String description) { this.description = description; return this; }
            public Builder eventTypes(List<EventTypeBinding> eventTypes) { this.eventTypes = eventTypes; return this; }
            public Builder target(String target) { this.target = target; return this; }
            public Builder queue(String queue) { this.queue = queue; return this; }
            public Builder dispatchPoolId(String dispatchPoolId) { this.dispatchPoolId = dispatchPoolId; return this; }
            public Builder mode(DispatchMode mode) { this.mode = mode; return this; }
            public Builder timeoutSeconds(int timeoutSeconds) { this.timeoutSeconds = timeoutSeconds; return this; }
            public Builder maxRetries(int maxRetries) { this.maxRetries = maxRetries; return this; }
            public Builder delaySeconds(int delaySeconds) { this.delaySeconds = delaySeconds; return this; }
            public Builder sequence(int sequence) { this.sequence = sequence; return this; }
            public Builder maxAgeSeconds(int maxAgeSeconds) { this.maxAgeSeconds = maxAgeSeconds; return this; }
            public Builder dataOnly(boolean dataOnly) { this.dataOnly = dataOnly; return this; }

            public CreateSubscriptionRequest build() {
                return new CreateSubscriptionRequest(code, name, description, eventTypes, target,
                    queue, dispatchPoolId, mode, timeoutSeconds, maxRetries, delaySeconds,
                    sequence, maxAgeSeconds, dataOnly);
            }
        }
    }

    public record UpdateSubscriptionRequest(
        String name,
        String description,
        List<EventTypeBinding> eventTypes,
        String target,
        DispatchMode mode,
        int timeoutSeconds,
        int maxRetries
    ) {}
}
