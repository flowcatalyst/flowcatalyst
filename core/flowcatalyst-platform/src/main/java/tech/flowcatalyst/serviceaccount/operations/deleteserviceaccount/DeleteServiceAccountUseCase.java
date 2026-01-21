package tech.flowcatalyst.serviceaccount.operations.deleteserviceaccount;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;
import tech.flowcatalyst.serviceaccount.entity.ServiceAccount;
import tech.flowcatalyst.serviceaccount.repository.ServiceAccountRepository;

import java.util.Map;

/**
 * Use case for deleting a service account.
 */
@ApplicationScoped
public class DeleteServiceAccountUseCase {

    @Inject
    ServiceAccountRepository repository;

    @Inject
    UnitOfWork unitOfWork;

    public Result<ServiceAccountDeleted> execute(DeleteServiceAccountCommand command, ExecutionContext context) {
        // Find service account
        ServiceAccount sa = repository.findByIdOptional(command.serviceAccountId()).orElse(null);
        if (sa == null) {
            return Result.failure(new UseCaseError.NotFoundError(
                "SERVICE_ACCOUNT_NOT_FOUND",
                "Service account not found",
                Map.of("serviceAccountId", command.serviceAccountId())
            ));
        }

        // Create event before deletion
        ServiceAccountDeleted event = ServiceAccountDeleted.fromContext(context)
            .serviceAccountId(sa.id)
            .code(sa.code)
            .name(sa.name)
            .applicationId(sa.applicationId)
            .build();

        // Delete atomically
        return unitOfWork.commitDelete(sa, event, command);
    }
}
