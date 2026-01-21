package tech.flowcatalyst.sdk.outbox.dto;

import lombok.Builder;
import lombok.With;

import java.util.List;
import java.util.Map;

/**
 * DTO for creating an event in the outbox.
 */
@Builder(toBuilder = true)
@With
public record CreateEventDto(
    String type,
    Map<String, Object> data,
    String partitionId,
    String source,
    String subject,
    String correlationId,
    String causationId,
    String deduplicationId,
    String messageGroup,
    List<ContextDataItem> contextData,
    Map<String, String> headers
) {
    public record ContextDataItem(String key, String value) {}

    /**
     * Create a new event DTO with required fields.
     */
    public static CreateEventDtoBuilder create(String type, Map<String, Object> data, String partitionId) {
        return CreateEventDto.builder()
            .type(type)
            .data(data)
            .partitionId(partitionId);
    }

    /**
     * Build the event payload for the outbox.
     */
    public Map<String, Object> toPayload() {
        var builder = new java.util.HashMap<String, Object>();
        builder.put("specVersion", "1.0");
        builder.put("type", type);

        if (source != null) builder.put("source", source);
        if (subject != null) builder.put("subject", subject);
        if (data != null) {
            try {
                builder.put("data", new com.fasterxml.jackson.databind.ObjectMapper().writeValueAsString(data));
            } catch (Exception e) {
                builder.put("data", data.toString());
            }
        }
        if (correlationId != null) builder.put("correlationId", correlationId);
        if (causationId != null) builder.put("causationId", causationId);
        if (deduplicationId != null) builder.put("deduplicationId", deduplicationId);
        if (messageGroup != null) builder.put("messageGroup", messageGroup);
        if (contextData != null && !contextData.isEmpty()) builder.put("contextData", contextData);

        return builder;
    }
}
