package tech.flowcatalyst.platform.security.secrets.providers;

import io.quarkus.arc.lookup.LookupIfProperty;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import org.eclipse.microprofile.config.inject.ConfigProperty;
import org.jboss.logging.Logger;
import software.amazon.awssdk.services.secretsmanager.SecretsManagerClient;
import software.amazon.awssdk.services.secretsmanager.model.*;
import tech.flowcatalyst.platform.security.secrets.SecretProvider;
import tech.flowcatalyst.platform.security.secrets.SecretResolutionException;

/**
 * Secret provider that uses AWS Secrets Manager.
 *
 * Reference format: aws-sm://secret-name
 *
 * Configuration:
 * - AWS credentials via standard AWS SDK chain (env vars, instance profile, etc.)
 * - Enabled when quarkus.secretsmanager.endpoint-override or AWS credentials are configured
 */
@ApplicationScoped
@LookupIfProperty(name = "flowcatalyst.secrets.aws-sm.enabled", stringValue = "true", lookupIfMissing = false)
public class AwsSecretsManagerProvider implements SecretProvider {

    private static final Logger LOG = Logger.getLogger(AwsSecretsManagerProvider.class);

    private static final String PREFIX = "aws-sm://";

    @Inject
    SecretsManagerClient client;

    @Override
    public String resolve(String reference) throws SecretResolutionException {
        if (!canHandle(reference)) {
            throw new SecretResolutionException("Invalid reference format for AWS Secrets Manager provider");
        }

        String secretName = extractSecretName(reference);

        try {
            GetSecretValueRequest request = GetSecretValueRequest.builder()
                .secretId(secretName)
                .build();

            GetSecretValueResponse response = client.getSecretValue(request);

            if (response.secretString() != null) {
                return response.secretString();
            } else {
                throw new SecretResolutionException(
                    "Secret is stored as binary, but string expected: " + secretName);
            }
        } catch (ResourceNotFoundException e) {
            throw new SecretResolutionException("Secret not found: " + secretName, e);
        } catch (Exception e) {
            throw new SecretResolutionException("Failed to retrieve secret from AWS Secrets Manager: " + secretName, e);
        }
    }

    @Override
    public ValidationResult validate(String reference) {
        if (!canHandle(reference)) {
            return ValidationResult.failure("Invalid reference format for AWS Secrets Manager");
        }

        String secretName = extractSecretName(reference);

        try {
            // Use DescribeSecret to check existence without retrieving the value
            DescribeSecretRequest request = DescribeSecretRequest.builder()
                .secretId(secretName)
                .build();

            DescribeSecretResponse response = client.describeSecret(request);

            // Check if secret is scheduled for deletion
            if (response.deletedDate() != null) {
                return ValidationResult.failure("Secret is scheduled for deletion");
            }

            return ValidationResult.success("Secret exists in AWS Secrets Manager");
        } catch (ResourceNotFoundException e) {
            return ValidationResult.failure("Secret not found: " + secretName);
        } catch (Exception e) {
            LOG.debugf(e, "Failed to validate secret: %s", secretName);
            return ValidationResult.failure("Failed to access secret: " + e.getMessage());
        }
    }

    @Override
    public boolean canHandle(String reference) {
        return reference != null && reference.startsWith(PREFIX);
    }

    @Override
    public String getType() {
        return "aws-sm";
    }

    private String extractSecretName(String reference) {
        return reference.substring(PREFIX.length());
    }
}
