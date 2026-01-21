package tech.flowcatalyst.serviceaccount.operations.updateserviceaccount;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;
import tech.flowcatalyst.serviceaccount.entity.ServiceAccount;
import tech.flowcatalyst.serviceaccount.repository.ServiceAccountRepository;

import java.time.Instant;
import java.util.ArrayList;
import java.util.Map;

/**
 * Use case for updating a service account's metadata.
 */
@ApplicationScoped
public class UpdateServiceAccountUseCase {

    @Inject
    ServiceAccountRepository repository;

    @Inject
    UnitOfWork unitOfWork;

    public Result<ServiceAccountUpdated> execute(UpdateServiceAccountCommand command, ExecutionContext context) {
        // Find service account
        ServiceAccount sa = repository.findByIdOptional(command.serviceAccountId()).orElse(null);
        if (sa == null) {
            return Result.failure(new UseCaseError.NotFoundError(
                "SERVICE_ACCOUNT_NOT_FOUND",
                "Service account not found",
                Map.of("serviceAccountId", command.serviceAccountId())
            ));
        }

        // Update fields if provided
        boolean changed = false;

        if (command.name() != null && !command.name().isBlank()) {
            sa.name = command.name();
            changed = true;
        }

        if (command.description() != null) {
            sa.description = command.description();
            changed = true;
        }

        if (command.clientIds() != null) {
            sa.clientIds = new ArrayList<>(command.clientIds());
            changed = true;
        }

        if (!changed) {
            return Result.failure(new UseCaseError.ValidationError(
                "NO_CHANGES",
                "No changes provided",
                Map.of()
            ));
        }

        sa.updatedAt = Instant.now();

        // Create event
        ServiceAccountUpdated event = ServiceAccountUpdated.fromContext(context)
            .serviceAccountId(sa.id)
            .code(sa.code)
            .name(sa.name)
            .description(sa.description)
            .build();

        return unitOfWork.commit(sa, event, command);
    }
}
