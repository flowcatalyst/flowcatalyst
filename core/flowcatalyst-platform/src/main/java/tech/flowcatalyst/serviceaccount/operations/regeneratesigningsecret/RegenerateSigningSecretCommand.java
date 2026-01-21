package tech.flowcatalyst.serviceaccount.operations.regeneratesigningsecret;

/**
 * Command to regenerate a service account's signing secret.
 *
 * @param serviceAccountId The service account ID
 */
public record RegenerateSigningSecretCommand(
    String serviceAccountId
) {}
