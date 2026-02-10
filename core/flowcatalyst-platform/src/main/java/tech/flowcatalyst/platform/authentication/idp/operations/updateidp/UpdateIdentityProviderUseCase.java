package tech.flowcatalyst.platform.authentication.idp.operations.updateidp;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.authentication.idp.IdentityProvider;
import tech.flowcatalyst.platform.authentication.idp.IdentityProviderRepository;
import tech.flowcatalyst.platform.authentication.idp.IdentityProviderType;
import tech.flowcatalyst.platform.authentication.idp.events.IdentityProviderUpdated;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.UseCase;
import tech.flowcatalyst.platform.common.errors.UseCaseError;
import tech.flowcatalyst.platform.security.secrets.SecretService;

import java.util.ArrayList;
import java.util.Map;

/**
 * Use case for updating an Identity Provider.
 */
@ApplicationScoped
public class UpdateIdentityProviderUseCase implements UseCase<UpdateIdentityProviderCommand, IdentityProviderUpdated> {

    @Inject
    IdentityProviderRepository idpRepo;

    @Inject
    UnitOfWork unitOfWork;

    @Inject
    SecretService secretService;

    @Override
    public boolean authorizeResource(UpdateIdentityProviderCommand command, ExecutionContext context) {
        return true;
    }

    @Override
    public Result<IdentityProviderUpdated> doExecute(UpdateIdentityProviderCommand command, ExecutionContext context) {
        // Validate ID
        if (command.identityProviderId() == null || command.identityProviderId().isBlank()) {
            return Result.failure(new UseCaseError.ValidationError(
                "ID_REQUIRED",
                "Identity provider ID is required",
                Map.of()
            ));
        }

        // Find existing
        IdentityProvider idp = idpRepo.findByIdOptional(command.identityProviderId()).orElse(null);
        if (idp == null) {
            return Result.failure(new UseCaseError.NotFoundError(
                "IDENTITY_PROVIDER_NOT_FOUND",
                "Identity provider not found",
                Map.of("identityProviderId", command.identityProviderId())
            ));
        }

        // Apply updates
        if (command.name() != null && !command.name().isBlank()) {
            idp.name = command.name();
        }

        if (command.oidcIssuerUrl() != null) {
            idp.oidcIssuerUrl = command.oidcIssuerUrl();
        }

        if (command.oidcClientId() != null) {
            idp.oidcClientId = command.oidcClientId();
        }

        if (command.oidcClientSecretRef() != null && !command.oidcClientSecretRef().isBlank()) {
            // Encrypt client secret before storing
            idp.oidcClientSecretRef = secretService.prepareForStorage("encrypt:" + command.oidcClientSecretRef());
        }

        if (command.oidcMultiTenant() != null) {
            idp.oidcMultiTenant = command.oidcMultiTenant();
        }

        if (command.oidcIssuerPattern() != null) {
            idp.oidcIssuerPattern = command.oidcIssuerPattern();
        }

        if (command.allowedEmailDomains() != null) {
            idp.allowedEmailDomains = new ArrayList<>(
                command.allowedEmailDomains().stream().map(String::toLowerCase).toList()
            );
        }

        // Validate OIDC-specific fields if OIDC type
        if (idp.type == IdentityProviderType.OIDC) {
            // For multi-tenant: require issuer pattern
            // For single-tenant: require issuer URL
            if (idp.oidcMultiTenant) {
                if (idp.oidcIssuerPattern == null || idp.oidcIssuerPattern.isBlank()) {
                    return Result.failure(new UseCaseError.ValidationError(
                        "OIDC_ISSUER_PATTERN_REQUIRED",
                        "OIDC issuer pattern is required for multi-tenant OIDC identity providers",
                        Map.of()
                    ));
                }
            } else {
                if (idp.oidcIssuerUrl == null || idp.oidcIssuerUrl.isBlank()) {
                    return Result.failure(new UseCaseError.ValidationError(
                        "OIDC_ISSUER_URL_REQUIRED",
                        "OIDC issuer URL is required for single-tenant OIDC identity providers",
                        Map.of()
                    ));
                }
            }
            if (idp.oidcClientId == null || idp.oidcClientId.isBlank()) {
                return Result.failure(new UseCaseError.ValidationError(
                    "OIDC_CLIENT_ID_REQUIRED",
                    "OIDC client ID is required for OIDC identity providers",
                    Map.of()
                ));
            }
        }

        // Create domain event
        var event = IdentityProviderUpdated.fromContext(context)
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
