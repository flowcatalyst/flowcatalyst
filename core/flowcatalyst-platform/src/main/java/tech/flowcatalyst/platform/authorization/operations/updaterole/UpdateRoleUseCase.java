package tech.flowcatalyst.platform.authorization.operations.updaterole;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.authorization.AuthRole;
import tech.flowcatalyst.platform.authorization.AuthRoleRepository;
import tech.flowcatalyst.platform.authorization.PermissionRegistry;
import tech.flowcatalyst.platform.authorization.events.RoleUpdated;
import tech.flowcatalyst.platform.common.AuthorizationContext;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;

import java.util.Map;

/**
 * Use case for updating a Role.
 */
@ApplicationScoped
public class UpdateRoleUseCase {

    @Inject
    AuthRoleRepository roleRepo;

    @Inject
    PermissionRegistry permissionRegistry;

    @Inject
    UnitOfWork unitOfWork;

    public Result<RoleUpdated> execute(UpdateRoleCommand command, ExecutionContext context) {
        // Authorization check: can principal manage roles with this prefix?
        AuthorizationContext authz = context.authz();
        if (authz != null && !authz.canManageResourceWithPrefix(command.roleName())) {
            return Result.failure(new UseCaseError.AuthorizationError(
                "NOT_AUTHORIZED",
                "Not authorized to update this role",
                Map.of("roleName", command.roleName())
            ));
        }

        AuthRole role = roleRepo.findByName(command.roleName()).orElse(null);

        if (role == null) {
            return Result.failure(new UseCaseError.NotFoundError(
                "ROLE_NOT_FOUND",
                "Role not found",
                Map.of("roleName", command.roleName())
            ));
        }

        boolean permissionsChanged = false;

        if (role.source == AuthRole.RoleSource.CODE) {
            // CODE roles can only have clientManaged updated
            if (command.clientManaged() != null) {
                role.clientManaged = command.clientManaged();
            }
        } else {
            // DATABASE and SDK roles can be fully updated
            if (command.displayName() != null) {
                role.displayName = command.displayName();
            }
            if (command.description() != null) {
                role.description = command.description();
            }
            if (command.permissions() != null) {
                role.permissions = command.permissions();
                permissionsChanged = true;
            }
            if (command.clientManaged() != null) {
                role.clientManaged = command.clientManaged();
            }
        }

        // Create domain event
        RoleUpdated event = RoleUpdated.fromContext(context)
            .roleId(role.id)
            .roleName(role.name)
            .displayName(role.displayName)
            .description(role.description)
            .permissions(role.permissions)
            .clientManaged(role.clientManaged)
            .build();

        // Commit atomically
        Result<RoleUpdated> result = unitOfWork.commit(role, event, command);

        // Update registry if permissions changed
        if (result instanceof Result.Success && permissionsChanged) {
            permissionRegistry.registerRoleDynamic(role.name, role.permissions, role.description);
        }

        return result;
    }
}
