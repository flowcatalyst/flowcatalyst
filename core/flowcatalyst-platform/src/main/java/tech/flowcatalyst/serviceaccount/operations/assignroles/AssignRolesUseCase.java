package tech.flowcatalyst.serviceaccount.operations.assignroles;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.serviceaccount.entity.ServiceAccount;
import tech.flowcatalyst.serviceaccount.repository.ServiceAccountRepository;

import java.time.Instant;
import java.util.*;
import java.util.stream.Collectors;

/**
 * Use case for assigning roles to a service account.
 * This is a declarative assignment - the provided list replaces all existing roles.
 */
@ApplicationScoped
public class AssignRolesUseCase {

    private static final String ASSIGNMENT_SOURCE = "admin";

    @Inject
    ServiceAccountRepository repository;

    @Inject
    UnitOfWork unitOfWork;

    public Result<RolesAssigned> execute(AssignRolesCommand command, ExecutionContext context) {
        // Find service account
        ServiceAccount sa = repository.findByIdOptional(command.serviceAccountId()).orElse(null);
        if (sa == null) {
            return Result.failure(new UseCaseError.NotFoundError(
                "SERVICE_ACCOUNT_NOT_FOUND",
                "Service account not found",
                Map.of("serviceAccountId", command.serviceAccountId())
            ));
        }

        // Get current role names
        Set<String> currentRoles = sa.getRoleNames();
        Set<String> newRoles = new HashSet<>(command.roleNames() != null ? command.roleNames() : List.of());

        // Calculate diff
        List<String> addedRoles = newRoles.stream()
            .filter(r -> !currentRoles.contains(r))
            .sorted()
            .collect(Collectors.toList());

        List<String> removedRoles = currentRoles.stream()
            .filter(r -> !newRoles.contains(r))
            .sorted()
            .collect(Collectors.toList());

        // If no changes, still commit to record the event
        // This allows auditing of "no-op" assignments

        // Update roles
        sa.roles = newRoles.stream()
            .sorted()
            .map(roleName -> new Principal.RoleAssignment(roleName, ASSIGNMENT_SOURCE))
            .collect(Collectors.toList());

        sa.updatedAt = Instant.now();

        // Create event
        RolesAssigned event = RolesAssigned.fromContext(context)
            .serviceAccountId(sa.id)
            .code(sa.code)
            .roleNames(new ArrayList<>(newRoles))
            .addedRoles(addedRoles)
            .removedRoles(removedRoles)
            .build();

        return unitOfWork.commit(sa, event, command);
    }
}
