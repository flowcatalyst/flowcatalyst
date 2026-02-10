package tech.flowcatalyst.platform.principal.operations.assignapplicationaccess;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.application.Application;
import tech.flowcatalyst.platform.application.ApplicationClientConfigRepository;
import tech.flowcatalyst.platform.application.ApplicationRepository;
import tech.flowcatalyst.platform.client.ClientAccessService;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.UseCase;
import tech.flowcatalyst.platform.common.errors.UseCaseError;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.platform.principal.PrincipalRepository;
import tech.flowcatalyst.platform.principal.PrincipalType;
import tech.flowcatalyst.platform.principal.events.ApplicationAccessAssigned;

import java.time.Instant;
import java.util.ArrayList;
import java.util.HashSet;
import java.util.List;
import java.util.Map;
import java.util.Set;

/**
 * Use case for batch assigning application access to a user.
 *
 * <p>This replaces the user's complete application access set in one operation,
 * computing the delta (added/removed) for the event.
 *
 * <p>Validation rules:
 * <ul>
 *   <li>Principal must exist and be USER type</li>
 *   <li>Each requested application must exist and be active</li>
 *   <li>At least one of user's accessible clients must have each app enabled</li>
 * </ul>
 */
@ApplicationScoped
public class AssignApplicationAccessUseCase implements UseCase<AssignApplicationAccessCommand, ApplicationAccessAssigned> {

    @Inject
    PrincipalRepository principalRepo;

    @Inject
    ApplicationRepository applicationRepo;

    @Inject
    ApplicationClientConfigRepository appConfigRepo;

    @Inject
    ClientAccessService clientAccessService;

    @Inject
    UnitOfWork unitOfWork;

    @Override
    public boolean authorizeResource(AssignApplicationAccessCommand command, ExecutionContext context) {
        return true;
    }

    @Override
    public Result<ApplicationAccessAssigned> doExecute(AssignApplicationAccessCommand command, ExecutionContext context) {
        // Find the user
        var principal = principalRepo.findById(command.userId());

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

        // Normalize requested applications (dedupe)
        Set<String> requestedApps = new HashSet<>(command.applicationIds() != null ? command.applicationIds() : List.of());

        // If no apps requested, allow clearing all access
        if (requestedApps.isEmpty()) {
            return commitAccessChange(principal, requestedApps, context);
        }

        // Validate all requested applications exist and are active
        List<Application> applications = applicationRepo.findByIds(requestedApps);
        Set<String> foundAppIds = new HashSet<>();
        List<String> inactiveApps = new ArrayList<>();

        for (Application app : applications) {
            foundAppIds.add(app.id);
            if (!app.active) {
                inactiveApps.add(app.id);
            }
        }

        // Check for applications that don't exist
        Set<String> missingApps = new HashSet<>(requestedApps);
        missingApps.removeAll(foundAppIds);

        if (!missingApps.isEmpty()) {
            return Result.failure(new UseCaseError.ValidationError(
                "APPLICATIONS_NOT_FOUND",
                "Some applications do not exist: " + String.join(", ", missingApps),
                Map.of("missingApplicationIds", new ArrayList<>(missingApps))
            ));
        }

        if (!inactiveApps.isEmpty()) {
            return Result.failure(new UseCaseError.ValidationError(
                "APPLICATIONS_INACTIVE",
                "Some applications are inactive: " + String.join(", ", inactiveApps),
                Map.of("inactiveApplicationIds", inactiveApps)
            ));
        }

        // Validate that each app is enabled for at least one of user's accessible clients
        Set<String> accessibleClientIds = clientAccessService.getAccessibleClients(principal);
        List<String> inaccessibleApps = new ArrayList<>();

        for (String appId : requestedApps) {
            boolean appAccessible = false;
            for (String clientId : accessibleClientIds) {
                if (appConfigRepo.isApplicationEnabledForClient(appId, clientId)) {
                    appAccessible = true;
                    break;
                }
            }
            if (!appAccessible) {
                inaccessibleApps.add(appId);
            }
        }

        if (!inaccessibleApps.isEmpty()) {
            return Result.failure(new UseCaseError.ValidationError(
                "APPLICATIONS_NOT_ACCESSIBLE",
                "Some applications are not enabled for any of the user's accessible clients",
                Map.of("inaccessibleApplicationIds", inaccessibleApps)
            ));
        }

        return commitAccessChange(principal, requestedApps, context);
    }

    private Result<ApplicationAccessAssigned> commitAccessChange(
            Principal principal,
            Set<String> requestedApps,
            ExecutionContext context) {

        // Compute current apps
        Set<String> currentApps = principal.getAccessibleApplicationIds();

        // Compute delta
        List<String> added = requestedApps.stream()
            .filter(a -> !currentApps.contains(a))
            .toList();

        List<String> removed = currentApps.stream()
            .filter(a -> !requestedApps.contains(a))
            .toList();

        // Apply changes
        principal.accessibleApplicationIds = new ArrayList<>(requestedApps);
        principal.updatedAt = Instant.now();

        // Create domain event
        var event = ApplicationAccessAssigned.fromContext(context)
            .userId(principal.id)
            .applicationIds(new ArrayList<>(requestedApps))
            .added(added)
            .removed(removed)
            .build();

        // Commit atomically
        return unitOfWork.commit(principal, event, null);
    }
}
