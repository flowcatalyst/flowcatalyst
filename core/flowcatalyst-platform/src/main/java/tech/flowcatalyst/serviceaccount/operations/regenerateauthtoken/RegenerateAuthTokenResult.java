package tech.flowcatalyst.serviceaccount.operations.regenerateauthtoken;

import tech.flowcatalyst.platform.common.Result;

/**
 * Result of regenerating an auth token.
 *
 * @param result    The operation result (success/failure)
 * @param authToken The new auth token (null on failure) - shown only once
 */
public record RegenerateAuthTokenResult(
    Result<AuthTokenRegenerated> result,
    String authToken
) {
    public boolean isSuccess() {
        return result instanceof Result.Success;
    }
}
