package tech.flowcatalyst.platform.principal;

import java.time.Instant;

/**
 * Service account information (embedded in Principal for SERVICE type).
 *
 * <p>Service accounts can have multiple OAuth clients for authentication,
 * linked via {@link tech.flowcatalyst.platform.authentication.oauth.OAuthClient#serviceAccountPrincipalId}.
 *
 * <p>Legacy fields (clientId, clientSecretHash) are kept for backwards compatibility
 * but new service accounts should use separate OAuthClient entities.
 */
public class ServiceAccount {

    /**
     * Optional code/identifier for the service account.
     */
    public String code;

    /**
     * Human-readable description of what this service account is used for.
     */
    public String description;

    /**
     * @deprecated Use separate OAuthClient entities linked via serviceAccountPrincipalId.
     * Kept for backwards compatibility with existing service accounts.
     */
    @Deprecated
    public String clientId;

    /**
     * @deprecated Use separate OAuthClient entities with encrypted secrets.
     * Kept for backwards compatibility with existing service accounts.
     */
    @Deprecated
    public String clientSecretHash; // Argon2id hash

    /**
     * When this service account was last used for authentication.
     */
    public Instant lastUsedAt;

    public ServiceAccount() {
    }
}
