package tech.flowcatalyst.platform.authentication.domain;

import java.util.List;
import java.util.Optional;

/**
 * Repository interface for EmailDomainMapping entities.
 */
public interface EmailDomainMappingRepository {

    // Read operations
    Optional<EmailDomainMapping> findByIdOptional(String id);
    Optional<EmailDomainMapping> findByEmailDomain(String emailDomain);
    List<EmailDomainMapping> findByIdentityProviderId(String identityProviderId);
    List<EmailDomainMapping> findByScopeType(ScopeType scopeType);
    List<EmailDomainMapping> findByPrimaryClientId(String primaryClientId);
    List<EmailDomainMapping> listAll();
    boolean existsByEmailDomain(String emailDomain);

    /**
     * Check if an email domain is configured as an anchor domain (scopeType=ANCHOR).
     * This is the replacement for AnchorDomainRepository.existsByDomain().
     *
     * @param emailDomain the email domain to check
     * @return true if this domain is an anchor domain
     */
    boolean isAnchorDomain(String emailDomain);

    // Write operations
    void persist(EmailDomainMapping mapping);
    void update(EmailDomainMapping mapping);
    void delete(EmailDomainMapping mapping);
}
