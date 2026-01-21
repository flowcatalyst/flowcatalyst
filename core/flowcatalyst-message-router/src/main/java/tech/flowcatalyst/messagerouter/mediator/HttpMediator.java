package tech.flowcatalyst.messagerouter.mediator;

import com.fasterxml.jackson.databind.ObjectMapper;
import io.smallrye.faulttolerance.api.CircuitBreakerName;
import jakarta.annotation.PreDestroy;
import jakarta.enterprise.context.ApplicationScoped;
import org.eclipse.microprofile.faulttolerance.CircuitBreaker;
import org.jboss.logging.Logger;
import tech.flowcatalyst.messagerouter.model.MediationOutcome;
import tech.flowcatalyst.messagerouter.model.MediationResponse;
import tech.flowcatalyst.messagerouter.model.MediationType;
import tech.flowcatalyst.messagerouter.model.MessagePointer;
import tech.flowcatalyst.messagerouter.warning.WarningService;

import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.time.Duration;
import java.util.Optional;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;

@ApplicationScoped
public class HttpMediator implements Mediator {

    private static final Logger LOG = Logger.getLogger(HttpMediator.class);

    private final HttpClient httpClient;
    private final ExecutorService executorService;
    private final long timeoutMillis;
    private final WarningService warningService;
    private final ObjectMapper objectMapper = new ObjectMapper();

    public HttpMediator(
            @org.eclipse.microprofile.config.inject.ConfigProperty(name = "mediator.http.version", defaultValue = "HTTP_2") String httpVersion,
            @org.eclipse.microprofile.config.inject.ConfigProperty(name = "mediator.http.timeout.ms", defaultValue = "900000") long timeoutMillis,
            WarningService warningService) {
        this.executorService = Executors.newVirtualThreadPerTaskExecutor();
        this.timeoutMillis = timeoutMillis;
        this.warningService = warningService;

        HttpClient.Version version = "HTTP_1_1".equalsIgnoreCase(httpVersion)
            ? HttpClient.Version.HTTP_1_1
            : HttpClient.Version.HTTP_2;

        LOG.infof("Initializing HttpMediator with HTTP version: %s, timeout: %dms", version, timeoutMillis);

        this.httpClient = HttpClient.newBuilder()
            .version(version)
            .connectTimeout(Duration.ofSeconds(30))
            .executor(executorService)
            .build();
    }

    @PreDestroy
    void cleanup() {
        LOG.info("Shutting down HttpMediator executor service");
        executorService.shutdown();
        try {
            if (!executorService.awaitTermination(10, TimeUnit.SECONDS)) {
                LOG.warn("HttpMediator executor did not terminate within 10 seconds, forcing shutdown");
                executorService.shutdownNow();
            }
        } catch (InterruptedException e) {
            LOG.warn("Interrupted while shutting down HttpMediator executor");
            executorService.shutdownNow();
            Thread.currentThread().interrupt();
        }
    }

    @Override
    @CircuitBreaker(
        requestVolumeThreshold = 10,
        failureRatio = 0.5,
        delay = 5000,
        successThreshold = 3,
        failOn = {java.net.http.HttpTimeoutException.class, java.io.IOException.class}
    )
    @CircuitBreakerName("http-mediator")
    public MediationOutcome process(MessagePointer message) {
        final int maxRetries = 3;
        final long retryDelayMs = 1000;
        MediationError lastError = null;

        for (int attempt = 1; attempt <= maxRetries; attempt++) {
            MediationOutcome outcome = attemptProcess(message);

            // If successful or a permanent error (config error), return immediately
            if (outcome.result() == tech.flowcatalyst.messagerouter.model.MediationResult.SUCCESS ||
                outcome.result() == tech.flowcatalyst.messagerouter.model.MediationResult.ERROR_CONFIG) {
                return outcome;
            }

            // For retryable errors, check if we should retry
            lastError = outcome.error();
            boolean shouldRetry = lastError != null && lastError.isRetryable();

            if (!shouldRetry || attempt == maxRetries) {
                // All retries exhausted or non-retryable error
                if (attempt == maxRetries && shouldRetry) {
                    LOG.warnf("Message [%s] failed after %d attempts: %s - returning with error",
                        message.id(), maxRetries, lastError != null ? lastError.message() : "unknown");
                }
                return outcome;
            }

            // Retry with backoff
            long delayMs = retryDelayMs * attempt; // Simple backoff: 1s, 2s, 3s
            LOG.debugf("Message [%s] attempt %d failed (%s), retrying in %dms",
                message.id(), attempt, lastError != null ? lastError.message() : "unknown", delayMs);
            try {
                Thread.sleep(delayMs);
            } catch (InterruptedException ie) {
                Thread.currentThread().interrupt();
                LOG.warnf("Interrupted while retrying message: %s", message.id());
                return MediationOutcome.errorProcess(
                    new MediationError.NetworkError(ie)
                );
            }
        }

        // Should not reach here, but return last error if we do
        return MediationOutcome.errorProcess(lastError);
    }

