package tech.flowcatalyst.platform.authentication.domain.operations.deletemapping;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.authentication.domain.EmailDomainMapping;
import tech.flowcatalyst.platform.authentication.domain.EmailDomainMappingRepository;
import tech.flowcatalyst.platform.authentication.domain.events.EmailDomainMappingDeleted;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;

import java.util.Map;

/**
 * Use case for deleting an Email Domain Mapping.
 */
@ApplicationScoped
public class DeleteEmailDomainMappingUseCase {

    @Inject
    EmailDomainMappingRepository mappingRepo;

    @Inject
    UnitOfWork unitOfWork;

    public Result<EmailDomainMappingDeleted> execute(DeleteEmailDomainMappingCommand command, ExecutionContext context) {
        // Validate ID
        if (command.emailDomainMappingId() == null || command.emailDomainMappingId().isBlank()) {
            return Result.failure(new UseCaseError.ValidationError(
                "ID_REQUIRED",
                "Email domain mapping ID is required",
                Map.of()
            ));
        }

        // Find existing
        EmailDomainMapping mapping = mappingRepo.findByIdOptional(command.emailDomainMappingId()).orElse(null);
        if (mapping == null) {
            return Result.failure(new UseCaseError.NotFoundError(
                "EMAIL_DOMAIN_MAPPING_NOT_FOUND",
                "Email domain mapping not found",
                Map.of("emailDomainMappingId", command.emailDomainMappingId())
            ));
        }

        // Create domain event
        var event = EmailDomainMappingDeleted.fromContext(context)
            .emailDomainMappingId(mapping.id)
            .emailDomain(mapping.emailDomain)
            .identityProviderId(mapping.identityProviderId)
            .build();

        // Commit atomically (delete)
        return unitOfWork.commitDelete(mapping, event, command);
    }
}
