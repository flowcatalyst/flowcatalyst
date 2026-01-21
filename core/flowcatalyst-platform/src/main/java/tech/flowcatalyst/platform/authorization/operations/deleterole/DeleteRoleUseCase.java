package tech.flowcatalyst.platform.authorization.operations.deleterole;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.authorization.AuthRole;
import tech.flowcatalyst.platform.authorization.AuthRoleRepository;
import tech.flowcatalyst.platform.authorization.PermissionRegistry;
import tech.flowcatalyst.platform.authorization.events.RoleDeleted;
import tech.flowcatalyst.platform.common.AuthorizationContext;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;

import java.util.Map;

/**
 * Use case for deleting a Role.
 */
@ApplicationScoped
public class DeleteRoleUseCase {

    @Inject
    AuthRoleRepository roleRepo;

    @Inject
    PermissionRegistry permissionRegistry;

    @Inject
    UnitOfWork unitOfWork;

    public Result<RoleDeleted> execute(DeleteRoleCommand command, ExecutionContext context) {
        // Authorization check: can principal manage roles with this prefix?
        AuthorizationContext authz = context.authz();
        if (authz != null && !authz.canManageResourceWithPrefix(command.roleName())) {
            return Result.failure(new UseCaseError.AuthorizationError(
                "NOT_AUTHORIZED",
                "Not authorized to delete this role",
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

        if (role.source == AuthRole.RoleSource.CODE) {
            return Result.failure(new UseCaseError.BusinessRuleViolation(
                "CANNOT_DELETE_CODE_ROLE",
                "Cannot delete CODE-defined role",
                Map.of("roleName", command.roleName())
            ));
        }

        // Create domain event (before deletion)
        RoleDeleted event = RoleDeleted.fromContext(context)
            .roleId(role.id)
            .roleName(role.name)
            .build();

        // Delete via commitDelete
        Result<RoleDeleted> result = unitOfWork.commitDelete(role, event, command);

        // Unregister from PermissionRegistry after successful commit
        if (result instanceof Result.Success) {
            permissionRegistry.unregisterRole(role.name);
        }

        return result;
    }
}
