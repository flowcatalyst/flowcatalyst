package tech.flowcatalyst.platform.authentication.idp.operations.createidp;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.authentication.idp.IdentityProvider;
import tech.flowcatalyst.platform.authentication.idp.IdentityProviderRepository;
import tech.flowcatalyst.platform.authentication.idp.IdentityProviderType;
import tech.flowcatalyst.platform.authentication.idp.events.IdentityProviderCreated;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;
import tech.flowcatalyst.platform.security.secrets.SecretService;
import tech.flowcatalyst.platform.shared.EntityType;
import tech.flowcatalyst.platform.shared.TsidGenerator;

import java.util.ArrayList;
import java.util.List;
import java.util.Map;

/**
 * Use case for creating an Identity Provider.
 */
@ApplicationScoped
public class CreateIdentityProviderUseCase {

    @Inject
    IdentityProviderRepository idpRepo;

    @Inject
    UnitOfWork unitOfWork;

    @Inject
    SecretService secretService;

    public Result<IdentityProviderCreated> execute(CreateIdentityProviderCommand command, ExecutionContext context) {
        // Validate code
        if (command.code() == null || command.code().isBlank()) {
            return Result.failure(new UseCaseError.ValidationError(
                "CODE_REQUIRED",
                "Identity provider code is required",
                Map.of()
            ));
        }

        // Validate code format (alphanumeric with hyphens)
        if (!command.code().matches("^[a-z0-9-]+$")) {
            return Result.failure(new UseCaseError.ValidationError(
                "INVALID_CODE_FORMAT",
                "Code must be lowercase alphanumeric with hyphens",
                Map.of("code", command.code())
            ));
        }

        // Validate name
        if (command.name() == null || command.name().isBlank()) {
            return Result.failure(new UseCaseError.ValidationError(
                "NAME_REQUIRED",
                "Identity provider name is required",
                Map.of()
            ));
        }

        // Validate type
        if (command.type() == null) {
            return Result.failure(new UseCaseError.ValidationError(
                "TYPE_REQUIRED",
                "Identity provider type is required",
                Map.of()
            ));
        }

        // Validate OIDC-specific fields
        if (command.type() == IdentityProviderType.OIDC) {
            // For multi-tenant: require issuer pattern
            // For single-tenant: require issuer URL
            if (command.oidcMultiTenant()) {
                if (command.oidcIssuerPattern() == null || command.oidcIssuerPattern().isBlank()) {
                    return Result.failure(new UseCaseError.ValidationError(
                        "OIDC_ISSUER_PATTERN_REQUIRED",
                        "OIDC issuer pattern is required for multi-tenant OIDC identity providers",
                        Map.of()
                    ));
                }
            } else {
                if (command.oidcIssuerUrl() == null || command.oidcIssuerUrl().isBlank()) {
                    return Result.failure(new UseCaseError.ValidationError(
                        "OIDC_ISSUER_URL_REQUIRED",
                        "OIDC issuer URL is required for single-tenant OIDC identity providers",
                        Map.of()
                    ));
                }
            }
            if (command.oidcClientId() == null || command.oidcClientId().isBlank()) {
                return Result.failure(new UseCaseError.ValidationError(
                    "OIDC_CLIENT_ID_REQUIRED",
                    "OIDC client ID is required for OIDC identity providers",
                    Map.of()
                ));
            }
        }

        // Check uniqueness
        if (idpRepo.existsByCode(command.code())) {
            return Result.failure(new UseCaseError.BusinessRuleViolation(
                "CODE_EXISTS",
                "Identity provider with this code already exists",
                Map.of("code", command.code())
            ));
        }

        // Normalize allowed email domains to lowercase
        List<String> normalizedDomains = command.allowedEmailDomains() != null
            ? command.allowedEmailDomains().stream().map(String::toLowerCase).toList()
            : List.of();

        // Create identity provider
        var idp = new IdentityProvider();
        idp.id = TsidGenerator.generate(EntityType.IDENTITY_PROVIDER);
        idp.code = command.code();
        idp.name = command.name();
        idp.type = command.type();
        idp.oidcIssuerUrl = command.oidcIssuerUrl();
        idp.oidcClientId = command.oidcClientId();
        // Encrypt client secret if provided
        if (command.oidcClientSecretRef() != null && !command.oidcClientSecretRef().isBlank()) {
            idp.oidcClientSecretRef = secretService.prepareForStorage("encrypt:" + command.oidcClientSecretRef());
        }
        idp.oidcMultiTenant = command.oidcMultiTenant();
        idp.oidcIssuerPattern = command.oidcIssuerPattern();
        idp.allowedEmailDomains = new ArrayList<>(normalizedDomains);

        // Create domain event
        var event = IdentityProviderCreated.fromContext(context)
            .identityProviderId(idp.id)
            .code(idp.code)
            .name(idp.name)
            .type(idp.type.name())
            .oidcIssuerUrl(idp.oidcIssuerUrl)
            .oidcMultiTenant(idp.oidcMultiTenant)
            .allowedEmailDomains(idp.allowedEmailDomains)
            .build();

        // Commit atomically
        return unitOfWork.commit(idp, event, command);
    }
}
