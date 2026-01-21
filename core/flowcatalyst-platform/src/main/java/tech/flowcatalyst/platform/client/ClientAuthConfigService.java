package tech.flowcatalyst.platform.client;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.transaction.Transactional;
import org.jboss.logging.Logger;
import tech.flowcatalyst.platform.authentication.AuthProvider;
import tech.flowcatalyst.platform.security.secrets.SecretProvider.ValidationResult;
import tech.flowcatalyst.platform.security.secrets.SecretService;
import tech.flowcatalyst.platform.shared.TsidGenerator;

import java.time.Instant;
import java.util.ArrayList;
import java.util.List;
import java.util.Optional;

/**
 * Service for managing ClientAuthConfig entities with secure secret handling.
 *
 * SECURITY MODEL:
 * - Secrets are stored in external secret managers (AWS, GCP, Vault) by infrastructure teams
 * - This service only stores and validates secret REFERENCES (URIs), never plaintext
 * - Secret resolution (getting plaintext) requires Super Admin role
 * - Validation (checking a reference is accessible) is safe for any admin
 */
@ApplicationScoped
public class ClientAuthConfigService {

    private static final Logger LOG = Logger.getLogger(ClientAuthConfigService.class);

    @Inject
    ClientAuthConfigRepository repository;

    @Inject
    SecretService secretService;

    /**
     * Find auth config by email domain.
     */
    public Optional<ClientAuthConfig> findByEmailDomain(String emailDomain) {
        return repository.findByEmailDomain(emailDomain.toLowerCase());
    }

    /**
     * Find auth config by ID.
     */
    public Optional<ClientAuthConfig> findById(String id) {
        return repository.findByIdOptional(id);
    }

    /**
     * List all auth configs.
     */
    public List<ClientAuthConfig> listAll() {
        return repository.listAll();
    }

    /**
     * List auth configs by client ID (searches both primaryClientId and legacy clientId).
     */
    public List<ClientAuthConfig> findByClientId(String clientId) {
        // Search both primaryClientId and legacy clientId for backwards compatibility
        return repository.findByClientId(clientId);
    }

    /**
     * List auth configs by config type.
     */
    public List<ClientAuthConfig> findByConfigType(AuthConfigType configType) {
        return repository.findByConfigType(configType);
    }

    /**
     * Create a new auth config with INTERNAL authentication.
     *
     * @param emailDomain The email domain
     * @param configType The config type (ANCHOR, PARTNER, CLIENT)
     * @param primaryClientId The primary client ID (required for CLIENT type, null for others)
     * @return The created config
     */
    @Transactional
    public ClientAuthConfig createInternal(String emailDomain, AuthConfigType configType, String primaryClientId) {
        return create(emailDomain, configType, primaryClientId, AuthProvider.INTERNAL, null, null, null, false, null);
    }

    /**
     * Create a new auth config with OIDC authentication.
     *
     * @param emailDomain The email domain
     * @param configType The config type (ANCHOR, PARTNER, CLIENT)
     * @param primaryClientId The primary client ID (required for CLIENT type, null for others)
     * @param oidcIssuerUrl OIDC issuer URL
     * @param oidcClientId OIDC client ID
     * @param oidcClientSecretRef Reference to secret in external store (e.g., aws-sm://secret-name)
     * @return The created config
     */
    @Transactional
    public ClientAuthConfig createOidc(
            String emailDomain,
            AuthConfigType configType,
            String primaryClientId,
            String oidcIssuerUrl,
            String oidcClientId,
            String oidcClientSecretRef) {
        return createOidc(emailDomain, configType, primaryClientId, oidcIssuerUrl, oidcClientId, oidcClientSecretRef, false, null);
    }

    /**
     * Create a new auth config with OIDC authentication (with multi-tenant support).
     *
     * @param emailDomain The email domain
     * @param configType The config type (ANCHOR, PARTNER, CLIENT)
     * @param primaryClientId The primary client ID (required for CLIENT type, null for others)
     * @param oidcIssuerUrl OIDC issuer URL
     * @param oidcClientId OIDC client ID
     * @param oidcClientSecretRef Reference to secret in external store (e.g., aws-sm://secret-name)
     * @param oidcMultiTenant Whether this is a multi-tenant OIDC configuration
     * @param oidcIssuerPattern Pattern for validating multi-tenant issuers
     * @return The created config
     */
    @Transactional
    public ClientAuthConfig createOidc(
            String emailDomain,
            AuthConfigType configType,
            String primaryClientId,
            String oidcIssuerUrl,
            String oidcClientId,
            String oidcClientSecretRef,
            boolean oidcMultiTenant,
            String oidcIssuerPattern) {
        return create(emailDomain, configType, primaryClientId, AuthProvider.OIDC, oidcIssuerUrl, oidcClientId,
                oidcClientSecretRef, oidcMultiTenant, oidcIssuerPattern);
    }

