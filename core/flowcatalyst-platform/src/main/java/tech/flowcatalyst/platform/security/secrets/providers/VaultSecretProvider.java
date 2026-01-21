package tech.flowcatalyst.platform.security.secrets.providers;

import io.quarkus.arc.lookup.LookupIfProperty;
import io.quarkus.vault.VaultKVSecretEngine;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import org.jboss.logging.Logger;
import tech.flowcatalyst.platform.security.secrets.SecretProvider;
import tech.flowcatalyst.platform.security.secrets.SecretResolutionException;

import java.util.Map;

/**
 * Secret provider that uses HashiCorp Vault KV secrets engine.
 *
 * Reference format: vault://path/to/secret#key
 * - path/to/secret: The path in Vault (relative to the mount point)
 * - key: The key within the secret (defaults to "value" if not specified)
 *
 * Configuration:
 * - Standard Quarkus Vault configuration (quarkus.vault.*)
 * - flowcatalyst.secrets.vault.enabled: Must be true to enable this provider
 */
@ApplicationScoped
@LookupIfProperty(name = "flowcatalyst.secrets.vault.enabled", stringValue = "true", lookupIfMissing = false)
public class VaultSecretProvider implements SecretProvider {

    private static final Logger LOG = Logger.getLogger(VaultSecretProvider.class);

    private static final String PREFIX = "vault://";
    private static final String DEFAULT_KEY = "value";

    @Inject
    VaultKVSecretEngine kvEngine;

    @Override
    public String resolve(String reference) throws SecretResolutionException {
        if (!canHandle(reference)) {
            throw new SecretResolutionException("Invalid reference format for Vault provider");
        }

        PathAndKey parsed = parseReference(reference);

        try {
            Map<String, String> secret = kvEngine.readSecret(parsed.path);

            if (secret == null || secret.isEmpty()) {
                throw new SecretResolutionException("Secret not found: " + parsed.path);
            }

            String value = secret.get(parsed.key);
            if (value == null) {
                throw new SecretResolutionException(
                    String.format("Key '%s' not found in secret: %s", parsed.key, parsed.path));
            }

            return value;
        } catch (SecretResolutionException e) {
            throw e;
        } catch (Exception e) {
            throw new SecretResolutionException("Failed to retrieve secret from Vault: " + parsed.path, e);
        }
    }

    @Override
    public ValidationResult validate(String reference) {
        if (!canHandle(reference)) {
            return ValidationResult.failure("Invalid reference format for Vault");
        }

        PathAndKey parsed = parseReference(reference);

        try {
            Map<String, String> secret = kvEngine.readSecret(parsed.path);

            if (secret == null || secret.isEmpty()) {
                return ValidationResult.failure("Secret not found: " + parsed.path);
            }

            if (!secret.containsKey(parsed.key)) {
                return ValidationResult.failure(
                    String.format("Key '%s' not found in secret (available keys: %s)",
                        parsed.key, String.join(", ", secret.keySet())));
            }

            return ValidationResult.success("Secret exists in Vault with key '" + parsed.key + "'");
        } catch (Exception e) {
            LOG.debugf(e, "Failed to validate secret: %s", parsed.path);
            return ValidationResult.failure("Failed to access secret: " + e.getMessage());
        }
    }

    @Override
    public boolean canHandle(String reference) {
        return reference != null && reference.startsWith(PREFIX);
    }

    @Override
    public String getType() {
        return "vault";
    }

    private PathAndKey parseReference(String reference) {
        String pathAndKey = reference.substring(PREFIX.length());

        int hashIndex = pathAndKey.indexOf('#');
        if (hashIndex > 0) {
            return new PathAndKey(
                pathAndKey.substring(0, hashIndex),
                pathAndKey.substring(hashIndex + 1)
            );
        } else {
            return new PathAndKey(pathAndKey, DEFAULT_KEY);
        }
    }

    private record PathAndKey(String path, String key) {}
}
