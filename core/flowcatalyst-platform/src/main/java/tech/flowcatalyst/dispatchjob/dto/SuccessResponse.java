package tech.flowcatalyst.dispatchjob.dto;

import com.fasterxml.jackson.annotation.JsonProperty;

public record SuccessResponse(
    @JsonProperty("message") String message
) {
}
