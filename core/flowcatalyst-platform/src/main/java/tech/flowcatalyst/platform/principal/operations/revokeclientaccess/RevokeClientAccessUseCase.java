package tech.flowcatalyst.platform.principal.operations.revokeclientaccess;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.client.ClientAccessGrant;
import tech.flowcatalyst.platform.client.ClientAccessGrantRepository;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.platform.principal.PrincipalRepository;
import tech.flowcatalyst.platform.principal.PrincipalType;
import tech.flowcatalyst.platform.principal.events.ClientAccessRevoked;

import java.util.Map;
import java.util.Optional;

/**
 * Use case for revoking a user's access to a client.
 */
@ApplicationScoped
public class RevokeClientAccessUseCase {

    @Inject
    PrincipalRepository principalRepo;

    @Inject
    ClientAccessGrantRepository grantRepo;

    @Inject
    UnitOfWork unitOfWork;

    public Result<ClientAccessRevoked> execute(RevokeClientAccessCommand command, ExecutionContext context) {
        // Validate user exists
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

        // Find the grant
        Optional<ClientAccessGrant> grantOpt = grantRepo.findByPrincipalIdAndClientId(principal.id, command.clientId());

        if (grantOpt.isEmpty()) {
            return Result.failure(new UseCaseError.NotFoundError(
                "GRANT_NOT_FOUND",
                "User does not have access to this client",
                Map.of("userId", String.valueOf(command.userId()), "clientId", String.valueOf(command.clientId()))
            ));
        }

        ClientAccessGrant grant = grantOpt.get();

        // Create domain event
        ClientAccessRevoked event = ClientAccessRevoked.fromContext(context)
            .userId(principal.id)
            .clientId(command.clientId())
            .build();

        // Delete and commit atomically
        return unitOfWork.commitDelete(grant, event, command);
    }
}