    /**
     * Create a new auth config.
     *
     * @param emailDomain The email domain
     * @param configType The config type (ANCHOR, PARTNER, CLIENT)
     * @param primaryClientId The primary client ID (required for CLIENT type, null for others)
     * @param authProvider The auth provider type
     * @param oidcIssuerUrl OIDC issuer URL (required for OIDC)
     * @param oidcClientId OIDC client ID (required for OIDC)
     * @param oidcClientSecretRef Reference to secret in external store (not plaintext!)
     * @param oidcMultiTenant Whether this is a multi-tenant OIDC configuration
     * @param oidcIssuerPattern Pattern for validating multi-tenant issuers
     * @return The created config
     */
    @Transactional
    public ClientAuthConfig create(
            String emailDomain,
            AuthConfigType configType,
            String primaryClientId,
            AuthProvider authProvider,
            String oidcIssuerUrl,
            String oidcClientId,
            String oidcClientSecretRef,
            boolean oidcMultiTenant,
            String oidcIssuerPattern) {

        String normalizedDomain = emailDomain.toLowerCase();

        // Check for duplicate domain
        if (repository.existsByEmailDomain(normalizedDomain)) {
            throw new IllegalArgumentException("Auth config already exists for domain: " + normalizedDomain);
        }

        ClientAuthConfig config = new ClientAuthConfig();
        config.id = TsidGenerator.generate();
        config.emailDomain = normalizedDomain;
        config.configType = configType;
        config.primaryClientId = primaryClientId;
        config.clientId = primaryClientId; // Backwards compatibility
        config.additionalClientIds = new ArrayList<>();
        config.grantedClientIds = new ArrayList<>();
        config.authProvider = authProvider;
        config.createdAt = Instant.now();
        config.updatedAt = Instant.now();

        // Validate config type constraints
        config.validateConfigTypeConstraints();

        if (authProvider == AuthProvider.OIDC) {
            config.oidcIssuerUrl = oidcIssuerUrl;
            config.oidcClientId = oidcClientId;
            config.oidcMultiTenant = oidcMultiTenant;
            config.oidcIssuerPattern = oidcIssuerPattern;

            // Prepare secret reference for storage (encrypts if encrypt: prefix used)
            if (oidcClientSecretRef != null && !oidcClientSecretRef.isBlank()) {
                if (!secretService.isValidFormat(oidcClientSecretRef)) {
                    throw new IllegalArgumentException(
                        "Invalid secret reference format. Use encrypt:, aws-sm://, aws-ps://, gcp-sm://, or vault:// prefix");
                }
                config.oidcClientSecretRef = secretService.prepareForStorage(oidcClientSecretRef);
            }

            config.validateOidcConfig();
        }

        repository.persist(config);
        LOG.infof("Created auth config for domain: %s (type: %s, provider: %s, primaryClientId: %s)",
            normalizedDomain, configType, authProvider, primaryClientId);

        return config;
    }

    /**
     * Update an existing OIDC auth config.
     *
     * @param id The config ID
     * @param oidcIssuerUrl New OIDC issuer URL
     * @param oidcClientId New OIDC client ID
     * @param oidcClientSecretRef New reference to secret (not plaintext!)
     * @return The updated config
     */
    @Transactional
    public ClientAuthConfig updateOidc(
            String id,
            String oidcIssuerUrl,
            String oidcClientId,
            String oidcClientSecretRef) {
        return updateOidc(id, oidcIssuerUrl, oidcClientId, oidcClientSecretRef, null, null);
    }

