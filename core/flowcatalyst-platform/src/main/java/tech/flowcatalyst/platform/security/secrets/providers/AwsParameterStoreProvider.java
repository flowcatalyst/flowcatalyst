package tech.flowcatalyst.platform.security.secrets.providers;

import io.quarkus.arc.lookup.LookupIfProperty;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import org.jboss.logging.Logger;
import software.amazon.awssdk.services.ssm.SsmClient;
import software.amazon.awssdk.services.ssm.model.*;
import tech.flowcatalyst.platform.security.secrets.SecretProvider;
import tech.flowcatalyst.platform.security.secrets.SecretResolutionException;

/**
 * Secret provider that uses AWS Systems Manager Parameter Store.
 *
 * Reference format: aws-ps://parameter-name
 *
 * Parameters are expected to be stored as SecureString (encrypted with KMS).
 *
 * Configuration:
 * - AWS credentials via standard AWS SDK chain (env vars, instance profile, etc.)
 * - Enabled via flowcatalyst.secrets.aws-ps.enabled=true
 */
@ApplicationScoped
@LookupIfProperty(name = "flowcatalyst.secrets.aws-ps.enabled", stringValue = "true", lookupIfMissing = false)
public class AwsParameterStoreProvider implements SecretProvider {

    private static final Logger LOG = Logger.getLogger(AwsParameterStoreProvider.class);

    private static final String PREFIX = "aws-ps://";

    @Inject
    SsmClient client;

    @Override
    public String resolve(String reference) throws SecretResolutionException {
        if (!canHandle(reference)) {
            throw new SecretResolutionException("Invalid reference format for AWS Parameter Store provider");
        }

        String parameterName = extractParameterName(reference);

        try {
            GetParameterRequest request = GetParameterRequest.builder()
                .name(parameterName)
                .withDecryption(true)
                .build();

            GetParameterResponse response = client.getParameter(request);
            return response.parameter().value();
        } catch (ParameterNotFoundException e) {
            throw new SecretResolutionException("Parameter not found: " + parameterName, e);
        } catch (Exception e) {
            throw new SecretResolutionException("Failed to retrieve parameter from AWS Parameter Store: " + parameterName, e);
        }
    }

    @Override
    public ValidationResult validate(String reference) {
        if (!canHandle(reference)) {
            return ValidationResult.failure("Invalid reference format for AWS Parameter Store");
        }

        String parameterName = extractParameterName(reference);

        try {
            // Use DescribeParameters to check existence without retrieving the value
            DescribeParametersRequest request = DescribeParametersRequest.builder()
                .parameterFilters(
                    ParameterStringFilter.builder()
                        .key("Name")
                        .option("Equals")
                        .values(parameterName)
                        .build()
                )
                .build();

            DescribeParametersResponse response = client.describeParameters(request);

            if (response.parameters().isEmpty()) {
                return ValidationResult.failure("Parameter not found: " + parameterName);
            }

            ParameterMetadata param = response.parameters().get(0);
            String type = param.type().toString();

            return ValidationResult.success("Parameter exists in AWS Parameter Store (type: " + type + ")");
        } catch (Exception e) {
            LOG.debugf(e, "Failed to validate parameter: %s", parameterName);
            return ValidationResult.failure("Failed to access parameter: " + e.getMessage());
        }
    }

    @Override
    public boolean canHandle(String reference) {
        return reference != null && reference.startsWith(PREFIX);
    }

    @Override
    public String getType() {
        return "aws-ps";
    }

    private String extractParameterName(String reference) {
        return reference.substring(PREFIX.length());
    }
}
