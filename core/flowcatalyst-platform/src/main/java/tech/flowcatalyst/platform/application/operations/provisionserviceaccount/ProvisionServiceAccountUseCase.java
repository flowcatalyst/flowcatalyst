package tech.flowcatalyst.platform.application.operations.provisionserviceaccount;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.application.Application;
import tech.flowcatalyst.platform.application.ApplicationRepository;
import tech.flowcatalyst.platform.application.events.ServiceAccountProvisioned;
import tech.flowcatalyst.platform.authentication.oauth.OAuthClient;
import tech.flowcatalyst.platform.authentication.oauth.OAuthClientRepository;
import tech.flowcatalyst.platform.authorization.platform.PlatformApplicationServiceRole;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;
import tech.flowcatalyst.platform.principal.ManagedApplicationScope;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.platform.principal.PrincipalRepository;
import tech.flowcatalyst.platform.principal.PrincipalType;
import tech.flowcatalyst.platform.principal.ServiceAccount;
import tech.flowcatalyst.platform.security.secrets.SecretService;
import tech.flowcatalyst.platform.shared.TsidGenerator;
import tech.flowcatalyst.serviceaccount.operations.ServiceAccountOperations;
import tech.flowcatalyst.serviceaccount.operations.createserviceaccount.CreateServiceAccountCommand;
import tech.flowcatalyst.serviceaccount.operations.createserviceaccount.CreateServiceAccountResult;

import java.security.SecureRandom;
import java.time.Instant;
import java.util.List;
import java.util.Map;

/**
 * Use case for provisioning a service account for an application.
 *
 * <p>Creates:
 * <ul>
 *   <li>A Principal of type SERVICE</li>
 *   <li>An OAuthClient with client_credentials grant</li>
 *   <li>Updates the Application with the service account reference</li>
 * </ul>
 *
 * <p>All changes are committed atomically via UnitOfWork.commitAll().
 */
@ApplicationScoped
public class ProvisionServiceAccountUseCase {

    private static final SecureRandom SECURE_RANDOM = new SecureRandom();
    private static final String CLIENT_SECRET_CHARS = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789";
    private static final int CLIENT_SECRET_LENGTH = 48;

    @Inject
    ApplicationRepository applicationRepo;

    @Inject
    PrincipalRepository principalRepo;

    @Inject
    OAuthClientRepository oauthClientRepo;

    @Inject
    SecretService secretService;

    @Inject
    UnitOfWork unitOfWork;

    @Inject
    ServiceAccountOperations serviceAccountOperations;

