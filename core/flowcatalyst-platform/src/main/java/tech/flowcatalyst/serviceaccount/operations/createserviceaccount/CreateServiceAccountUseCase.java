package tech.flowcatalyst.serviceaccount.operations.createserviceaccount;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.dispatchjob.model.SignatureAlgorithm;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;
import tech.flowcatalyst.platform.security.secrets.SecretService;
import tech.flowcatalyst.platform.shared.TsidGenerator;
import tech.flowcatalyst.serviceaccount.entity.ServiceAccount;
import tech.flowcatalyst.serviceaccount.entity.WebhookAuthType;
import tech.flowcatalyst.serviceaccount.entity.WebhookCredentials;
import tech.flowcatalyst.serviceaccount.repository.ServiceAccountRepository;

import java.security.SecureRandom;
import java.time.Instant;
import java.util.ArrayList;
import java.util.HexFormat;
import java.util.Map;

/**
 * Use case for creating a new service account with webhook credentials.
 */
@ApplicationScoped
public class CreateServiceAccountUseCase {

    private static final String TOKEN_PREFIX = "fc_";
    private static final int TOKEN_LENGTH = 24;
    private static final int SECRET_BYTES = 32;
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
            return new CreateServiceAccountResult(
                Result.failure(new UseCaseError.ValidationError(
                    "CODE_REQUIRED",
                    "Service account code is required",
                    Map.of()
                )),
                null, null, null
            );
        }

        // Validate code format (lowercase, alphanumeric with dashes)
        if (!isValidCode(command.code())) {
            return new CreateServiceAccountResult(
                Result.failure(new UseCaseError.ValidationError(
                    "INVALID_CODE_FORMAT",
                    "Code must be lowercase alphanumeric with dashes (e.g., 'my-service')",
                    Map.of("code", command.code())
                )),
                null, null, null
            );
        }

        // Check if code already exists
        if (repository.findByCode(command.code()).isPresent()) {
            return new CreateServiceAccountResult(
                Result.failure(new UseCaseError.BusinessRuleViolation(
                    "CODE_EXISTS",
                    "Service account with this code already exists",
                    Map.of("code", command.code())
                )),
                null, null, null
            );
        }

        // Validate name
        if (command.name() == null || command.name().isBlank()) {
            return new CreateServiceAccountResult(
                Result.failure(new UseCaseError.ValidationError(
                    "NAME_REQUIRED",
                    "Service account name is required",
                    Map.of()
                )),
                null, null, null
            );
        }

        // Generate credentials
        String authToken = generateBearerToken();
        String signingSecret = generateSigningSecret();

        // Create service account
        ServiceAccount sa = new ServiceAccount();
        sa.id = TsidGenerator.generate();
        sa.code = command.code();
        sa.name = command.name();
        sa.description = command.description();
        sa.clientIds = command.clientIds() != null ? new ArrayList<>(command.clientIds()) : new ArrayList<>();
        sa.applicationId = command.applicationId();
        sa.active = true;
        sa.createdAt = Instant.now();
        sa.updatedAt = Instant.now();

        // Create webhook credentials
        WebhookCredentials creds = new WebhookCredentials();
        creds.authType = WebhookAuthType.BEARER_TOKEN;
        creds.authTokenRef = secretService.prepareForStorage("encrypt:" + authToken);
        creds.signingSecretRef = secretService.prepareForStorage("encrypt:" + signingSecret);
        creds.signingAlgorithm = SignatureAlgorithm.HMAC_SHA256;
        creds.createdAt = Instant.now();
        sa.webhookCredentials = creds;

        // Create domain event
        ServiceAccountCreated event = ServiceAccountCreated.fromContext(context)
            .serviceAccountId(sa.id)
            .code(sa.code)
            .name(sa.name)
            .clientIds(sa.clientIds)
            .applicationId(sa.applicationId)
            .build();

        // Commit atomically
        Result<ServiceAccountCreated> result = unitOfWork.commit(sa, event, command);

        // Return credentials only on success (shown once)
        if (result instanceof Result.Success) {
            return new CreateServiceAccountResult(result, sa, authToken, signingSecret);
        } else {
            return new CreateServiceAccountResult(result, null, null, null);
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
     * Validate code format: lowercase alphanumeric with dashes, no leading/trailing dashes.
     */
    private boolean isValidCode(String code) {
        return code.matches("^[a-z0-9]+(-[a-z0-9]+)*$");
    }
}
