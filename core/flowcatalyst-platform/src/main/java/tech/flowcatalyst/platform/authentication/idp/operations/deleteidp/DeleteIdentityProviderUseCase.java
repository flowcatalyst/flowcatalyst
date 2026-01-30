package tech.flowcatalyst.platform.authentication.idp.operations.deleteidp;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.authentication.domain.EmailDomainMappingRepository;
import tech.flowcatalyst.platform.authentication.idp.IdentityProvider;
import tech.flowcatalyst.platform.authentication.idp.IdentityProviderRepository;
import tech.flowcatalyst.platform.authentication.idp.events.IdentityProviderDeleted;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;

import java.util.Map;

/**
 * Use case for deleting an Identity Provider.
 */
@ApplicationScoped
public class DeleteIdentityProviderUseCase {

    @Inject
    IdentityProviderRepository idpRepo;

    @Inject
    EmailDomainMappingRepository mappingRepo;

    @Inject
    UnitOfWork unitOfWork;

    public Result<IdentityProviderDeleted> execute(DeleteIdentityProviderCommand command, ExecutionContext context) {
        // Validate ID
        if (command.identityProviderId() == null || command.identityProviderId().isBlank()) {
            return Result.failure(new UseCaseError.ValidationError(
                "ID_REQUIRED",
                "Identity provider ID is required",
                Map.of()
            ));
        }

        // Find existing
        IdentityProvider idp = idpRepo.findByIdOptional(command.identityProviderId()).orElse(null);
        if (idp == null) {
            return Result.failure(new UseCaseError.NotFoundError(
                "IDENTITY_PROVIDER_NOT_FOUND",
                "Identity provider not found",
                Map.of("identityProviderId", command.identityProviderId())
            ));
        }

        // Check for dependent email domain mappings
        var dependentMappings = mappingRepo.findByIdentityProviderId(idp.id);
        if (!dependentMappings.isEmpty()) {
            return Result.failure(new UseCaseError.BusinessRuleViolation(
                "IDP_HAS_MAPPINGS",
                "Cannot delete identity provider that has email domain mappings",
                Map.of(
                    "identityProviderId", idp.id,
                    "mappingCount", dependentMappings.size(),
                    "domains", dependentMappings.stream().map(m -> m.emailDomain).toList()
                )
            ));
        }

        // Create domain event
        var event = IdentityProviderDeleted.fromContext(context)
            .identityProviderId(idp.id)
            .code(idp.code)
            .name(idp.name)
            .build();

        // Commit atomically (delete)
        return unitOfWork.commitDelete(idp, event, command);
    }
}
