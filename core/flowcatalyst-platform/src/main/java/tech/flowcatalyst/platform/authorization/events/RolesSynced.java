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
import java.util.List;

/**
 * Event emitted when roles are synced for an application.
 *
 * <p>Event type: {@code platform:control-plane:role:synced}
 */
@Builder
public record RolesSynced(
    String eventId,
    Instant time,
    String executionId,
    String correlationId,
    String causationId,
    String principalId,
    String applicationId,
    String applicationCode,
    int rolesCreated,
    int rolesUpdated,
    int rolesDeleted,
    List<String> syncedRoleNames
) implements DomainEvent {

    private static final String EVENT_TYPE = "platform:control-plane:role:synced";
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
        return "platform.application." + applicationId + ".roles";
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
            return MAPPER.writeValueAsString(new Data(applicationId, applicationCode, rolesCreated, rolesUpdated, rolesDeleted, syncedRoleNames));
        } catch (JsonProcessingException e) {
            throw new RuntimeException("Failed to serialize event data", e);
        }
    }

    public record Data(
        String applicationId,
        String applicationCode,
        int rolesCreated,
        int rolesUpdated,
        int rolesDeleted,
        List<String> syncedRoleNames
    ) {}

    /**
     * Create a pre-configured builder with event metadata from the execution context.
     */
    public static RolesSyncedBuilder fromContext(ExecutionContext ctx) {
        return RolesSynced.builder()
            .eventId(TsidGenerator.generate())
            .time(Instant.now())
            .executionId(ctx.executionId())
            .correlationId(ctx.correlationId())
            .causationId(ctx.causationId())
            .principalId(ctx.principalId());
    }
}
