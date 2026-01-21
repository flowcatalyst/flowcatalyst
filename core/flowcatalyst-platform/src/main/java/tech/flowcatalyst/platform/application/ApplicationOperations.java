package tech.flowcatalyst.platform.application;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.application.events.*;
import tech.flowcatalyst.platform.application.operations.activateapplication.ActivateApplicationCommand;
import tech.flowcatalyst.platform.application.operations.activateapplication.ActivateApplicationUseCase;
import tech.flowcatalyst.platform.application.operations.createapplication.CreateApplicationCommand;
import tech.flowcatalyst.platform.application.operations.createapplication.CreateApplicationUseCase;
import tech.flowcatalyst.platform.application.operations.deactivateapplication.DeactivateApplicationCommand;
import tech.flowcatalyst.platform.application.operations.deactivateapplication.DeactivateApplicationUseCase;
import tech.flowcatalyst.platform.application.operations.deleteapplication.DeleteApplicationCommand;
import tech.flowcatalyst.platform.application.operations.deleteapplication.DeleteApplicationUseCase;
import tech.flowcatalyst.platform.application.operations.updateapplication.UpdateApplicationCommand;
import tech.flowcatalyst.platform.application.operations.updateapplication.UpdateApplicationUseCase;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;

import java.util.List;
import java.util.Optional;

/**
 * ApplicationOperations - Single point of discovery for Application aggregate.
 *
 * <p>All write operations on Applications go through this service. This provides:
 * <ul>
 *   <li>A single entry point for all Application mutations</li>
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
public class ApplicationOperations {

    // ========================================================================
    // Write Operations (Use Cases)
    // ========================================================================

    @Inject
    CreateApplicationUseCase createApplicationUseCase;

    @Inject
    UpdateApplicationUseCase updateApplicationUseCase;

    @Inject
    ActivateApplicationUseCase activateApplicationUseCase;

    @Inject
    DeactivateApplicationUseCase deactivateApplicationUseCase;

    @Inject
    DeleteApplicationUseCase deleteApplicationUseCase;

    /**
     * Create a new Application.
     *
     * @param command The command containing application details
     * @param context The execution context
     * @return Success with ApplicationCreated, or Failure with error
     */
    public Result<ApplicationCreated> createApplication(
            CreateApplicationCommand command,
            ExecutionContext context
    ) {
        return createApplicationUseCase.execute(command, context);
    }

    /**
     * Update an Application's metadata.
     *
     * @param command The command containing update details
     * @param context The execution context
     * @return Success with ApplicationUpdated, or Failure with error
     */
    public Result<ApplicationUpdated> updateApplication(
            UpdateApplicationCommand command,
            ExecutionContext context
    ) {
        return updateApplicationUseCase.execute(command, context);
    }

    /**
     * Activate an Application.
     *
     * @param command The command identifying the application to activate
     * @param context The execution context
     * @return Success with ApplicationActivated, or Failure with error
     */
    public Result<ApplicationActivated> activateApplication(
            ActivateApplicationCommand command,
            ExecutionContext context
    ) {
        return activateApplicationUseCase.execute(command, context);
    }

    /**
     * Deactivate an Application.
     *
     * @param command The command identifying the application to deactivate
     * @param context The execution context
     * @return Success with ApplicationDeactivated, or Failure with error
     */
    public Result<ApplicationDeactivated> deactivateApplication(
            DeactivateApplicationCommand command,
            ExecutionContext context
    ) {
        return deactivateApplicationUseCase.execute(command, context);
    }

    /**
     * Delete an Application.
     *
     * @param command The command identifying the application to delete
     * @param context The execution context
     * @return Success with ApplicationDeleted, or Failure with error
     */
    public Result<ApplicationDeleted> deleteApplication(
            DeleteApplicationCommand command,
            ExecutionContext context
    ) {
        return deleteApplicationUseCase.execute(command, context);
    }

    // ========================================================================
    // Read Operations (Queries)
    // ========================================================================

    @Inject
    ApplicationRepository repo;

    /**
     * Find an Application by ID.
     */
    public Optional<Application> findById(String id) {
        return repo.findByIdOptional(id);
    }

    /**
     * Find an Application by code.
     */
    public Optional<Application> findByCode(String code) {
        return repo.findByCode(code);
    }

    /**
     * Find all Applications.
     */
    public List<Application> findAll() {
        return repo.listAll();
    }

    /**
     * Find all active Applications.
     */
    public List<Application> findAllActive() {
        return repo.findAllActive();
    }
}
