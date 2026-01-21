package tech.flowcatalyst.subscription.panache;

import io.quarkus.hibernate.orm.panache.PanacheRepositoryBase;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.persistence.EntityManager;
import jakarta.transaction.Transactional;
import tech.flowcatalyst.subscription.Subscription;
import tech.flowcatalyst.subscription.entity.SubscriptionConfigEntity;
import tech.flowcatalyst.subscription.entity.SubscriptionEntity;
import tech.flowcatalyst.subscription.entity.SubscriptionEventTypeEntity;
import tech.flowcatalyst.subscription.mapper.SubscriptionMapper;

import java.time.Instant;
import java.util.List;

/**
 * Write-side repository for Subscription entities.
 * Extends PanacheRepositoryBase for efficient entity persistence.
 */
@ApplicationScoped
@Transactional
public class SubscriptionWriteRepository implements PanacheRepositoryBase<SubscriptionEntity, String> {

    @Inject
    EntityManager em;

    /**
     * Persist a new subscription with its event types and config.
     */
    public void persistSubscription(Subscription subscription) {
        Subscription toSave = subscription;
        if (toSave.createdAt() == null) {
            toSave = toSave.withCreatedAt(Instant.now());
        }
        toSave = toSave.withUpdatedAt(Instant.now());

        SubscriptionEntity entity = SubscriptionMapper.toEntity(toSave);
        persist(entity);

        // Save event types
        saveEventTypes(toSave.id(), toSave.eventTypes());

        // Save config entries
        saveConfigEntries(toSave.id(), toSave.customConfig());
    }

    /**
     * Update an existing subscription with its event types and config.
     */
    public void updateSubscription(Subscription subscription) {
        Subscription toUpdate = subscription.withUpdatedAt(Instant.now());

        SubscriptionEntity entity = findById(subscription.id());
        if (entity != null) {
            SubscriptionMapper.updateEntity(entity, toUpdate);
        }

        // Update event types
        saveEventTypes(subscription.id(), subscription.eventTypes());

        // Update config entries
        saveConfigEntries(subscription.id(), subscription.customConfig());
    }

    /**
     * Delete a subscription and its related entities.
     */
    public boolean deleteSubscription(String id) {
        // Delete event types first
        em.createQuery("DELETE FROM SubscriptionEventTypeEntity WHERE subscriptionId = :id")
            .setParameter("id", id)
            .executeUpdate();

        // Delete config entries
        em.createQuery("DELETE FROM SubscriptionConfigEntity WHERE subscriptionId = :id")
            .setParameter("id", id)
            .executeUpdate();

        return deleteById(id);
    }

    /**
     * Save event types to the normalized table.
     * Replaces all existing event types for the subscription.
     */
    private void saveEventTypes(String subscriptionId, List<tech.flowcatalyst.subscription.EventTypeBinding> eventTypes) {
        // Delete existing
        em.createQuery("DELETE FROM SubscriptionEventTypeEntity WHERE subscriptionId = :id")
            .setParameter("id", subscriptionId)
            .executeUpdate();

        // Insert new
        if (eventTypes != null) {
            List<SubscriptionEventTypeEntity> entities = SubscriptionMapper.toEventTypeEntities(subscriptionId, eventTypes);
            for (SubscriptionEventTypeEntity entity : entities) {
                em.persist(entity);
            }
        }
    }

    /**
     * Save config entries to the normalized table.
     * Replaces all existing config entries for the subscription.
     */
    private void saveConfigEntries(String subscriptionId, List<tech.flowcatalyst.subscription.ConfigEntry> configEntries) {
        // Delete existing
        em.createQuery("DELETE FROM SubscriptionConfigEntity WHERE subscriptionId = :id")
            .setParameter("id", subscriptionId)
            .executeUpdate();

        // Insert new
        if (configEntries != null) {
            List<SubscriptionConfigEntity> entities = SubscriptionMapper.toConfigEntities(subscriptionId, configEntries);
            for (SubscriptionConfigEntity entity : entities) {
                em.persist(entity);
            }
        }
    }
}
