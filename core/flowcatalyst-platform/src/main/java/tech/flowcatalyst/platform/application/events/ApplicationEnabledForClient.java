package tech.flowcatalyst.platform.application.events;

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
 * Event emitted when an application is enabled for a client.
 *
 * <p>Event type: {@code platform:control-plane:application-client-config:enabled}
 */
@Builder
public record ApplicationEnabledForClient(
    // Event metadata
    String eventId,
    Instant time,
    String executionId,
    String correlationId,
    String causationId,
    String principalId,

    // Event-specific payload
    String configId,
    String applicationId,
    String applicationCode,
    String applicationName,
    String clientId,
    String clientIdentifier,
    String clientName,
    String baseUrlOverride,
    String websiteOverride
) implements DomainEvent {

    private static final String EVENT_TYPE = "platform:control-plane:application-client-config:enabled";
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
        return "platform.application-client-config." + configId;
    }

    @Override
    @JsonIgnore
    public String messageGroup() {
        return "platform:client:" + clientId;
    }

    @Override
    @JsonIgnore
    public String toDataJson() {
        try {
            return MAPPER.writeValueAsString(new Data(
                configId, applicationId, applicationCode, applicationName,
                clientId, clientIdentifier, clientName, baseUrlOverride, websiteOverride
            ));
        } catch (JsonProcessingException e) {
            throw new RuntimeException("Failed to serialize event data", e);
        }
    }

    public record Data(
        String configId,
        String applicationId,
        String applicationCode,
        String applicationName,
        String clientId,
        String clientIdentifier,
        String clientName,
        String baseUrlOverride,
        String websiteOverride
    ) {}

    /**
     * Create a pre-configured builder with event metadata from the execution context.
     */
    public static ApplicationEnabledForClientBuilder fromContext(ExecutionContext ctx) {
        return ApplicationEnabledForClient.builder()
            .eventId(TsidGenerator.generate())
            .time(Instant.now())
            .executionId(ctx.executionId())
            .correlationId(ctx.correlationId())
            .causationId(ctx.causationId())
            .principalId(ctx.principalId());
    }
}
