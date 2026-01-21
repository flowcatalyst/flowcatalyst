package tech.flowcatalyst.serviceaccount.operations.regenerateauthtoken;

import com.fasterxml.jackson.annotation.JsonIgnore;
import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.SerializationFeature;
import com.fasterxml.jackson.datatype.jsr310.JavaTimeModule;
import lombok.Builder;
import tech.flowcatalyst.platform.common.DomainEvent;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.shared.TsidGenerator;
import tech.flowcatalyst.serviceaccount.entity.WebhookAuthType;

import java.time.Instant;

/**
 * Event emitted when a service account's auth token is regenerated.
 *
 * <p>Event type: {@code platform:iam:service-account:auth-token-regenerated}
 */
@Builder
public record AuthTokenRegenerated(
    String eventId,
    Instant time,
    String executionId,
    String correlationId,
    String causationId,
    String principalId,
    String serviceAccountId,
    String code,
    WebhookAuthType authType,
    boolean isCustomToken
) implements DomainEvent {

    private static final String EVENT_TYPE = "platform:iam:service-account:auth-token-regenerated";
    private static final String SPEC_VERSION = "1.0";
    private static final String SOURCE = "platform:iam";

    @JsonIgnore
    private static final ObjectMapper MAPPER = new ObjectMapper()
        .registerModule(new JavaTimeModule())
        .disable(SerializationFeature.WRITE_DATES_AS_TIMESTAMPS);

    @Override @JsonIgnore public String eventType() { return EVENT_TYPE; }
    @Override @JsonIgnore public String specVersion() { return SPEC_VERSION; }
    @Override @JsonIgnore public String source() { return SOURCE; }
    @Override @JsonIgnore public String subject() { return "platform.service-account." + serviceAccountId; }
    @Override @JsonIgnore public String messageGroup() { return "platform:service-account:" + serviceAccountId; }

    @Override
    @JsonIgnore
    public String toDataJson() {
        try {
            return MAPPER.writeValueAsString(new Data(serviceAccountId, code, authType, isCustomToken));
        } catch (JsonProcessingException e) {
            throw new RuntimeException("Failed to serialize event data", e);
        }
    }

    public record Data(String serviceAccountId, String code, WebhookAuthType authType, boolean isCustomToken) {}

    /**
     * Create a pre-configured builder with event metadata from the execution context.
     */
    public static AuthTokenRegeneratedBuilder fromContext(ExecutionContext ctx) {
        return AuthTokenRegenerated.builder()
            .eventId(TsidGenerator.generate())
            .time(Instant.now())
            .executionId(ctx.executionId())
            .correlationId(ctx.correlationId())
            .causationId(ctx.causationId())
            .principalId(ctx.principalId());
    }
}
