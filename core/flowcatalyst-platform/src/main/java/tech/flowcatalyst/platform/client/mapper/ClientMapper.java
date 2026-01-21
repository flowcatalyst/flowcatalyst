package tech.flowcatalyst.platform.client.mapper;

import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import tech.flowcatalyst.platform.client.Client;
import tech.flowcatalyst.platform.client.ClientNote;
import tech.flowcatalyst.platform.client.ClientStatus;
import tech.flowcatalyst.platform.client.entity.ClientEntity;

import java.util.ArrayList;
import java.util.List;

/**
 * Mapper for converting between Client domain model and JPA entity.
 */
public final class ClientMapper {

    private static final ObjectMapper objectMapper = new ObjectMapper();

    private ClientMapper() {
    }

    public static Client toDomain(ClientEntity entity) {
        if (entity == null) {
            return null;
        }

        Client domain = new Client();
        domain.id = entity.id;
        domain.name = entity.name;
        domain.identifier = entity.identifier;
        domain.status = entity.status != null ? entity.status : ClientStatus.ACTIVE;
        domain.statusReason = entity.statusReason;
        domain.statusChangedAt = entity.statusChangedAt;
        domain.notes = parseNotes(entity.notes);
        domain.createdAt = entity.createdAt;
        domain.updatedAt = entity.updatedAt;
        return domain;
    }

    public static ClientEntity toEntity(Client domain) {
        if (domain == null) {
            return null;
        }

        ClientEntity entity = new ClientEntity();
        entity.id = domain.id;
        entity.name = domain.name;
        entity.identifier = domain.identifier;
        entity.status = domain.status != null ? domain.status : ClientStatus.ACTIVE;
        entity.statusReason = domain.statusReason;
        entity.statusChangedAt = domain.statusChangedAt;
        entity.notes = toJson(domain.notes);
        entity.createdAt = domain.createdAt;
        entity.updatedAt = domain.updatedAt;
        return entity;
    }

    public static void updateEntity(ClientEntity entity, Client domain) {
        entity.name = domain.name;
        entity.identifier = domain.identifier;
        entity.status = domain.status != null ? domain.status : ClientStatus.ACTIVE;
        entity.statusReason = domain.statusReason;
        entity.statusChangedAt = domain.statusChangedAt;
        entity.notes = toJson(domain.notes);
        entity.updatedAt = domain.updatedAt;
    }

    private static List<ClientNote> parseNotes(String json) {
        if (json == null || json.isBlank()) {
            return new ArrayList<>();
        }
        try {
            return objectMapper.readValue(json, new TypeReference<List<ClientNote>>() {});
        } catch (Exception e) {
            return new ArrayList<>();
        }
    }

    private static String toJson(Object obj) {
        if (obj == null) {
            return null;
        }
        try {
            return objectMapper.writeValueAsString(obj);
        } catch (Exception e) {
            return null;
        }
    }
}
