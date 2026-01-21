package tech.flowcatalyst.platform.principal.operations.deleteuser;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.client.ClientAccessGrantRepository;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.platform.principal.PrincipalRepository;
import tech.flowcatalyst.platform.principal.PrincipalType;
import tech.flowcatalyst.platform.principal.events.UserDeleted;

import java.util.Map;

/**
 * Use case for deleting a user.
 */
@ApplicationScoped
public class DeleteUserUseCase {

    @Inject
    PrincipalRepository principalRepo;

    @Inject
    ClientAccessGrantRepository grantRepo;

    @Inject
    UnitOfWork unitOfWork;

    public Result<UserDeleted> execute(DeleteUserCommand command, ExecutionContext context) {
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

        String email = principal.userIdentity != null ? principal.userIdentity.email : null;

        // Delete client access grants
        grantRepo.deleteByPrincipalId(principal.id);

        // Create domain event
        UserDeleted event = UserDeleted.fromContext(context)
            .userId(principal.id)
            .email(email)
            .build();

        // Delete and commit atomically
        return unitOfWork.commitDelete(principal, event, command);
    }
}
