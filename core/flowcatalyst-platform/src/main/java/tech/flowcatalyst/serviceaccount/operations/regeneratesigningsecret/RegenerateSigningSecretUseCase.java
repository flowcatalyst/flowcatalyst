package tech.flowcatalyst.serviceaccount.operations.regeneratesigningsecret;

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
import java.util.HexFormat;
import java.util.Map;

/**
 * Use case for regenerating a service account's signing secret.
 */
@ApplicationScoped
public class RegenerateSigningSecretUseCase {

    private static final int SECRET_BYTES = 32;

    private final SecureRandom secureRandom = new SecureRandom();

    @Inject
    ServiceAccountRepository repository;

    @Inject
    SecretService secretService;

    @Inject
    UnitOfWork unitOfWork;

    public RegenerateSigningSecretResult execute(RegenerateSigningSecretCommand command, ExecutionContext context) {
        // Find service account
        ServiceAccount sa = repository.findByIdOptional(command.serviceAccountId()).orElse(null);
        if (sa == null) {
            return new RegenerateSigningSecretResult(
                Result.failure(new UseCaseError.NotFoundError(
                    "SERVICE_ACCOUNT_NOT_FOUND",
                    "Service account not found",
                    Map.of("serviceAccountId", command.serviceAccountId())
                )),
                null
            );
        }

        if (sa.webhookCredentials == null) {
            return new RegenerateSigningSecretResult(
                Result.failure(new UseCaseError.BusinessRuleViolation(
                    "NO_CREDENTIALS",
                    "Service account has no webhook credentials",
                    Map.of("serviceAccountId", command.serviceAccountId())
                )),
                null
            );
        }

        // Generate new signing secret
        String newSecret = generateSigningSecret();

        // Update credentials
        sa.webhookCredentials.signingSecretRef = secretService.prepareForStorage("encrypt:" + newSecret);
        sa.webhookCredentials.regeneratedAt = Instant.now();
        sa.updatedAt = Instant.now();

        // Create event
        SigningSecretRegenerated event = SigningSecretRegenerated.fromContext(context)
            .serviceAccountId(sa.id)
            .code(sa.code)
            .build();

        Result<SigningSecretRegenerated> result = unitOfWork.commit(sa, event, command);

        if (result instanceof Result.Success) {
            return new RegenerateSigningSecretResult(result, newSecret);
        } else {
            return new RegenerateSigningSecretResult(result, null);
        }
    }

    private String generateSigningSecret() {
        byte[] bytes = new byte[SECRET_BYTES];
        secureRandom.nextBytes(bytes);
        return HexFormat.of().formatHex(bytes);
    }
}
