package tech.flowcatalyst.subscription.panache;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.persistence.EntityManager;
import jakarta.persistence.TypedQuery;
import tech.flowcatalyst.subscription.*;
import tech.flowcatalyst.subscription.entity.SubscriptionConfigEntity;
import tech.flowcatalyst.subscription.entity.SubscriptionEntity;
import tech.flowcatalyst.subscription.entity.SubscriptionEventTypeEntity;
import tech.flowcatalyst.subscription.mapper.SubscriptionMapper;

import java.util.List;
import java.util.Optional;

/**
 * Read-side repository for Subscription entities.
 * Uses EntityManager directly to return domain objects without conflicts.
 */
@ApplicationScoped
public class SubscriptionReadRepository implements SubscriptionRepository {

    @Inject
    EntityManager em;

    @Inject
    SubscriptionWriteRepository writeRepo;

    @Override
    public Subscription findById(String id) {
        return findByIdOptional(id).orElse(null);
    }

    @Override
    public Optional<Subscription> findByIdOptional(String id) {
        SubscriptionEntity entity = em.find(SubscriptionEntity.class, id);
        if (entity == null) {
            return Optional.empty();
        }
        return Optional.of(toDomainWithRelations(entity));
    }

    @Override
    public Optional<Subscription> findByCodeAndClient(String code, String clientId) {
        String jpql;
        TypedQuery<SubscriptionEntity> query;
        if (clientId == null) {
            jpql = "FROM SubscriptionEntity WHERE code = :code AND clientId IS NULL";
            query = em.createQuery(jpql, SubscriptionEntity.class)
                .setParameter("code", code);
        } else {
            jpql = "FROM SubscriptionEntity WHERE code = :code AND clientId = :clientId";
            query = em.createQuery(jpql, SubscriptionEntity.class)
                .setParameter("code", code)
                .setParameter("clientId", clientId);
        }
        var results = query.getResultList();
        return results.isEmpty() ? Optional.empty() : Optional.of(toDomainWithRelations(results.get(0)));
    }

    @Override
    public List<Subscription> findByClientId(String clientId) {
        return em.createQuery("FROM SubscriptionEntity WHERE clientId = :clientId", SubscriptionEntity.class)
            .setParameter("clientId", clientId)
            .getResultList()
            .stream()
            .map(this::toDomainWithRelations)
            .toList();
    }

    @Override
    public List<Subscription> findAnchorLevel() {
        return em.createQuery("FROM SubscriptionEntity WHERE clientId IS NULL", SubscriptionEntity.class)
            .getResultList()
            .stream()
            .map(this::toDomainWithRelations)
            .toList();
    }

    @Override
    public List<Subscription> findByDispatchPoolId(String dispatchPoolId) {
        return em.createQuery("FROM SubscriptionEntity WHERE dispatchPoolId = :poolId", SubscriptionEntity.class)
            .setParameter("poolId", dispatchPoolId)
            .getResultList()
            .stream()
            .map(this::toDomainWithRelations)
            .toList();
    }

    @Override
    public List<Subscription> findByEventTypeId(String eventTypeId) {
        @SuppressWarnings("unchecked")
        List<SubscriptionEntity> results = em.createNativeQuery(
                "SELECT s.* FROM subscriptions s " +
                "JOIN subscription_event_types e ON s.id = e.subscription_id " +
                "WHERE e.event_type_id = :eventTypeId",
                SubscriptionEntity.class)
            .setParameter("eventTypeId", eventTypeId)
            .getResultList();
        return results.stream().map(this::toDomainWithRelations).toList();
    }

    @Override
    public List<Subscription> findByStatus(SubscriptionStatus status) {
        return em.createQuery("FROM SubscriptionEntity WHERE status = :status", SubscriptionEntity.class)
            .setParameter("status", status)
            .getResultList()
            .stream()
            .map(this::toDomainWithRelations)
            .toList();
    }

    @Override
    public List<Subscription> findActive() {
        return findByStatus(SubscriptionStatus.ACTIVE);
    }

    @Override
    public List<Subscription> findWithFilters(String clientId, SubscriptionStatus status, SubscriptionSource source, String dispatchPoolId) {
        StringBuilder jpql = new StringBuilder("FROM SubscriptionEntity WHERE 1=1");

        if (clientId != null) {
            jpql.append(" AND clientId = :clientId");
        }
        if (status != null) {
            jpql.append(" AND status = :status");
        }
        if (source != null) {
            jpql.append(" AND source = :source");
        }
        if (dispatchPoolId != null) {
            jpql.append(" AND dispatchPoolId = :dispatchPoolId");
        }

        TypedQuery<SubscriptionEntity> query = em.createQuery(jpql.toString(), SubscriptionEntity.class);

        if (clientId != null) {
            query.setParameter("clientId", clientId);
        }
        if (status != null) {
            query.setParameter("status", status);
        }
        if (source != null) {
            query.setParameter("source", source);
        }
        if (dispatchPoolId != null) {
            query.setParameter("dispatchPoolId", dispatchPoolId);
        }

        return query.getResultList()
            .stream()
            .map(this::toDomainWithRelations)
            .toList();
    }

