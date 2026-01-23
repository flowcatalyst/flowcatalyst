package tech.flowcatalyst.serviceaccount.operations.createserviceaccount;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.dispatchjob.model.SignatureAlgorithm;
import tech.flowcatalyst.platform.authentication.oauth.OAuthClient;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;
import tech.flowcatalyst.platform.principal.ManagedApplicationScope;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.platform.principal.PrincipalType;
import tech.flowcatalyst.platform.security.secrets.SecretService;
import tech.flowcatalyst.platform.shared.EntityType;
import tech.flowcatalyst.platform.shared.TsidGenerator;
import tech.flowcatalyst.serviceaccount.entity.ServiceAccount;
import tech.flowcatalyst.serviceaccount.entity.WebhookAuthType;
import tech.flowcatalyst.serviceaccount.entity.WebhookCredentials;
import tech.flowcatalyst.serviceaccount.repository.ServiceAccountRepository;

import java.security.SecureRandom;
import java.time.Instant;
import java.util.ArrayList;
import java.util.HexFormat;
import java.util.List;
import java.util.Map;

/**
 * Use case for creating a new service account with all associated entities.
 *
 * <p>Creating a service account atomically creates:</p>
 * <ul>
 *   <li>ServiceAccount - entity with webhook credentials (authToken, signingSecret)</li>
 *   <li>Principal (type=SERVICE) - identity entity for role assignments</li>
 *   <li>OAuthClient (CONFIDENTIAL) - for OAuth2 client_credentials authentication</li>
 * </ul>
 *
 * <p>This is the ONLY way to create a Principal of type SERVICE or an OAuthClient
 * of type CONFIDENTIAL. The three entities have a 1:1:1 relationship.</p>
 */
@ApplicationScoped
public class CreateServiceAccountUseCase {

    private static final String TOKEN_PREFIX = "fc_";
    private static final int TOKEN_LENGTH = 24;
    private static final int SECRET_BYTES = 32;
    private static final int CLIENT_SECRET_LENGTH = 48;
    private static final String ALPHANUMERIC = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789";

    private final SecureRandom secureRandom = new SecureRandom();

    @Inject
    ServiceAccountRepository repository;

    @Inject
    SecretService secretService;

    @Inject
    UnitOfWork unitOfWork;

