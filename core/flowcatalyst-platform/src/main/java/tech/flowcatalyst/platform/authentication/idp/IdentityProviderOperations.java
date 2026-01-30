package tech.flowcatalyst.platform.authentication.idp;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.authentication.idp.events.IdentityProviderCreated;
import tech.flowcatalyst.platform.authentication.idp.events.IdentityProviderDeleted;
import tech.flowcatalyst.platform.authentication.idp.events.IdentityProviderUpdated;
import tech.flowcatalyst.platform.authentication.idp.operations.createidp.CreateIdentityProviderCommand;
import tech.flowcatalyst.platform.authentication.idp.operations.createidp.CreateIdentityProviderUseCase;
import tech.flowcatalyst.platform.authentication.idp.operations.deleteidp.DeleteIdentityProviderCommand;
import tech.flowcatalyst.platform.authentication.idp.operations.deleteidp.DeleteIdentityProviderUseCase;
import tech.flowcatalyst.platform.authentication.idp.operations.updateidp.UpdateIdentityProviderCommand;
import tech.flowcatalyst.platform.authentication.idp.operations.updateidp.UpdateIdentityProviderUseCase;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;

import java.util.List;
import java.util.Optional;

/**
 * IdentityProviderOperations - Single point of discovery for Identity Provider aggregate.
 *
 * <p>All write operations on Identity Providers go through this service. This provides:
 * <ul>
 *   <li>A single entry point for all Identity Provider mutations</li>
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
public class IdentityProviderOperations {

    // ========================================================================
    // Write Operations (Use Cases)
    // ========================================================================

    @Inject
    CreateIdentityProviderUseCase createUseCase;

    @Inject
    UpdateIdentityProviderUseCase updateUseCase;

    @Inject
    DeleteIdentityProviderUseCase deleteUseCase;

    /**
     * Create a new Identity Provider.
     *
     * @param command The command containing identity provider details
     * @param context The execution context
     * @return Success with IdentityProviderCreated, or Failure with error
     */
    public Result<IdentityProviderCreated> createIdentityProvider(CreateIdentityProviderCommand command, ExecutionContext context) {
        return createUseCase.execute(command, context);
    }

    /**
     * Update an Identity Provider.
     *
     * @param command The command containing update details
     * @param context The execution context
     * @return Success with IdentityProviderUpdated, or Failure with error
     */
    public Result<IdentityProviderUpdated> updateIdentityProvider(UpdateIdentityProviderCommand command, ExecutionContext context) {
        return updateUseCase.execute(command, context);
    }

    /**
     * Delete an Identity Provider.
     *
     * @param command The command identifying the identity provider to delete
     * @param context The execution context
     * @return Success with IdentityProviderDeleted, or Failure with error
     */
    public Result<IdentityProviderDeleted> deleteIdentityProvider(DeleteIdentityProviderCommand command, ExecutionContext context) {
        return deleteUseCase.execute(command, context);
    }

    // ========================================================================
    // Read Operations (Queries)
    // ========================================================================

    @Inject
    IdentityProviderRepository idpRepo;

    /**
     * Find an Identity Provider by ID.
     */
    public Optional<IdentityProvider> findById(String id) {
        return idpRepo.findByIdOptional(id);
    }

    /**
     * Find an Identity Provider by code.
     */
    public Optional<IdentityProvider> findByCode(String code) {
        return idpRepo.findByCode(code);
    }

    /**
     * Find all Identity Providers.
     */
    public List<IdentityProvider> findAll() {
        return idpRepo.listAll();
    }

    /**
     * Find Identity Providers by type.
     */
    public List<IdentityProvider> findByType(IdentityProviderType type) {
        return idpRepo.findByType(type);
    }
}
