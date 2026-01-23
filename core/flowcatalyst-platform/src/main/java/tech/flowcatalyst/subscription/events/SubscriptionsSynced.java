package tech.flowcatalyst.subscription.events;

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
 * Event emitted when subscriptions are synced for an application.
 *
 * <p>Event type: {@code platform:messaging:subscription:synced}
 */
@Builder
public record SubscriptionsSynced(
    String eventId,
    Instant time,
    String executionId,
    String correlationId,
    String causationId,
    String principalId,
    String applicationCode,
    int subscriptionsCreated,
    int subscriptionsUpdated,
    int subscriptionsDeleted,
    List<String> syncedSubscriptionCodes
) implements DomainEvent {

    private static final String EVENT_TYPE = "platform:messaging:subscription:synced";
    private static final String SPEC_VERSION = "1.0";
    private static final String SOURCE = "platform:messaging";

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
        return "platform.application." + applicationCode + ".subscriptions";
    }

    @Override
    @JsonIgnore
    public String messageGroup() {
        return "platform:application:" + applicationCode;
    }

    @Override
    @JsonIgnore
    public String toDataJson() {
        try {
            return MAPPER.writeValueAsString(new Data(
                applicationCode,
                subscriptionsCreated,
                subscriptionsUpdated,
                subscriptionsDeleted,
                syncedSubscriptionCodes
            ));
        } catch (JsonProcessingException e) {
            throw new RuntimeException("Failed to serialize event data", e);
        }
    }

    public record Data(
        String applicationCode,
        int subscriptionsCreated,
        int subscriptionsUpdated,
        int subscriptionsDeleted,
        List<String> syncedSubscriptionCodes
    ) {}

    /**
     * Create a pre-configured builder with event metadata from the execution context.
     */
    public static SubscriptionsSyncedBuilder fromContext(ExecutionContext ctx) {
        return SubscriptionsSynced.builder()
            .eventId(TsidGenerator.generate(EntityType.EVENT))
            .time(Instant.now())
            .executionId(ctx.executionId())
            .correlationId(ctx.correlationId())
            .causationId(ctx.causationId())
            .principalId(ctx.principalId());
    }
}
