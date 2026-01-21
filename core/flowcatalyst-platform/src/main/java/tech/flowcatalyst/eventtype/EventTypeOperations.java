package tech.flowcatalyst.eventtype;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.eventtype.events.*;
import tech.flowcatalyst.eventtype.operations.addschema.AddSchemaCommand;
import tech.flowcatalyst.eventtype.operations.addschema.AddSchemaUseCase;
import tech.flowcatalyst.eventtype.operations.archiveeventtype.ArchiveEventTypeCommand;
import tech.flowcatalyst.eventtype.operations.archiveeventtype.ArchiveEventTypeUseCase;
import tech.flowcatalyst.eventtype.operations.createeventtype.CreateEventTypeCommand;
import tech.flowcatalyst.eventtype.operations.createeventtype.CreateEventTypeUseCase;
import tech.flowcatalyst.eventtype.operations.deleteeventtype.DeleteEventTypeCommand;
import tech.flowcatalyst.eventtype.operations.deleteeventtype.DeleteEventTypeUseCase;
import tech.flowcatalyst.eventtype.operations.deprecateschema.DeprecateSchemaCommand;
import tech.flowcatalyst.eventtype.operations.deprecateschema.DeprecateSchemaUseCase;
import tech.flowcatalyst.eventtype.operations.finaliseschema.FinaliseSchemaCommand;
import tech.flowcatalyst.eventtype.operations.finaliseschema.FinaliseSchemaUseCase;
import tech.flowcatalyst.eventtype.operations.updateeventtype.UpdateEventTypeCommand;
import tech.flowcatalyst.eventtype.operations.updateeventtype.UpdateEventTypeUseCase;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;

import java.util.List;
import java.util.Optional;

/**
 * EventTypeOperations - Single point of discovery for EventType aggregate.
 *
 * <p>All write operations on EventTypes go through this service. This provides:
 * <ul>
 *   <li>A single entry point for all EventType mutations</li>
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
public class EventTypeOperations {

    // ========================================================================
    // Write Operations (Use Cases)
    // ========================================================================

    @Inject
    CreateEventTypeUseCase createEventTypeUseCase;

    @Inject
    UpdateEventTypeUseCase updateEventTypeUseCase;

    @Inject
    AddSchemaUseCase addSchemaUseCase;

    @Inject
    FinaliseSchemaUseCase finaliseSchemaUseCase;

    @Inject
    DeprecateSchemaUseCase deprecateSchemaUseCase;

    @Inject
    ArchiveEventTypeUseCase archiveEventTypeUseCase;

    @Inject
    DeleteEventTypeUseCase deleteEventTypeUseCase;

    /**
     * Create a new EventType.
     *
     * @param command The command containing event type details
     * @param context The execution context
     * @return Success with EventTypeCreated, or Failure with error
     */
    public Result<EventTypeCreated> createEventType(
            CreateEventTypeCommand command,
            ExecutionContext context
    ) {
        return createEventTypeUseCase.execute(command, context);
    }

    /**
     * Update an EventType's metadata.
     *
     * @param command The command containing update details
     * @param context The execution context
     * @return Success with EventTypeUpdated, or Failure with error
     */
    public Result<EventTypeUpdated> updateEventType(
            UpdateEventTypeCommand command,
            ExecutionContext context
    ) {
        return updateEventTypeUseCase.execute(command, context);
    }

    /**
     * Add a new schema version to an EventType.
     *
     * @param command The command containing schema details
     * @param context The execution context
     * @return Success with SchemaAdded, or Failure with error
     */
    public Result<SchemaAdded> addSchema(
            AddSchemaCommand command,
            ExecutionContext context
    ) {
        return addSchemaUseCase.execute(command, context);
    }

    /**
     * Finalise a schema version.
     *
     * @param command The command identifying the schema to finalise
     * @param context The execution context
     * @return Success with SchemaFinalised, or Failure with error
     */
    public Result<SchemaFinalised> finaliseSchema(
            FinaliseSchemaCommand command,
            ExecutionContext context
    ) {
        return finaliseSchemaUseCase.execute(command, context);
    }

    /**
     * Deprecate a schema version.
     *
     * @param command The command identifying the schema to deprecate
     * @param context The execution context
     * @return Success with SchemaDeprecated, or Failure with error
     */
    public Result<SchemaDeprecated> deprecateSchema(
            DeprecateSchemaCommand command,
            ExecutionContext context
    ) {
        return deprecateSchemaUseCase.execute(command, context);
    }

    /**
     * Archive an EventType.
     *
     * @param command The command identifying the event type to archive
     * @param context The execution context
     * @return Success with EventTypeArchived, or Failure with error
     */
    public Result<EventTypeArchived> archiveEventType(
            ArchiveEventTypeCommand command,
            ExecutionContext context
    ) {
        return archiveEventTypeUseCase.execute(command, context);
    }

    /**
     * Delete an EventType.
     *
     * @param command The command identifying the event type to delete
     * @param context The execution context
     * @return Success with EventTypeDeleted, or Failure with error
     */
    public Result<EventTypeDeleted> deleteEventType(
            DeleteEventTypeCommand command,
            ExecutionContext context
    ) {
        return deleteEventTypeUseCase.execute(command, context);
    }

    // ========================================================================
    // Read Operations (Queries)
    // ========================================================================

    @Inject
    EventTypeRepository repo;

    /**
     * Find an EventType by ID.
     */
    public Optional<EventType> findById(String id) {
        return repo.findByIdOptional(id);
    }

    /**
     * Find an EventType by code.
     */
    public Optional<EventType> findByCode(String code) {
        return repo.findByCode(code);
    }

    /**
     * Find all EventTypes ordered by code.
     */
    public List<EventType> findAll() {
        return repo.findAllOrdered();
    }

    /**
     * Find all current (non-archived) EventTypes.
     */
    public List<EventType> findCurrent() {
        return repo.findCurrent();
    }

    /**
     * Find all archived EventTypes.
     */
    public List<EventType> findArchived() {
        return repo.findArchived();
    }

    /**
     * Find EventTypes by code prefix.
     */
    public List<EventType> findByCodePrefix(String prefix) {
        return repo.findByCodePrefix(prefix);
    }

    /**
     * Get distinct application names.
     */
    public List<String> getDistinctApplications() {
        return repo.findDistinctApplications();
    }

    /**
     * Get distinct subdomains for an application.
     */
    public List<String> getDistinctSubdomains(String application) {
        return repo.findDistinctSubdomains(application);
    }

    /**
     * Get distinct subdomains for multiple applications.
     */
    public List<String> getDistinctSubdomains(List<String> applications) {
        return repo.findDistinctSubdomains(applications);
    }

    /**
     * Get distinct aggregates for an application and subdomain.
     */
    public List<String> getDistinctAggregates(String application, String subdomain) {
        return repo.findDistinctAggregates(application, subdomain);
    }

    /**
     * Get distinct aggregates for multiple applications and subdomains.
     */
    public List<String> getDistinctAggregates(List<String> applications, List<String> subdomains) {
        return repo.findDistinctAggregates(applications, subdomains);
    }

    /**
     * Find EventTypes with filters.
     */
    public List<EventType> findWithFilters(
            List<String> applications,
            List<String> subdomains,
            List<String> aggregates,
            EventTypeStatus status
    ) {
        return repo.findWithFilters(applications, subdomains, aggregates, status);
    }
}
