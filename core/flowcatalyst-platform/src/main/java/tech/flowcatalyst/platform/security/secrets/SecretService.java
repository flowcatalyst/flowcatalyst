package tech.flowcatalyst.platform.security.secrets;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.enterprise.inject.Instance;
import jakarta.inject.Inject;
import org.jboss.logging.Logger;
import tech.flowcatalyst.platform.security.secrets.SecretProvider.ValidationResult;
import tech.flowcatalyst.platform.security.secrets.providers.EncryptedSecretProvider;

import java.util.Optional;

/**
 * Service for resolving secrets from external stores.
 *
 * SECURITY MODEL:
 * - Secret resolution (getting plaintext values) requires Super Admin role
 * - Secret validation (checking if a reference is resolvable) is safe for any admin
 * - Secrets are provisioned by infrastructure teams, not through this service
 *
 * Reference formats:
 * - aws-sm://secret-name - AWS Secrets Manager
 * - aws-ps://parameter-name - AWS Parameter Store
 * - gcp-sm://projects/PROJECT/secrets/NAME/versions/VERSION - GCP Secret Manager
 * - vault://path/to/secret#key - HashiCorp Vault
 * - encrypted:BASE64_CIPHERTEXT - Locally encrypted (legacy/migration support)
 */
@ApplicationScoped
public class SecretService {

    private static final Logger LOG = Logger.getLogger(SecretService.class);

    @Inject
    Instance<SecretProvider> providers;

    @Inject
    EncryptedSecretProvider encryptedProvider;

    /**
     * Resolve a secret reference to its plaintext value.
     *
     * SECURITY: This method should only be called by system processes that
     * need the actual secret value (e.g., OIDC client authentication).
     * The calling code must ensure the operation is authorized.
     *
     * For user-initiated operations, use resolveWithPermissionCheck() instead.
     *
     * @param reference The secret reference
     * @return The plaintext secret value
     * @throws SecretResolutionException if no provider can handle the reference
     */
    public String resolve(String reference) {
        if (reference == null || reference.isBlank()) {
            throw new SecretResolutionException("Secret reference cannot be null or blank");
        }

        return findProviderForReference(reference)
            .orElseThrow(() -> new SecretResolutionException(
                "No secret provider found that can handle reference: " + maskReference(reference)))
            .resolve(reference);
    }

    /**
     * Resolve a secret reference, returning empty if the reference is null.
     *
     * @param reference The secret reference (may be null)
     * @return Optional containing the plaintext if reference is not null
     */
    public Optional<String> resolveOptional(String reference) {
        if (reference == null || reference.isBlank()) {
            return Optional.empty();
        }
        return Optional.of(resolve(reference));
    }

    /**
     * Validate that a secret reference is resolvable without returning the value.
     * This is safe to call for any authenticated admin user.
     *
     * @param reference The secret reference to validate
     * @return ValidationResult indicating success or failure
     */
    public ValidationResult validate(String reference) {
        if (reference == null || reference.isBlank()) {
            return ValidationResult.failure("Secret reference cannot be null or blank");
        }

        Optional<SecretProvider> provider = findProviderForReference(reference);
        if (provider.isEmpty()) {
            return ValidationResult.failure(
                "Unknown secret reference format. Supported formats: aws-sm://, aws-ps://, gcp-sm://, vault://");
        }

        try {
            return provider.get().validate(reference);
        } catch (Exception e) {
            LOG.debugf(e, "Secret validation failed for reference: %s", maskReference(reference));
            return ValidationResult.failure("Validation failed: " + e.getMessage());
        }
    }

    /**
     * Check if a reference format is recognized by any provider.
     *
     * @param reference The secret reference
     * @return true if a provider can handle this reference format
     */
    public boolean isValidFormat(String reference) {
        if (reference == null || reference.isBlank()) {
            return false;
        }
        return findProviderForReference(reference).isPresent();
    }

    /**
     * Get the provider type for a reference.
     *
     * @param reference The secret reference
     * @return Optional containing the provider type if recognized
     */
    public Optional<String> getProviderType(String reference) {
        return findProviderForReference(reference)
            .map(SecretProvider::getType);
    }

    /**
     * Prepare a secret reference for storage.
     * If the reference uses the "encrypt:" prefix, it will be encrypted
     * and converted to "encrypted:" format.
     *
     * @param reference The secret reference (may be encrypt:PLAINTEXT or other format)
     * @return The reference ready for storage (encrypted if needed)
     * @throws SecretResolutionException if encryption fails
     */
    public String prepareForStorage(String reference) {
        if (reference == null || reference.isBlank()) {
            return reference;
        }

        // If it's a plaintext reference, encrypt it
        if (encryptedProvider.isPlaintextReference(reference)) {
            LOG.debug("Encrypting plaintext secret reference for storage");
            return encryptedProvider.encryptReference(reference);
        }

        // Otherwise return as-is (it's already a proper reference)
        return reference;
    }

    /**
     * Check if local encryption is available.
     *
     * @return true if encrypt: prefix can be used
     */
    public boolean isEncryptionAvailable() {
        return encryptedProvider.isEncryptionAvailable();
    }

    private Optional<SecretProvider> findProviderForReference(String reference) {
        for (SecretProvider provider : providers) {
            if (provider.canHandle(reference)) {
                return Optional.of(provider);
            }
        }
        return Optional.empty();
    }

    private String maskReference(String reference) {
        if (reference == null || reference.length() <= 20) {
            return "***";
        }
        // Show the prefix (provider type) but mask the rest
        int prefixEnd = reference.indexOf("://");
        if (prefixEnd > 0 && prefixEnd < 15) {
            return reference.substring(0, prefixEnd + 3) + "***";
        }
        return reference.substring(0, 15) + "...";
    }
}
