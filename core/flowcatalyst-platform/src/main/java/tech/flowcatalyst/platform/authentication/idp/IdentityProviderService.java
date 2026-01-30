package tech.flowcatalyst.platform.authentication.idp;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.authentication.domain.EmailDomainMapping;
import tech.flowcatalyst.platform.authentication.domain.EmailDomainMappingRepository;
import tech.flowcatalyst.platform.security.secrets.SecretService;

import java.util.List;
import java.util.Optional;

/**
 * Service for Identity Provider operations with secure secret handling.
 *
 * <p>Provides combined lookups across IdentityProvider and EmailDomainMapping
 * entities, and handles secret resolution for OIDC authentication flows.
 */
@ApplicationScoped
public class IdentityProviderService {

    @Inject
    IdentityProviderRepository idpRepository;

    @Inject
    EmailDomainMappingRepository mappingRepository;

    @Inject
    SecretService secretService;

    /**
     * Find an identity provider by ID.
     */
    public Optional<IdentityProvider> findById(String id) {
        return idpRepository.findByIdOptional(id);
    }

    /**
     * Find an identity provider by code.
     */
    public Optional<IdentityProvider> findByCode(String code) {
        return idpRepository.findByCode(code);
    }

    /**
     * Find all identity providers.
     */
    public List<IdentityProvider> findAll() {
        return idpRepository.listAll();
    }

    /**
     * Find an email domain mapping by email domain.
     */
    public Optional<EmailDomainMapping> findMappingByEmailDomain(String emailDomain) {
        return mappingRepository.findByEmailDomain(emailDomain.toLowerCase());
    }

    /**
     * Find an email domain mapping by ID.
     */
    public Optional<EmailDomainMapping> findMappingById(String id) {
        return mappingRepository.findByIdOptional(id);
    }

    /**
     * Find the identity provider for an email domain.
     * Looks up the EmailDomainMapping first, then resolves the IdentityProvider.
     *
     * @param emailDomain The email domain
     * @return Optional containing the identity provider if found
     */
    public Optional<IdentityProvider> findByEmailDomain(String emailDomain) {
        return mappingRepository.findByEmailDomain(emailDomain.toLowerCase())
            .flatMap(mapping -> idpRepository.findByIdOptional(mapping.identityProviderId));
    }

    /**
     * Find both the identity provider and email domain mapping for an email domain.
     *
     * @param emailDomain The email domain
     * @return Optional containing both entities if found
     */
    public Optional<AuthenticationConfig> findAuthConfigByEmailDomain(String emailDomain) {
        return mappingRepository.findByEmailDomain(emailDomain.toLowerCase())
            .flatMap(mapping -> idpRepository.findByIdOptional(mapping.identityProviderId)
                .map(idp -> new AuthenticationConfig(idp, mapping)));
    }

    /**
     * Resolve the OIDC client secret for an identity provider.
     * This returns the plaintext secret for use in OIDC authentication.
     *
     * <p>SECURITY: This method should only be called by system processes
     * that need the actual secret value (e.g., OIDC token exchange).
     *
     * @param idp The identity provider
     * @return Optional containing the plaintext secret if configured
     */
    public Optional<String> resolveClientSecret(IdentityProvider idp) {
        if (idp == null || !idp.hasClientSecret()) {
            return Optional.empty();
        }
        return secretService.resolveOptional(idp.oidcClientSecretRef);
    }

    /**
     * Check if a secret reference is valid.
     */
    public boolean isValidSecretFormat(String secretRef) {
        return secretService.isValidFormat(secretRef);
    }

    /**
     * Prepare a secret reference for storage (encrypts if needed).
     */
    public String prepareSecretForStorage(String secretRef) {
        return secretService.prepareForStorage(secretRef);
    }

    /**
     * Combined result containing both IdentityProvider and EmailDomainMapping.
     * Used when both are needed for authentication flow.
     */
    public record AuthenticationConfig(
        IdentityProvider identityProvider,
        EmailDomainMapping emailDomainMapping
    ) {
        /**
         * Check if the email domain is allowed by the identity provider's restrictions.
         */
        public boolean isEmailDomainAllowed() {
            return identityProvider.isEmailDomainAllowed(emailDomainMapping.emailDomain);
        }
    }
}
