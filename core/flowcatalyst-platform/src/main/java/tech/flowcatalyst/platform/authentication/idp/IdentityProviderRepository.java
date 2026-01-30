package tech.flowcatalyst.platform.authentication.idp;

import java.util.List;
import java.util.Optional;

/**
 * Repository interface for IdentityProvider entities.
 */
public interface IdentityProviderRepository {

    // Read operations
    Optional<IdentityProvider> findByIdOptional(String id);
    Optional<IdentityProvider> findByCode(String code);
    List<IdentityProvider> findByType(IdentityProviderType type);
    List<IdentityProvider> listAll();
    boolean existsByCode(String code);

    // Write operations
    void persist(IdentityProvider idp);
    void update(IdentityProvider idp);
    void delete(IdentityProvider idp);
}
