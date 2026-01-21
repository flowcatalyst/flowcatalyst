package tech.flowcatalyst.dispatchpool.events;

import com.fasterxml.jackson.annotation.JsonIgnore;
import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.SerializationFeature;
import com.fasterxml.jackson.datatype.jsr310.JavaTimeModule;
import lombok.Builder;
import tech.flowcatalyst.dispatchpool.DispatchPoolStatus;
import tech.flowcatalyst.platform.common.DomainEvent;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.shared.TsidGenerator;

import java.time.Instant;

/**
 * Event emitted when a dispatch pool is created.
 *
 * <p>Event type: {@code platform:control-plane:dispatch-pool:created}
 *
 * <p>Use {@link #fromContext(ExecutionContext)} to create a pre-configured builder.
 */
@Builder
public record DispatchPoolCreated(
    // Event metadata
    String eventId,
    Instant time,
    String executionId,
    String correlationId,
    String causationId,
    String principalId,

    // Event-specific payload
    String poolId,
    String code,
    String name,
    String description,
    int rateLimit,
    int concurrency,
    String clientId,
    String clientIdentifier,
    DispatchPoolStatus status
) implements DomainEvent {

    private static final String EVENT_TYPE = "platform:control-plane:dispatch-pool:created";
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
        return "platform.dispatch-pool." + poolId;
    }

    @Override
    @JsonIgnore
    public String messageGroup() {
        // Group by pool for ordering
        return "platform:dispatch-pool:" + poolId;
    }

    @Override
    @JsonIgnore
    public String toDataJson() {
        try {
            return MAPPER.writeValueAsString(new Data(
                poolId, code, name, description, rateLimit, concurrency,
                clientId, clientIdentifier, status
            ));
        } catch (JsonProcessingException e) {
            throw new RuntimeException("Failed to serialize event data", e);
        }
    }

    public record Data(
        String poolId,
        String code,
        String name,
        String description,
        int rateLimit,
        int concurrency,
        String clientId,
        String clientIdentifier,
        DispatchPoolStatus status
    ) {}

    /**
     * Create a pre-configured builder with event metadata from the execution context.
     *
     * @param ctx The execution context containing correlation/causation IDs
     * @return A builder with metadata fields pre-populated
     */
    public static DispatchPoolCreatedBuilder fromContext(ExecutionContext ctx) {
        return DispatchPoolCreated.builder()
            .eventId(TsidGenerator.generate())
            .time(Instant.now())
            .executionId(ctx.executionId())
            .correlationId(ctx.correlationId())
            .causationId(ctx.causationId())
            .principalId(ctx.principalId());
    }
}
