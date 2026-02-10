package tech.flowcatalyst.platform.authentication.domain.events;

import com.fasterxml.jackson.annotation.JsonIgnore;
import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.SerializationFeature;
import com.fasterxml.jackson.datatype.jsr310.JavaTimeModule;
import lombok.Builder;

import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.shared.EntityType;
import tech.flowcatalyst.platform.shared.TsidGenerator;

import java.time.Instant;
import java.util.List;

/**
 * Event emitted when an Email Domain Mapping is updated.
 *
 * <p>Event type: {@code platform:iam:email-domain-mapping:updated}
 */
@Builder
public record EmailDomainMappingUpdated(
    String eventId,
    Instant time,
    String executionId,
    String correlationId,
    String causationId,
    String principalId,
    String emailDomainMappingId,
    String emailDomain,
    String identityProviderId,
    String scopeType,
    String primaryClientId,
    List<String> additionalClientIds,
    List<String> grantedClientIds
) implements EmailDomainMappingEvent {

    private static final String EVENT_TYPE = "platform:iam:email-domain-mapping:updated";
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
        return "platform.email-domain-mapping." + emailDomainMappingId;
    }

    @Override
    @JsonIgnore
    public String messageGroup() {
        return "platform:email-domain-mapping:" + emailDomainMappingId;
    }

    @Override
    @JsonIgnore
    public String toDataJson() {
        try {
            return MAPPER.writeValueAsString(new Data(emailDomainMappingId, emailDomain,
                identityProviderId, scopeType, primaryClientId, additionalClientIds, grantedClientIds));
        } catch (JsonProcessingException e) {
            throw new RuntimeException("Failed to serialize event data", e);
        }
    }

    public record Data(
        String emailDomainMappingId,
        String emailDomain,
        String identityProviderId,
        String scopeType,
        String primaryClientId,
        List<String> additionalClientIds,
        List<String> grantedClientIds
    ) {}

    /**
     * Create a pre-configured builder with event metadata from the execution context.
     */
    public static EmailDomainMappingUpdatedBuilder fromContext(ExecutionContext ctx) {
        return EmailDomainMappingUpdated.builder()
            .eventId(TsidGenerator.generate(EntityType.EVENT))
            .time(Instant.now())
            .executionId(ctx.executionId())
            .correlationId(ctx.correlationId())
            .causationId(ctx.causationId())
            .principalId(ctx.principalId());
    }
}
