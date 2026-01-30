package tech.flowcatalyst.platform.authentication.domain.operations.createmapping;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.authentication.domain.EmailDomainMapping;
import tech.flowcatalyst.platform.authentication.domain.EmailDomainMappingRepository;
import tech.flowcatalyst.platform.authentication.domain.ScopeType;
import tech.flowcatalyst.platform.authentication.domain.events.EmailDomainMappingCreated;
import tech.flowcatalyst.platform.authentication.idp.IdentityProvider;
import tech.flowcatalyst.platform.authentication.idp.IdentityProviderRepository;
import tech.flowcatalyst.platform.client.Client;
import tech.flowcatalyst.platform.client.ClientRepository;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;
import tech.flowcatalyst.platform.shared.EntityType;
import tech.flowcatalyst.platform.shared.TsidGenerator;

import java.util.ArrayList;
import java.util.List;
import java.util.Map;

/**
 * Use case for creating an Email Domain Mapping.
 */
@ApplicationScoped
public class CreateEmailDomainMappingUseCase {

    @Inject
    EmailDomainMappingRepository mappingRepo;

    @Inject
    IdentityProviderRepository idpRepo;

    @Inject
    ClientRepository clientRepo;

    @Inject
    UnitOfWork unitOfWork;

    public Result<EmailDomainMappingCreated> execute(CreateEmailDomainMappingCommand command, ExecutionContext context) {
        // Validate email domain
        if (command.emailDomain() == null || command.emailDomain().isBlank()) {
            return Result.failure(new UseCaseError.ValidationError(
                "EMAIL_DOMAIN_REQUIRED",
                "Email domain is required",
                Map.of()
            ));
        }

        // Normalize email domain to lowercase
        String normalizedDomain = command.emailDomain().toLowerCase().trim();

        // Validate email domain format
        if (!normalizedDomain.matches("^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$")) {
            return Result.failure(new UseCaseError.ValidationError(
                "INVALID_EMAIL_DOMAIN_FORMAT",
                "Invalid email domain format",
                Map.of("emailDomain", normalizedDomain)
            ));
        }

        // Validate identity provider exists
        if (command.identityProviderId() == null || command.identityProviderId().isBlank()) {
            return Result.failure(new UseCaseError.ValidationError(
                "IDENTITY_PROVIDER_ID_REQUIRED",
                "Identity provider ID is required",
                Map.of()
            ));
        }

        IdentityProvider idp = idpRepo.findByIdOptional(command.identityProviderId()).orElse(null);
        if (idp == null) {
            return Result.failure(new UseCaseError.NotFoundError(
                "IDENTITY_PROVIDER_NOT_FOUND",
                "Identity provider not found",
                Map.of("identityProviderId", command.identityProviderId())
            ));
        }

        // Validate scope type
        if (command.scopeType() == null) {
            return Result.failure(new UseCaseError.ValidationError(
                "SCOPE_TYPE_REQUIRED",
                "Scope type is required",
                Map.of()
            ));
        }

        // Validate scope type constraints
        var constraintResult = validateScopeTypeConstraints(command);
        if (constraintResult instanceof Result.Failure<Void> f) {
            return Result.failure(f.error());
        }

        // Validate referenced clients exist
        var clientValidation = validateClients(command);
        if (clientValidation instanceof Result.Failure<Void> f) {
            return Result.failure(f.error());
        }

        // Check uniqueness
        if (mappingRepo.existsByEmailDomain(normalizedDomain)) {
            return Result.failure(new UseCaseError.BusinessRuleViolation(
                "EMAIL_DOMAIN_EXISTS",
                "Email domain mapping already exists",
                Map.of("emailDomain", normalizedDomain)
            ));
        }

        // Validate email domain is allowed by IDP (if IDP has restrictions)
        if (!idp.isEmailDomainAllowed(normalizedDomain)) {
            return Result.failure(new UseCaseError.BusinessRuleViolation(
                "EMAIL_DOMAIN_NOT_ALLOWED",
                "Email domain is not in the identity provider's allowed domains list",
                Map.of(
                    "emailDomain", normalizedDomain,
                    "identityProviderId", idp.id,
                    "allowedDomains", idp.allowedEmailDomains
                )
            ));
        }

        // Create mapping
        var mapping = new EmailDomainMapping();
        mapping.id = TsidGenerator.generate(EntityType.EMAIL_DOMAIN_MAPPING);
        mapping.emailDomain = normalizedDomain;
        mapping.identityProviderId = idp.id;
        mapping.scopeType = command.scopeType();
        mapping.primaryClientId = command.primaryClientId();
        mapping.additionalClientIds = command.additionalClientIds() != null
            ? new ArrayList<>(command.additionalClientIds())
            : new ArrayList<>();
        mapping.grantedClientIds = command.grantedClientIds() != null
            ? new ArrayList<>(command.grantedClientIds())
            : new ArrayList<>();
        mapping.requiredOidcTenantId = command.requiredOidcTenantId();
        mapping.allowedRoleIds = command.allowedRoleIds() != null
            ? new ArrayList<>(command.allowedRoleIds())
            : new ArrayList<>();

        // Create domain event
        var event = EmailDomainMappingCreated.fromContext(context)
            .emailDomainMappingId(mapping.id)
            .emailDomain(mapping.emailDomain)
            .identityProviderId(mapping.identityProviderId)
            .scopeType(mapping.scopeType.name())
            .primaryClientId(mapping.primaryClientId)
            .additionalClientIds(mapping.additionalClientIds)
            .grantedClientIds(mapping.grantedClientIds)
            .build();

        // Commit atomically
        return unitOfWork.commit(mapping, event, command);
    }

