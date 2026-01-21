package tech.flowcatalyst.serviceaccount.operations.createserviceaccount;

import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.serviceaccount.entity.ServiceAccount;

/**
 * Result of creating a service account.
 *
 * <p>Contains the result of the operation plus the generated credentials
 * which are only available at creation time.</p>
 *
 * @param result        The operation result (success/failure)
 * @param serviceAccount The created service account (null on failure)
 * @param authToken     The generated auth token (null on failure) - shown only once
 * @param signingSecret The generated signing secret (null on failure) - shown only once
 */
public record CreateServiceAccountResult(
    Result<ServiceAccountCreated> result,
    ServiceAccount serviceAccount,
    String authToken,
    String signingSecret
) {
    public boolean isSuccess() {
        return result instanceof Result.Success;
    }

    public boolean isFailure() {
        return result instanceof Result.Failure;
    }
}
