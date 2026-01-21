package tech.flowcatalyst.messagerouter.endpoint;

import com.fasterxml.jackson.annotation.JsonInclude;
import org.eclipse.microprofile.openapi.annotations.media.Schema;

@Schema(description = "Response from seeding messages")
@JsonInclude(JsonInclude.Include.NON_NULL)
public record SeedMessageResponse(

    @Schema(description = "Status of the seeding operation", examples = {"success", "error"})
    String status,

    @Schema(description = "Number of messages successfully sent", examples = {"50", "100", "1000"})
    Integer messagesSent,

    @Schema(description = "Total number of messages requested", examples = {"50", "100", "1000"})
    Integer totalRequested,

    @Schema(description = "Error message if operation failed", examples = {"Failed to connect to SQS", "Queue not found"})
    String message
) {

    // Success response
    public static SeedMessageResponse success(int messagesSent, int totalRequested) {
        return new SeedMessageResponse("success", messagesSent, totalRequested, null);
    }

    // Error response
    public static SeedMessageResponse error(String message) {
        return new SeedMessageResponse("error", null, null, message);
    }
}
