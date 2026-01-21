package tech.flowcatalyst.platform.principal;

import java.util.List;
import java.util.Optional;

/**
 * Repository interface for AnchorDomain entities.
 */
public interface AnchorDomainRepository {

    // Read operations
    Optional<AnchorDomain> findByIdOptional(String id);
    List<AnchorDomain> listAll();
    boolean existsByDomain(String domain);

    // Write operations
    void persist(AnchorDomain domain);
    void delete(AnchorDomain domain);
}
