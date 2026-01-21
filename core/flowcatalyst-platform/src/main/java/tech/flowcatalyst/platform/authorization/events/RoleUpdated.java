package tech.flowcatalyst.platform.authorization.events;

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
import java.util.Set;

/**
 * Event emitted when a Role is updated.
 *
 * <p>Event type: {@code platform:control-plane:role:updated}
 */
@Builder
public record RoleUpdated(
    String eventId,
    Instant time,
    String executionId,
    String correlationId,
    String causationId,
    String principalId,
    String roleId,
    String roleName,
    String displayName,
    String description,
    Set<String> permissions,
    boolean clientManaged
) implements DomainEvent {

    private static final String EVENT_TYPE = "platform:control-plane:role:updated";
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
        return "platform.role." + roleId;
    }

    @Override
    @JsonIgnore
    public String messageGroup() {
        return "platform:role:" + roleId;
    }

    @Override
    @JsonIgnore
    public String toDataJson() {
        try {
            return MAPPER.writeValueAsString(new Data(roleId, roleName, displayName, description, permissions, clientManaged));
        } catch (JsonProcessingException e) {
            throw new RuntimeException("Failed to serialize event data", e);
        }
    }

    public record Data(
        String roleId,
        String roleName,
        String displayName,
        String description,
        Set<String> permissions,
        boolean clientManaged
    ) {}

    /**
     * Create a pre-configured builder with event metadata from the execution context.
     */
    public static RoleUpdatedBuilder fromContext(ExecutionContext ctx) {
        return RoleUpdated.builder()
            .eventId(TsidGenerator.generate())
            .time(Instant.now())
            .executionId(ctx.executionId())
            .correlationId(ctx.correlationId())
            .causationId(ctx.causationId())
            .principalId(ctx.principalId());
    }
}
