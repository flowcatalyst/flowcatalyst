package tech.flowcatalyst.platform.authorization;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.authorization.events.*;
import tech.flowcatalyst.platform.authorization.operations.createrole.CreateRoleCommand;
import tech.flowcatalyst.platform.authorization.operations.createrole.CreateRoleUseCase;
import tech.flowcatalyst.platform.authorization.operations.deleterole.DeleteRoleCommand;
import tech.flowcatalyst.platform.authorization.operations.deleterole.DeleteRoleUseCase;
import tech.flowcatalyst.platform.authorization.operations.syncroles.SyncRolesCommand;
import tech.flowcatalyst.platform.authorization.operations.syncroles.SyncRolesUseCase;
import tech.flowcatalyst.platform.authorization.operations.updaterole.UpdateRoleCommand;
import tech.flowcatalyst.platform.authorization.operations.updaterole.UpdateRoleUseCase;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;

import java.util.List;
import java.util.Optional;

/**
 * RoleOperations - Single point of discovery for Role aggregate.
 *
 * <p>All write operations on Roles go through this service. This provides:
 * <ul>
 *   <li>A single entry point for all Role mutations</li>
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
public class RoleOperations {

    // ========================================================================
    // Write Operations (Use Cases)
    // ========================================================================

    @Inject
    CreateRoleUseCase createRoleUseCase;

    @Inject
    UpdateRoleUseCase updateRoleUseCase;

    @Inject
    DeleteRoleUseCase deleteRoleUseCase;

    @Inject
    SyncRolesUseCase syncRolesUseCase;

    /**
     * Create a new Role.
     *
     * @param command The command containing role details
     * @param context The execution context
     * @return Success with RoleCreated, or Failure with error
     */
    public Result<RoleCreated> createRole(CreateRoleCommand command, ExecutionContext context) {
        return createRoleUseCase.execute(command, context);
    }

    /**
     * Update a Role.
     *
     * @param command The command containing update details
     * @param context The execution context
     * @return Success with RoleUpdated, or Failure with error
     */
    public Result<RoleUpdated> updateRole(UpdateRoleCommand command, ExecutionContext context) {
        return updateRoleUseCase.execute(command, context);
    }

    /**
     * Delete a Role.
     *
     * @param command The command identifying the role to delete
     * @param context The execution context
     * @return Success with RoleDeleted, or Failure with error
     */
    public Result<RoleDeleted> deleteRole(DeleteRoleCommand command, ExecutionContext context) {
        return deleteRoleUseCase.execute(command, context);
    }

    /**
     * Sync roles from an external application (SDK).
     *
     * @param command The command containing sync details
     * @param context The execution context
     * @return Success with RolesSynced, or Failure with error
     */
    public Result<RolesSynced> syncRoles(SyncRolesCommand command, ExecutionContext context) {
        return syncRolesUseCase.execute(command, context);
    }

    // ========================================================================
    // Read Operations (Queries)
    // ========================================================================

    @Inject
    AuthRoleRepository roleRepo;

    /**
     * Find a Role by name.
     */
    public Optional<AuthRole> findByName(String name) {
        return roleRepo.findByName(name);
    }

    /**
     * Find all Roles.
     */
    public List<AuthRole> findAll() {
        return roleRepo.listAll();
    }

    /**
     * Find Roles by application code.
     */
    public List<AuthRole> findByApplicationCode(String applicationCode) {
        return roleRepo.findByApplicationCode(applicationCode);
    }

    /**
     * Find Roles by source.
     */
    public List<AuthRole> findBySource(AuthRole.RoleSource source) {
        return roleRepo.findBySource(source);
    }
}