    @Override
    public List<Subscription> findActiveByEventTypeAndClient(String eventTypeId, String clientId) {
        @SuppressWarnings("unchecked")
        List<SubscriptionEntity> results = em.createNativeQuery(
                "SELECT s.* FROM subscriptions s " +
                "JOIN subscription_event_types e ON s.id = e.subscription_id " +
                "WHERE e.event_type_id = :eventTypeId AND s.status = 'ACTIVE' " +
                "AND (s.client_id = :clientId OR s.client_id IS NULL)",
                SubscriptionEntity.class)
            .setParameter("eventTypeId", eventTypeId)
            .setParameter("clientId", clientId)
            .getResultList();
        return results.stream().map(this::toDomainWithRelations).toList();
    }

    @Override
    public List<Subscription> findActiveByEventTypeCodeAndClient(String eventTypeCode, String clientId) {
        @SuppressWarnings("unchecked")
        List<SubscriptionEntity> results = em.createNativeQuery(
                "SELECT s.* FROM subscriptions s " +
                "JOIN subscription_event_types e ON s.id = e.subscription_id " +
                "WHERE e.event_type_code = :eventTypeCode AND s.status = 'ACTIVE' " +
                "AND (s.client_id = :clientId OR s.client_id IS NULL)",
                SubscriptionEntity.class)
            .setParameter("eventTypeCode", eventTypeCode)
            .setParameter("clientId", clientId)
            .getResultList();
        return results.stream().map(this::toDomainWithRelations).toList();
    }

    @Override
    public List<Subscription> listAll() {
        return em.createQuery("FROM SubscriptionEntity", SubscriptionEntity.class)
            .getResultList()
            .stream()
            .map(this::toDomainWithRelations)
            .toList();
    }

    @Override
    public long count() {
        return em.createQuery("SELECT COUNT(e) FROM SubscriptionEntity e", Long.class)
            .getSingleResult();
    }

    @Override
    public boolean existsByCodeAndClient(String code, String clientId) {
        String jpql;
        TypedQuery<Long> query;
        if (clientId == null) {
            jpql = "SELECT COUNT(e) FROM SubscriptionEntity e WHERE e.code = :code AND e.clientId IS NULL";
            query = em.createQuery(jpql, Long.class)
                .setParameter("code", code);
        } else {
            jpql = "SELECT COUNT(e) FROM SubscriptionEntity e WHERE e.code = :code AND e.clientId = :clientId";
            query = em.createQuery(jpql, Long.class)
                .setParameter("code", code)
                .setParameter("clientId", clientId);
        }
        return query.getSingleResult() > 0;
    }

    @Override
    public boolean existsByDispatchPoolId(String dispatchPoolId) {
        Long count = em.createQuery(
                "SELECT COUNT(e) FROM SubscriptionEntity e WHERE e.dispatchPoolId = :poolId", Long.class)
            .setParameter("poolId", dispatchPoolId)
            .getSingleResult();
        return count > 0;
    }

    // Write operations delegate to WriteRepository
    @Override
    public void persist(Subscription subscription) {
        writeRepo.persistSubscription(subscription);
    }

    @Override
    public void update(Subscription subscription) {
        writeRepo.updateSubscription(subscription);
    }

    @Override
    public void delete(Subscription subscription) {
        writeRepo.deleteSubscription(subscription.id());
    }

    @Override
    public boolean deleteById(String id) {
        return writeRepo.deleteSubscription(id);
    }

    // ========================================================================
    // Helper Methods
    // ========================================================================

    private Subscription toDomainWithRelations(SubscriptionEntity entity) {
        Subscription base = SubscriptionMapper.toDomain(entity);

        // Load event types
        List<SubscriptionEventTypeEntity> eventTypeEntities = em.createQuery(
                "FROM SubscriptionEventTypeEntity WHERE subscriptionId = :id", SubscriptionEventTypeEntity.class)
            .setParameter("id", entity.id)
            .getResultList();
        List<EventTypeBinding> eventTypes = SubscriptionMapper.toEventTypeBindings(eventTypeEntities);

        // Load config entries
        List<SubscriptionConfigEntity> configEntities = em.createQuery(
                "FROM SubscriptionConfigEntity WHERE subscriptionId = :id", SubscriptionConfigEntity.class)
            .setParameter("id", entity.id)
            .getResultList();
        List<ConfigEntry> configEntries = SubscriptionMapper.toConfigEntries(configEntities);

        return base.toBuilder()
            .eventTypes(eventTypes)
            .customConfig(configEntries)
            .build();
    }
}
