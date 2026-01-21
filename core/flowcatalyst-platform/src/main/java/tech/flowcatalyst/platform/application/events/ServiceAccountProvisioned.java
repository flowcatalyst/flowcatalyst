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
 * Event emitted when a service account is provisioned for an Application.
 *
 * <p>Note: The client secret is NOT included in this event for security reasons.
 * It is only returned once at provisioning time via the API response.
 *
 * <p>Event type: {@code platform:control-plane:application:service-account-provisioned}
 */
@Builder
public record ServiceAccountProvisioned(
    // Event metadata
    String eventId,
    Instant time,
    String executionId,
    String correlationId,
    String causationId,
    String principalId,

    // Event-specific payload (what happened)
    String applicationId,
    String applicationCode,
    String applicationName,
    String serviceAccountId,  // New standalone ServiceAccount entity ID
    String serviceAccountPrincipalId,  // Legacy Principal ID (deprecated)
    String serviceAccountName,
    String oauthClientId,
    String oauthClientClientId  // The client_id used for OAuth (not the entity ID)
) implements DomainEvent {

    private static final String EVENT_TYPE = "platform:control-plane:application:service-account-provisioned";
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
        return "platform.application." + applicationId;
    }

    @Override
    @JsonIgnore
    public String messageGroup() {
        return "platform:application:" + applicationId;
    }

    @Override
    @JsonIgnore
    public String toDataJson() {
        try {
            return MAPPER.writeValueAsString(new Data(
                applicationId, applicationCode, applicationName,
                serviceAccountId, serviceAccountPrincipalId, serviceAccountName,
                oauthClientId, oauthClientClientId
            ));
        } catch (JsonProcessingException e) {
            throw new RuntimeException("Failed to serialize event data", e);
        }
    }

    /**
     * The event data schema.
     */
    public record Data(
        String applicationId,
        String applicationCode,
        String applicationName,
        String serviceAccountId,
        String serviceAccountPrincipalId,
        String serviceAccountName,
        String oauthClientId,
        String oauthClientClientId
    ) {}

    /**
     * Create a pre-configured builder with event metadata from the execution context.
     */
    public static ServiceAccountProvisionedBuilder fromContext(ExecutionContext ctx) {
        return ServiceAccountProvisioned.builder()
            .eventId(TsidGenerator.generate())
            .time(Instant.now())
            .executionId(ctx.executionId())
            .correlationId(ctx.correlationId())
            .causationId(ctx.causationId())
            .principalId(ctx.principalId());
    }
}
