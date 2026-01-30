package tech.flowcatalyst.platform.authorization.operations.syncroles;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.transaction.Transactional;
import tech.flowcatalyst.platform.application.Application;
import tech.flowcatalyst.platform.application.ApplicationRepository;
import tech.flowcatalyst.platform.authorization.AuthRole;
import tech.flowcatalyst.platform.authorization.AuthRoleRepository;
import tech.flowcatalyst.platform.authorization.PermissionInput;
import tech.flowcatalyst.platform.authorization.PermissionRegistry;
import tech.flowcatalyst.platform.authorization.events.RolesSynced;
import tech.flowcatalyst.platform.common.AuthorizationContext;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;
import tech.flowcatalyst.platform.shared.EntityType;
import tech.flowcatalyst.platform.shared.TsidGenerator;

import java.util.*;

/**
 * Use case for syncing roles from an external application (SDK).
 *
 * Note: This use case handles multiple entities and uses a custom commit strategy.
 */
@ApplicationScoped
public class SyncRolesUseCase {

    @Inject
    AuthRoleRepository roleRepo;

    @Inject
    ApplicationRepository appRepo;

    @Inject
    PermissionRegistry permissionRegistry;

    @Inject
    UnitOfWork unitOfWork;

    @Transactional
    public Result<RolesSynced> execute(SyncRolesCommand command, ExecutionContext context) {
        // Authorization check: can principal manage this application?
        AuthorizationContext authz = context.authz();
        if (authz != null && !authz.canAccessApplication(command.applicationId())) {
            return Result.failure(new UseCaseError.AuthorizationError(
                "NOT_AUTHORIZED",
                "Not authorized to sync roles for this application",
                Map.of("applicationId", command.applicationId())
            ));
        }

        Application app = appRepo.findByIdOptional(command.applicationId()).orElse(null);
        if (app == null) {
            return Result.failure(new UseCaseError.NotFoundError(
                "APPLICATION_NOT_FOUND",
                "Application not found",
                Map.of("applicationId", command.applicationId())
            ));
        }

        // Validate all permission strings have correct format before processing
        Result<Void> validationResult = validatePermissions(command.roles());
        if (validationResult instanceof Result.Failure<Void> f) {
            return Result.failure(f.error());
        }

        Set<String> syncedRoleNames = new HashSet<>();
        int rolesCreated = 0;
        int rolesUpdated = 0;
        int rolesDeleted = 0;

        for (SyncRolesCommand.SyncRoleItem item : command.roles()) {
            String fullRoleName = app.code + ":" + item.name();
            syncedRoleNames.add(fullRoleName);

            Optional<AuthRole> existingOpt = roleRepo.findByName(fullRoleName);

            // Build permission strings from structured inputs
            Set<String> permissionStrings = item.buildPermissionStrings();

            if (existingOpt.isPresent()) {
                AuthRole existing = existingOpt.get();
                if (existing.source == AuthRole.RoleSource.SDK) {
                    // Update existing SDK role
                    existing.displayName = item.displayName() != null ?
                        item.displayName() : formatDisplayName(item.name());
                    existing.description = item.description();
                    existing.permissions = permissionStrings;
                    existing.clientManaged = item.clientManaged();

                    roleRepo.update(existing);
                    permissionRegistry.registerRoleDynamic(fullRoleName, existing.permissions, existing.description);
                    rolesUpdated++;
                }
                // Don't update CODE or DATABASE roles from SDK sync
            } else {
                // Create new SDK role
                AuthRole role = new AuthRole();
                role.id = TsidGenerator.generate(EntityType.ROLE);
                role.applicationId = app.id;
                role.applicationCode = app.code;
                role.name = fullRoleName;
                role.displayName = item.displayName() != null ? item.displayName() : formatDisplayName(item.name());
                role.description = item.description();
                role.permissions = permissionStrings;
                role.source = AuthRole.RoleSource.SDK;
                role.clientManaged = item.clientManaged();

                roleRepo.persist(role);
                permissionRegistry.registerRoleDynamic(fullRoleName, role.permissions, role.description);
                rolesCreated++;
            }
        }

        if (command.removeUnlisted()) {
            // Remove SDK roles that weren't in the sync list
            List<AuthRole> existingRoles = roleRepo.findByApplicationCode(app.code);
            for (AuthRole existing : existingRoles) {
                if (existing.source == AuthRole.RoleSource.SDK && !syncedRoleNames.contains(existing.name)) {
                    permissionRegistry.unregisterRole(existing.name);
                    roleRepo.delete(existing);
                    rolesDeleted++;
                }
            }
        }

        // Create domain event
        RolesSynced event = RolesSynced.fromContext(context)
            .applicationId(app.id)
            .applicationCode(app.code)
            .rolesCreated(rolesCreated)
            .rolesUpdated(rolesUpdated)
            .rolesDeleted(rolesDeleted)
            .syncedRoleNames(new ArrayList<>(syncedRoleNames))
            .build();

        // Commit atomically with a placeholder entity (use app as the entity for the event)
        return unitOfWork.commit(app, event, command);
    }

    private String formatDisplayName(String roleName) {
        if (roleName == null || roleName.isBlank()) {
            return roleName;
        }
        String[] parts = roleName.split("-");
        StringBuilder sb = new StringBuilder();
        for (String part : parts) {
            if (!part.isEmpty()) {
                if (sb.length() > 0) {
                    sb.append(" ");
                }
                sb.append(Character.toUpperCase(part.charAt(0)));
                if (part.length() > 1) {
                    sb.append(part.substring(1));
                }
            }
        }
        return sb.toString();
    }

    /**
     * Validate all permissions in the sync request.
     * Each permission segment is validated individually.
     */
    private Result<Void> validatePermissions(List<SyncRolesCommand.SyncRoleItem> roles) {
        for (SyncRolesCommand.SyncRoleItem role : roles) {
            if (role.permissions() == null || role.permissions().isEmpty()) {
                continue;
            }

            for (PermissionInput permission : role.permissions()) {
                if (permission == null) {
                    return Result.failure(new UseCaseError.ValidationError(
                        "INVALID_PERMISSION",
                        "Permission cannot be null",
                        Map.of("role", role.name())
                    ));
                }

                String validationError = permission.validate();
                if (validationError != null) {
                    return Result.failure(new UseCaseError.ValidationError(
                        "INVALID_PERMISSION_FORMAT",
                        "Invalid permission for role '" + role.name() + "': " + validationError,
                        Map.of(
                            "role", role.name(),
                            "permission", permission.buildPermissionString(),
                            "error", validationError,
                            "expectedFormat", "{application}:{context}:{aggregate}:{action}",
                            "example", "myapp:orders:order:view"
                        )
                    ));
                }
            }
        }
        return Result.success(null);
    }
}
