package tech.flowcatalyst.platform.authentication.domain.operations.updatemapping;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.authentication.domain.EmailDomainMapping;
import tech.flowcatalyst.platform.authentication.domain.EmailDomainMappingRepository;
import tech.flowcatalyst.platform.authentication.domain.ScopeType;
import tech.flowcatalyst.platform.authentication.domain.events.EmailDomainMappingUpdated;
import tech.flowcatalyst.platform.authentication.idp.IdentityProvider;
import tech.flowcatalyst.platform.authentication.idp.IdentityProviderRepository;
import tech.flowcatalyst.platform.client.Client;
import tech.flowcatalyst.platform.client.ClientRepository;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.common.errors.UseCaseError;

import java.util.ArrayList;
import java.util.List;
import java.util.Map;

/**
 * Use case for updating an Email Domain Mapping.
 */
@ApplicationScoped
public class UpdateEmailDomainMappingUseCase {

    @Inject
    EmailDomainMappingRepository mappingRepo;

    @Inject
    IdentityProviderRepository idpRepo;

    @Inject
    ClientRepository clientRepo;

    @Inject
    UnitOfWork unitOfWork;

    public Result<EmailDomainMappingUpdated> execute(UpdateEmailDomainMappingCommand command, ExecutionContext context) {
        // Validate ID
        if (command.emailDomainMappingId() == null || command.emailDomainMappingId().isBlank()) {
            return Result.failure(new UseCaseError.ValidationError(
                "ID_REQUIRED",
                "Email domain mapping ID is required",
                Map.of()
            ));
        }

        // Find existing
        EmailDomainMapping mapping = mappingRepo.findByIdOptional(command.emailDomainMappingId()).orElse(null);
        if (mapping == null) {
            return Result.failure(new UseCaseError.NotFoundError(
                "EMAIL_DOMAIN_MAPPING_NOT_FOUND",
                "Email domain mapping not found",
                Map.of("emailDomainMappingId", command.emailDomainMappingId())
            ));
        }

        // Track if IDP changed
        String newIdpId = command.identityProviderId() != null ? command.identityProviderId() : mapping.identityProviderId;
        boolean idpChanged = !newIdpId.equals(mapping.identityProviderId);

        // Validate new identity provider if changed
        IdentityProvider idp = null;
        if (idpChanged) {
            idp = idpRepo.findByIdOptional(newIdpId).orElse(null);
            if (idp == null) {
                return Result.failure(new UseCaseError.NotFoundError(
                    "IDENTITY_PROVIDER_NOT_FOUND",
                    "Identity provider not found",
                    Map.of("identityProviderId", newIdpId)
                ));
            }

            // Validate email domain is allowed by new IDP
            if (!idp.isEmailDomainAllowed(mapping.emailDomain)) {
                return Result.failure(new UseCaseError.BusinessRuleViolation(
                    "EMAIL_DOMAIN_NOT_ALLOWED",
                    "Email domain is not in the identity provider's allowed domains list",
                    Map.of(
                        "emailDomain", mapping.emailDomain,
                        "identityProviderId", idp.id,
                        "allowedDomains", idp.allowedEmailDomains
                    )
                ));
            }

            mapping.identityProviderId = newIdpId;
        }

        // Apply scope type and client association updates
        ScopeType newScopeType = command.scopeType() != null ? command.scopeType() : mapping.scopeType;
        String newPrimaryClientId = command.primaryClientId();
        List<String> newAdditionalClientIds = command.additionalClientIds();
        List<String> newGrantedClientIds = command.grantedClientIds();

        // If scope type changed, validate and reset client associations
        if (command.scopeType() != null && command.scopeType() != mapping.scopeType) {
            mapping.scopeType = command.scopeType();

            // Reset client associations based on new scope type
            switch (mapping.scopeType) {
                case ANCHOR -> {
                    mapping.primaryClientId = null;
                    mapping.additionalClientIds = new ArrayList<>();
                    mapping.grantedClientIds = new ArrayList<>();
                }
                case PARTNER -> {
                    mapping.primaryClientId = null;
                    mapping.additionalClientIds = new ArrayList<>();
                    // grantedClientIds can be set
                    if (newGrantedClientIds != null) {
                        mapping.grantedClientIds = new ArrayList<>(newGrantedClientIds);
                    }
                }
                case CLIENT -> {
                    // primaryClientId is required, additionalClientIds allowed
                    mapping.grantedClientIds = new ArrayList<>();
                    if (newPrimaryClientId != null) {
                        mapping.primaryClientId = newPrimaryClientId;
                    }
                    if (newAdditionalClientIds != null) {
                        mapping.additionalClientIds = new ArrayList<>(newAdditionalClientIds);
                    }
                }
            }
        } else {
            // Scope type unchanged, apply field updates if provided
            if (newPrimaryClientId != null) {
                mapping.primaryClientId = newPrimaryClientId;
            }
            if (newAdditionalClientIds != null) {
                mapping.additionalClientIds = new ArrayList<>(newAdditionalClientIds);
            }
            if (newGrantedClientIds != null) {
                mapping.grantedClientIds = new ArrayList<>(newGrantedClientIds);
            }
        }

        // Update requiredOidcTenantId if provided (use empty string to clear)
        if (command.requiredOidcTenantId() != null) {
            mapping.requiredOidcTenantId = command.requiredOidcTenantId().isBlank()
                ? null
                : command.requiredOidcTenantId();
        }

        // Update allowedRoleIds if provided
        if (command.allowedRoleIds() != null) {
            mapping.allowedRoleIds = new ArrayList<>(command.allowedRoleIds());
        }

        // Validate scope type constraints after updates
        var constraintResult = validateScopeTypeConstraints(mapping);
        if (constraintResult instanceof Result.Failure<Void> f) {
            return Result.failure(f.error());
        }

        // Validate referenced clients exist
        var clientValidation = validateClients(mapping);
        if (clientValidation instanceof Result.Failure<Void> f) {
            return Result.failure(f.error());
        }

        // Create domain event
        var event = EmailDomainMappingUpdated.fromContext(context)
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

    private Result<Void> validateScopeTypeConstraints(EmailDomainMapping mapping) {
        switch (mapping.scopeType) {
            case ANCHOR -> {
                if (mapping.primaryClientId != null) {
                    return Result.failure(new UseCaseError.ValidationError(
                        "ANCHOR_NO_PRIMARY_CLIENT",
                        "ANCHOR scope cannot have a primary client",
                        Map.of()
                    ));
                }
                if (mapping.additionalClientIds != null && !mapping.additionalClientIds.isEmpty()) {
                    return Result.failure(new UseCaseError.ValidationError(
                        "ANCHOR_NO_ADDITIONAL_CLIENTS",
                        "ANCHOR scope cannot have additional clients",
                        Map.of()
                    ));
                }
                if (mapping.grantedClientIds != null && !mapping.grantedClientIds.isEmpty()) {
                    return Result.failure(new UseCaseError.ValidationError(
                        "ANCHOR_NO_GRANTED_CLIENTS",
                        "ANCHOR scope cannot have granted clients",
                        Map.of()
                    ));
                }
            }
            case PARTNER -> {
                if (mapping.primaryClientId != null) {
                    return Result.failure(new UseCaseError.ValidationError(
                        "PARTNER_NO_PRIMARY_CLIENT",
                        "PARTNER scope cannot have a primary client",
                        Map.of()
                    ));
                }
                if (mapping.additionalClientIds != null && !mapping.additionalClientIds.isEmpty()) {
                    return Result.failure(new UseCaseError.ValidationError(
                        "PARTNER_NO_ADDITIONAL_CLIENTS",
                        "PARTNER scope cannot have additional clients",
                        Map.of()
                    ));
                }
            }
            case CLIENT -> {
                if (mapping.primaryClientId == null || mapping.primaryClientId.isBlank()) {
                    return Result.failure(new UseCaseError.ValidationError(
                        "CLIENT_REQUIRES_PRIMARY_CLIENT",
                        "CLIENT scope requires a primary client",
                        Map.of()
                    ));
                }
                if (mapping.grantedClientIds != null && !mapping.grantedClientIds.isEmpty()) {
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

    private Result<Void> validateClients(EmailDomainMapping mapping) {
        // Validate primary client if specified
        if (mapping.primaryClientId != null && !mapping.primaryClientId.isBlank()) {
            Client client = clientRepo.findByIdOptional(mapping.primaryClientId).orElse(null);
            if (client == null) {
                return Result.failure(new UseCaseError.NotFoundError(
                    "PRIMARY_CLIENT_NOT_FOUND",
                    "Primary client not found",
                    Map.of("clientId", mapping.primaryClientId)
                ));
            }
        }

        // Validate additional clients
        if (mapping.additionalClientIds != null) {
            for (String clientId : mapping.additionalClientIds) {
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
        if (mapping.grantedClientIds != null) {
            for (String clientId : mapping.grantedClientIds) {
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
