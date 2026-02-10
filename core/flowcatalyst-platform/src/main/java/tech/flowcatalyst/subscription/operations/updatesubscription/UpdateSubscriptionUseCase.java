package tech.flowcatalyst.subscription.operations.updatesubscription;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.dispatchpool.DispatchPool;
import tech.flowcatalyst.dispatchpool.DispatchPoolRepository;
import tech.flowcatalyst.eventtype.EventType;
import tech.flowcatalyst.eventtype.EventTypeRepository;
import tech.flowcatalyst.platform.common.AuthorizationContext;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UseCase;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.dispatch.DispatchMode;
import tech.flowcatalyst.platform.common.errors.UseCaseError;
import tech.flowcatalyst.serviceaccount.entity.ServiceAccount;
import tech.flowcatalyst.serviceaccount.repository.ServiceAccountRepository;
import tech.flowcatalyst.subscription.*;
import tech.flowcatalyst.subscription.events.SubscriptionUpdated;

import java.time.Instant;
import java.util.List;
import java.util.Map;
import java.util.Optional;

/**
 * Use case for updating an existing subscription.
 */
@ApplicationScoped
public class UpdateSubscriptionUseCase implements UseCase<UpdateSubscriptionCommand, SubscriptionUpdated> {

    @Inject
    SubscriptionRepository subscriptionRepo;

    @Inject
    DispatchPoolRepository poolRepo;

    @Inject
    ServiceAccountRepository serviceAccountRepo;

    @Inject
    EventTypeRepository eventTypeRepo;

    @Inject
    UnitOfWork unitOfWork;

    @Override
    public boolean authorizeResource(UpdateSubscriptionCommand command, ExecutionContext context) {
        return true;
    }

