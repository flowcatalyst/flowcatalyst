package tech.flowcatalyst.subscription.operations.syncsubscriptions;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.dispatchpool.DispatchPool;
import tech.flowcatalyst.dispatchpool.DispatchPoolRepository;
import tech.flowcatalyst.dispatch.DispatchMode;
import tech.flowcatalyst.eventtype.EventType;
import tech.flowcatalyst.eventtype.EventTypeRepository;
import tech.flowcatalyst.platform.application.Application;
import tech.flowcatalyst.platform.application.ApplicationRepository;
import tech.flowcatalyst.platform.common.AuthorizationContext;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;
import tech.flowcatalyst.platform.shared.EntityType;
import tech.flowcatalyst.platform.shared.TsidGenerator;
import tech.flowcatalyst.subscription.*;
import tech.flowcatalyst.subscription.events.SubscriptionsSynced;

import java.time.Instant;
import java.util.*;

/**
 * Use case for syncing subscriptions from an external application (SDK).
 *
 * <p>Subscriptions are synced as anchor-level (clientId = null) with source = API.
 * If a registered Application exists, its service account is used for webhook credentials.
 *
 * <p>Note: A registered Application entity is NOT required. Subscriptions can be
 * synced for modules/prefixes that are not registered applications. In this case,
 * the serviceAccountId will be null (unauthenticated webhooks).
 */
@ApplicationScoped
public class SyncSubscriptionsUseCase {

    @Inject
    SubscriptionRepository subscriptionRepo;

    @Inject
    ApplicationRepository appRepo;

    @Inject
    DispatchPoolRepository poolRepo;

    @Inject
    EventTypeRepository eventTypeRepo;

    @Inject
    UnitOfWork unitOfWork;

