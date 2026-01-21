package tech.flowcatalyst.subscription.operations.createsubscription;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.dispatchpool.DispatchPool;
import tech.flowcatalyst.dispatchpool.DispatchPoolRepository;
import tech.flowcatalyst.platform.client.Client;
import tech.flowcatalyst.platform.client.ClientRepository;
import tech.flowcatalyst.platform.common.AuthorizationContext;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;
import tech.flowcatalyst.serviceaccount.entity.ServiceAccount;
import tech.flowcatalyst.dispatch.DispatchMode;
import tech.flowcatalyst.platform.shared.TsidGenerator;
import tech.flowcatalyst.serviceaccount.repository.ServiceAccountRepository;
import tech.flowcatalyst.subscription.*;
import tech.flowcatalyst.subscription.events.SubscriptionCreated;

import java.time.Instant;
import java.util.Map;
import java.util.Optional;

/**
 * Use case for creating a new subscription.
 */
@ApplicationScoped
public class CreateSubscriptionUseCase {

    @Inject
    SubscriptionRepository subscriptionRepo;

    @Inject
    DispatchPoolRepository poolRepo;

    @Inject
    ClientRepository clientRepo;

    @Inject
    ServiceAccountRepository serviceAccountRepo;

    @Inject
    UnitOfWork unitOfWork;

