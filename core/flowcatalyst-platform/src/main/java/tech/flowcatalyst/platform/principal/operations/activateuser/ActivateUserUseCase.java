package tech.flowcatalyst.platform.principal.operations.activateuser;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.platform.principal.PrincipalRepository;
import tech.flowcatalyst.platform.principal.PrincipalType;
import tech.flowcatalyst.platform.principal.events.UserActivated;

import java.time.Instant;
import java.util.Map;

/**
 * Use case for activating a user.
 */
@ApplicationScoped
public class ActivateUserUseCase {

    @Inject
    PrincipalRepository principalRepo;

    @Inject
    UnitOfWork unitOfWork;

    public Result<UserActivated> execute(ActivateUserCommand command, ExecutionContext context) {
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

        if (principal.active) {
            return Result.failure(new UseCaseError.BusinessRuleViolation(
                "ALREADY_ACTIVE",
                "User is already active",
                Map.of("userId", String.valueOf(command.userId()))
            ));
        }

        // Activate user
        principal.active = true;
        principal.updatedAt = Instant.now();

        // Create domain event
        UserActivated event = UserActivated.fromContext(context)
            .userId(principal.id)
            .email(principal.userIdentity != null ? principal.userIdentity.email : null)
            .build();

        // Commit atomically
        return unitOfWork.commit(principal, event, command);
    }
}