    @Override
    public Result<SubscriptionUpdated> doExecute(UpdateSubscriptionCommand command, ExecutionContext context) {
        // Validate subscription ID
        if (command.subscriptionId() == null || command.subscriptionId().isBlank()) {
            return Result.failure(new UseCaseError.ValidationError(
                "SUBSCRIPTION_ID_REQUIRED",
                "Subscription ID is required",
                Map.of()
            ));
        }

        // Find existing subscription
        Optional<Subscription> existingOpt = subscriptionRepo.findByIdOptional(command.subscriptionId());
        if (existingOpt.isEmpty()) {
            return Result.failure(new UseCaseError.NotFoundError(
                "SUBSCRIPTION_NOT_FOUND",
                "Subscription not found",
                Map.of("subscriptionId", command.subscriptionId())
            ));
        }

        Subscription existing = existingOpt.get();

        // Authorization check: if subscription's service account is linked to an application, can principal manage it?
        AuthorizationContext authz = context.authz();
        if (authz != null && existing.serviceAccountId() != null) {
            Optional<ServiceAccount> serviceAccountOpt = serviceAccountRepo.findByIdOptional(existing.serviceAccountId());
            if (serviceAccountOpt.isPresent()) {
                ServiceAccount serviceAccount = serviceAccountOpt.get();
                if (serviceAccount.applicationId != null && !authz.canAccessApplication(serviceAccount.applicationId)) {
                    return Result.failure(new UseCaseError.AuthorizationError(
                        "NOT_AUTHORIZED",
                        "Not authorized to update this subscription",
                        Map.of("subscriptionId", command.subscriptionId(), "applicationId", serviceAccount.applicationId)
                    ));
                }
            }
        }

        // Validate dispatch pool if changing
        String dispatchPoolId = existing.dispatchPoolId();
        String dispatchPoolCode = existing.dispatchPoolCode();
        if (command.dispatchPoolId() != null && !command.dispatchPoolId().equals(existing.dispatchPoolId())) {
            Optional<DispatchPool> poolOpt = poolRepo.findByIdOptional(command.dispatchPoolId());
            if (poolOpt.isEmpty()) {
                return Result.failure(new UseCaseError.NotFoundError(
                    "DISPATCH_POOL_NOT_FOUND",
                    "Dispatch pool not found",
                    Map.of("dispatchPoolId", command.dispatchPoolId())
                ));
            }
            DispatchPool pool = poolOpt.get();
            dispatchPoolId = pool.id();
            dispatchPoolCode = pool.code();
        }

        // Validate event types if changing
        if (command.eventTypes() != null && command.eventTypes().isEmpty()) {
            return Result.failure(new UseCaseError.ValidationError(
                "EVENT_TYPES_REQUIRED",
                "At least one event type binding is required",
                Map.of()
            ));
        }

        // Validate that new event types have matching clientScoped value
        if (command.eventTypes() != null) {
            for (EventTypeBinding binding : command.eventTypes()) {
                if (binding.eventTypeId() == null && binding.eventTypeCode() == null) {
                    continue;
                }

                EventType eventType = null;
                if (binding.eventTypeId() != null) {
                    eventType = eventTypeRepo.findByIdOptional(binding.eventTypeId()).orElse(null);
                } else if (binding.eventTypeCode() != null) {
                    eventType = eventTypeRepo.findByCode(binding.eventTypeCode()).orElse(null);
                }

                if (eventType == null) {
                    return Result.failure(new UseCaseError.NotFoundError(
                        "EVENT_TYPE_NOT_FOUND",
                        "Event type not found",
                        Map.of("eventTypeId", String.valueOf(binding.eventTypeId()),
                               "eventTypeCode", String.valueOf(binding.eventTypeCode()))
                    ));
                }

                if (eventType.clientScoped() != existing.clientScoped()) {
                    return Result.failure(new UseCaseError.ValidationError(
                        "CLIENT_SCOPED_MISMATCH",
                        existing.clientScoped()
                            ? "Cannot bind non-client-scoped event type to client-scoped subscription"
                            : "Cannot bind client-scoped event type to non-client-scoped subscription",
                        Map.of("subscriptionClientScoped", existing.clientScoped(),
                               "eventTypeCode", eventType.code(),
                               "eventTypeClientScoped", eventType.clientScoped())
                    ));
                }
            }
        }

        // Validate service account if changing
        String newServiceAccountId = existing.serviceAccountId();
        if (command.serviceAccountId() != null && !command.serviceAccountId().equals(existing.serviceAccountId())) {
            if (!serviceAccountRepo.findByIdOptional(command.serviceAccountId()).isPresent()) {
                return Result.failure(new UseCaseError.NotFoundError(
                    "SERVICE_ACCOUNT_NOT_FOUND",
                    "Service account not found",
                    Map.of("serviceAccountId", command.serviceAccountId())
                ));
            }
            newServiceAccountId = command.serviceAccountId();
        }

        // Apply updates
        String newName = command.name() != null ? command.name() : existing.name();
        String newDescription = command.description() != null ? command.description() : existing.description();
        List<EventTypeBinding> newEventTypes = command.eventTypes() != null ? command.eventTypes() : existing.eventTypes();
        String newTarget = command.target() != null ? command.target() : existing.target();
        String newQueue = command.queue() != null ? command.queue() : existing.queue();
        List<ConfigEntry> newCustomConfig = command.customConfig() != null ? command.customConfig() : existing.customConfig();
        SubscriptionStatus newStatus = command.status() != null ? command.status() : existing.status();
        int newMaxAgeSeconds = command.maxAgeSeconds() != null ? command.maxAgeSeconds() : existing.maxAgeSeconds();
        int newDelaySeconds = command.delaySeconds() != null ? command.delaySeconds() : existing.delaySeconds();
        int newSequence = command.sequence() != null ? command.sequence() : existing.sequence();
        DispatchMode newMode = command.mode() != null ? command.mode() : existing.mode();
        int newTimeoutSeconds = command.timeoutSeconds() != null ? command.timeoutSeconds() : existing.timeoutSeconds();
        int newMaxRetries = command.maxRetries() != null ? command.maxRetries() : existing.maxRetries();
        boolean newDataOnly = command.dataOnly() != null ? command.dataOnly() : existing.dataOnly();

        // Create updated subscription (clientScoped is immutable - preserve existing value)
        Subscription updated = new Subscription(
            existing.id(),
            existing.code(),
            existing.applicationCode(),  // applicationCode is immutable - preserve existing
            newName,
            newDescription,
            existing.clientId(),
            existing.clientIdentifier(),
            existing.clientScoped(),  // immutable - preserve existing
            newEventTypes,
            newTarget,
            newQueue,
            newCustomConfig,
            existing.source(),
            newStatus,
            newMaxAgeSeconds,
            dispatchPoolId,
            dispatchPoolCode,
            newDelaySeconds,
            newSequence,
            newMode,
            newTimeoutSeconds,
            newMaxRetries,
            newServiceAccountId,
            newDataOnly,
            existing.createdAt(),
            Instant.now()
        );

        // Create domain event
        SubscriptionUpdated event = SubscriptionUpdated.fromContext(context)
            .subscriptionId(updated.id())
            .code(updated.code())
            .applicationCode(updated.applicationCode())
            .name(updated.name())
            .description(updated.description())
            .clientId(updated.clientId())
            .clientIdentifier(updated.clientIdentifier())
            .eventTypes(updated.eventTypes())
            .target(updated.target())
            .queue(updated.queue())
            .customConfig(updated.customConfig())
            .status(updated.status())
            .maxAgeSeconds(updated.maxAgeSeconds())
            .dispatchPoolId(updated.dispatchPoolId())
            .dispatchPoolCode(updated.dispatchPoolCode())
            .delaySeconds(updated.delaySeconds())
            .sequence(updated.sequence())
            .mode(updated.mode())
            .timeoutSeconds(updated.timeoutSeconds())
            .maxRetries(updated.maxRetries())
            .serviceAccountId(updated.serviceAccountId())
            .dataOnly(updated.dataOnly())
            .build();

        // Commit atomically
        return unitOfWork.commit(updated, event, command);
    }
}
