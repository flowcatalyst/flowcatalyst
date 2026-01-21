package tech.flowcatalyst.dispatchjob.model;

import com.fasterxml.jackson.annotation.JsonIgnoreProperties;
import com.fasterxml.jackson.annotation.JsonProperty;

/**
 * Response expected from a webhook target endpoint.
 *
 * <p>When FlowCatalyst dispatches a webhook to a customer's endpoint, the endpoint
 * should return this response format to indicate the processing result.</p>
 *
 * <h2>Response Format</h2>
 * <pre>{@code
 * {
 *   "status": "SUCCESS",
 *   "message": "Order processed successfully"
 * }
 * }</pre>
 *
 * <p>On error:</p>
 * <pre>{@code
 * {
 *   "status": "ERROR",
 *   "message": "Failed to process order",
 *   "errorDescription": "Database connection failed",
 *   "errorDetails": "Connection timeout after 30s to db.example.com:5432"
 * }
 * }</pre>
 *
 * <h2>Status Values</h2>
 * <ul>
 *   <li><b>SUCCESS</b> - Webhook processed successfully</li>
 *   <li><b>ERROR</b> - Webhook processing failed</li>
 * </ul>
 *
 * <p>Note: The HTTP status code also matters:
 * <ul>
 *   <li>2xx with SUCCESS status = Complete success</li>
 *   <li>4xx = Permanent error (won't retry)</li>
 *   <li>5xx = Transient error (will retry)</li>
 * </ul>
 */
@JsonIgnoreProperties(ignoreUnknown = true)
public record WebhookTargetResponse(
    /**
     * Processing status: SUCCESS or ERROR.
     */
    @JsonProperty("status")
    Status status,

    /**
     * Human-readable message (max 255 chars).
     */
    @JsonProperty("message")
    String message,

    /**
     * Brief error description when status is ERROR (max 255 chars).
     * Example: "Database connection failed"
     */
    @JsonProperty("errorDescription")
    String errorDescription,

    /**
     * Detailed error information when status is ERROR.
     * Example: Stack trace, detailed error message, etc.
     */
    @JsonProperty("errorDetails")
    String errorDetails
) {
    /**
     * Status of webhook processing at the target endpoint.
     */
    public enum Status {
        /**
         * Webhook was processed successfully.
         */
        SUCCESS,

        /**
         * Webhook processing failed.
         */
        ERROR
    }

    /**
     * Check if the response indicates success.
     */
    public boolean isSuccess() {
        return status == Status.SUCCESS;
    }

    /**
     * Check if the response indicates an error.
     */
    public boolean isError() {
        return status == Status.ERROR;
    }

    /**
     * Create a success response.
     */
    public static WebhookTargetResponse success(String message) {
        return new WebhookTargetResponse(Status.SUCCESS, message, null, null);
    }

    /**
     * Create an error response.
     */
    public static WebhookTargetResponse error(String message, String errorDescription, String errorDetails) {
        return new WebhookTargetResponse(Status.ERROR, message, errorDescription, errorDetails);
    }
}
