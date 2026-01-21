package tech.flowcatalyst.subscription;

import java.util.List;
import java.util.Optional;

/**
 * Repository interface for Subscription entities.
 * Exposes only approved data access methods - Panache internals are hidden.
 */
public interface SubscriptionRepository {

    // Read operations
    Subscription findById(String id);
    Optional<Subscription> findByIdOptional(String id);
    Optional<Subscription> findByCodeAndClient(String code, String clientId);
    List<Subscription> findByClientId(String clientId);
    List<Subscription> findAnchorLevel();
    List<Subscription> findByDispatchPoolId(String dispatchPoolId);
    List<Subscription> findByEventTypeId(String eventTypeId);
    List<Subscription> findByStatus(SubscriptionStatus status);
    List<Subscription> findActive();
    List<Subscription> findWithFilters(String clientId, SubscriptionStatus status, SubscriptionSource source, String dispatchPoolId);
    List<Subscription> findActiveByEventTypeAndClient(String eventTypeId, String clientId);
    List<Subscription> findActiveByEventTypeCodeAndClient(String eventTypeCode, String clientId);
    List<Subscription> listAll();
    long count();
    boolean existsByCodeAndClient(String code, String clientId);
    boolean existsByDispatchPoolId(String dispatchPoolId);

    // Write operations
    void persist(Subscription subscription);
    void update(Subscription subscription);
    void delete(Subscription subscription);
    boolean deleteById(String id);
}
