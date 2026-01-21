package tech.flowcatalyst.platform.principal;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.client.ClientAccessGrant;
import tech.flowcatalyst.platform.client.ClientAccessGrantRepository;
import tech.flowcatalyst.platform.client.ClientAccessService;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.principal.events.RolesAssigned;
import tech.flowcatalyst.platform.principal.events.*;
import tech.flowcatalyst.platform.principal.operations.activateuser.ActivateUserCommand;
import tech.flowcatalyst.platform.principal.operations.activateuser.ActivateUserUseCase;
import tech.flowcatalyst.platform.principal.operations.createuser.CreateUserCommand;
import tech.flowcatalyst.platform.principal.operations.createuser.CreateUserUseCase;
import tech.flowcatalyst.platform.principal.operations.deactivateuser.DeactivateUserCommand;
import tech.flowcatalyst.platform.principal.operations.deactivateuser.DeactivateUserUseCase;
import tech.flowcatalyst.platform.principal.operations.deleteuser.DeleteUserCommand;
import tech.flowcatalyst.platform.principal.operations.deleteuser.DeleteUserUseCase;
import tech.flowcatalyst.platform.principal.operations.grantclientaccess.GrantClientAccessCommand;
import tech.flowcatalyst.platform.principal.operations.grantclientaccess.GrantClientAccessUseCase;
import tech.flowcatalyst.platform.principal.operations.revokeclientaccess.RevokeClientAccessCommand;
import tech.flowcatalyst.platform.principal.operations.revokeclientaccess.RevokeClientAccessUseCase;
import tech.flowcatalyst.platform.principal.operations.updateuser.UpdateUserCommand;
import tech.flowcatalyst.platform.principal.operations.updateuser.UpdateUserUseCase;
import tech.flowcatalyst.platform.principal.operations.assignroles.AssignRolesCommand;
import tech.flowcatalyst.platform.principal.operations.assignroles.AssignRolesUseCase;

import java.util.List;
import java.util.Optional;
import java.util.Set;
import java.util.stream.Collectors;

/**
 * UserOperations - Single point of discovery for User aggregate.
 *
 * <p>All write operations on Users go through this service. This provides:
 * <ul>
 *   <li>A single entry point for all User mutations</li>
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
public class UserOperations {

    // ========================================================================
    // Write Operations (Use Cases)
    // ========================================================================

    @Inject
    CreateUserUseCase createUserUseCase;

    @Inject
    UpdateUserUseCase updateUserUseCase;

    @Inject
    ActivateUserUseCase activateUserUseCase;

    @Inject
    DeactivateUserUseCase deactivateUserUseCase;

    @Inject
    DeleteUserUseCase deleteUserUseCase;

    @Inject
    GrantClientAccessUseCase grantClientAccessUseCase;

    @Inject
    RevokeClientAccessUseCase revokeClientAccessUseCase;

    @Inject
    AssignRolesUseCase assignRolesUseCase;

    /**
     * Create a new User.
     *
     * @param command The command containing user details
     * @param context The execution context
     * @return Success with UserCreated, or Failure with error
     */
    public Result<UserCreated> createUser(CreateUserCommand command, ExecutionContext context) {
        return createUserUseCase.execute(command, context);
    }

    /**
     * Update a User.
     *
     * @param command The command containing update details
     * @param context The execution context
     * @return Success with UserUpdated, or Failure with error
     */
    public Result<UserUpdated> updateUser(UpdateUserCommand command, ExecutionContext context) {
        return updateUserUseCase.execute(command, context);
    }

    /**
     * Activate a User.
     *
     * @param command The command identifying the user to activate
     * @param context The execution context
     * @return Success with UserActivated, or Failure with error
     */
    public Result<UserActivated> activateUser(ActivateUserCommand command, ExecutionContext context) {
        return activateUserUseCase.execute(command, context);
    }

    /**
     * Deactivate a User.
     *
     * @param command The command identifying the user to deactivate
     * @param context The execution context
     * @return Success with UserDeactivated, or Failure with error
     */
    public Result<UserDeactivated> deactivateUser(DeactivateUserCommand command, ExecutionContext context) {
        return deactivateUserUseCase.execute(command, context);
    }

    /**
     * Delete a User.
     *
     * @param command The command identifying the user to delete
     * @param context The execution context
     * @return Success with UserDeleted, or Failure with error
     */
    public Result<UserDeleted> deleteUser(DeleteUserCommand command, ExecutionContext context) {
        return deleteUserUseCase.execute(command, context);
    }

    /**
     * Grant a user access to a client.
     *
     * @param command The command with user and client details
     * @param context The execution context
     * @return Success with ClientAccessGranted, or Failure with error
     */
    public Result<ClientAccessGranted> grantClientAccess(GrantClientAccessCommand command, ExecutionContext context) {
        return grantClientAccessUseCase.execute(command, context);
    }

    /**
     * Revoke a user's access to a client.
     *
     * @param command The command with user and client details
     * @param context The execution context
     * @return Success with ClientAccessRevoked, or Failure with error
     */
    public Result<ClientAccessRevoked> revokeClientAccess(RevokeClientAccessCommand command, ExecutionContext context) {
        return revokeClientAccessUseCase.execute(command, context);
    }

    /**
     * Assign roles to a user.
     *
     * <p>This is a batch operation that sets the complete role set.
     * Roles not in the list will be removed, new roles will be added.
     *
     * @param command The command containing user ID and desired roles
     * @param context The execution context
     * @return Success with RolesAssigned, or Failure with error
     */
    public Result<RolesAssigned> assignRoles(AssignRolesCommand command, ExecutionContext context) {
        return assignRolesUseCase.execute(command, context);
    }

    // ========================================================================
    // Read Operations (Queries)
    // ========================================================================

    @Inject
    PrincipalRepository principalRepo;

    @Inject
    ClientAccessGrantRepository grantRepo;

    @Inject
    ClientAccessService clientAccessService;

    /**
     * Find a User by ID.
     */
    public Optional<Principal> findById(String id) {
        Principal p = principalRepo.findById(id);
        if (p != null && p.type == PrincipalType.USER) {
            return Optional.of(p);
        }
        return Optional.empty();
    }

    /**
     * Find a User by email.
     */
    public Optional<Principal> findByEmail(String email) {
        return principalRepo.findByEmail(email);
    }

    /**
     * Find all Users.
     */
    public List<Principal> findAllUsers() {
        return principalRepo.findByType(PrincipalType.USER);
    }

    /**
     * Find Users by client ID.
     */
    public List<Principal> findByClientId(String clientId) {
        return principalRepo.findByClientId(clientId).stream()
            .filter(p -> p.type == PrincipalType.USER)
            .collect(Collectors.toList());
    }

    /**
     * Get client IDs a user has been granted access to (not including home client).
     */
    public Set<String> getGrantedClientIds(String userId) {
        return grantRepo.findByPrincipalId(userId).stream()
            .map(g -> g.clientId)
            .collect(Collectors.toSet());
    }

    /**
     * Check if user is an anchor domain user (global access).
     */
    public boolean isAnchorDomainUser(Principal user) {
        return clientAccessService.isAnchorDomainUser(user);
    }

    /**
     * Get all accessible client IDs for a user (home + grants + anchor).
     */
    public Set<String> getAccessibleClients(Principal user) {
        return clientAccessService.getAccessibleClients(user);
    }
}
