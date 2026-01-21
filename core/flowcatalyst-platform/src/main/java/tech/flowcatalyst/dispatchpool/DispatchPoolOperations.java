package tech.flowcatalyst.dispatchpool;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.dispatchpool.events.DispatchPoolCreated;
import tech.flowcatalyst.dispatchpool.events.DispatchPoolDeleted;
import tech.flowcatalyst.dispatchpool.events.DispatchPoolUpdated;
import tech.flowcatalyst.dispatchpool.operations.createpool.CreateDispatchPoolCommand;
import tech.flowcatalyst.dispatchpool.operations.createpool.CreateDispatchPoolUseCase;
import tech.flowcatalyst.dispatchpool.operations.deletepool.DeleteDispatchPoolCommand;
import tech.flowcatalyst.dispatchpool.operations.deletepool.DeleteDispatchPoolUseCase;
import tech.flowcatalyst.dispatchpool.operations.updatepool.UpdateDispatchPoolCommand;
import tech.flowcatalyst.dispatchpool.operations.updatepool.UpdateDispatchPoolUseCase;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;

import java.util.List;
import java.util.Optional;

/**
 * DispatchPoolOperations - Single point of discovery for DispatchPool aggregate.
 *
 * <p>All write operations on DispatchPools go through this service. This provides:
 * <ul>
 *   <li>A single entry point for all DispatchPool mutations</li>
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
public class DispatchPoolOperations {

    // ========================================================================
    // Write Operations (Use Cases)
    // ========================================================================

    @Inject
    CreateDispatchPoolUseCase createPoolUseCase;

    @Inject
    UpdateDispatchPoolUseCase updatePoolUseCase;

    @Inject
    DeleteDispatchPoolUseCase deletePoolUseCase;

    /**
     * Create a new DispatchPool.
     *
     * @param command The command containing pool details
     * @param context The execution context
     * @return Success with DispatchPoolCreated, or Failure with error
     */
    public Result<DispatchPoolCreated> createPool(
            CreateDispatchPoolCommand command,
            ExecutionContext context
    ) {
        return createPoolUseCase.execute(command, context);
    }

    /**
     * Update a DispatchPool's configuration.
     *
     * @param command The command containing update details
     * @param context The execution context
     * @return Success with DispatchPoolUpdated, or Failure with error
     */
    public Result<DispatchPoolUpdated> updatePool(
            UpdateDispatchPoolCommand command,
            ExecutionContext context
    ) {
        return updatePoolUseCase.execute(command, context);
    }

    /**
     * Delete (archive) a DispatchPool.
     *
     * @param command The command identifying the pool to delete
     * @param context The execution context
     * @return Success with DispatchPoolDeleted, or Failure with error
     */
    public Result<DispatchPoolDeleted> deletePool(
            DeleteDispatchPoolCommand command,
            ExecutionContext context
    ) {
        return deletePoolUseCase.execute(command, context);
    }

    // ========================================================================
    // Read Operations (Queries)
    // ========================================================================

    @Inject
    DispatchPoolRepository repo;

    /**
     * Find a DispatchPool by ID.
     */
    public Optional<DispatchPool> findById(String id) {
        return repo.findByIdOptional(id);
    }

    /**
     * Find a DispatchPool by code within a client scope.
     *
     * @param code The pool code
     * @param clientId The client ID (null for anchor-level pools)
     */
    public Optional<DispatchPool> findByCodeAndClientId(String code, String clientId) {
        return repo.findByCodeAndClientId(code, clientId);
    }

    /**
     * Find all DispatchPools.
     */
    public List<DispatchPool> findAll() {
        return repo.listAll();
    }

    /**
     * Find all non-archived DispatchPools.
     */
    public List<DispatchPool> findActive() {
        return repo.findActive();
    }

    /**
     * Find DispatchPools for a specific client.
     */
    public List<DispatchPool> findByClientId(String clientId) {
        return repo.findByClientId(clientId);
    }

    /**
     * Find anchor-level DispatchPools (clientId is null).
     */
    public List<DispatchPool> findAnchorLevel() {
        return repo.findAnchorLevel();
    }

    /**
     * Find DispatchPools with filters.
     *
     * @param clientId Filter by client (null to skip)
     * @param status Filter by status
     */
    public List<DispatchPool> findWithFilters(String clientId, DispatchPoolStatus status) {
        return repo.findWithFilters(clientId, status, false);
    }
}