    private MediationOutcome attemptProcess(MessagePointer message) {
        try {
            String payload = String.format("{\"messageId\":\"%s\"}", message.id());
            LOG.infof("HttpMediator: Attempting to process message [%s] via HTTP POST to [%s]",
                message.id(), message.mediationTarget());
            LOG.infof("HttpMediator: Payload: %s", payload);
            LOG.infof("HttpMediator: Authorization token (first 20 chars): %s...",
                message.authToken() != null && message.authToken().length() > 20 ?
                    message.authToken().substring(0, 20) : message.authToken());

            // Build HTTP request with configurable timeout (Content-Length set automatically by HttpClient)
            HttpRequest request = HttpRequest.newBuilder()
                .uri(URI.create(message.mediationTarget()))
                .header("Authorization", "Bearer " + message.authToken())
                .header("Content-Type", "application/json")
                .header("Accept", "application/json")
                .timeout(Duration.ofMillis(timeoutMillis))
                .POST(HttpRequest.BodyPublishers.ofString(payload))
                .build();

            LOG.infof("HttpMediator: HTTP request built. Sending to [%s] with HTTP version [%s], timeout: %dms",
                message.mediationTarget(), httpClient.version(), timeoutMillis);

            // Send request
            long sendStartTime = System.currentTimeMillis();
            HttpResponse<String> response = httpClient.send(request, HttpResponse.BodyHandlers.ofString());
            long sendDuration = System.currentTimeMillis() - sendStartTime;

            LOG.infof("HttpMediator: HTTP request completed in %dms. Status code: %d",
                sendDuration, response.statusCode());

            // Evaluate response
            int statusCode = response.statusCode();

            // Log full response details for debugging
            if (statusCode >= 400) {
                LOG.debugf("HTTP Response Status: %d, Body: %s", statusCode, response.body());
            }

            if (statusCode == 200) {
                // Parse response to check acknowledgment status
                try {
                    // Log raw response body for debugging
                    String responseBody = response.body();
                    LOG.infof("Message [%s] received 200 OK. Raw response body: %s", message.id(), responseBody);

                    MediationResponse mediationResponse = objectMapper.readValue(responseBody, MediationResponse.class);

                    LOG.infof("Message [%s] parsed MediationResponse - ack: %s, message: %s, delaySeconds: %s",
                        message.id(), mediationResponse.ack(), mediationResponse.message(), mediationResponse.delaySeconds());

                    if (mediationResponse.ack()) {
                        LOG.infof("Message [%s] processed successfully with ack=true - will ACK", message.id());
                        return MediationOutcome.success();
                    } else {
                        // ack=false means message is accepted but not ready to process yet (e.g., notBefore time not reached)
                        // Use the delay from the response if provided
                        Integer delaySeconds = mediationResponse.delaySeconds();
                        if (delaySeconds != null && delaySeconds > 0) {
                            LOG.warnf("Message [%s] received 200 OK but ack=false with delay=%ds - will NACK and retry after delay. Reason: %s",
                                message.id(), delaySeconds, mediationResponse.message());
                        } else {
                            LOG.warnf("Message [%s] received 200 OK but ack=false - will NACK and retry. Reason: %s",
                                message.id(), mediationResponse.message());
                        }
                        return MediationOutcome.errorProcess(delaySeconds);
                    }
                } catch (Exception e) {
                    // If response is not valid JSON or missing ack field, treat as success (backward compatibility)
                    LOG.warnf(e, "Message [%s] received 200 OK but response was not valid MediationResponse (parse error: %s). Response body: %s - treating as success and ACKing",
                        message.id(), e.getMessage(), response.body());
                    return MediationOutcome.success();
                }
            } else if (statusCode == 501) {
                // 501 Not Implemented - endpoint doesn't support this operation, should ACK to prevent retry
                LOG.errorf("Message [%s] failed with 501 Not Implemented - operation not supported at endpoint: %s",
                    message.id(), message.mediationTarget());
                warningService.addWarning(
                    "CONFIGURATION",
                    "CRITICAL",
                    String.format("Endpoint configuration error for message %s: HTTP 501 Not Implemented - Target: %s",
                        message.id(), message.mediationTarget()),
                    "HttpMediator"
                );
                return MediationOutcome.errorConfig(
                    new MediationError.HttpError(statusCode, response.body())
                );
            } else if (statusCode >= 500) {
                // 5xx Server errors (except 501) - transient infrastructure issues
                // Let SQS visibility timeout handle retries rather than quick retries
                LOG.warnf("Message [%s] failed with server error: %d - will be retried via queue visibility timeout", message.id(), statusCode);
                return MediationOutcome.errorProcess(
                    new MediationError.HttpError(statusCode, response.body())
                );
            } else if (statusCode == 400) {
                // 400 Bad Request - permanent configuration/data error, ACK to prevent retry
                // Extract reason from response body if available
                String reason = extractReasonFromResponse(response.body());
                LOG.errorf("Message [%s] failed with 400 Bad Request - configuration error: %s",
                    message.id(), reason);
                warningService.addWarning(
                    "CONFIGURATION",
                    "ERROR",
                    String.format("Bad request for message %s: HTTP 400 - %s - Target: %s",
                        message.id(), reason, message.mediationTarget()),
                    "HttpMediator"
                );
                return MediationOutcome.errorConfig(
                    new MediationError.HttpError(statusCode, response.body())
                );
            } else if (statusCode == 404) {
                // 404 Not Found - configuration error (endpoint doesn't exist at this URL)
                LOG.errorf("Message [%s] failed with 404 Not Found - configuration error: endpoint not found", message.id());
                warningService.addWarning(
                    "CONFIGURATION",
                    "ERROR",
                    String.format("Endpoint not found for message %s: HTTP 404 - Target: %s",
                        message.id(), message.mediationTarget()),
                    "HttpMediator"
                );
                return MediationOutcome.errorConfig(
                    new MediationError.HttpError(statusCode, response.body())
                );
            } else if (statusCode == 429) {
                // 429 Too Many Requests - rate limiting from target endpoint, NACK for retry
                // This is a transient error, not a configuration error
                Integer retryAfterSeconds = extractRetryAfterHeader(response);
                if (retryAfterSeconds != null) {
                    LOG.warnf("Message [%s] received 429 Too Many Requests with Retry-After=%ds - will NACK and retry after delay",
                        message.id(), retryAfterSeconds);
                } else {
                    // Default to 30 seconds if no Retry-After header
                    retryAfterSeconds = 30;
                    LOG.warnf("Message [%s] received 429 Too Many Requests (no Retry-After header) - will NACK and retry in %ds",
                        message.id(), retryAfterSeconds);
                }
                return MediationOutcome.errorProcess(
                    retryAfterSeconds,
                    new MediationError.RateLimited(Duration.ofSeconds(retryAfterSeconds))
                );
            } else if (statusCode >= 401 && statusCode < 500) {
                // All other 4xx errors indicate configuration problems:
                // 401 Unauthorized, 403 Forbidden, 405 Method Not Allowed, etc.
                // These are permanent errors, should ACK to prevent retry
                // (Note: 404 is handled separately above as a configuration error)
                String reason = extractReasonFromResponse(response.body());
                LOG.errorf("Message [%s] failed with %d %s - configuration error: %s",
                    message.id(), statusCode, getStatusDescription(statusCode), reason);
                warningService.addWarning(
                    "CONFIGURATION",
                    "ERROR",
                    String.format("Configuration error for message %s: HTTP %d %s - %s - Target: %s",
                        message.id(), statusCode, getStatusDescription(statusCode), reason, message.mediationTarget()),
                    "HttpMediator"
                );
                return MediationOutcome.errorConfig(
                    new MediationError.HttpError(statusCode, response.body())
                );
            } else {
                LOG.warnf("Message [%s] received unexpected status: %d - will be retried via queue visibility timeout", message.id(), statusCode);
                return MediationOutcome.errorProcess(
                    new MediationError.HttpError(statusCode, response.body())
                );
            }

        } catch (java.net.http.HttpConnectTimeoutException | java.net.ConnectException | java.nio.channels.UnresolvedAddressException e) {
            LOG.errorf(e, "Connection error processing message: %s", message.id());
            return MediationOutcome.errorConnection(
                new MediationError.NetworkError(e)
            );
        } catch (java.net.http.HttpTimeoutException e) {
            LOG.errorf(e, "Timeout processing message: %s", message.id());
            return MediationOutcome.errorProcess(
                new MediationError.Timeout(Duration.ofMillis(timeoutMillis))
            );
        } catch (java.io.IOException e) {
            // Catch other IO errors (like UnresolvedAddressException wrapped in IOException)
            LOG.errorf(e, "IO error processing message: %s", message.id());
            return MediationOutcome.errorConnection(
                new MediationError.NetworkError(e)
            );
        } catch (Exception e) {
            LOG.errorf(e, "Unexpected error processing message: %s", message.id());
            // Treat unexpected errors as transient - safer to retry than drop messages
            return MediationOutcome.errorProcess(
                new MediationError.NetworkError(e)
            );
        }
    }

