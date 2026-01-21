package tech.flowcatalyst.serviceaccount.operations.createserviceaccount;

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
import java.util.List;

/**
 * Event emitted when a new service account is created.
 *
 * <p>Event type: {@code platform:iam:service-account:created}
 */
@Builder
public record ServiceAccountCreated(
    String eventId,
    Instant time,
    String executionId,
    String correlationId,
    String causationId,
    String principalId,
    String serviceAccountId,
    String code,
    String name,
    List<String> clientIds,
    String applicationId
) implements DomainEvent {

    private static final String EVENT_TYPE = "platform:iam:service-account:created";
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
        return "platform.service-account." + serviceAccountId;
    }

    @Override
    @JsonIgnore
    public String messageGroup() {
        return "platform:service-account:" + serviceAccountId;
    }

    @Override
    @JsonIgnore
    public String toDataJson() {
        try {
            return MAPPER.writeValueAsString(new Data(serviceAccountId, code, name, clientIds, applicationId));
        } catch (JsonProcessingException e) {
            throw new RuntimeException("Failed to serialize event data", e);
        }
    }

    public record Data(
        String serviceAccountId,
        String code,
        String name,
        List<String> clientIds,
        String applicationId
    ) {}

    /**
     * Create a pre-configured builder with event metadata from the execution context.
     */
    public static ServiceAccountCreatedBuilder fromContext(ExecutionContext ctx) {
        return ServiceAccountCreated.builder()
            .eventId(TsidGenerator.generate())
            .time(Instant.now())
            .executionId(ctx.executionId())
            .correlationId(ctx.correlationId())
            .causationId(ctx.causationId())
            .principalId(ctx.principalId());
    }
}
