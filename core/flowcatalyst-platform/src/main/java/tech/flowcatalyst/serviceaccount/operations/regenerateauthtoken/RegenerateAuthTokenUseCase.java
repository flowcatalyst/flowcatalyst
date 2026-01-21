package tech.flowcatalyst.serviceaccount.operations.regenerateauthtoken;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;
import tech.flowcatalyst.platform.security.secrets.SecretService;
import tech.flowcatalyst.serviceaccount.entity.ServiceAccount;
import tech.flowcatalyst.serviceaccount.repository.ServiceAccountRepository;

import java.security.SecureRandom;
import java.time.Instant;
import java.util.Map;

/**
 * Use case for regenerating a service account's auth token.
 */
@ApplicationScoped
public class RegenerateAuthTokenUseCase {

    private static final String TOKEN_PREFIX = "fc_";
    private static final int TOKEN_LENGTH = 24;
    private static final String ALPHANUMERIC = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789";

    private final SecureRandom secureRandom = new SecureRandom();

    @Inject
    ServiceAccountRepository repository;

    @Inject
    SecretService secretService;

    @Inject
    UnitOfWork unitOfWork;

    public RegenerateAuthTokenResult execute(RegenerateAuthTokenCommand command, ExecutionContext context) {
        // Find service account
        ServiceAccount sa = repository.findByIdOptional(command.serviceAccountId()).orElse(null);
        if (sa == null) {
            return new RegenerateAuthTokenResult(
                Result.failure(new UseCaseError.NotFoundError(
                    "SERVICE_ACCOUNT_NOT_FOUND",
                    "Service account not found",
                    Map.of("serviceAccountId", command.serviceAccountId())
                )),
                null
            );
        }

        if (sa.webhookCredentials == null) {
            return new RegenerateAuthTokenResult(
                Result.failure(new UseCaseError.BusinessRuleViolation(
                    "NO_CREDENTIALS",
                    "Service account has no webhook credentials",
                    Map.of("serviceAccountId", command.serviceAccountId())
                )),
                null
            );
        }

        // Determine token to use
        String newToken;
        boolean isCustomToken;
        if (command.customToken() != null && !command.customToken().isBlank()) {
            newToken = command.customToken();
            isCustomToken = true;
        } else {
            newToken = generateBearerToken();
            isCustomToken = false;
        }

        // Update credentials
        sa.webhookCredentials.authTokenRef = secretService.prepareForStorage("encrypt:" + newToken);
        sa.webhookCredentials.regeneratedAt = Instant.now();
        sa.updatedAt = Instant.now();

        // Create event
        AuthTokenRegenerated event = AuthTokenRegenerated.fromContext(context)
            .serviceAccountId(sa.id)
            .code(sa.code)
            .authType(sa.webhookCredentials.authType)
            .isCustomToken(isCustomToken)
            .build();

        Result<AuthTokenRegenerated> result = unitOfWork.commit(sa, event, command);

        if (result instanceof Result.Success) {
            return new RegenerateAuthTokenResult(result, newToken);
        } else {
            return new RegenerateAuthTokenResult(result, null);
        }
    }

    private String generateBearerToken() {
        StringBuilder token = new StringBuilder(TOKEN_PREFIX);
        for (int i = 0; i < TOKEN_LENGTH; i++) {
            token.append(ALPHANUMERIC.charAt(secureRandom.nextInt(ALPHANUMERIC.length())));
        }
        return token.toString();
    }
}
