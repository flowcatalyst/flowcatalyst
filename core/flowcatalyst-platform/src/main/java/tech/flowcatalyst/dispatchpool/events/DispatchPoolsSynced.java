package tech.flowcatalyst.dispatchpool.events;

import com.fasterxml.jackson.annotation.JsonIgnore;
import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.SerializationFeature;
import com.fasterxml.jackson.datatype.jsr310.JavaTimeModule;
import lombok.Builder;
import tech.flowcatalyst.platform.common.DomainEvent;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.shared.EntityType;
import tech.flowcatalyst.platform.shared.TsidGenerator;

import java.time.Instant;
import java.util.List;

/**
 * Event emitted when dispatch pools are synced for an application.
 *
 * <p>Event type: {@code platform:control-plane:dispatch-pool:synced}
 */
@Builder
public record DispatchPoolsSynced(
    String eventId,
    Instant time,
    String executionId,
    String correlationId,
    String causationId,
    String principalId,
    String applicationCode,
    int poolsCreated,
    int poolsUpdated,
    int poolsDeleted,
    List<String> syncedPoolCodes
) implements DomainEvent {

    private static final String EVENT_TYPE = "platform:control-plane:dispatch-pool:synced";
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
        return "platform.dispatch-pools." + applicationCode;
    }

    @Override
    @JsonIgnore
    public String messageGroup() {
        return "platform:dispatch-pools";
    }

    @Override
    @JsonIgnore
    public String toDataJson() {
        try {
            return MAPPER.writeValueAsString(new Data(applicationCode, poolsCreated, poolsUpdated, poolsDeleted, syncedPoolCodes));
        } catch (JsonProcessingException e) {
            throw new RuntimeException("Failed to serialize event data", e);
        }
    }

    public record Data(
        String applicationCode,
        int poolsCreated,
        int poolsUpdated,
        int poolsDeleted,
        List<String> syncedPoolCodes
    ) {}

    /**
     * Create a pre-configured builder with event metadata from the execution context.
     */
    public static DispatchPoolsSyncedBuilder fromContext(ExecutionContext ctx) {
        return DispatchPoolsSynced.builder()
            .eventId(TsidGenerator.generate(EntityType.EVENT))
            .time(Instant.now())
            .executionId(ctx.executionId())
            .correlationId(ctx.correlationId())
            .causationId(ctx.causationId())
            .principalId(ctx.principalId());
    }
}
