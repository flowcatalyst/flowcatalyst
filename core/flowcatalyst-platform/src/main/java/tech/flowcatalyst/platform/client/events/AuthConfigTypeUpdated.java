package tech.flowcatalyst.platform.client.events;

import com.fasterxml.jackson.annotation.JsonIgnore;
import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.SerializationFeature;
import com.fasterxml.jackson.datatype.jsr310.JavaTimeModule;
import lombok.Builder;
import tech.flowcatalyst.platform.client.AuthConfigType;
import tech.flowcatalyst.platform.common.DomainEvent;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.shared.TsidGenerator;

import java.time.Instant;

/**
 * Event emitted when an auth config's type is changed.
 *
 * <p>Event type: {@code platform:iam:auth-config:type-updated}
 */
@Builder
public record AuthConfigTypeUpdated(
    String eventId,
    Instant time,
    String executionId,
    String correlationId,
    String causationId,
    String principalId,
    String authConfigId,
    String emailDomain,
    AuthConfigType previousType,
    AuthConfigType newType,
    String previousPrimaryClientId,
    String newPrimaryClientId
) implements DomainEvent {

    private static final String EVENT_TYPE = "platform:iam:auth-config:type-updated";
    private static final String SPEC_VERSION = "1.0";
    private static final String SOURCE = "platform:iam";

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
        return "platform.auth-config." + authConfigId;
    }

    @Override
    @JsonIgnore
    public String messageGroup() {
        return "platform:auth-config:" + authConfigId;
    }

    @Override
    @JsonIgnore
    public String toDataJson() {
        try {
            return MAPPER.writeValueAsString(new Data(
                authConfigId, emailDomain, previousType, newType,
                previousPrimaryClientId, newPrimaryClientId
            ));
        } catch (JsonProcessingException e) {
            throw new RuntimeException("Failed to serialize event data", e);
        }
    }

    public record Data(
        String authConfigId,
        String emailDomain,
        AuthConfigType previousType,
        AuthConfigType newType,
        String previousPrimaryClientId,
        String newPrimaryClientId
    ) {}

    /**
     * Create a pre-configured builder with event metadata from the execution context.
     */
    public static AuthConfigTypeUpdatedBuilder fromContext(ExecutionContext ctx) {
        return AuthConfigTypeUpdated.builder()
            .eventId(TsidGenerator.generate())
            .time(Instant.now())
            .executionId(ctx.executionId())
            .correlationId(ctx.correlationId())
            .causationId(ctx.causationId())
            .principalId(ctx.principalId());
    }
}
