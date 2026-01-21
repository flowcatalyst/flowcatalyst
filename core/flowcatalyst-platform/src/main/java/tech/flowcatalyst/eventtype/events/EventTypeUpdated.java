package tech.flowcatalyst.eventtype.events;

import com.fasterxml.jackson.annotation.JsonIgnore;
import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.SerializationFeature;
import com.fasterxml.jackson.datatype.jsr310.JavaTimeModule;
import lombok.Builder;
import tech.flowcatalyst.platform.common.DomainEvent;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.shared.TsidGenerator;

import java.time.Instant;

/**
 * Event emitted when an EventType's metadata is updated.
 *
 * <p>Event type: {@code platform:control-plane:eventtype:updated}
 *
 * <p>This event contains the updated name and/or description.
 * Only fields that were actually changed are included.
 */
@Builder
public record EventTypeUpdated(
    // Event metadata
    String eventId,
    Instant time,
    String executionId,
    String correlationId,
    String causationId,
    String principalId,

    // Event-specific payload
    String eventTypeId,
    String name,
    String description
) implements DomainEvent {

    private static final String EVENT_TYPE = "platform:control-plane:eventtype:updated";
    private static final String SPEC_VERSION = "1.0";
    private static final String SOURCE = "platform:control-plane";

    @JsonIgnore
    private static final ObjectMapper MAPPER = new ObjectMapper()
        .registerModule(new JavaTimeModule())
        .disable(SerializationFeature.WRITE_DATES_AS_TIMESTAMPS);

    @Override
    @JsonIgnore
    public String eventType() {
        return EVENT_TYPE;
    }

    @Override
    @JsonIgnore
    public String specVersion() {
        return SPEC_VERSION;
    }

    @Override
    @JsonIgnore
    public String source() {
        return SOURCE;
    }

    @Override
    @JsonIgnore
    public String subject() {
        return "platform.eventtype." + eventTypeId;
    }

    @Override
    @JsonIgnore
    public String messageGroup() {
        return "platform:eventtype:" + eventTypeId;
    }

    @Override
    @JsonIgnore
    public String toDataJson() {
        try {
            return MAPPER.writeValueAsString(new Data(eventTypeId, name, description));
        } catch (JsonProcessingException e) {
            throw new RuntimeException("Failed to serialize event data", e);
        }
    }

    public record Data(
        String eventTypeId,
        String name,
        String description
    ) {}

    /**
     * Create a pre-configured builder with event metadata from the execution context.
     */
    public static EventTypeUpdatedBuilder fromContext(ExecutionContext ctx) {
        return EventTypeUpdated.builder()
            .eventId(TsidGenerator.generate())
            .time(Instant.now())
            .executionId(ctx.executionId())
            .correlationId(ctx.correlationId())
            .causationId(ctx.causationId())
            .principalId(ctx.principalId());
    }
}
