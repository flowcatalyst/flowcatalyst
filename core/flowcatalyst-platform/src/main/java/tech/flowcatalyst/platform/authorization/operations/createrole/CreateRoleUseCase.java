package tech.flowcatalyst.platform.authorization.operations.createrole;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.application.Application;
import tech.flowcatalyst.platform.application.ApplicationRepository;
import tech.flowcatalyst.platform.authorization.AuthRole;
import tech.flowcatalyst.platform.authorization.AuthRoleRepository;
import tech.flowcatalyst.platform.authorization.PermissionRegistry;
import tech.flowcatalyst.platform.authorization.events.RoleCreated;
import tech.flowcatalyst.platform.common.AuthorizationContext;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;
import tech.flowcatalyst.platform.shared.TsidGenerator;

import java.util.HashSet;
import java.util.Map;

/**
 * Use case for creating a Role.
 */
@ApplicationScoped
public class CreateRoleUseCase {

    @Inject
    AuthRoleRepository roleRepo;

    @Inject
    ApplicationRepository appRepo;

    @Inject
    PermissionRegistry permissionRegistry;

    @Inject
    UnitOfWork unitOfWork;

    public Result<RoleCreated> execute(CreateRoleCommand command, ExecutionContext context) {
        // Authorization check: can principal manage this application?
        AuthorizationContext authz = context.authz();
        if (authz != null && !authz.canManageApplication(command.applicationId())) {
            return Result.failure(new UseCaseError.AuthorizationError(
                "NOT_AUTHORIZED",
                "Not authorized to create roles for this application",
                Map.of("applicationId", command.applicationId())
            ));
        }

        // Validate application exists
        Application app = appRepo.findByIdOptional(command.applicationId()).orElse(null);
        if (app == null) {
            return Result.failure(new UseCaseError.NotFoundError(
                "APPLICATION_NOT_FOUND",
                "Application not found",
                Map.of("applicationId", command.applicationId())
            ));
        }

        // Validate name
        if (command.name() == null || command.name().isBlank()) {
            return Result.failure(new UseCaseError.ValidationError(
                "NAME_REQUIRED",
                "Role name is required",
                Map.of()
            ));
        }

        // Construct full role name with app prefix
        String fullRoleName = app.code + ":" + command.name();

        // Check uniqueness
        if (roleRepo.existsByName(fullRoleName)) {
            return Result.failure(new UseCaseError.BusinessRuleViolation(
                "ROLE_EXISTS",
                "Role already exists",
                Map.of("roleName", fullRoleName)
            ));
        }

        // Create role
        AuthRole role = new AuthRole();
        role.id = TsidGenerator.generate();
        role.applicationId = app.id;
        role.applicationCode = app.code;
        role.name = fullRoleName;
        role.displayName = command.displayName() != null ? command.displayName() : formatDisplayName(command.name());
        role.description = command.description();
        role.permissions = command.permissions() != null ? command.permissions() : new HashSet<>();
        role.source = command.source() != null ? command.source() : AuthRole.RoleSource.DATABASE;
        role.clientManaged = command.clientManaged();

        // Create domain event
        RoleCreated event = RoleCreated.fromContext(context)
            .roleId(role.id)
            .roleName(role.name)
            .displayName(role.displayName)
            .description(role.description)
            .applicationId(role.applicationId)
            .applicationCode(role.applicationCode)
            .permissions(role.permissions)
            .source(role.source.name())
            .clientManaged(role.clientManaged)
            .build();

        // Commit atomically
        Result<RoleCreated> result = unitOfWork.commit(role, event, command);

        // Register into PermissionRegistry for runtime use (after successful commit)
        if (result instanceof Result.Success) {
            permissionRegistry.registerRoleDynamic(fullRoleName, role.permissions, role.description);
        }

        return result;
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
}