    /**
     * Result of provisioning a service account.
     *
     * <p>Includes both the domain event result and the generated credentials,
     * which are only available at provisioning time.
     *
     * @param result        The Result from UnitOfWork (success with event, or failure)
     * @param serviceAccountId The new ServiceAccount entity ID (null on failure)
     * @param principal     The created service account principal (null on failure)
     * @param oauthClient   The created OAuth client (null on failure)
     * @param clientSecret  The plaintext OAuth client secret (null on failure, only available once)
     * @param authToken     The webhook auth token (null on failure, only available once)
     * @param signingSecret The webhook signing secret (null on failure, only available once)
     */
    public record ProvisionResult(
        Result<ServiceAccountProvisioned> result,
        String serviceAccountId,
        Principal principal,
        OAuthClient oauthClient,
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
                null, null, null, null, null, null
            );
        }

        // Check if already has a service account (check both new and legacy fields)
        if (app.serviceAccountId != null || app.serviceAccountPrincipalId != null) {
            return new ProvisionResult(
                Result.failure(new UseCaseError.BusinessRuleViolation(
                    "ALREADY_PROVISIONED",
                    "Application already has a service account",
                    Map.of("applicationId", command.applicationId(),
                           "existingServiceAccountId", app.serviceAccountId != null ? app.serviceAccountId : "",
                           "existingPrincipalId", app.serviceAccountPrincipalId != null ? app.serviceAccountPrincipalId : "")
                )),
                null, null, null, null, null, null
            );
        }

        // First, create the new ServiceAccount entity with webhook credentials
        CreateServiceAccountCommand saCommand = new CreateServiceAccountCommand(
            app.code + "-service",  // code
            app.name + " Service Account",  // name
            "Service account for " + app.name,  // description
            null,  // clientId (not tenant-scoped)
            app.id  // applicationId
        );

        CreateServiceAccountResult saResult = serviceAccountOperations.create(saCommand, context);
        if (saResult.isFailure()) {
            // Propagate the failure from service account creation
            return new ProvisionResult(
                Result.failure(((Result.Failure<?>)saResult.result()).error()),
                null, null, null, null, null, null
            );
        }

        // Generate IDs (raw TSIDs - prefix added at API boundary)
        String principalId = TsidGenerator.generate();
        String oauthClientId = TsidGenerator.generate();
        String clientIdValue = TsidGenerator.generate();  // Raw TSID, "oauth_" prefix added at API boundary

        // Generate client secret (only available now)
        String clientSecret = generateClientSecret();
        String encryptedSecretRef = secretService.prepareForStorage("encrypt:" + clientSecret);

        // Create service account principal
        Principal principal = new Principal();
        principal.id = principalId;
        principal.type = PrincipalType.SERVICE;
        principal.name = app.name + " Service Account";
        principal.applicationId = app.id;  // Legacy field for backwards compatibility
        principal.active = true;
        principal.serviceAccount = new ServiceAccount();
        principal.serviceAccount.code = app.code + "-service";
        principal.serviceAccount.description = "Service account for " + app.name;

        // Set managed application scope - this service account can only manage this application
        principal.managedApplicationScope = ManagedApplicationScope.SPECIFIC;
        principal.managedApplicationIds = List.of(app.id);

        // Assign platform:application-service role
        principal.roles.add(new Principal.RoleAssignment(
            PlatformApplicationServiceRole.ROLE_NAME,
            "service-account-provisioning",
            Instant.now()
        ));

        // Create OAuth client for service account
        OAuthClient oauthClient = new OAuthClient();
        oauthClient.id = oauthClientId;
        oauthClient.clientId = clientIdValue;
        oauthClient.clientName = app.name + " Service Client";
        oauthClient.clientType = OAuthClient.ClientType.CONFIDENTIAL;
        oauthClient.clientSecretRef = encryptedSecretRef;
        oauthClient.serviceAccountPrincipalId = principalId;
        oauthClient.grantTypes = List.of("client_credentials");
        oauthClient.defaultScopes = "openid";
        oauthClient.pkceRequired = false;
        oauthClient.active = true;

        // Update application with reference to service account (both new and legacy fields)
        String serviceAccountId = saResult.serviceAccount().id;
        app.serviceAccountId = serviceAccountId;
        app.serviceAccountPrincipalId = principalId;  // Legacy field for backward compatibility
        app.updatedAt = Instant.now();

        // Create domain event
        ServiceAccountProvisioned event = ServiceAccountProvisioned.fromContext(context)
            .applicationId(app.id)
            .applicationCode(app.code)
            .applicationName(app.name)
            .serviceAccountId(serviceAccountId)
            .serviceAccountPrincipalId(principalId)
            .serviceAccountName(principal.name)
            .oauthClientId(oauthClientId)
            .oauthClientClientId(clientIdValue)
            .build();

        // Commit all changes atomically (ServiceAccount was already committed separately)
        Result<ServiceAccountProvisioned> result = unitOfWork.commitAll(
            List.of(principal, oauthClient, app),
            event,
            command
        );

        if (result instanceof Result.Success) {
            return new ProvisionResult(
                result,
                serviceAccountId,
                principal,
                oauthClient,
                clientSecret,
                saResult.authToken(),
                saResult.signingSecret()
            );
        } else {
            return new ProvisionResult(result, null, null, null, null, null, null);
        }
    }

    private String generateClientSecret() {
        StringBuilder sb = new StringBuilder(CLIENT_SECRET_LENGTH);
        for (int i = 0; i < CLIENT_SECRET_LENGTH; i++) {
            sb.append(CLIENT_SECRET_CHARS.charAt(SECURE_RANDOM.nextInt(CLIENT_SECRET_CHARS.length())));
        }
        return sb.toString();
    }
}
