package tech.flowcatalyst.platform.principal.operations.deactivateuser;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.platform.principal.PrincipalRepository;
import tech.flowcatalyst.platform.principal.PrincipalType;
import tech.flowcatalyst.platform.principal.events.UserDeactivated;

import java.time.Instant;
import java.util.Map;

/**
 * Use case for deactivating a user.
 */
@ApplicationScoped
public class DeactivateUserUseCase {

    @Inject
    PrincipalRepository principalRepo;

    @Inject
    UnitOfWork unitOfWork;

    public Result<UserDeactivated> execute(DeactivateUserCommand command, ExecutionContext context) {
        // Find the user
        Principal principal = principalRepo.findById(command.userId());

        if (principal == null) {
            return Result.failure(new UseCaseError.NotFoundError(
                "USER_NOT_FOUND",
                "User not found",
                Map.of("userId", String.valueOf(command.userId()))
            ));
        }

        if (principal.type != PrincipalType.USER) {
            return Result.failure(new UseCaseError.ValidationError(
                "NOT_A_USER",
                "Principal is not a user",
                Map.of("userId", String.valueOf(command.userId()), "type", principal.type.name())
            ));
        }

        if (!principal.active) {
            return Result.failure(new UseCaseError.BusinessRuleViolation(
                "ALREADY_INACTIVE",
                "User is already inactive",
                Map.of("userId", String.valueOf(command.userId()))
            ));
        }

        // Deactivate user
        principal.active = false;
        principal.updatedAt = Instant.now();

        // Create domain event
        UserDeactivated event = UserDeactivated.fromContext(context)
            .userId(principal.id)
            .email(principal.userIdentity != null ? principal.userIdentity.email : null)
            .reason(command.reason())
            .build();

        // Commit atomically
        return unitOfWork.commit(principal, event, command);
    }
}
