package tech.flowcatalyst.dispatchjob.service;

import io.quarkus.cache.CacheInvalidate;
import io.quarkus.cache.CacheResult;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import org.jboss.logging.Logger;
import tech.flowcatalyst.dispatchjob.entity.DispatchJob;
import tech.flowcatalyst.platform.security.secrets.SecretService;
import tech.flowcatalyst.serviceaccount.repository.ServiceAccountRepository;

import java.util.Optional;

/**
 * Service for resolving webhook credentials from ServiceAccounts.
 */
@ApplicationScoped
public class CredentialsService {

    private static final Logger LOG = Logger.getLogger(CredentialsService.class);
    private static final String SERVICE_ACCOUNT_CACHE = "service-account-credentials";

    @Inject
    ServiceAccountRepository serviceAccountRepository;

    @Inject
    SecretService secretService;

    /**
     * Resolved credentials for webhook signing.
     * Contains decrypted auth token and signing secret.
     */
    public record ResolvedCredentials(
        String authToken,
        String signingSecret
    ) {}

    /**
     * Resolve credentials for a dispatch job from its ServiceAccount.
     *
     * @param job The dispatch job to resolve credentials for
     * @return Resolved credentials, or empty if not found
     */
    public Optional<ResolvedCredentials> resolveCredentials(DispatchJob job) {
        if (job.serviceAccountId == null) {
            LOG.warnf("Dispatch job [%s] has no serviceAccountId", job.id);
            return Optional.empty();
        }
        return resolveFromServiceAccount(job.serviceAccountId);
    }

    /**
     * Validate that a service account exists and has valid webhook credentials.
     *
     * @param serviceAccountId The service account ID to validate
     * @return true if the service account exists, is active, and has webhook credentials
     */
    public boolean validateServiceAccount(String serviceAccountId) {
        if (serviceAccountId == null) {
            return false;
        }
        return serviceAccountRepository.findByIdOptional(serviceAccountId)
            .filter(sa -> sa.active && sa.webhookCredentials != null)
            .isPresent();
    }

    /**
     * Resolve credentials from a ServiceAccount, decrypting the stored values.
     *
     * @param serviceAccountId The service account ID
     * @return Resolved credentials with decrypted values, or empty if not found
     */
    @CacheResult(cacheName = SERVICE_ACCOUNT_CACHE)
    public Optional<ResolvedCredentials> resolveFromServiceAccount(String serviceAccountId) {
        LOG.debugf("Loading service account credentials [%s] from database (cache miss)", serviceAccountId);

        return serviceAccountRepository.findByIdOptional(serviceAccountId)
            .filter(sa -> sa.active && sa.webhookCredentials != null)
            .map(sa -> {
                // Decrypt the credentials
                String authToken = secretService.resolve(sa.webhookCredentials.authTokenRef);
                String signingSecret = secretService.resolve(sa.webhookCredentials.signingSecretRef);
                return new ResolvedCredentials(authToken, signingSecret);
            });
    }

    /**
     * Invalidate service account credentials cache.
     */
    @CacheInvalidate(cacheName = SERVICE_ACCOUNT_CACHE)
    public void invalidateServiceAccountCache(String serviceAccountId) {
        LOG.debugf("Invalidated cache for service account credentials [%s]", serviceAccountId);
    }
}
