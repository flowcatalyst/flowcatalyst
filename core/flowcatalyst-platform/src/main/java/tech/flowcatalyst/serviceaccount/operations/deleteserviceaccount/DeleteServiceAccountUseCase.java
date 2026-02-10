package tech.flowcatalyst.serviceaccount.operations.deleteserviceaccount;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.authentication.oauth.OAuthClient;
import tech.flowcatalyst.platform.authentication.oauth.OAuthClientRepository;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.UseCase;
import tech.flowcatalyst.platform.common.errors.UseCaseError;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.platform.principal.PrincipalRepository;
import tech.flowcatalyst.serviceaccount.entity.ServiceAccount;
import tech.flowcatalyst.serviceaccount.repository.ServiceAccountRepository;

import java.util.ArrayList;
import java.util.List;
import java.util.Map;
import java.util.Optional;

/**
 * Use case for deleting a service account and all linked entities.
 *
 * <p>Deleting a service account atomically deletes:
 * <ul>
 *   <li>ServiceAccount - the main entity</li>
 *   <li>Principal (type=SERVICE) - the linked identity entity</li>
 *   <li>OAuthClient (CONFIDENTIAL) - the linked OAuth client</li>
 * </ul>
 *
 * <p>All deletions occur within a single transaction.</p>
 */
@ApplicationScoped
public class DeleteServiceAccountUseCase implements UseCase<DeleteServiceAccountCommand, ServiceAccountDeleted> {

    @Inject
    ServiceAccountRepository repository;

    @Inject
    PrincipalRepository principalRepository;

    @Inject
    OAuthClientRepository oauthClientRepository;

    @Inject
    UnitOfWork unitOfWork;

    @Override
    public boolean authorizeResource(DeleteServiceAccountCommand command, ExecutionContext context) {
        return true;
    }

    @Override
    public Result<ServiceAccountDeleted> doExecute(DeleteServiceAccountCommand command, ExecutionContext context) {
        // Find service account
        ServiceAccount sa = repository.findByIdOptional(command.serviceAccountId()).orElse(null);
        if (sa == null) {
            return Result.failure(new UseCaseError.NotFoundError(
                "SERVICE_ACCOUNT_NOT_FOUND",
                "Service account not found",
                Map.of("serviceAccountId", command.serviceAccountId())
            ));
        }

        // Find linked Principal (type=SERVICE with serviceAccountId FK)
        Optional<Principal> principalOpt = principalRepository.findByServiceAccountId(sa.id);
        Principal principal = principalOpt.orElse(null);

        // Find linked OAuthClient (via serviceAccountPrincipalId)
        OAuthClient oauthClient = null;
        if (principal != null) {
            oauthClient = oauthClientRepository.findByServiceAccountPrincipalId(principal.id).orElse(null);
        }

        // Build list of entities to delete
        List<Object> entitiesToDelete = new ArrayList<>();

        // Delete OAuthClient first (references Principal)
        if (oauthClient != null) {
            entitiesToDelete.add(oauthClient);
        }

        // Delete Principal second (references ServiceAccount)
        if (principal != null) {
            entitiesToDelete.add(principal);
        }

        // Delete ServiceAccount last (main entity)
        entitiesToDelete.add(sa);

        // Create event before deletion
        ServiceAccountDeleted event = ServiceAccountDeleted.fromContext(context)
            .serviceAccountId(sa.id)
            .deletedPrincipalId(principal != null ? principal.id : null)
            .deletedOauthClientId(oauthClient != null ? oauthClient.id : null)
            .code(sa.code)
            .name(sa.name)
            .applicationId(sa.applicationId)
            .build();

        // Delete all atomically
        return unitOfWork.commitDeleteAll(entitiesToDelete, event, command);
    }
}