    public Result<SubscriptionsSynced> execute(SyncSubscriptionsCommand command, ExecutionContext context) {
        if (command.applicationCode() == null || command.applicationCode().isBlank()) {
            return Result.failure(new UseCaseError.ValidationError(
                "APPLICATION_CODE_REQUIRED",
                "Application code is required",
                Map.of()
            ));
        }

        String codePrefix = command.applicationCode() + ":";

        // Look up application (optional - subscriptions can exist for modules without registered apps)
        Application app = appRepo.findByCode(command.applicationCode()).orElse(null);
        String serviceAccountId = app != null ? app.serviceAccountId : null;

        // Authorization check: can principal manage resources with this prefix?
        AuthorizationContext authz = context.authz();
        if (authz != null && !authz.canManageResourceWithPrefix(codePrefix)) {
            return Result.failure(new UseCaseError.AuthorizationError(
                "NOT_AUTHORIZED",
                "Not authorized to sync subscriptions for this application",
                Map.of("applicationCode", command.applicationCode())
            ));
        }

        Set<String> syncedCodes = new HashSet<>();
        int created = 0;
        int updated = 0;
        int deleted = 0;

        for (SyncSubscriptionsCommand.SyncSubscriptionItem item : command.subscriptions()) {
            // Validate required fields
            if (item.code() == null || item.code().isBlank()) {
                return Result.failure(new UseCaseError.ValidationError(
                    "CODE_REQUIRED",
                    "Subscription code is required",
                    Map.of()
                ));
            }

            String code = item.code().toLowerCase();
            syncedCodes.add(code);

            // Resolve dispatch pool by code
            DispatchPool pool = null;
            if (item.dispatchPoolCode() != null && !item.dispatchPoolCode().isBlank()) {
                pool = poolRepo.findByCodeAndClientId(item.dispatchPoolCode(), null).orElse(null);
                if (pool == null) {
                    return Result.failure(new UseCaseError.NotFoundError(
                        "DISPATCH_POOL_NOT_FOUND",
                        "Dispatch pool not found: " + item.dispatchPoolCode(),
                        Map.of("dispatchPoolCode", item.dispatchPoolCode(), "subscriptionCode", code)
                    ));
                }
            }

            // Resolve event type bindings
            List<EventTypeBinding> eventTypeBindings = new ArrayList<>();
            if (item.eventTypes() != null) {
                for (SyncSubscriptionsCommand.EventTypeBindingItem bindingItem : item.eventTypes()) {
                    EventType eventType = eventTypeRepo.findByCode(bindingItem.eventTypeCode()).orElse(null);
                    if (eventType == null) {
                        return Result.failure(new UseCaseError.NotFoundError(
                            "EVENT_TYPE_NOT_FOUND",
                            "Event type not found: " + bindingItem.eventTypeCode(),
                            Map.of("eventTypeCode", bindingItem.eventTypeCode(), "subscriptionCode", code)
                        ));
                    }
                    eventTypeBindings.add(new EventTypeBinding(
                        eventType.id(),
                        eventType.code(),
                        bindingItem.specVersion()
                    ));
                }
            }

            // Check if subscription exists (anchor-level)
            Optional<Subscription> existingOpt = subscriptionRepo.findByCodeAndClient(code, null);

            if (existingOpt.isPresent()) {
                Subscription existing = existingOpt.get();
                // Only update API-sourced subscriptions
                if (existing.source() == SubscriptionSource.API) {
                    Subscription updatedSub = existing.toBuilder()
                        .applicationCode(command.applicationCode()) // ensure applicationCode is set/updated
                        .name(item.name() != null ? item.name() : existing.name())
                        .description(item.description())
                        .eventTypes(eventTypeBindings.isEmpty() ? existing.eventTypes() : eventTypeBindings)
                        .target(item.target() != null ? item.target() : existing.target())
                        .queue(item.queue() != null ? item.queue() : existing.queue())
                        .customConfig(item.customConfig() != null ? item.customConfig() : existing.customConfig())
                        .maxAgeSeconds(item.maxAgeSeconds() != null ? item.maxAgeSeconds() : existing.maxAgeSeconds())
                        .dispatchPoolId(pool != null ? pool.id() : existing.dispatchPoolId())
                        .dispatchPoolCode(pool != null ? pool.code() : existing.dispatchPoolCode())
                        .delaySeconds(item.delaySeconds() != null ? item.delaySeconds() : existing.delaySeconds())
                        .sequence(item.sequence() != null ? item.sequence() : existing.sequence())
                        .mode(item.mode() != null ? item.mode() : existing.mode())
                        .timeoutSeconds(item.timeoutSeconds() != null ? item.timeoutSeconds() : existing.timeoutSeconds())
                        .maxRetries(item.maxRetries() != null ? item.maxRetries() : existing.maxRetries())
                        .dataOnly(item.dataOnly() != null ? item.dataOnly() : existing.dataOnly())
                        .updatedAt(Instant.now())
                        .build();
                    subscriptionRepo.update(updatedSub);
                    updated++;
                }
                // Don't update UI-sourced subscriptions from SDK sync
            } else {
                // Validate required fields for creation
                if (item.target() == null || item.target().isBlank()) {
                    return Result.failure(new UseCaseError.ValidationError(
                        "TARGET_REQUIRED",
                        "Target URL is required for new subscription: " + code,
                        Map.of("subscriptionCode", code)
                    ));
                }
                if (item.queue() == null || item.queue().isBlank()) {
                    return Result.failure(new UseCaseError.ValidationError(
                        "QUEUE_REQUIRED",
                        "Queue name is required for new subscription: " + code,
                        Map.of("subscriptionCode", code)
                    ));
                }
                if (pool == null) {
                    return Result.failure(new UseCaseError.ValidationError(
                        "DISPATCH_POOL_REQUIRED",
                        "Dispatch pool is required for new subscription: " + code,
                        Map.of("subscriptionCode", code)
                    ));
                }

                // Create new subscription (default to non-client-scoped for SDK sync)
                boolean clientScoped = item.clientScoped() != null ? item.clientScoped() : false;
                Instant now = Instant.now();
                Subscription newSub = new Subscription(
                    TsidGenerator.generate(EntityType.SUBSCRIPTION),
                    code,
                    command.applicationCode(), // application/module that owns this subscription
                    item.name() != null ? item.name() : code,
                    item.description(),
                    null, // anchor-level
                    null, // no client identifier
                    clientScoped,
                    eventTypeBindings,
                    item.target(),
                    item.queue(),
                    item.customConfig(),
                    SubscriptionSource.API,
                    SubscriptionStatus.ACTIVE,
                    item.maxAgeSeconds() != null ? item.maxAgeSeconds() : Subscription.DEFAULT_MAX_AGE_SECONDS,
                    pool.id(),
                    pool.code(),
                    item.delaySeconds() != null ? item.delaySeconds() : Subscription.DEFAULT_DELAY_SECONDS,
                    item.sequence() != null ? item.sequence() : Subscription.DEFAULT_SEQUENCE,
                    item.mode() != null ? item.mode() : DispatchMode.IMMEDIATE,
                    item.timeoutSeconds() != null ? item.timeoutSeconds() : Subscription.DEFAULT_TIMEOUT_SECONDS,
                    item.maxRetries() != null ? item.maxRetries() : Subscription.DEFAULT_MAX_RETRIES,
                    serviceAccountId,
                    item.dataOnly() != null ? item.dataOnly() : Subscription.DEFAULT_DATA_ONLY,
                    now,
                    now
                );
                subscriptionRepo.persist(newSub);
                created++;
            }
        }

        if (command.removeUnlisted()) {
            // Remove API-sourced anchor-level subscriptions not in the sync list
            List<Subscription> existing = subscriptionRepo.findAnchorLevel();
            for (Subscription sub : existing) {
                if (sub.source() == SubscriptionSource.API && !syncedCodes.contains(sub.code())) {
                    subscriptionRepo.delete(sub);
                    deleted++;
                }
            }
        }

        // Create domain event
        SubscriptionsSynced event = SubscriptionsSynced.fromContext(context)
            .applicationCode(command.applicationCode())
            .subscriptionsCreated(created)
            .subscriptionsUpdated(updated)
            .subscriptionsDeleted(deleted)
            .syncedSubscriptionCodes(new ArrayList<>(syncedCodes))
            .build();

        // Commit - no entity to persist (subscriptions already persisted via repository)
        return unitOfWork.commitAll(List.of(), event, command);
    }
}
