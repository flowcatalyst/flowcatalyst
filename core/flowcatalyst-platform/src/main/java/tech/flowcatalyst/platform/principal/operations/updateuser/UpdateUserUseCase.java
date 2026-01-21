package tech.flowcatalyst.platform.principal.operations.updateuser;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.platform.principal.PrincipalRepository;
import tech.flowcatalyst.platform.principal.PrincipalType;
import tech.flowcatalyst.platform.principal.events.UserUpdated;

import java.time.Instant;
import java.util.Map;

/**
 * Use case for updating a user.
 */
@ApplicationScoped
public class UpdateUserUseCase {

    @Inject
    PrincipalRepository principalRepo;

    @Inject
    UnitOfWork unitOfWork;

    public Result<UserUpdated> execute(UpdateUserCommand command, ExecutionContext context) {
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

        // Update fields
        if (command.name() != null && !command.name().isBlank()) {
            principal.name = command.name();
        }

        if (command.clientId() != null) {
            if (command.clientId().isEmpty()) {
                // Clear client ID (empty string signals clearing)
                principal.clientId = null;
            } else {
                principal.clientId = command.clientId();
            }
        }

        principal.updatedAt = Instant.now();

        // Create domain event
        UserUpdated event = UserUpdated.fromContext(context)
            .userId(principal.id)
            .name(principal.name)
            .clientId(principal.clientId)
            .build();

        // Commit atomically
        return unitOfWork.commit(principal, event, command);
    }
}