    private Result<Void> validateScopeTypeConstraints(CreateEmailDomainMappingCommand command) {
        switch (command.scopeType()) {
            case ANCHOR -> {
                if (command.primaryClientId() != null) {
                    return Result.failure(new UseCaseError.ValidationError(
                        "ANCHOR_NO_PRIMARY_CLIENT",
                        "ANCHOR scope cannot have a primary client",
                        Map.of()
                    ));
                }
                if (command.additionalClientIds() != null && !command.additionalClientIds().isEmpty()) {
                    return Result.failure(new UseCaseError.ValidationError(
                        "ANCHOR_NO_ADDITIONAL_CLIENTS",
                        "ANCHOR scope cannot have additional clients",
                        Map.of()
                    ));
                }
                if (command.grantedClientIds() != null && !command.grantedClientIds().isEmpty()) {
                    return Result.failure(new UseCaseError.ValidationError(
                        "ANCHOR_NO_GRANTED_CLIENTS",
                        "ANCHOR scope cannot have granted clients",
                        Map.of()
                    ));
                }
            }
            case PARTNER -> {
                if (command.primaryClientId() != null) {
                    return Result.failure(new UseCaseError.ValidationError(
                        "PARTNER_NO_PRIMARY_CLIENT",
                        "PARTNER scope cannot have a primary client",
                        Map.of()
                    ));
                }
                if (command.additionalClientIds() != null && !command.additionalClientIds().isEmpty()) {
                    return Result.failure(new UseCaseError.ValidationError(
                        "PARTNER_NO_ADDITIONAL_CLIENTS",
                        "PARTNER scope cannot have additional clients",
                        Map.of()
                    ));
                }
            }
            case CLIENT -> {
                if (command.primaryClientId() == null || command.primaryClientId().isBlank()) {
                    return Result.failure(new UseCaseError.ValidationError(
                        "CLIENT_REQUIRES_PRIMARY_CLIENT",
                        "CLIENT scope requires a primary client",
                        Map.of()
                    ));
                }
                if (command.grantedClientIds() != null && !command.grantedClientIds().isEmpty()) {
                    return Result.failure(new UseCaseError.ValidationError(
                        "CLIENT_NO_GRANTED_CLIENTS",
                        "CLIENT scope cannot have granted clients",
                        Map.of()
                    ));
                }
            }
        }
        return Result.success(null);
    }

    private Result<Void> validateClients(CreateEmailDomainMappingCommand command) {
        // Validate primary client if specified
        if (command.primaryClientId() != null && !command.primaryClientId().isBlank()) {
            Client client = clientRepo.findByIdOptional(command.primaryClientId()).orElse(null);
            if (client == null) {
                return Result.failure(new UseCaseError.NotFoundError(
                    "PRIMARY_CLIENT_NOT_FOUND",
                    "Primary client not found",
                    Map.of("clientId", command.primaryClientId())
                ));
            }
        }

        // Validate additional clients
        if (command.additionalClientIds() != null) {
            for (String clientId : command.additionalClientIds()) {
                Client client = clientRepo.findByIdOptional(clientId).orElse(null);
                if (client == null) {
                    return Result.failure(new UseCaseError.NotFoundError(
                        "ADDITIONAL_CLIENT_NOT_FOUND",
                        "Additional client not found",
                        Map.of("clientId", clientId)
                    ));
                }
            }
        }

        // Validate granted clients
        if (command.grantedClientIds() != null) {
            for (String clientId : command.grantedClientIds()) {
                Client client = clientRepo.findByIdOptional(clientId).orElse(null);
                if (client == null) {
                    return Result.failure(new UseCaseError.NotFoundError(
                        "GRANTED_CLIENT_NOT_FOUND",
                        "Granted client not found",
                        Map.of("clientId", clientId)
                    ));
                }
            }
        }

        return Result.success(null);
    }
}
