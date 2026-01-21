package tech.flowcatalyst.serviceaccount.operations;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.serviceaccount.entity.ServiceAccount;
import tech.flowcatalyst.serviceaccount.operations.assignroles.AssignRolesCommand;
import tech.flowcatalyst.serviceaccount.operations.assignroles.AssignRolesUseCase;
import tech.flowcatalyst.serviceaccount.operations.assignroles.RolesAssigned;
import tech.flowcatalyst.serviceaccount.operations.createserviceaccount.CreateServiceAccountCommand;
import tech.flowcatalyst.serviceaccount.operations.createserviceaccount.CreateServiceAccountResult;
import tech.flowcatalyst.serviceaccount.operations.createserviceaccount.CreateServiceAccountUseCase;
import tech.flowcatalyst.serviceaccount.operations.deleteserviceaccount.DeleteServiceAccountCommand;
import tech.flowcatalyst.serviceaccount.operations.deleteserviceaccount.DeleteServiceAccountUseCase;
import tech.flowcatalyst.serviceaccount.operations.deleteserviceaccount.ServiceAccountDeleted;
import tech.flowcatalyst.serviceaccount.operations.regenerateauthtoken.RegenerateAuthTokenCommand;
import tech.flowcatalyst.serviceaccount.operations.regenerateauthtoken.RegenerateAuthTokenResult;
import tech.flowcatalyst.serviceaccount.operations.regenerateauthtoken.RegenerateAuthTokenUseCase;
import tech.flowcatalyst.serviceaccount.operations.regeneratesigningsecret.RegenerateSigningSecretCommand;
import tech.flowcatalyst.serviceaccount.operations.regeneratesigningsecret.RegenerateSigningSecretResult;
import tech.flowcatalyst.serviceaccount.operations.regeneratesigningsecret.RegenerateSigningSecretUseCase;
import tech.flowcatalyst.serviceaccount.operations.updateserviceaccount.ServiceAccountUpdated;
import tech.flowcatalyst.serviceaccount.operations.updateserviceaccount.UpdateServiceAccountCommand;
import tech.flowcatalyst.serviceaccount.operations.updateserviceaccount.UpdateServiceAccountUseCase;
import tech.flowcatalyst.serviceaccount.repository.ServiceAccountRepository;
import tech.flowcatalyst.serviceaccount.repository.ServiceAccountFilter;

import java.util.List;
import java.util.Optional;

/**
 * Facade for all service account operations.
 * Provides a single entry point for mutations (which require ExecutionContext)
 * and queries (which don't require context).
 */
@ApplicationScoped
public class ServiceAccountOperations {

    @Inject
    CreateServiceAccountUseCase createUseCase;

    @Inject
    UpdateServiceAccountUseCase updateUseCase;

    @Inject
    RegenerateAuthTokenUseCase regenerateAuthTokenUseCase;

    @Inject
    RegenerateSigningSecretUseCase regenerateSigningSecretUseCase;

    @Inject
    AssignRolesUseCase assignRolesUseCase;

    @Inject
    DeleteServiceAccountUseCase deleteUseCase;

    @Inject
    ServiceAccountRepository repository;

    // ==================== MUTATIONS (require ExecutionContext) ====================

    /**
     * Create a new service account with generated credentials.
     * Returns the created service account and credentials (shown only once).
     */
    public CreateServiceAccountResult create(CreateServiceAccountCommand command, ExecutionContext context) {
        return createUseCase.execute(command, context);
    }

    /**
     * Update a service account's metadata.
     */
    public Result<ServiceAccountUpdated> update(UpdateServiceAccountCommand command, ExecutionContext context) {
        return updateUseCase.execute(command, context);
    }

    /**
     * Regenerate a service account's auth token.
     * Returns the new token (shown only once).
     */
    public RegenerateAuthTokenResult regenerateAuthToken(String serviceAccountId, ExecutionContext context) {
        return regenerateAuthTokenUseCase.execute(
            RegenerateAuthTokenCommand.generate(serviceAccountId),
            context
        );
    }

    /**
     * Update a service account's auth token with a custom value.
     * Returns the token that was set.
     */
    public RegenerateAuthTokenResult updateAuthToken(String serviceAccountId, String customToken, ExecutionContext context) {
        return regenerateAuthTokenUseCase.execute(
            RegenerateAuthTokenCommand.withCustomToken(serviceAccountId, customToken),
            context
        );
    }

    /**
     * Regenerate a service account's signing secret.
     * Returns the new secret (shown only once).
     */
    public RegenerateSigningSecretResult regenerateSigningSecret(String serviceAccountId, ExecutionContext context) {
        return regenerateSigningSecretUseCase.execute(
            new RegenerateSigningSecretCommand(serviceAccountId),
            context
        );
    }

    /**
     * Assign roles to a service account (declarative - replaces all existing roles).
     */
    public Result<RolesAssigned> assignRoles(String serviceAccountId, List<String> roleNames, ExecutionContext context) {
        return assignRolesUseCase.execute(
            new AssignRolesCommand(serviceAccountId, roleNames),
            context
        );
    }

    /**
     * Delete a service account.
     */
    public Result<ServiceAccountDeleted> delete(String serviceAccountId, ExecutionContext context) {
        return deleteUseCase.execute(
            new DeleteServiceAccountCommand(serviceAccountId),
            context
        );
    }

    // ==================== QUERIES (no context needed) ====================

    /**
     * Find a service account by ID.
     */
    public Optional<ServiceAccount> findById(String id) {
        return repository.findByIdOptional(id);
    }

    /**
     * Find a service account by unique code.
     */
    public Optional<ServiceAccount> findByCode(String code) {
        return repository.findByCode(code);
    }

    /**
     * Find a service account by application ID.
     */
    public Optional<ServiceAccount> findByApplicationId(String applicationId) {
        return repository.findByApplicationId(applicationId);
    }

    /**
     * Find all service accounts matching the filter.
     */
    public List<ServiceAccount> findWithFilter(ServiceAccountFilter filter) {
        return repository.findWithFilter(filter);
    }

    /**
     * Count service accounts matching the filter.
     */
    public long countWithFilter(ServiceAccountFilter filter) {
        return repository.countWithFilter(filter);
    }

    /**
     * Find all active service accounts.
     */
    public List<ServiceAccount> findActive() {
        return repository.findActive();
    }
}
