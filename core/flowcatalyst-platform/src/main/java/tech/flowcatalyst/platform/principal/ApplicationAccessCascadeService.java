package tech.flowcatalyst.platform.principal;

import io.quarkus.logging.Log;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.application.ApplicationClientConfigRepository;
import tech.flowcatalyst.platform.client.ClientAccessService;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.principal.events.ApplicationAccessAssigned;

import java.time.Instant;
import java.util.ArrayList;
import java.util.List;
import java.util.Set;

/**
 * Service for cascading application access revocation.
 *
 * <p>When an application is disabled for a client, users who had access
 * to that application through that client may need their access revoked.
 * This service handles that cascading logic.
 *
 * <p>The cascade rules are:
 * <ol>
 *   <li>Find all users with access to the disabled application</li>
 *   <li>For each user, check if ANY of their accessible clients still has the app enabled</li>
 *   <li>If not, revoke the user's access to that application</li>
 * </ol>
 */
@ApplicationScoped
public class ApplicationAccessCascadeService {

    @Inject
    PrincipalRepository principalRepo;

    @Inject
    ApplicationClientConfigRepository appConfigRepo;

    @Inject
    ClientAccessService clientAccessService;

    @Inject
    UnitOfWork unitOfWork;

    /**
     * Handle cascading revocation when an application is disabled for a client.
     *
     * <p>For each user who has access to the application:
     * <ul>
     *   <li>Check if any of their other accessible clients still have the app enabled</li>
     *   <li>If not, remove the app from their accessible applications</li>
     * </ul>
     *
     * @param applicationId The application that was disabled
     * @param disabledClientId The client for which the app was disabled
     * @param ctx Execution context for event emission
     * @return List of user IDs that had their access revoked
     */
    public List<String> cascadeApplicationDisabled(
            String applicationId,
            String disabledClientId,
            ExecutionContext ctx) {

        Log.infof("Cascading application access revocation: appId=%s, clientId=%s",
            applicationId, disabledClientId);

        List<String> affectedUserIds = new ArrayList<>();

        // Find all principals (users) who have access to this application
        List<Principal> principalsWithAccess = principalRepo.findByAccessibleApplicationId(applicationId);

        Log.infof("Found %d principals with access to application %s",
            principalsWithAccess.size(), applicationId);

        for (Principal principal : principalsWithAccess) {
            // Only process USER type principals
            if (principal.type != PrincipalType.USER) {
                continue;
            }

            // Get all clients this user can access
            Set<String> accessibleClientIds = clientAccessService.getAccessibleClients(principal);

            // Check if any of their accessible clients (other than the disabled one)
            // still has this application enabled
            boolean stillHasAccess = false;
            for (String clientId : accessibleClientIds) {
                // Skip the client that was just disabled
                if (clientId.equals(disabledClientId)) {
                    continue;
                }

                if (appConfigRepo.isApplicationEnabledForClient(applicationId, clientId)) {
                    stillHasAccess = true;
                    break;
                }
            }

            if (!stillHasAccess) {
                // Revoke access to this application
                Log.infof("Revoking application access for user %s: no remaining clients with app %s enabled",
                    principal.id, applicationId);

                revokeApplicationAccess(principal, applicationId, ctx);
                affectedUserIds.add(principal.id);
            }
        }

        Log.infof("Cascade complete: %d users had their access revoked", affectedUserIds.size());

        return affectedUserIds;
    }

    /**
     * Revoke a user's access to a specific application.
     */
    private void revokeApplicationAccess(Principal principal, String applicationId, ExecutionContext ctx) {
        // Compute current apps
        Set<String> currentApps = principal.getAccessibleApplicationIds();

        // Remove the revoked app
        List<String> newAppIds = currentApps.stream()
            .filter(id -> !id.equals(applicationId))
            .toList();

        // Apply changes
        principal.accessibleApplicationIds = new ArrayList<>(newAppIds);
        principal.updatedAt = Instant.now();

        // Create domain event
        var event = ApplicationAccessAssigned.fromContext(ctx)
            .userId(principal.id)
            .applicationIds(newAppIds)
            .added(List.of())
            .removed(List.of(applicationId))
            .build();

        // Commit atomically
        Result<ApplicationAccessAssigned> result = unitOfWork.commit(principal, event, null);

        if (result instanceof Result.Failure<ApplicationAccessAssigned> f) {
            Log.errorf("Failed to revoke application access for user %s: %s",
                principal.id, f.error().message());
        }
    }
}
