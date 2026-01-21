package tech.flowcatalyst.subscription.events;

import com.fasterxml.jackson.annotation.JsonIgnore;
import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.SerializationFeature;
import com.fasterxml.jackson.datatype.jsr310.JavaTimeModule;
import lombok.Builder;
import tech.flowcatalyst.dispatch.DispatchMode;
import tech.flowcatalyst.platform.common.DomainEvent;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.shared.TsidGenerator;
import tech.flowcatalyst.subscription.*;

import java.time.Instant;
import java.util.List;

/**
 * Event emitted when a subscription is updated.
 *
 * <p>Event type: {@code platform:control-plane:subscription:updated}
 */
@Builder
public record SubscriptionUpdated(
    // Event metadata
    String eventId,
    Instant time,
    String executionId,
    String correlationId,
    String causationId,
    String principalId,

    // Event-specific payload
    String subscriptionId,
    String code,
    String name,
    String description,
    String clientId,
    String clientIdentifier,
    List<EventTypeBinding> eventTypes,
    String target,
    String queue,
    List<ConfigEntry> customConfig,
    SubscriptionStatus status,
    int maxAgeSeconds,
    String dispatchPoolId,
    String dispatchPoolCode,
    int delaySeconds,
    int sequence,
    DispatchMode mode,
    int timeoutSeconds,
    int maxRetries,
    String serviceAccountId,
    boolean dataOnly
) implements DomainEvent {

    private static final String EVENT_TYPE = "platform:control-plane:subscription:updated";
    private static final String SPEC_VERSION = "1.0";
    private static final String SOURCE_NAME = "platform:control-plane";

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
        return SOURCE_NAME;
    }

    @Override
    @JsonIgnore
    public String subject() {
        return "platform.subscription." + subscriptionId;
    }

    @Override
    @JsonIgnore
    public String messageGroup() {
        return "platform:subscription:" + (clientId != null ? clientId : "anchor");
    }

    @Override
    @JsonIgnore
    public String toDataJson() {
        try {
            return MAPPER.writeValueAsString(new Data(
                subscriptionId, code, name, description, clientId, clientIdentifier,
                eventTypes, target, queue, customConfig, status,
                maxAgeSeconds, dispatchPoolId, dispatchPoolCode,
                delaySeconds, sequence, mode, timeoutSeconds, maxRetries,
                serviceAccountId, dataOnly
            ));
        } catch (JsonProcessingException e) {
            throw new RuntimeException("Failed to serialize event data", e);
        }
    }

    public record Data(
        String subscriptionId,
        String code,
        String name,
        String description,
        String clientId,
        String clientIdentifier,
        List<EventTypeBinding> eventTypes,
        String target,
        String queue,
        List<ConfigEntry> customConfig,
        SubscriptionStatus status,
        int maxAgeSeconds,
        String dispatchPoolId,
        String dispatchPoolCode,
        int delaySeconds,
        int sequence,
        DispatchMode mode,
        int timeoutSeconds,
        int maxRetries,
        String serviceAccountId,
        boolean dataOnly
    ) {}

    /**
     * Create a pre-configured builder with event metadata from the execution context.
     */
    public static SubscriptionUpdatedBuilder fromContext(ExecutionContext ctx) {
        return SubscriptionUpdated.builder()
            .eventId(TsidGenerator.generate())
            .time(Instant.now())
            .executionId(ctx.executionId())
            .correlationId(ctx.correlationId())
            .causationId(ctx.causationId())
            .principalId(ctx.principalId());
    }
}
