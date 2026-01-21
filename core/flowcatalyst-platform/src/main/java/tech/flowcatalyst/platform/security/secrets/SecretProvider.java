package tech.flowcatalyst.platform.security.secrets;

/**
 * Interface for resolving secrets from external stores.
 *
 * SECURITY NOTE: This interface is read-only by design. Secrets should be
 * provisioned by infrastructure teams directly in the secret stores
 * (AWS Secrets Manager, GCP Secret Manager, Vault, etc.). The platform
 * only stores references (URIs) to these secrets and resolves them at runtime.
 *
 * Secret resolution is a privileged operation that requires Super Admin role.
 */
public interface SecretProvider {

    /**
     * Resolve a secret reference to its plaintext value.
     *
     * SECURITY: This is a privileged operation. Callers must verify
     * the requesting principal has Super Admin role before calling.
     *
     * @param reference The secret reference (URI to external store)
     * @return The plaintext secret value
     * @throws SecretResolutionException if the secret cannot be resolved
     */
    String resolve(String reference) throws SecretResolutionException;

    /**
     * Validate that a secret reference is resolvable without returning the value.
     * This is safe to call without Super Admin privileges.
     *
     * @param reference The secret reference to validate
     * @return ValidationResult indicating success or failure with message
     */
    ValidationResult validate(String reference);

    /**
     * Check if this provider can handle the given reference.
     * Used to determine which provider should resolve a reference.
     *
     * @param reference The secret reference
     * @return true if this provider can handle the reference
     */
    boolean canHandle(String reference);

    /**
     * Get the provider type identifier.
     *
     * @return The provider type (e.g., "encrypted", "aws-sm", "aws-ps", "gcp-sm", "vault")
     */
    String getType();

    /**
     * Result of validating a secret reference.
     */
    record ValidationResult(boolean valid, String message) {
        public static ValidationResult success() {
            return new ValidationResult(true, "Secret is accessible");
        }

        public static ValidationResult success(String message) {
            return new ValidationResult(true, message);
        }

        public static ValidationResult failure(String message) {
            return new ValidationResult(false, message);
        }
    }
}