    @Override
    public MediationType getMediationType() {
        return MediationType.HTTP;
    }

    /**
     * Get human-readable description for HTTP status codes
     */
    private String getStatusDescription(int statusCode) {
        return switch (statusCode) {
            case 401 -> "Unauthorized";
            case 402 -> "Payment Required";
            case 403 -> "Forbidden";
            case 404 -> "Not Found";
            case 405 -> "Method Not Allowed";
            case 406 -> "Not Acceptable";
            case 407 -> "Proxy Authentication Required";
            case 408 -> "Request Timeout";
            case 409 -> "Conflict";
            case 410 -> "Gone";
            case 411 -> "Length Required";
            case 412 -> "Precondition Failed";
            case 413 -> "Payload Too Large";
            case 414 -> "URI Too Long";
            case 415 -> "Unsupported Media Type";
            case 416 -> "Range Not Satisfiable";
            case 417 -> "Expectation Failed";
            case 418 -> "I'm a teapot";
            case 421 -> "Misdirected Request";
            case 422 -> "Unprocessable Entity";
            case 423 -> "Locked";
            case 424 -> "Failed Dependency";
            case 425 -> "Too Early";
            case 426 -> "Upgrade Required";
            case 428 -> "Precondition Required";
            case 429 -> "Too Many Requests";
            case 431 -> "Request Header Fields Too Large";
            case 451 -> "Unavailable For Legal Reasons";
            default -> "Client Error";
        };
    }

