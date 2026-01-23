package tech.flowcatalyst.serviceaccount.operations.createserviceaccount;

import tech.flowcatalyst.platform.authentication.oauth.OAuthClient;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.serviceaccount.entity.ServiceAccount;

/**
 * Result of creating a service account.
 *
 * <p>Contains the result of the operation plus the generated credentials
 * which are only available at creation time.</p>
 *
 * <p>A service account creation atomically creates:</p>
 * <ul>
 *   <li>ServiceAccount - for webhook credentials</li>
 *   <li>Principal (type=SERVICE) - for identity and role assignments</li>
 *   <li>OAuthClient (CONFIDENTIAL) - for OAuth client_credentials authentication</li>
 * </ul>
 *
 * @param result         The operation result (success/failure)
 * @param serviceAccount The created service account (null on failure)
 * @param principal      The created principal (null on failure)
 * @param oauthClient    The created OAuth client (null on failure)
 * @param authToken      The generated webhook auth token (null on failure) - shown only once
 * @param signingSecret  The generated webhook signing secret (null on failure) - shown only once
 * @param clientId       The OAuth client_id for authentication (null on failure) - use with clientSecret
 * @param clientSecret   The OAuth client_secret (null on failure) - shown only once
 */
public record CreateServiceAccountResult(
    Result<ServiceAccountCreated> result,
    ServiceAccount serviceAccount,
    Principal principal,
    OAuthClient oauthClient,
    String authToken,
    String signingSecret,
    String clientId,
    String clientSecret
) {
    public boolean isSuccess() {
        return result instanceof Result.Success;
    }

    public boolean isFailure() {
        return result instanceof Result.Failure;
    }

    /**
     * Create a failure result with no entities or credentials.
     */
    public static CreateServiceAccountResult failure(Result<ServiceAccountCreated> result) {
        return new CreateServiceAccountResult(result, null, null, null, null, null, null, null);
    }
}
