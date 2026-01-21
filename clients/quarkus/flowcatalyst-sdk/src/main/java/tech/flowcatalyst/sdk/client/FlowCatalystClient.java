package tech.flowcatalyst.sdk.client;

import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.DeserializationFeature;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.datatype.jsr310.JavaTimeModule;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.sdk.client.auth.OidcTokenManager;
import tech.flowcatalyst.sdk.client.resources.*;
import tech.flowcatalyst.sdk.config.FlowCatalystConfig;
import tech.flowcatalyst.sdk.exception.AuthenticationException;
import tech.flowcatalyst.sdk.exception.FlowCatalystException;
import tech.flowcatalyst.sdk.exception.ValidationException;

import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;
import java.util.Map;

/**
 * Main client for the FlowCatalyst API.
 *
 * <p>Provides access to all FlowCatalyst control plane APIs using OIDC client credentials.
 *
 * <p>Example usage:
 * <pre>{@code
 * @Inject
 * FlowCatalystClient client;
 *
 * // List event types
 * var result = client.eventTypes().list();
 *
 * // Create a subscription
 * var subscription = client.subscriptions().create(createRequest);
 * }</pre>
 */
@ApplicationScoped
public class FlowCatalystClient {

    private final FlowCatalystConfig config;
    private final OidcTokenManager tokenManager;
    private final HttpClient httpClient;
    private final ObjectMapper objectMapper;

    private EventTypes eventTypes;
    private Subscriptions subscriptions;
    private DispatchPools dispatchPools;
    private Roles roles;
    private Permissions permissions;
    private Applications applications;

    @Inject
    public FlowCatalystClient(FlowCatalystConfig config, OidcTokenManager tokenManager) {
        this.config = config;
        this.tokenManager = tokenManager;
        this.httpClient = HttpClient.newBuilder()
            .connectTimeout(Duration.ofSeconds(config.http().timeout()))
            .build();
        this.objectMapper = new ObjectMapper()
            .registerModule(new JavaTimeModule())
            .configure(DeserializationFeature.FAIL_ON_UNKNOWN_PROPERTIES, false);
    }

    /**
     * Get the Event Types resource.
     */
    public EventTypes eventTypes() {
        if (eventTypes == null) {
            eventTypes = new EventTypes(this);
        }
        return eventTypes;
    }

    /**
     * Get the Subscriptions resource.
     */
    public Subscriptions subscriptions() {
        if (subscriptions == null) {
            subscriptions = new Subscriptions(this);
        }
        return subscriptions;
    }

    /**
     * Get the Dispatch Pools resource.
     */
    public DispatchPools dispatchPools() {
        if (dispatchPools == null) {
            dispatchPools = new DispatchPools(this);
        }
        return dispatchPools;
    }

    /**
     * Get the Roles resource.
     */
    public Roles roles() {
        if (roles == null) {
            roles = new Roles(this);
        }
        return roles;
    }

    /**
     * Get the Permissions resource.
     */
    public Permissions permissions() {
        if (permissions == null) {
            permissions = new Permissions(this);
        }
        return permissions;
    }

    /**
     * Get the Applications resource.
     */
    public Applications applications() {
        if (applications == null) {
            applications = new Applications(this);
        }
        return applications;
    }

    /**
     * Make an authenticated API request.
     */
    public <T> T request(String method, String endpoint, Object body, TypeReference<T> responseType) {
        int attempts = 0;
        FlowCatalystException lastException = null;

        while (attempts < config.http().retryAttempts()) {
            try {
                return doRequest(method, endpoint, body, responseType, attempts > 0);
            } catch (AuthenticationException | ValidationException e) {
                throw e;
            } catch (FlowCatalystException e) {
                lastException = e;
                attempts++;
                if (attempts < config.http().retryAttempts()) {
                    sleep(config.http().retryDelay() * attempts);
                }
            }
        }

        throw lastException != null ? lastException : new FlowCatalystException("Request failed");
    }

    /**
     * Make an authenticated API request without a response body.
     */
    public void requestVoid(String method, String endpoint, Object body) {
        request(method, endpoint, body, new TypeReference<Map<String, Object>>() {});
    }

    private <T> T doRequest(String method, String endpoint, Object body,
                            TypeReference<T> responseType, boolean isRetry) {
        try {
            String token = isRetry ? tokenManager.refreshToken() : tokenManager.getAccessToken();

            String url = config.baseUrl().replaceAll("/$", "") + endpoint;

            HttpRequest.Builder requestBuilder = HttpRequest.newBuilder()
                .uri(URI.create(url))
                .header("Authorization", "Bearer " + token)
                .header("Accept", "application/json")
                .header("Content-Type", "application/json")
                .timeout(Duration.ofSeconds(config.http().timeout()));

            if (body != null) {
                String jsonBody = objectMapper.writeValueAsString(body);
                requestBuilder.method(method, HttpRequest.BodyPublishers.ofString(jsonBody));
            } else {
                requestBuilder.method(method, HttpRequest.BodyPublishers.noBody());
            }

            HttpResponse<String> response = httpClient.send(
                requestBuilder.build(),
                HttpResponse.BodyHandlers.ofString()
            );

            return handleResponse(response, responseType);
        } catch (FlowCatalystException e) {
            throw e;
        } catch (Exception e) {
            throw new FlowCatalystException("Request failed: " + e.getMessage(), e);
        }
    }

    @SuppressWarnings("unchecked")
    private <T> T handleResponse(HttpResponse<String> response, TypeReference<T> responseType) {
        int status = response.statusCode();
        String body = response.body();

        try {
            Map<String, Object> data = body != null && !body.isBlank()
                ? objectMapper.readValue(body, new TypeReference<>() {})
                : Map.of();

            if (status == 401) {
                throw AuthenticationException.tokenExpired();
            }

            if (status == 403) {
                throw new FlowCatalystException(
                    (String) data.getOrDefault("error", "Access forbidden"),
                    403, null, data
                );
            }

            if (status == 404) {
                throw new FlowCatalystException(
                    (String) data.getOrDefault("error", "Resource not found"),
                    404, null, data
                );
            }

            if (status == 422) {
                throw ValidationException.fromResponse(data);
            }

            if (status >= 400 && status < 500) {
                throw new FlowCatalystException(
                    (String) data.getOrDefault("error", "Client error: " + status),
                    status, null, data
                );
            }

            if (status >= 500) {
                throw new FlowCatalystException(
                    (String) data.getOrDefault("error", "Server error: " + status),
                    status, null, data
                );
            }

            if (body == null || body.isBlank()) {
                return null;
            }

            return objectMapper.readValue(body, responseType);
        } catch (FlowCatalystException e) {
            throw e;
        } catch (Exception e) {
            throw new FlowCatalystException("Failed to parse response", e);
        }
    }

    private void sleep(int millis) {
        try {
            Thread.sleep(millis);
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
        }
    }

    public String getBaseUrl() {
        return config.baseUrl();
    }

    public ObjectMapper getObjectMapper() {
        return objectMapper;
    }
}
