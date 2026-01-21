package tech.flowcatalyst.platform.client.mapper;

import tech.flowcatalyst.platform.client.ClientAccessGrant;
import tech.flowcatalyst.platform.client.entity.ClientAccessGrantEntity;

/**
 * Mapper for converting between ClientAccessGrant domain model and JPA entity.
 */
public final class ClientAccessGrantMapper {

    private ClientAccessGrantMapper() {
    }

    public static ClientAccessGrant toDomain(ClientAccessGrantEntity entity) {
        if (entity == null) {
            return null;
        }

        ClientAccessGrant domain = new ClientAccessGrant();
        domain.id = entity.id;
        domain.principalId = entity.principalId;
        domain.clientId = entity.clientId;
        domain.grantedAt = entity.grantedAt;
        domain.expiresAt = entity.expiresAt;
        return domain;
    }

    public static ClientAccessGrantEntity toEntity(ClientAccessGrant domain) {
        if (domain == null) {
            return null;
        }

        ClientAccessGrantEntity entity = new ClientAccessGrantEntity();
        entity.id = domain.id;
        entity.principalId = domain.principalId;
        entity.clientId = domain.clientId;
        entity.grantedAt = domain.grantedAt;
        entity.expiresAt = domain.expiresAt;
        return entity;
    }
}
