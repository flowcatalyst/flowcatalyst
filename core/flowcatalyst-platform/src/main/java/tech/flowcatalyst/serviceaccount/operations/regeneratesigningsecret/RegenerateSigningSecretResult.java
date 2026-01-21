package tech.flowcatalyst.serviceaccount.operations.regeneratesigningsecret;

import tech.flowcatalyst.platform.common.Result;

/**
 * Result of regenerating a signing secret.
 *
 * @param result        The operation result (success/failure)
 * @param signingSecret The new signing secret (null on failure) - shown only once
 */
public record RegenerateSigningSecretResult(
    Result<SigningSecretRegenerated> result,
    String signingSecret
) {
    public boolean isSuccess() {
        return result instanceof Result.Success;
    }
}
