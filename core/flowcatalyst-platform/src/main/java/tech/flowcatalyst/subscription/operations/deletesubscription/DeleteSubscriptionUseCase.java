package tech.flowcatalyst.subscription.operations.deletesubscription;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UseCase;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;
import tech.flowcatalyst.serviceaccount.repository.ServiceAccountRepository;
import tech.flowcatalyst.subscription.Subscription;
import tech.flowcatalyst.subscription.SubscriptionRepository;
import tech.flowcatalyst.subscription.events.SubscriptionDeleted;

import java.util.Map;
import java.util.Optional;

/**
 * Use case for deleting an existing subscription.
 *
 * Note: This is a hard delete, not a soft delete.
 */
@ApplicationScoped
public class DeleteSubscriptionUseCase implements UseCase<DeleteSubscriptionCommand, SubscriptionDeleted> {

    @Inject
    SubscriptionRepository subscriptionRepo;

    @Inject
    ServiceAccountRepository serviceAccountRepo;

    @Inject
    UnitOfWork unitOfWork;

    @Override
    public boolean authorizeResource(DeleteSubscriptionCommand command, ExecutionContext context) {
        var authz = context.authz();
        if (authz == null) return true;
        if (command.subscriptionId() == null || command.subscriptionId().isBlank()) return true;
        var subscription = subscriptionRepo.findByIdOptional(command.subscriptionId()).orElse(null);
        if (subscription == null) return true;
        if (subscription.serviceAccountId() == null) return true;
        var serviceAccount = serviceAccountRepo.findByIdOptional(subscription.serviceAccountId()).orElse(null);
        if (serviceAccount == null) return true;
        if (serviceAccount.applicationId == null) return true;
        return authz.canAccessApplication(serviceAccount.applicationId);
    }

    @Override
    public Result<SubscriptionDeleted> doExecute(DeleteSubscriptionCommand command, ExecutionContext context) {
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

        // Create domain event
        SubscriptionDeleted event = SubscriptionDeleted.fromContext(context)
            .subscriptionId(existing.id())
            .code(existing.code())
            .applicationCode(existing.applicationCode())
            .clientId(existing.clientId())
            .clientIdentifier(existing.clientIdentifier())
            .eventTypes(existing.eventTypes())
            .build();

        // Commit delete atomically (deletes entity, emits event, creates audit log)
        return unitOfWork.commitDelete(existing, event, command);
    }
}
