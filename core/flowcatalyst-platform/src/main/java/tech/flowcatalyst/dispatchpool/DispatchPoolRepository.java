package tech.flowcatalyst.dispatchpool;

import java.util.List;
import java.util.Optional;

/**
 * Repository interface for DispatchPool entities.
 * Exposes only approved data access methods - Panache internals are hidden.
 */
public interface DispatchPoolRepository {

    // Read operations
    DispatchPool findById(String id);
    Optional<DispatchPool> findByIdOptional(String id);
    Optional<DispatchPool> findByCodeAndClientId(String code, String clientId);
    List<DispatchPool> findByClientId(String clientId);
    List<DispatchPool> findAnchorLevel();
    List<DispatchPool> findByStatus(DispatchPoolStatus status);
    List<DispatchPool> findActive();
    List<DispatchPool> findAllNonArchived();
    List<DispatchPool> findWithFilters(String clientId, DispatchPoolStatus status, boolean includeArchived);
    List<DispatchPool> listAll();
    long count();
    boolean existsByCodeAndClientId(String code, String clientId);

    // Write operations
    void persist(DispatchPool pool);
    void update(DispatchPool pool);
    void delete(DispatchPool pool);
    boolean deleteById(String id);
}
