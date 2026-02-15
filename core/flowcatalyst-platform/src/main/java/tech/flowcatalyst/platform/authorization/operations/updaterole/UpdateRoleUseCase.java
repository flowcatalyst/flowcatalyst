package tech.flowcatalyst.platform.authorization.operations.updaterole;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.authorization.AuthRole;
import tech.flowcatalyst.platform.authorization.AuthRoleRepository;
import tech.flowcatalyst.platform.authorization.PermissionInput;
import tech.flowcatalyst.platform.authorization.PermissionRegistry;
import tech.flowcatalyst.platform.authorization.events.RoleUpdated;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.UseCase;
import tech.flowcatalyst.platform.common.errors.UseCaseError;

import java.util.List;
import java.util.Map;
import java.util.Set;

/**
 * Use case for updating a Role.
 */
@ApplicationScoped
public class UpdateRoleUseCase implements UseCase<UpdateRoleCommand, RoleUpdated> {

    @Inject
    AuthRoleRepository roleRepo;

    @Inject
    PermissionRegistry permissionRegistry;

    @Inject
    UnitOfWork unitOfWork;

    @Override
    public boolean authorizeResource(UpdateRoleCommand command, ExecutionContext context) {
        var authz = context.authz();
        if (authz == null) return true;
        if (command.roleName() == null || command.roleName().isBlank()) return true;
        return authz.canAccessResourceWithPrefix(command.roleName());
    }

    @Override
    public Result<RoleUpdated> doExecute(UpdateRoleCommand command, ExecutionContext context) {
        AuthRole role = roleRepo.findByName(command.roleName()).orElse(null);

        if (role == null) {
            return Result.failure(new UseCaseError.NotFoundError(
                "ROLE_NOT_FOUND",
                "Role not found",
                Map.of("roleName", command.roleName())
            ));
        }

        // Validate permission formats if permissions are being updated
        if (command.permissions() != null) {
            Result<Void> permissionValidation = validatePermissions(command.permissions(), command.roleName());
            if (permissionValidation instanceof Result.Failure<Void> f) {
                return Result.failure(f.error());
            }
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
                role.permissions = command.buildPermissionStrings();
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

    /**
     * Validate all permissions.
     * Each permission segment is validated individually.
     */
    private Result<Void> validatePermissions(List<PermissionInput> permissions, String roleName) {
        if (permissions == null || permissions.isEmpty()) {
            return Result.success(null);
        }

        for (PermissionInput permission : permissions) {
            if (permission == null) {
                return Result.failure(new UseCaseError.ValidationError(
                    "INVALID_PERMISSION",
                    "Permission cannot be null",
                    Map.of("role", roleName)
                ));
            }

            String validationError = permission.validate();
            if (validationError != null) {
                return Result.failure(new UseCaseError.ValidationError(
                    "INVALID_PERMISSION_FORMAT",
                    "Invalid permission for role '" + roleName + "': " + validationError,
                    Map.of(
                        "role", roleName,
                        "permission", permission.buildPermissionString(),
                        "error", validationError,
                        "expectedFormat", "{application}:{context}:{aggregate}:{action}",
                        "example", "myapp:orders:order:view"
                    )
                ));
            }
        }
        return Result.success(null);
    }
}
