package tech.flowcatalyst.outbox.api;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import org.jboss.logging.Logger;
import tech.flowcatalyst.outbox.config.OutboxProcessorConfig;
import tech.flowcatalyst.outbox.model.OutboxItem;
import tech.flowcatalyst.outbox.model.OutboxStatus;

import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;
import java.util.List;
import java.util.concurrent.Executors;

/**
 * HTTP client for FlowCatalyst batch APIs.
 * Uses Java 21 HttpClient with virtual threads for efficient concurrency.
 * Returns per-item results with appropriate status codes.
 */
@ApplicationScoped
public class FlowCatalystApiClient {

    private static final Logger LOG = Logger.getLogger(FlowCatalystApiClient.class);

    @Inject
    OutboxProcessorConfig config;

    @Inject
    ObjectMapper objectMapper;

    private final HttpClient httpClient;

    public FlowCatalystApiClient() {
        this.httpClient = HttpClient.newBuilder()
            .executor(Executors.newVirtualThreadPerTaskExecutor())
            .connectTimeout(Duration.ofSeconds(10))
            .build();
    }

    /**
     * Send a batch of events to FlowCatalyst.
     *
     * @param items List of outbox items containing event payloads
     * @return BatchResult with per-item status information
     */
    public BatchResult createEventsBatch(List<OutboxItem> items) {
        List<JsonNode> payloads = items.stream()
            .map(this::parsePayload)
            .toList();

        List<String> ids = items.stream().map(OutboxItem::id).toList();
        return post("/api/events/batch", payloads, ids);
    }

    /**
     * Send a batch of dispatch jobs to FlowCatalyst.
     *
     * @param items List of outbox items containing dispatch job payloads
     * @return BatchResult with per-item status information
     */
    public BatchResult createDispatchJobsBatch(List<OutboxItem> items) {
        List<JsonNode> payloads = items.stream()
            .map(this::parsePayload)
            .toList();

        List<String> ids = items.stream().map(OutboxItem::id).toList();
        return post("/api/dispatch/jobs/batch", payloads, ids);
    }

    private BatchResult post(String path, Object body, List<String> ids) {
        String json;
        try {
            json = objectMapper.writeValueAsString(body);
        } catch (JsonProcessingException e) {
            LOG.errorf(e, "Failed to serialize request body");
            return BatchResult.allFailed(ids, OutboxStatus.INTERNAL_ERROR, "Failed to serialize request: " + e.getMessage());
        }

        String url = config.apiBaseUrl() + path;

        HttpRequest.Builder requestBuilder = HttpRequest.newBuilder()
            .uri(URI.create(url))
            .header("Content-Type", "application/json")
            .timeout(Duration.ofSeconds(30))
            .POST(HttpRequest.BodyPublishers.ofString(json));

        // Add authorization header if token is configured
        config.apiToken().ifPresent(token ->
            requestBuilder.header("Authorization", "Bearer " + token));

        HttpRequest request = requestBuilder.build();

        try {
            HttpResponse<String> response = httpClient.send(request, HttpResponse.BodyHandlers.ofString());
            int statusCode = response.statusCode();

            if (statusCode >= 200 && statusCode < 300) {
                LOG.debugf("POST %s returned %d", path, statusCode);
                return BatchResult.allSuccess(ids.size());
            }

            // Map HTTP status codes to OutboxStatus
            OutboxStatus status = OutboxStatus.fromHttpCode(statusCode);
            String errorMessage = "API error: " + statusCode + " - " + response.body();
            LOG.errorf("API error: POST %s returned %d: %s", path, statusCode, response.body());

            return BatchResult.allFailed(ids, status, errorMessage);

        } catch (java.net.http.HttpTimeoutException e) {
            LOG.errorf(e, "Timeout calling API: POST %s", path);
            return BatchResult.allFailed(ids, OutboxStatus.GATEWAY_ERROR, "Request timeout: " + e.getMessage());
        } catch (java.io.IOException e) {
            LOG.errorf(e, "IO error calling API: POST %s", path);
            return BatchResult.allFailed(ids, OutboxStatus.GATEWAY_ERROR, "IO error: " + e.getMessage());
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
            LOG.errorf(e, "Request interrupted: POST %s", path);
            return BatchResult.allFailed(ids, OutboxStatus.INTERNAL_ERROR, "Request interrupted");
        } catch (Exception e) {
            LOG.errorf(e, "Failed to call API: POST %s", path);
            return BatchResult.allFailed(ids, OutboxStatus.INTERNAL_ERROR, "Failed to call API: " + e.getMessage());
        }
    }

    private JsonNode parsePayload(OutboxItem item) {
        try {
            return objectMapper.readTree(item.payload());
        } catch (JsonProcessingException e) {
            throw new RuntimeException("Invalid JSON payload for item " + item.id(), e);
        }
    }
}
