package tech.flowcatalyst.serviceaccount.operations.regenerateauthtoken;

/**
 * Command to regenerate a service account's auth token.
 *
 * @param serviceAccountId The service account ID
 * @param customToken      Optional custom token to use instead of generating one
 */
public record RegenerateAuthTokenCommand(
    String serviceAccountId,
    String customToken
) {
    public static RegenerateAuthTokenCommand generate(String serviceAccountId) {
        return new RegenerateAuthTokenCommand(serviceAccountId, null);
    }

    public static RegenerateAuthTokenCommand withCustomToken(String serviceAccountId, String customToken) {
        return new RegenerateAuthTokenCommand(serviceAccountId, customToken);
    }
}
