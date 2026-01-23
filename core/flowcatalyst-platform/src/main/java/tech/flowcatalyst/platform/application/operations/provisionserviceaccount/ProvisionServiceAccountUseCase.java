package tech.flowcatalyst.platform.application.operations.provisionserviceaccount;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.application.Application;
import tech.flowcatalyst.platform.application.ApplicationRepository;
import tech.flowcatalyst.platform.application.events.ServiceAccountProvisioned;
import tech.flowcatalyst.platform.authentication.oauth.OAuthClient;
import tech.flowcatalyst.platform.authorization.platform.PlatformApplicationServiceRole;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.serviceaccount.operations.ServiceAccountOperations;
import tech.flowcatalyst.serviceaccount.operations.createserviceaccount.CreateServiceAccountCommand;
import tech.flowcatalyst.serviceaccount.operations.createserviceaccount.CreateServiceAccountResult;

import java.time.Instant;
import java.util.List;
import java.util.Map;

/**
 * Use case for provisioning a service account for an application.
 *
 * <p>This use case delegates to {@link CreateServiceAccountUseCase} which atomically creates:
 * <ul>
 *   <li>ServiceAccount - for webhook credentials</li>
 *   <li>Principal (type=SERVICE) - for identity and role assignments</li>
 *   <li>OAuthClient (CONFIDENTIAL) - for OAuth client_credentials authentication</li>
 * </ul>
 *
 * <p>Then links the Application to the created service account and assigns the
 * platform:application-service role.
 */
@ApplicationScoped
public class ProvisionServiceAccountUseCase {

    @Inject
    ApplicationRepository applicationRepo;

    @Inject
    ServiceAccountOperations serviceAccountOperations;

    @Inject
    UnitOfWork unitOfWork;

    /**
     * Result of provisioning a service account.
     *
     * <p>Includes both the domain event result and the generated credentials,
     * which are only available at provisioning time.
     *
     * @param result          The Result from UnitOfWork (success with event, or failure)
     * @param serviceAccountId The new ServiceAccount entity ID (null on failure)
     * @param principal       The created service account principal (null on failure)
     * @param oauthClient     The created OAuth client (null on failure)
     * @param clientId        The OAuth client_id for authentication (null on failure)
     * @param clientSecret    The plaintext OAuth client secret (null on failure, only available once)
     * @param authToken       The webhook auth token (null on failure, only available once)
     * @param signingSecret   The webhook signing secret (null on failure, only available once)
     */
    public record ProvisionResult(
        Result<ServiceAccountProvisioned> result,
        String serviceAccountId,
        Principal principal,
        OAuthClient oauthClient,
        String clientId,
        String clientSecret,
        String authToken,
        String signingSecret
    ) {
        public boolean isSuccess() {
            return result instanceof Result.Success;
        }

        public boolean isFailure() {
            return result instanceof Result.Failure;
        }

        public UseCaseError error() {
            if (result instanceof Result.Failure<ServiceAccountProvisioned> f) {
                return f.error();
            }
            return null;
        }
    }

    public ProvisionResult execute(
            ProvisionServiceAccountCommand command,
            ExecutionContext context
    ) {
        // Load and validate application
        Application app = applicationRepo.findByIdOptional(command.applicationId())
            .orElse(null);

        if (app == null) {
            return new ProvisionResult(
                Result.failure(new UseCaseError.NotFoundError(
                    "APPLICATION_NOT_FOUND",
                    "Application not found",
                    Map.of("applicationId", command.applicationId())
                )),
                null, null, null, null, null, null, null
            );
        }

        // Check if already has a service account
        if (app.serviceAccountId != null) {
            return new ProvisionResult(
                Result.failure(new UseCaseError.BusinessRuleViolation(
                    "ALREADY_PROVISIONED",
                    "Application already has a service account",
                    Map.of("applicationId", command.applicationId(),
                           "existingServiceAccountId", app.serviceAccountId)
                )),
                null, null, null, null, null, null, null
            );
        }

        // Create service account using the unified flow
        // This atomically creates: ServiceAccount + Principal + OAuthClient
        CreateServiceAccountCommand saCommand = new CreateServiceAccountCommand(
            app.code + "-service",  // code
            app.name + " Service Account",  // name
            "Service account for " + app.name,  // description
            null,  // clientIds (not tenant-scoped)
            app.id  // applicationId
        );

        CreateServiceAccountResult saResult = serviceAccountOperations.create(saCommand, context);
        if (saResult.isFailure()) {
            return new ProvisionResult(
                Result.failure(((Result.Failure<?>)saResult.result()).error()),
                null, null, null, null, null, null, null
            );
        }

        // Assign the platform:application-service role to the created principal
        if (saResult.principal() != null) {
            saResult.principal().roles.add(new Principal.RoleAssignment(
                PlatformApplicationServiceRole.ROLE_NAME,
                "service-account-provisioning",
                Instant.now()
            ));
        }

        // Link application to the service account
        app.serviceAccountId = saResult.serviceAccount().id;
        app.serviceAccountPrincipalId = saResult.principal() != null ? saResult.principal().id : null;
        app.updatedAt = Instant.now();

        // Create domain event for the provisioning (linking to application)
        ServiceAccountProvisioned event = ServiceAccountProvisioned.fromContext(context)
            .applicationId(app.id)
            .applicationCode(app.code)
            .applicationName(app.name)
            .serviceAccountId(saResult.serviceAccount().id)
            .serviceAccountPrincipalId(saResult.principal() != null ? saResult.principal().id : null)
            .serviceAccountName(saResult.serviceAccount().name)
            .oauthClientId(saResult.oauthClient() != null ? saResult.oauthClient().id : null)
            .oauthClientClientId(saResult.clientId())
            .build();

        // Commit the application update (ServiceAccount/Principal/OAuthClient already committed)
        // Also persists the role update to the principal
        List<Object> toCommit = saResult.principal() != null
            ? List.of(saResult.principal(), app)
            : List.of(app);

        Result<ServiceAccountProvisioned> result = unitOfWork.commitAll(
            toCommit,
            event,
            command
        );

        if (result instanceof Result.Success) {
            return new ProvisionResult(
                result,
                saResult.serviceAccount().id,
                saResult.principal(),
                saResult.oauthClient(),
                saResult.clientId(),
                saResult.clientSecret(),
                saResult.authToken(),
                saResult.signingSecret()
            );
        } else {
            return new ProvisionResult(result, null, null, null, null, null, null, null);
        }
    }
}
