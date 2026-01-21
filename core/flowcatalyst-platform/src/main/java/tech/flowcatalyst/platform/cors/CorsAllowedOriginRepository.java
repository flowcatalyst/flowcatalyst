package tech.flowcatalyst.platform.cors;

import java.util.List;
import java.util.Optional;

/**
 * Repository interface for CorsAllowedOrigin entities.
 */
public interface CorsAllowedOriginRepository {

    Optional<CorsAllowedOrigin> findById(String id);
    Optional<CorsAllowedOrigin> findByOrigin(String origin);
    List<CorsAllowedOrigin> listAll();
    boolean existsByOrigin(String origin);

    void persist(CorsAllowedOrigin origin);
    void delete(CorsAllowedOrigin origin);
}