    public CreateServiceAccountResult execute(CreateServiceAccountCommand command, ExecutionContext context) {
        // Validate code
        if (command.code() == null || command.code().isBlank()) {
            return CreateServiceAccountResult.failure(
                Result.failure(new UseCaseError.ValidationError(
                    "CODE_REQUIRED",
                    "Service account code is required",
                    Map.of()
                ))
            );
        }

        // Validate code format (lowercase, alphanumeric with dashes)
        if (!isValidCode(command.code())) {
            return CreateServiceAccountResult.failure(
                Result.failure(new UseCaseError.ValidationError(
                    "INVALID_CODE_FORMAT",
                    "Code must be lowercase alphanumeric with dashes (e.g., 'my-service')",
                    Map.of("code", command.code())
                ))
            );
        }

        // Check if code already exists
        if (repository.findByCode(command.code()).isPresent()) {
            return CreateServiceAccountResult.failure(
                Result.failure(new UseCaseError.BusinessRuleViolation(
                    "CODE_EXISTS",
                    "Service account with this code already exists",
                    Map.of("code", command.code())
                ))
            );
        }

        // Validate name
        if (command.name() == null || command.name().isBlank()) {
            return CreateServiceAccountResult.failure(
                Result.failure(new UseCaseError.ValidationError(
                    "NAME_REQUIRED",
                    "Service account name is required",
                    Map.of()
                ))
            );
        }

        // Generate all IDs upfront
        String serviceAccountId = TsidGenerator.generate(EntityType.SERVICE_ACCOUNT);
        String principalId = TsidGenerator.generate(EntityType.PRINCIPAL);
        String oauthClientEntityId = TsidGenerator.generate(EntityType.OAUTH_CLIENT);
        String oauthClientId = TsidGenerator.generate(EntityType.OAUTH_CLIENT);

        // Generate credentials
        String authToken = generateBearerToken();
        String signingSecret = generateSigningSecret();
        String clientSecret = generateClientSecret();

        Instant now = Instant.now();

        // ============================================================
        // 1. Create ServiceAccount (webhook credentials)
        // ============================================================
        ServiceAccount sa = new ServiceAccount();
        sa.id = serviceAccountId;
        sa.code = command.code();
        sa.name = command.name();
        sa.description = command.description();
        sa.clientIds = command.clientIds() != null ? new ArrayList<>(command.clientIds()) : new ArrayList<>();
        sa.applicationId = command.applicationId();
        sa.active = true;
        sa.createdAt = now;
        sa.updatedAt = now;

        // Create webhook credentials
        WebhookCredentials creds = new WebhookCredentials();
        creds.authType = WebhookAuthType.BEARER_TOKEN;
        creds.authTokenRef = secretService.prepareForStorage("encrypt:" + authToken);
        creds.signingSecretRef = secretService.prepareForStorage("encrypt:" + signingSecret);
        creds.signingAlgorithm = SignatureAlgorithm.HMAC_SHA256;
        creds.createdAt = now;
        sa.webhookCredentials = creds;

        // ============================================================
        // 2. Create Principal (type=SERVICE, linked to ServiceAccount)
        // ============================================================
        Principal principal = new Principal();
        principal.id = principalId;
        principal.type = PrincipalType.SERVICE;
        principal.serviceAccountId = serviceAccountId;  // FK to ServiceAccount
        principal.name = command.name();
        principal.active = true;
        principal.createdAt = now;
        principal.updatedAt = now;

        // Set managed application scope if applicationId is provided
        if (command.applicationId() != null) {
            principal.managedApplicationScope = ManagedApplicationScope.SPECIFIC;
            principal.managedApplicationIds = List.of(command.applicationId());
        } else {
            principal.managedApplicationScope = ManagedApplicationScope.NONE;
            principal.managedApplicationIds = new ArrayList<>();
        }

        // ============================================================
        // 3. Create OAuthClient (CONFIDENTIAL, linked to Principal)
        // ============================================================
        OAuthClient oauthClient = new OAuthClient();
        oauthClient.id = oauthClientEntityId;
        oauthClient.clientId = oauthClientId;
        oauthClient.clientName = command.name() + " OAuth Client";
        oauthClient.clientType = OAuthClient.ClientType.CONFIDENTIAL;
        oauthClient.clientSecretRef = secretService.prepareForStorage("encrypt:" + clientSecret);
        oauthClient.serviceAccountPrincipalId = principalId;  // FK to Principal
        oauthClient.grantTypes = List.of("client_credentials");
        oauthClient.defaultScopes = "openid";
        oauthClient.pkceRequired = false;
        oauthClient.active = true;
        oauthClient.createdAt = now;
        oauthClient.updatedAt = now;

        // ============================================================
        // 4. Create domain event
        // ============================================================
        ServiceAccountCreated event = ServiceAccountCreated.fromContext(context)
            .serviceAccountId(sa.id)
            .createdPrincipalId(principalId)
            .oauthClientId(oauthClientEntityId)
            .oauthClientClientId(oauthClientId)
            .code(sa.code)
            .name(sa.name)
            .clientIds(sa.clientIds)
            .applicationId(sa.applicationId)
            .build();

        // ============================================================
        // 5. Commit all entities atomically
        // ============================================================
        Result<ServiceAccountCreated> result = unitOfWork.commitAll(
            List.of(sa, principal, oauthClient),
            event,
            command
        );

        // Return credentials only on success (shown once)
        if (result instanceof Result.Success) {
            return new CreateServiceAccountResult(
                result,
                sa,
                principal,
                oauthClient,
                authToken,
                signingSecret,
                oauthClientId,  // The client_id for OAuth
                clientSecret
            );
        } else {
            return CreateServiceAccountResult.failure(result);
        }
    }

    /**
     * Generate a bearer token: fc_ + 24 random alphanumeric characters.
     */
    private String generateBearerToken() {
        StringBuilder token = new StringBuilder(TOKEN_PREFIX);
        for (int i = 0; i < TOKEN_LENGTH; i++) {
            token.append(ALPHANUMERIC.charAt(secureRandom.nextInt(ALPHANUMERIC.length())));
        }
        return token.toString();
    }

    /**
     * Generate a signing secret: 32 random bytes, hex-encoded (64 characters).
     */
    private String generateSigningSecret() {
        byte[] bytes = new byte[SECRET_BYTES];
        secureRandom.nextBytes(bytes);
        return HexFormat.of().formatHex(bytes);
    }

    /**
     * Generate OAuth client secret: 48 alphanumeric characters.
     */
    private String generateClientSecret() {
        StringBuilder sb = new StringBuilder(CLIENT_SECRET_LENGTH);
        for (int i = 0; i < CLIENT_SECRET_LENGTH; i++) {
            sb.append(ALPHANUMERIC.charAt(secureRandom.nextInt(ALPHANUMERIC.length())));
        }
        return sb.toString();
    }

    /**
     * Validate code format: lowercase alphanumeric with dashes, no leading/trailing dashes.
     */
    private boolean isValidCode(String code) {
        return code.matches("^[a-z0-9]+(-[a-z0-9]+)*$");
    }
}
