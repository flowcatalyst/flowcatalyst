package tech.flowcatalyst.platform.authentication.domain;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.authentication.domain.events.EmailDomainMappingCreated;
import tech.flowcatalyst.platform.authentication.domain.events.EmailDomainMappingDeleted;
import tech.flowcatalyst.platform.authentication.domain.events.EmailDomainMappingUpdated;
import tech.flowcatalyst.platform.authentication.domain.operations.createmapping.CreateEmailDomainMappingCommand;
import tech.flowcatalyst.platform.authentication.domain.operations.createmapping.CreateEmailDomainMappingUseCase;
import tech.flowcatalyst.platform.authentication.domain.operations.deletemapping.DeleteEmailDomainMappingCommand;
import tech.flowcatalyst.platform.authentication.domain.operations.deletemapping.DeleteEmailDomainMappingUseCase;
import tech.flowcatalyst.platform.authentication.domain.operations.updatemapping.UpdateEmailDomainMappingCommand;
import tech.flowcatalyst.platform.authentication.domain.operations.updatemapping.UpdateEmailDomainMappingUseCase;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;

import java.util.List;
import java.util.Optional;

/**
 * EmailDomainMappingOperations - Single point of discovery for Email Domain Mapping aggregate.
 *
 * <p>All write operations on Email Domain Mappings go through this service. This provides:
 * <ul>
 *   <li>A single entry point for all Email Domain Mapping mutations</li>
 *   <li>Consistent execution context handling</li>
 *   <li>Clear documentation of available operations</li>
 * </ul>
 *
 * <p>Each operation:
 * <ul>
 *   <li>Takes a command describing what to do</li>
 *   <li>Takes an execution context for tracing and principal info</li>
 *   <li>Returns a Result containing either the domain event or an error</li>
 *   <li>Atomically commits the entity, event, and audit log</li>
 * </ul>
 *
 * <p>Read operations do not require execution context and do not emit events.
 */
@ApplicationScoped
public class EmailDomainMappingOperations {

    // ========================================================================
    // Write Operations (Use Cases)
    // ========================================================================

    @Inject
    CreateEmailDomainMappingUseCase createUseCase;

    @Inject
    UpdateEmailDomainMappingUseCase updateUseCase;

    @Inject
    DeleteEmailDomainMappingUseCase deleteUseCase;

    /**
     * Create a new Email Domain Mapping.
     *
     * @param command The command containing mapping details
     * @param context The execution context
     * @return Success with EmailDomainMappingCreated, or Failure with error
     */
    public Result<EmailDomainMappingCreated> createMapping(CreateEmailDomainMappingCommand command, ExecutionContext context) {
        return createUseCase.execute(command, context);
    }

    /**
     * Update an Email Domain Mapping.
     *
     * @param command The command containing update details
     * @param context The execution context
     * @return Success with EmailDomainMappingUpdated, or Failure with error
     */
    public Result<EmailDomainMappingUpdated> updateMapping(UpdateEmailDomainMappingCommand command, ExecutionContext context) {
        return updateUseCase.execute(command, context);
    }

    /**
     * Delete an Email Domain Mapping.
     *
     * @param command The command identifying the mapping to delete
     * @param context The execution context
     * @return Success with EmailDomainMappingDeleted, or Failure with error
     */
    public Result<EmailDomainMappingDeleted> deleteMapping(DeleteEmailDomainMappingCommand command, ExecutionContext context) {
        return deleteUseCase.execute(command, context);
    }

    // ========================================================================
    // Read Operations (Queries)
    // ========================================================================

    @Inject
    EmailDomainMappingRepository mappingRepo;

    /**
     * Find an Email Domain Mapping by ID.
     */
    public Optional<EmailDomainMapping> findById(String id) {
        return mappingRepo.findByIdOptional(id);
    }

    /**
     * Find an Email Domain Mapping by email domain.
     */
    public Optional<EmailDomainMapping> findByEmailDomain(String emailDomain) {
        return mappingRepo.findByEmailDomain(emailDomain);
    }

    /**
     * Find all Email Domain Mappings.
     */
    public List<EmailDomainMapping> findAll() {
        return mappingRepo.listAll();
    }

    /**
     * Find Email Domain Mappings by identity provider ID.
     */
    public List<EmailDomainMapping> findByIdentityProviderId(String identityProviderId) {
        return mappingRepo.findByIdentityProviderId(identityProviderId);
    }

    /**
     * Find Email Domain Mappings by scope type.
     */
    public List<EmailDomainMapping> findByScopeType(ScopeType scopeType) {
        return mappingRepo.findByScopeType(scopeType);
    }

    /**
     * Find Email Domain Mappings by primary client ID.
     */
    public List<EmailDomainMapping> findByPrimaryClientId(String primaryClientId) {
        return mappingRepo.findByPrimaryClientId(primaryClientId);
    }
}
