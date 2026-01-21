package tech.flowcatalyst.platform.cors;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.cors.events.CorsOriginAdded;
import tech.flowcatalyst.platform.cors.events.CorsOriginDeleted;
import tech.flowcatalyst.platform.cors.operations.addorigin.AddCorsOriginCommand;
import tech.flowcatalyst.platform.cors.operations.addorigin.AddCorsOriginUseCase;
import tech.flowcatalyst.platform.cors.operations.deleteorigin.DeleteCorsOriginCommand;
import tech.flowcatalyst.platform.cors.operations.deleteorigin.DeleteCorsOriginUseCase;
import tech.flowcatalyst.platform.cors.CorsAllowedOriginRepository;

import java.util.List;
import java.util.Optional;
import java.util.Set;

/**
 * CorsOperations - Single point of discovery for CORS origin management.
 *
 * <p>All write operations on CORS origins go through this service. This provides:
 * <ul>
 *   <li>A single entry point for all CORS origin mutations</li>
 *   <li>Consistent execution context handling</li>
 *   <li>Clear documentation of available operations</li>
 * </ul>
 *
 * <p>Each operation:
 * <ul>
 *   <li>Takes a command describing what to do</li>
 *   <li>Takes an execution context for tracing and principal info</li>
 *   <li>Returns a Result containing either the domain event or an error</li>
 *   <li>Atomically commits the entity, event, and audit log</li>
 * </ul>
 *
 * <p>Read operations do not require execution context and do not emit events.
 */
@ApplicationScoped
public class CorsOperations {

    // ========================================================================
    // Write Operations (Use Cases)
    // ========================================================================

    @Inject
    AddCorsOriginUseCase addCorsOriginUseCase;

    @Inject
    DeleteCorsOriginUseCase deleteCorsOriginUseCase;

    /**
     * Add a new CORS allowed origin.
     *
     * @param command The command containing origin details
     * @param context The execution context
     * @return Success with CorsOriginAdded, or Failure with error
     */
    public Result<CorsOriginAdded> addOrigin(AddCorsOriginCommand command, ExecutionContext context) {
        Result<CorsOriginAdded> result = addCorsOriginUseCase.execute(command, context);
        if (result.isSuccess()) {
            corsService.invalidateCache();
        }
        return result;
    }

    /**
     * Delete a CORS allowed origin.
     *
     * @param command The command identifying the origin to delete
     * @param context The execution context
     * @return Success with CorsOriginDeleted, or Failure with error
     */
    public Result<CorsOriginDeleted> deleteOrigin(DeleteCorsOriginCommand command, ExecutionContext context) {
        Result<CorsOriginDeleted> result = deleteCorsOriginUseCase.execute(command, context);
        if (result.isSuccess()) {
            corsService.invalidateCache();
        }
        return result;
    }

    // ========================================================================
    // Read Operations (Queries)
    // ========================================================================

    @Inject
    CorsAllowedOriginRepository repository;

    @Inject
    CorsService corsService;

    /**
     * Get all allowed origins (cached).
     */
    public Set<String> getAllowedOrigins() {
        return corsService.getAllowedOrigins();
    }

    /**
     * Check if an origin is allowed (cached).
     */
    public boolean isOriginAllowed(String origin) {
        return corsService.isOriginAllowed(origin);
    }

    /**
     * List all CORS entries.
     */
    public List<CorsAllowedOrigin> listAll() {
        return repository.listAll();
    }

    /**
     * Find a CORS entry by ID.
     */
    public Optional<CorsAllowedOrigin> findById(String id) {
        return repository.findById(id);
    }

    /**
     * Find a CORS entry by origin.
     */
    public Optional<CorsAllowedOrigin> findByOrigin(String origin) {
        return repository.findByOrigin(origin);
    }
}