    /**
     * Extract Retry-After header value in seconds.
     * Supports both delta-seconds format (e.g., "120") and HTTP-date format.
     *
     * @param response the HTTP response
     * @return the retry delay in seconds, or null if not present or invalid
     */
    private Integer extractRetryAfterHeader(HttpResponse<String> response) {
        return response.headers()
            .firstValue("Retry-After")
            .map(value -> {
                try {
                    // Try to parse as integer (delta-seconds)
                    return Integer.parseInt(value);
                } catch (NumberFormatException e) {
                    // Could be HTTP-date format, but for simplicity we'll use default
                    LOG.debugf("Retry-After header '%s' is not in delta-seconds format, using default", value);
                    return null;
                }
            })
            .orElse(null);
    }

    /**
     * Extract reason/message from response body (typically JSON with "message" or "error" field)
     * Falls back to a default message if parsing fails
     */
    private String extractReasonFromResponse(String responseBody) {
        try {
            // Try to parse as JSON and extract message or error field
            if (responseBody != null && !responseBody.isEmpty()) {
                var jsonNode = objectMapper.readTree(responseBody);
                if (jsonNode.has("message")) {
                    return jsonNode.get("message").asText("Unknown error");
                } else if (jsonNode.has("error")) {
                    return jsonNode.get("error").asText("Unknown error");
                } else if (jsonNode.has("reason")) {
                    return jsonNode.get("reason").asText("Unknown error");
                }
            }
        } catch (Exception e) {
            // Ignore parsing errors, return generic message
        }
        return responseBody != null && !responseBody.isEmpty() ? responseBody : "Unknown error";
    }

}
