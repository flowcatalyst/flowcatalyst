package tech.flowcatalyst.platform.principal.operations.grantclientaccess;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.client.Client;
import tech.flowcatalyst.platform.client.ClientAccessGrant;
import tech.flowcatalyst.platform.client.ClientAccessGrantRepository;
import tech.flowcatalyst.platform.client.ClientRepository;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.platform.principal.PrincipalRepository;
import tech.flowcatalyst.platform.principal.PrincipalType;
import tech.flowcatalyst.platform.principal.events.ClientAccessGranted;
import tech.flowcatalyst.platform.shared.EntityType;
import tech.flowcatalyst.platform.shared.TsidGenerator;

import java.util.Map;

/**
 * Use case for granting a user access to a client.
 */
@ApplicationScoped
public class GrantClientAccessUseCase {

    @Inject
    PrincipalRepository principalRepo;

    @Inject
    ClientRepository clientRepo;

    @Inject
    ClientAccessGrantRepository grantRepo;

    @Inject
    UnitOfWork unitOfWork;

    public Result<ClientAccessGranted> execute(GrantClientAccessCommand command, ExecutionContext context) {
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

        // Validate client exists
        Client client = clientRepo.findById(command.clientId());

        if (client == null) {
            return Result.failure(new UseCaseError.NotFoundError(
                "CLIENT_NOT_FOUND",
                "Client not found",
                Map.of("clientId", String.valueOf(command.clientId()))
            ));
        }

        // Check if grant already exists
        if (grantRepo.existsByPrincipalIdAndClientId(principal.id, client.id)) {
            return Result.failure(new UseCaseError.BusinessRuleViolation(
                "GRANT_EXISTS",
                "User already has access to this client",
                Map.of("userId", String.valueOf(command.userId()), "clientId", String.valueOf(command.clientId()))
            ));
        }

        // Check if this is the user's home client
        if (principal.clientId != null && principal.clientId.equals(client.id)) {
            return Result.failure(new UseCaseError.BusinessRuleViolation(
                "IS_HOME_CLIENT",
                "Cannot grant access to user's home client",
                Map.of("userId", String.valueOf(command.userId()), "clientId", String.valueOf(command.clientId()))
            ));
        }

        // Create grant
        ClientAccessGrant grant = new ClientAccessGrant();
        grant.id = TsidGenerator.generate(EntityType.CLIENT_ACCESS_GRANT);
        grant.principalId = principal.id;
        grant.clientId = client.id;
        grant.expiresAt = command.expiresAt();

        // Create domain event
        ClientAccessGranted event = ClientAccessGranted.fromContext(context)
            .userId(principal.id)
            .clientId(client.id)
            .grantId(grant.id)
            .expiresAt(grant.expiresAt)
            .build();

        // Commit atomically
        return unitOfWork.commit(grant, event, command);
    }
}