    public Result<SubscriptionCreated> execute(CreateSubscriptionCommand command, ExecutionContext context) {
        // Validate code
        if (command.code() == null || command.code().isBlank()) {
            return Result.failure(new UseCaseError.ValidationError(
                "CODE_REQUIRED",
                "Code is required",
                Map.of()
            ));
        }

        // Validate code format
        if (!isValidCode(command.code())) {
            return Result.failure(new UseCaseError.ValidationError(
                "INVALID_CODE_FORMAT",
                "Code must be lowercase alphanumeric with hyphens, starting with a letter",
                Map.of("code", command.code())
            ));
        }

        // Validate name
        if (command.name() == null || command.name().isBlank()) {
            return Result.failure(new UseCaseError.ValidationError(
                "NAME_REQUIRED",
                "Name is required",
                Map.of()
            ));
        }

        // Validate target
        if (command.target() == null || command.target().isBlank()) {
            return Result.failure(new UseCaseError.ValidationError(
                "TARGET_REQUIRED",
                "Target URL is required",
                Map.of()
            ));
        }

        // Validate queue
        if (command.queue() == null || command.queue().isBlank()) {
            return Result.failure(new UseCaseError.ValidationError(
                "QUEUE_REQUIRED",
                "Queue name is required",
                Map.of()
            ));
        }

        // Validate event types
        if (command.eventTypes() == null || command.eventTypes().isEmpty()) {
            return Result.failure(new UseCaseError.ValidationError(
                "EVENT_TYPES_REQUIRED",
                "At least one event type binding is required",
                Map.of()
            ));
        }

        // Validate dispatch pool
        if (command.dispatchPoolId() == null || command.dispatchPoolId().isBlank()) {
            return Result.failure(new UseCaseError.ValidationError(
                "DISPATCH_POOL_REQUIRED",
                "Dispatch pool ID is required",
                Map.of()
            ));
        }

        Optional<DispatchPool> poolOpt = poolRepo.findByIdOptional(command.dispatchPoolId());
        if (poolOpt.isEmpty()) {
            return Result.failure(new UseCaseError.NotFoundError(
                "DISPATCH_POOL_NOT_FOUND",
                "Dispatch pool not found",
                Map.of("dispatchPoolId", command.dispatchPoolId())
            ));
        }
        DispatchPool pool = poolOpt.get();

        // Validate service account
        if (command.serviceAccountId() == null || command.serviceAccountId().isBlank()) {
            return Result.failure(new UseCaseError.ValidationError(
                "SERVICE_ACCOUNT_REQUIRED",
                "Service account ID is required for webhook credentials",
                Map.of()
            ));
        }
        Optional<ServiceAccount> serviceAccountOpt = serviceAccountRepo.findByIdOptional(command.serviceAccountId());
        if (serviceAccountOpt.isEmpty()) {
            return Result.failure(new UseCaseError.NotFoundError(
                "SERVICE_ACCOUNT_NOT_FOUND",
                "Service account not found",
                Map.of("serviceAccountId", command.serviceAccountId())
            ));
        }
        ServiceAccount serviceAccount = serviceAccountOpt.get();

        // Authorization check: if service account is linked to an application, can principal manage it?
        AuthorizationContext authz = context.authz();
        if (authz != null && serviceAccount.applicationId != null &&
                !authz.canManageApplication(serviceAccount.applicationId)) {
            return Result.failure(new UseCaseError.AuthorizationError(
                "NOT_AUTHORIZED",
                "Not authorized to create subscriptions for this application",
                Map.of("serviceAccountId", command.serviceAccountId(), "applicationId", serviceAccount.applicationId)
            ));
        }

        // Validate client (if provided)
        String clientIdentifier = null;
        if (command.clientId() != null && !command.clientId().isBlank()) {
            Optional<Client> clientOpt = clientRepo.findByIdOptional(command.clientId());
            if (clientOpt.isEmpty()) {
                return Result.failure(new UseCaseError.NotFoundError(
                    "CLIENT_NOT_FOUND",
                    "Client not found",
                    Map.of("clientId", command.clientId())
                ));
            }
            clientIdentifier = clientOpt.get().identifier;
        }

        // Check code uniqueness within client scope
        if (subscriptionRepo.existsByCodeAndClient(command.code(), command.clientId())) {
            return Result.failure(new UseCaseError.BusinessRuleViolation(
                "CODE_EXISTS",
                "A subscription with this code already exists in this scope",
                Map.of("code", command.code())
            ));
        }

        // Apply defaults
        int maxAgeSeconds = command.maxAgeSeconds() != null ? command.maxAgeSeconds() : Subscription.DEFAULT_MAX_AGE_SECONDS;
        int delaySeconds = command.delaySeconds() != null ? command.delaySeconds() : Subscription.DEFAULT_DELAY_SECONDS;
        int sequence = command.sequence() != null ? command.sequence() : Subscription.DEFAULT_SEQUENCE;
        int timeoutSeconds = command.timeoutSeconds() != null ? command.timeoutSeconds() : Subscription.DEFAULT_TIMEOUT_SECONDS;
        int maxRetries = command.maxRetries() != null ? command.maxRetries() : Subscription.DEFAULT_MAX_RETRIES;
        DispatchMode mode = command.mode() != null ? command.mode() : DispatchMode.IMMEDIATE;
        SubscriptionSource source = command.source() != null ? command.source() : SubscriptionSource.UI;
        boolean dataOnly = command.dataOnly() != null ? command.dataOnly() : Subscription.DEFAULT_DATA_ONLY;

        // Create subscription
        Instant now = Instant.now();
        Subscription subscription = new Subscription(
            TsidGenerator.generate(),
            command.code().toLowerCase(),
            command.name(),
            command.description(),
            command.clientId(),
            clientIdentifier,
            command.eventTypes(),
            command.target(),
            command.queue(),
            command.customConfig(),
            source,
            SubscriptionStatus.ACTIVE,
            maxAgeSeconds,
            pool.id(),
            pool.code(),
            delaySeconds,
            sequence,
            mode,
            timeoutSeconds,
            maxRetries,
            command.serviceAccountId(),
            dataOnly,
            now,
            now
        );

        // Create domain event
        SubscriptionCreated event = SubscriptionCreated.fromContext(context)
            .subscriptionId(subscription.id())
            .code(subscription.code())
            .name(subscription.name())
            .description(subscription.description())
            .clientId(subscription.clientId())
            .clientIdentifier(subscription.clientIdentifier())
            .eventTypes(subscription.eventTypes())
            .target(subscription.target())
            .queue(subscription.queue())
            .customConfig(subscription.customConfig())
            .subscriptionSource(subscription.source())
            .status(subscription.status())
            .maxAgeSeconds(subscription.maxAgeSeconds())
            .dispatchPoolId(subscription.dispatchPoolId())
            .dispatchPoolCode(subscription.dispatchPoolCode())
            .delaySeconds(subscription.delaySeconds())
            .sequence(subscription.sequence())
            .mode(subscription.mode())
            .timeoutSeconds(subscription.timeoutSeconds())
            .maxRetries(subscription.maxRetries())
            .serviceAccountId(subscription.serviceAccountId())
            .dataOnly(subscription.dataOnly())
            .build();

        // Commit atomically
        return unitOfWork.commit(subscription, event, command);
    }

    private boolean isValidCode(String code) {
        if (code == null || code.isBlank()) {
            return false;
        }
        return code.matches("^[a-z][a-z0-9-]*$");
    }
}
