package tech.flowcatalyst.platform.authentication.oauth.panache;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.persistence.EntityManager;
import tech.flowcatalyst.platform.authentication.oauth.OAuthClient;
import tech.flowcatalyst.platform.authentication.oauth.OAuthClientRepository;
import tech.flowcatalyst.platform.authentication.oauth.entity.OAuthClientEntity;
import tech.flowcatalyst.platform.authentication.oauth.mapper.OAuthClientMapper;

import java.util.List;
import java.util.Optional;

/**
 * Read-side repository for OAuthClient entities.
 * Uses EntityManager directly to return domain objects without conflicts.
 */
@ApplicationScoped
public class OAuthClientReadRepository implements OAuthClientRepository {

    @Inject
    EntityManager em;

    @Inject
    OAuthClientWriteRepository writeRepo;

    @Override
    public Optional<OAuthClient> findByIdOptional(String id) {
        OAuthClientEntity entity = em.find(OAuthClientEntity.class, id);
        return Optional.ofNullable(entity).map(OAuthClientMapper::toDomain);
    }

    @Override
    public Optional<OAuthClient> findByClientId(String clientId) {
        var results = em.createQuery(
                "FROM OAuthClientEntity WHERE clientId = :clientId AND active = true", OAuthClientEntity.class)
            .setParameter("clientId", clientId)
            .getResultList();
        return results.isEmpty() ? Optional.empty() : Optional.of(OAuthClientMapper.toDomain(results.get(0)));
    }

    @Override
    public Optional<OAuthClient> findByClientIdIncludingInactive(String clientId) {
        var results = em.createQuery(
                "FROM OAuthClientEntity WHERE clientId = :clientId", OAuthClientEntity.class)
            .setParameter("clientId", clientId)
            .getResultList();
        return results.isEmpty() ? Optional.empty() : Optional.of(OAuthClientMapper.toDomain(results.get(0)));
    }

    @Override
    public List<OAuthClient> findByApplicationIdAndActive(String applicationId, boolean active) {
        @SuppressWarnings("unchecked")
        List<OAuthClientEntity> results = em.createNativeQuery(
                "SELECT c.* FROM oauth_clients c " +
                "JOIN oauth_client_application_ids a ON c.id = a.oauth_client_id " +
                "WHERE a.application_id = :appId AND c.active = :active",
                OAuthClientEntity.class)
            .setParameter("appId", applicationId)
            .setParameter("active", active)
            .getResultList();
        return results.stream().map(OAuthClientMapper::toDomain).toList();
    }

    @Override
    public List<OAuthClient> findByApplicationId(String applicationId) {
        @SuppressWarnings("unchecked")
        List<OAuthClientEntity> results = em.createNativeQuery(
                "SELECT c.* FROM oauth_clients c " +
                "JOIN oauth_client_application_ids a ON c.id = a.oauth_client_id " +
                "WHERE a.application_id = :appId",
                OAuthClientEntity.class)
            .setParameter("appId", applicationId)
            .getResultList();
        return results.stream().map(OAuthClientMapper::toDomain).toList();
    }

    @Override
    public List<OAuthClient> findByActive(boolean active) {
        return em.createQuery("FROM OAuthClientEntity WHERE active = :active", OAuthClientEntity.class)
            .setParameter("active", active)
            .getResultList()
            .stream()
            .map(OAuthClientMapper::toDomain)
            .toList();
    }

    @Override
    public List<OAuthClient> listAll() {
        return em.createQuery("FROM OAuthClientEntity", OAuthClientEntity.class)
            .getResultList()
            .stream()
            .map(OAuthClientMapper::toDomain)
            .toList();
    }

    @Override
    public boolean isOriginAllowedByAnyClient(String origin) {
        Long count = em.createNativeQuery(
                "SELECT COUNT(*) FROM oauth_client_allowed_origins o " +
                "JOIN oauth_clients c ON c.id = o.oauth_client_id " +
                "WHERE o.allowed_origin = :origin AND c.active = true")
            .setParameter("origin", origin)
            .getSingleResult() instanceof Number n ? n.longValue() : 0L;
        return count > 0;
    }

    @Override
    public boolean isOriginUsedByAnyClient(String origin) {
        // Check both redirect URIs and allowed origins
        Long redirectCount = em.createNativeQuery(
                "SELECT COUNT(*) FROM oauth_client_redirect_uris WHERE redirect_uri LIKE :pattern")
            .setParameter("pattern", origin + "%")
            .getSingleResult() instanceof Number n ? n.longValue() : 0L;

        Long originCount = em.createNativeQuery(
                "SELECT COUNT(*) FROM oauth_client_allowed_origins WHERE allowed_origin = :origin")
            .setParameter("origin", origin)
            .getSingleResult() instanceof Number n ? n.longValue() : 0L;

        return redirectCount > 0 || originCount > 0;
    }

    @Override
    public List<String> findClientNamesUsingOrigin(String origin) {
        @SuppressWarnings("unchecked")
        List<String> names = em.createNativeQuery(
                "SELECT DISTINCT c.client_name FROM oauth_clients c " +
                "LEFT JOIN oauth_client_redirect_uris r ON c.id = r.oauth_client_id " +
                "LEFT JOIN oauth_client_allowed_origins o ON c.id = o.oauth_client_id " +
                "WHERE r.redirect_uri LIKE :pattern OR o.allowed_origin = :origin")
            .setParameter("pattern", origin + "%")
            .setParameter("origin", origin)
            .getResultList();
        return names;
    }

    // Write operations delegate to WriteRepository
    @Override
    public void persist(OAuthClient client) {
        writeRepo.persistClient(client);
    }

    @Override
    public void update(OAuthClient client) {
        writeRepo.updateClient(client);
    }

    @Override
    public void delete(OAuthClient client) {
        writeRepo.deleteClientById(client.id);
    }

    @Override
    public long deleteByServiceAccountPrincipalId(String principalId) {
        return writeRepo.deleteByServiceAccountPrincipalId(principalId);
    }
}
