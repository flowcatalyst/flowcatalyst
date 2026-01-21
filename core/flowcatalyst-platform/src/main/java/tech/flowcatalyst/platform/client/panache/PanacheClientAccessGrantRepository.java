package tech.flowcatalyst.platform.client.panache;

import io.quarkus.hibernate.orm.panache.PanacheRepositoryBase;
import jakarta.enterprise.context.ApplicationScoped;
import tech.flowcatalyst.platform.client.ClientAccessGrant;
import tech.flowcatalyst.platform.client.ClientAccessGrantRepository;
import tech.flowcatalyst.platform.client.entity.ClientAccessGrantEntity;
import tech.flowcatalyst.platform.client.mapper.ClientAccessGrantMapper;

import java.time.Instant;
import java.util.List;
import java.util.Optional;

/**
 * Panache-based implementation of ClientAccessGrantRepository.
 */
@ApplicationScoped
public class PanacheClientAccessGrantRepository
    implements ClientAccessGrantRepository, PanacheRepositoryBase<ClientAccessGrantEntity, String> {

    @Override
    public List<ClientAccessGrant> findByPrincipalId(String principalId) {
        return find("principalId", principalId)
            .stream()
            .map(ClientAccessGrantMapper::toDomain)
            .toList();
    }

    @Override
    public List<ClientAccessGrant> findByClientId(String clientId) {
        return find("clientId", clientId)
            .stream()
            .map(ClientAccessGrantMapper::toDomain)
            .toList();
    }

    @Override
    public Optional<ClientAccessGrant> findByPrincipalIdAndClientId(String principalId, String clientId) {
        return find("principalId = ?1 and clientId = ?2", principalId, clientId)
            .firstResultOptional()
            .map(ClientAccessGrantMapper::toDomain);
    }

    @Override
    public boolean existsByPrincipalIdAndClientId(String principalId, String clientId) {
        return count("principalId = ?1 and clientId = ?2", principalId, clientId) > 0;
    }

    @Override
    public void persist(ClientAccessGrant grant) {
        if (grant.grantedAt == null) {
            grant.grantedAt = Instant.now();
        }
        ClientAccessGrantEntity entity = ClientAccessGrantMapper.toEntity(grant);
        persist(entity);
    }

    @Override
    public void delete(ClientAccessGrant grant) {
        deleteById(grant.id);
    }

    @Override
    public void deleteByPrincipalId(String principalId) {
        delete("principalId", principalId);
    }

    @Override
    public long deleteByPrincipalIdAndClientId(String principalId, String clientId) {
        return delete("principalId = ?1 and clientId = ?2", principalId, clientId);
    }
}
