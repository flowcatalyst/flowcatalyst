package tech.flowcatalyst.subscription.mapper;

import tech.flowcatalyst.subscription.*;
import tech.flowcatalyst.subscription.entity.SubscriptionConfigEntity;
import tech.flowcatalyst.subscription.entity.SubscriptionEntity;
import tech.flowcatalyst.subscription.entity.SubscriptionEventTypeEntity;

import java.util.ArrayList;
import java.util.List;

/**
 * Mapper for converting between Subscription domain and entity.
 */
public final class SubscriptionMapper {

    private SubscriptionMapper() {
    }

    /**
     * Convert entity to domain object.
     */
    public static Subscription toDomain(SubscriptionEntity entity) {
        if (entity == null) {
            return null;
        }

        return Subscription.builder()
            .id(entity.id)
            .code(entity.code)
            .applicationCode(entity.applicationCode)
            .name(entity.name)
            .description(entity.description)
            .clientId(entity.clientId)
            .clientIdentifier(entity.clientIdentifier)
            .clientScoped(entity.clientScoped)
            .eventTypes(new ArrayList<>()) // loaded separately
            .target(entity.target)
            .queue(entity.queue)
            .customConfig(new ArrayList<>()) // loaded separately
            .source(entity.source)
            .status(entity.status)
            .maxAgeSeconds(entity.maxAgeSeconds)
            .dispatchPoolId(entity.dispatchPoolId)
            .dispatchPoolCode(entity.dispatchPoolCode)
            .delaySeconds(entity.delaySeconds)
            .sequence(entity.sequence)
            .mode(entity.mode)
            .timeoutSeconds(entity.timeoutSeconds)
            .maxRetries(entity.maxRetries)
            .serviceAccountId(entity.serviceAccountId)
            .dataOnly(entity.dataOnly)
            .createdAt(entity.createdAt)
            .updatedAt(entity.updatedAt)
            .build();
    }

    /**
     * Convert domain object to entity.
     */
    public static SubscriptionEntity toEntity(Subscription domain) {
        if (domain == null) {
            return null;
        }

        SubscriptionEntity entity = new SubscriptionEntity();
        entity.id = domain.id();
        entity.code = domain.code();
        entity.applicationCode = domain.applicationCode();
        entity.name = domain.name();
        entity.description = domain.description();
        entity.clientId = domain.clientId();
        entity.clientIdentifier = domain.clientIdentifier();
        entity.clientScoped = domain.clientScoped();
        entity.target = domain.target();
        entity.queue = domain.queue();
        entity.source = domain.source();
        entity.status = domain.status();
        entity.maxAgeSeconds = domain.maxAgeSeconds();
        entity.dispatchPoolId = domain.dispatchPoolId();
        entity.dispatchPoolCode = domain.dispatchPoolCode();
        entity.delaySeconds = domain.delaySeconds();
        entity.sequence = domain.sequence();
        entity.mode = domain.mode();
        entity.timeoutSeconds = domain.timeoutSeconds();
        entity.maxRetries = domain.maxRetries();
        entity.serviceAccountId = domain.serviceAccountId();
        entity.dataOnly = domain.dataOnly();
        entity.createdAt = domain.createdAt();
        entity.updatedAt = domain.updatedAt();

        return entity;
    }

    /**
     * Update existing entity with values from domain object.
     */
    public static void updateEntity(SubscriptionEntity entity, Subscription domain) {
        entity.code = domain.code();
        entity.applicationCode = domain.applicationCode();
        entity.name = domain.name();
        entity.description = domain.description();
        entity.clientId = domain.clientId();
        entity.clientIdentifier = domain.clientIdentifier();
        entity.target = domain.target();
        entity.queue = domain.queue();
        entity.source = domain.source();
        entity.status = domain.status();
        entity.maxAgeSeconds = domain.maxAgeSeconds();
        entity.dispatchPoolId = domain.dispatchPoolId();
        entity.dispatchPoolCode = domain.dispatchPoolCode();
        entity.delaySeconds = domain.delaySeconds();
        entity.sequence = domain.sequence();
        entity.mode = domain.mode();
        entity.timeoutSeconds = domain.timeoutSeconds();
        entity.maxRetries = domain.maxRetries();
        entity.serviceAccountId = domain.serviceAccountId();
        entity.dataOnly = domain.dataOnly();
        entity.updatedAt = domain.updatedAt();
    }

    /**
     * Convert event type entities to domain bindings.
     */
    public static List<EventTypeBinding> toEventTypeBindings(List<SubscriptionEventTypeEntity> entities) {
        if (entities == null) {
            return new ArrayList<>();
        }
        return entities.stream()
            .map(e -> new EventTypeBinding(e.eventTypeId, e.eventTypeCode, e.specVersion))
            .toList();
    }

    /**
     * Convert domain bindings to event type entities.
     */
    public static List<SubscriptionEventTypeEntity> toEventTypeEntities(String subscriptionId, List<EventTypeBinding> bindings) {
        if (bindings == null) {
            return new ArrayList<>();
        }
        return bindings.stream()
            .map(b -> new SubscriptionEventTypeEntity(subscriptionId, b.eventTypeId(), b.eventTypeCode(), b.specVersion()))
            .toList();
    }

    /**
     * Convert config entities to domain entries.
     */
    public static List<ConfigEntry> toConfigEntries(List<SubscriptionConfigEntity> entities) {
        if (entities == null) {
            return new ArrayList<>();
        }
        return entities.stream()
            .map(e -> new ConfigEntry(e.key, e.value))
            .toList();
    }

    /**
     * Convert domain entries to config entities.
     */
    public static List<SubscriptionConfigEntity> toConfigEntities(String subscriptionId, List<ConfigEntry> entries) {
        if (entries == null) {
            return new ArrayList<>();
        }
        return entries.stream()
            .map(e -> new SubscriptionConfigEntity(subscriptionId, e.key(), e.value()))
            .toList();
    }
}
