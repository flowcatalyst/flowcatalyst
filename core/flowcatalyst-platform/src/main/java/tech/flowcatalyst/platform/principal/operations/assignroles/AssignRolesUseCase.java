package tech.flowcatalyst.platform.principal.operations.assignroles;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.authorization.RoleService;
import tech.flowcatalyst.platform.client.ClientAccessService;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.platform.principal.PrincipalRepository;
import tech.flowcatalyst.platform.principal.PrincipalType;
import tech.flowcatalyst.platform.principal.events.RolesAssigned;

import java.time.Instant;
import java.util.ArrayList;
import java.util.HashSet;
import java.util.List;
import java.util.Map;
import java.util.Set;
import java.util.stream.Collectors;

/**
 * Use case for batch assigning roles to a user.
 *
 * <p>This replaces the user's complete role set in one operation,
 * computing the delta (added/removed) for the event.
 */
@ApplicationScoped
public class AssignRolesUseCase {

    @Inject
    PrincipalRepository principalRepo;

    @Inject
    RoleService roleService;

    @Inject
    ClientAccessService clientAccessService;

    @Inject
    UnitOfWork unitOfWork;

    public Result<RolesAssigned> execute(AssignRolesCommand command, ExecutionContext context) {
        // Find the user
        Principal principal = principalRepo.findById(command.userId());

        if (principal == null) {
            return Result.failure(new UseCaseError.NotFoundError(
                "USER_NOT_FOUND",
                "User not found",
                Map.of("userId", command.userId())
            ));
        }

        if (principal.type != PrincipalType.USER) {
            return Result.failure(new UseCaseError.ValidationError(
                "NOT_A_USER",
                "Principal is not a user",
                Map.of("userId", command.userId(), "type", principal.type.name())
            ));
        }

        // Normalize requested roles (dedupe)
        Set<String> requestedRoles = new HashSet<>(command.roles() != null ? command.roles() : List.of());

        // Validate all requested roles exist
        List<String> invalidRoles = requestedRoles.stream()
            .filter(r -> !roleService.isValidRole(r))
            .toList();

        if (!invalidRoles.isEmpty()) {
            return Result.failure(new UseCaseError.ValidationError(
                "INVALID_ROLES",
                "Some roles are not defined: " + String.join(", ", invalidRoles),
                Map.of("invalidRoles", invalidRoles)
            ));
        }

        // Check super admin role restriction
        boolean isAnchorUser = clientAccessService.isAnchorDomainUser(principal);
        for (String roleName : requestedRoles) {
            if (roleService.isSuperAdminRole(roleName) && !isAnchorUser) {
                return Result.failure(new UseCaseError.ValidationError(
                    "SUPER_ADMIN_RESTRICTED",
                    "Super Admin role can only be assigned to users from anchor domains",
                    Map.of("roleName", roleName)
                ));
            }
        }

        // Compute current roles
        Set<String> currentRoles = principal.getRoleNames();

        // Compute delta
        List<String> added = requestedRoles.stream()
            .filter(r -> !currentRoles.contains(r))
            .toList();

        List<String> removed = currentRoles.stream()
            .filter(r -> !requestedRoles.contains(r))
            .toList();

        // Apply changes
        // Remove roles not in requested set
        principal.roles.removeIf(r -> removed.contains(r.roleName));

        // Add new roles
        Instant now = Instant.now();
        for (String roleName : added) {
            principal.roles.add(new Principal.RoleAssignment(roleName, "MANUAL", now));
        }

        principal.updatedAt = now;

        // Create domain event
        RolesAssigned event = RolesAssigned.fromContext(context)
            .userId(principal.id)
            .roles(new ArrayList<>(requestedRoles))
            .added(added)
            .removed(removed)
            .build();

        // Commit atomically
        return unitOfWork.commit(principal, event, command);
    }
}
