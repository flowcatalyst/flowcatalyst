package tech.flowcatalyst.platform.principal.events;

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
 * Event emitted when roles are assigned to a user.
 *
 * <p>This is a batch event that captures the complete set of roles
 * after assignment, along with what was added and removed.
 *
 * <p>Event type: {@code platform:iam:user:roles-assigned}
 */
@Builder
public record RolesAssigned(
    String eventId,
    Instant time,
    String executionId,
    String correlationId,
    String causationId,
    String principalId,
    String userId,
    List<String> roles,
    List<String> added,
    List<String> removed
) implements DomainEvent {

    private static final String EVENT_TYPE = "platform:iam:user:roles-assigned";
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
        return "platform.user." + userId;
    }

    @Override
    @JsonIgnore
    public String messageGroup() {
        return "platform:user:" + userId;
    }

    @Override
    @JsonIgnore
    public String toDataJson() {
        try {
            return MAPPER.writeValueAsString(new Data(userId, roles, added, removed));
        } catch (JsonProcessingException e) {
            throw new RuntimeException("Failed to serialize event data", e);
        }
    }

    public record Data(
        String userId,
        List<String> roles,
        List<String> added,
        List<String> removed
    ) {}

    /**
     * Create a pre-configured builder with event metadata from the execution context.
     */
    public static RolesAssignedBuilder fromContext(ExecutionContext ctx) {
        return RolesAssigned.builder()
            .eventId(TsidGenerator.generate(EntityType.EVENT))
            .time(Instant.now())
            .executionId(ctx.executionId())
            .correlationId(ctx.correlationId())
            .causationId(ctx.causationId())
            .principalId(ctx.principalId());
    }
}