    /**
     * Update an existing OIDC auth config with multi-tenant support.
     *
     * @param id The config ID
     * @param oidcIssuerUrl New OIDC issuer URL
     * @param oidcClientId New OIDC client ID
     * @param oidcClientSecretRef New reference to secret (not plaintext!)
     * @param oidcMultiTenant Whether this is a multi-tenant OIDC configuration (null to keep existing)
     * @param oidcIssuerPattern Pattern for validating multi-tenant issuers (null to keep existing)
     * @return The updated config
     */
    @Transactional
    public ClientAuthConfig updateOidc(
            String id,
            String oidcIssuerUrl,
            String oidcClientId,
            String oidcClientSecretRef,
            Boolean oidcMultiTenant,
            String oidcIssuerPattern) {

        ClientAuthConfig config = repository.findByIdOptional(id)
            .orElseThrow(() -> new IllegalArgumentException("Auth config not found: " + id));

        if (config.authProvider != AuthProvider.OIDC) {
            throw new IllegalArgumentException("Cannot update OIDC settings on non-OIDC config");
        }

        config.oidcIssuerUrl = oidcIssuerUrl;
        config.oidcClientId = oidcClientId;

        // Update multi-tenant settings if provided
        if (oidcMultiTenant != null) {
            config.oidcMultiTenant = oidcMultiTenant;
        }
        if (oidcIssuerPattern != null) {
            config.oidcIssuerPattern = oidcIssuerPattern.isBlank() ? null : oidcIssuerPattern;
        }

        // Update secret reference if provided
        if (oidcClientSecretRef != null && !oidcClientSecretRef.isBlank()) {
            if (!secretService.isValidFormat(oidcClientSecretRef)) {
                throw new IllegalArgumentException(
                    "Invalid secret reference format. Use encrypt:, aws-sm://, aws-ps://, gcp-sm://, or vault:// prefix");
            }
            config.oidcClientSecretRef = secretService.prepareForStorage(oidcClientSecretRef);
        }

        config.validateOidcConfig();
        config.updatedAt = Instant.now();
        repository.update(config);

        LOG.infof("Updated auth config for domain: %s (multiTenant: %s)", config.emailDomain, config.oidcMultiTenant);

        return config;
    }

    /**
     * Update the client binding for an auth config.
     * Only works for CLIENT type configs.
     *
     * @param id The config ID
     * @param primaryClientId The new primary client ID (required for CLIENT type)
     * @return The updated config
     * @deprecated Use type-specific methods like {@link #updatePrimaryClient(String, String)}
     */
    @Deprecated
    @Transactional
    public ClientAuthConfig updateClientBinding(String id, String primaryClientId) {
        ClientAuthConfig config = repository.findByIdOptional(id)
            .orElseThrow(() -> new IllegalArgumentException("Auth config not found: " + id));

        // Update both fields for backwards compatibility
        config.clientId = primaryClientId;
        config.primaryClientId = primaryClientId;
        config.updatedAt = Instant.now();
        repository.update(config);

        LOG.infof("Updated client binding for domain: %s to clientId: %s",
            config.emailDomain, primaryClientId != null ? primaryClientId : "null");

        return config;
    }

    /**
     * Update the config type for an auth config.
     * This will reset client associations based on the new type.
     *
     * @param id The config ID
     * @param newType The new config type
     * @param primaryClientId The primary client ID (required for CLIENT type, must be null for others)
     * @return The updated config
     */
    @Transactional
    public ClientAuthConfig updateConfigType(String id, AuthConfigType newType, String primaryClientId) {
        ClientAuthConfig config = repository.findByIdOptional(id)
            .orElseThrow(() -> new IllegalArgumentException("Auth config not found: " + id));

        config.configType = newType;
        config.primaryClientId = primaryClientId;
        config.clientId = primaryClientId; // Backwards compatibility

        // Reset lists based on new type
        switch (newType) {
            case ANCHOR -> {
                config.additionalClientIds = new ArrayList<>();
                config.grantedClientIds = new ArrayList<>();
            }
            case PARTNER -> {
                config.additionalClientIds = new ArrayList<>();
                // grantedClientIds can be set separately
            }
            case CLIENT -> {
                config.grantedClientIds = new ArrayList<>();
                // additionalClientIds can be set separately
            }
        }

        config.validateConfigTypeConstraints();
        config.updatedAt = Instant.now();
        repository.update(config);

        LOG.infof("Updated config type for domain: %s to %s (primaryClientId: %s)",
            config.emailDomain, newType, primaryClientId);

        return config;
    }

    /**
     * Update the primary client for a CLIENT type config.
     *
     * @param id The config ID
     * @param primaryClientId The new primary client ID (required)
     * @return The updated config
     */
    @Transactional
    public ClientAuthConfig updatePrimaryClient(String id, String primaryClientId) {
        ClientAuthConfig config = repository.findByIdOptional(id)
            .orElseThrow(() -> new IllegalArgumentException("Auth config not found: " + id));

        if (config.getEffectiveConfigType() != AuthConfigType.CLIENT) {
            throw new IllegalArgumentException("Cannot update primary client on non-CLIENT config type");
        }

        if (primaryClientId == null || primaryClientId.isBlank()) {
            throw new IllegalArgumentException("Primary client ID is required for CLIENT config type");
        }

        config.primaryClientId = primaryClientId;
        config.clientId = primaryClientId; // Backwards compatibility
        config.validateConfigTypeConstraints();
        config.updatedAt = Instant.now();
        repository.update(config);

        LOG.infof("Updated primary client for domain: %s to %s", config.emailDomain, primaryClientId);

        return config;
    }

