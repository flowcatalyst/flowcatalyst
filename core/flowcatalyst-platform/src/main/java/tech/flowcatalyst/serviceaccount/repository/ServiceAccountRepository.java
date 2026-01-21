package tech.flowcatalyst.serviceaccount.repository;

import tech.flowcatalyst.serviceaccount.entity.ServiceAccount;

import java.util.List;
import java.util.Optional;

/**
 * Repository interface for ServiceAccount entities.
 * Exposes only approved data access methods - Panache internals are hidden.
 */
public interface ServiceAccountRepository {

    // Read operations
    ServiceAccount findById(String id);
    Optional<ServiceAccount> findByIdOptional(String id);
    Optional<ServiceAccount> findByCode(String code);
    Optional<ServiceAccount> findByApplicationId(String applicationId);
    List<ServiceAccount> findByClientId(String clientId);
    List<ServiceAccount> findActive();
    List<ServiceAccount> findWithFilter(ServiceAccountFilter filter);
    List<ServiceAccount> listAll();
    long count();
    long countWithFilter(ServiceAccountFilter filter);

    // Write operations
    void persist(ServiceAccount serviceAccount);
    void update(ServiceAccount serviceAccount);
    void delete(ServiceAccount serviceAccount);
    boolean deleteById(String id);
}