    /**
     * Update the additional client IDs for a CLIENT type config.
     *
     * @param id The config ID
     * @param additionalClientIds List of additional client IDs (can be empty)
     * @return The updated config
     */
    @Transactional
    public ClientAuthConfig updateAdditionalClients(String id, List<String> additionalClientIds) {
        ClientAuthConfig config = repository.findByIdOptional(id)
            .orElseThrow(() -> new IllegalArgumentException("Auth config not found: " + id));

        if (config.getEffectiveConfigType() != AuthConfigType.CLIENT) {
            throw new IllegalArgumentException("Additional clients are only allowed for CLIENT config type");
        }

        config.additionalClientIds = additionalClientIds != null ? new ArrayList<>(additionalClientIds) : new ArrayList<>();
        config.validateConfigTypeConstraints();
        config.updatedAt = Instant.now();
        repository.update(config);

        LOG.infof("Updated additional clients for domain: %s to %s", config.emailDomain, config.additionalClientIds);

        return config;
    }

    /**
     * Update the granted client IDs for a PARTNER type config.
     *
     * @param id The config ID
     * @param grantedClientIds List of granted client IDs (can be empty)
     * @return The updated config
     */
    @Transactional
    public ClientAuthConfig updateGrantedClients(String id, List<String> grantedClientIds) {
        ClientAuthConfig config = repository.findByIdOptional(id)
            .orElseThrow(() -> new IllegalArgumentException("Auth config not found: " + id));

        if (config.getEffectiveConfigType() != AuthConfigType.PARTNER) {
            throw new IllegalArgumentException("Granted clients are only allowed for PARTNER config type");
        }

        config.grantedClientIds = grantedClientIds != null ? new ArrayList<>(grantedClientIds) : new ArrayList<>();
        config.validateConfigTypeConstraints();
        config.updatedAt = Instant.now();
        repository.update(config);

        LOG.infof("Updated granted clients for domain: %s to %s", config.emailDomain, config.grantedClientIds);

        return config;
    }

    /**
     * Delete an auth config.
     */
    @Transactional
    public void delete(String id) {
        ClientAuthConfig config = repository.findByIdOptional(id)
            .orElseThrow(() -> new IllegalArgumentException("Auth config not found: " + id));

        repository.delete(config);
        LOG.infof("Deleted auth config for domain: %s", config.emailDomain);
    }

    /**
     * Validate that the secret reference is accessible.
     * This checks the reference can be resolved without returning the actual value.
     *
     * @param secretRef The secret reference to validate
     * @return ValidationResult indicating success or failure
     */
    public ValidationResult validateSecretReference(String secretRef) {
        if (secretRef == null || secretRef.isBlank()) {
            return ValidationResult.failure("Secret reference is required");
        }

        return secretService.validate(secretRef);
    }

    /**
     * Resolve the OIDC client secret for a config.
     * This returns the plaintext secret for use in OIDC authentication.
     *
     * SECURITY: This method should only be called by system processes
     * that need the actual secret value (e.g., OIDC token exchange).
     * The calling code must ensure Super Admin authorization.
     *
     * @param config The auth config
     * @return Optional containing the plaintext secret if configured
     */
    public Optional<String> resolveClientSecret(ClientAuthConfig config) {
        if (config == null || !config.hasClientSecret()) {
            return Optional.empty();
        }

        return secretService.resolveOptional(config.oidcClientSecretRef);
    }

    /**
     * Get OIDC configuration with resolved secret.
     * Returns a DTO with the plaintext secret for use in OIDC flows.
     *
     * SECURITY: This method should only be called by system processes.
     * The calling code must ensure Super Admin authorization.
     */
    public Optional<OidcConfig> getOidcConfig(String emailDomain) {
        return findByEmailDomain(emailDomain)
            .filter(config -> config.authProvider == AuthProvider.OIDC)
            .map(config -> new OidcConfig(
                config.oidcIssuerUrl,
                config.oidcClientId,
                resolveClientSecret(config).orElse(null)
            ));
    }

    /**
     * DTO containing resolved OIDC configuration.
     */
    public record OidcConfig(
        String issuerUrl,
        String clientId,
        String clientSecret
    ) {}
}
