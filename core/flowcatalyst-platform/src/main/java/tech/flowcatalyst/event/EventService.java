package tech.flowcatalyst.event;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.persistence.PersistenceException;
import jakarta.ws.rs.BadRequestException;
import org.hibernate.exception.ConstraintViolationException;
import org.jboss.logging.Logger;
import tech.flowcatalyst.dispatchjob.entity.DispatchJob;
import tech.flowcatalyst.event.operations.CreateEvent;
import tech.flowcatalyst.platform.batch.BatchEventWriter;
import tech.flowcatalyst.platform.shared.TsidGenerator;

import java.time.Instant;
import java.util.ArrayList;
import java.util.List;
import java.util.Optional;

/**
 * Service for Event operations.
 *
 * <p>Handles event creation and validation. Events are stored in MongoDB
 * for high-volume write performance, with later projection to PostgreSQL
 * for efficient querying.</p>
 *
 * <p>After event creation, dispatch jobs are automatically created for
 * matching subscriptions via {@link EventDispatchService}.</p>
 */
@ApplicationScoped
public class EventService {

    private static final Logger LOG = Logger.getLogger(EventService.class);

    @Inject
    EventRepository eventRepository;

    @Inject
    EventDispatchService eventDispatchService;

    @Inject
    BatchEventWriter batchEventWriter;

    /**
     * Create a new event and dispatch jobs for matching subscriptions.
     *
     * @param operation The create event operation
     * @return The result containing the event and created dispatch jobs
     * @throws BadRequestException if validation fails or duplicate deduplication ID
     */
    public EventCreateResult create(CreateEvent operation) {
        // Validate required fields
        validateCreateEvent(operation);

        // Check for duplicate deduplication ID
        if (operation.deduplicationId() != null) {
            Optional<Event> existing = eventRepository.findByDeduplicationId(operation.deduplicationId());
            if (existing.isPresent()) {
                // Return the existing event for idempotency (no new dispatch jobs)
                return new EventCreateResult(existing.get(), List.of(), true);
            }
        }

        // Create the event
        Event event = new Event(
            TsidGenerator.generate(),
            operation.specVersion(),
            operation.type(),
            operation.source(),
            operation.subject(),
            operation.time() != null ? operation.time() : Instant.now(),
            operation.data(),
            operation.correlationId(),
            operation.causationId(),
            operation.deduplicationId(),
            operation.messageGroup(),
            operation.contextData()
        );

        try {
            eventRepository.insert(event);
        } catch (PersistenceException e) {
            // Handle duplicate key error for deduplicationId
            if (e.getCause() instanceof ConstraintViolationException && operation.deduplicationId() != null) {
                // Race condition - another request created the event
                Event existingEvent = eventRepository.findByDeduplicationId(operation.deduplicationId())
                    .orElseThrow(() -> new RuntimeException("Unexpected state: duplicate key but event not found"));
                return new EventCreateResult(existingEvent, List.of(), true);
            }
            throw e;
        }

        // Create dispatch jobs for matching subscriptions
        List<DispatchJob> dispatchJobs = eventDispatchService.createDispatchJobsForEvent(event);

        LOG.infof("Created event [%s] type=%s with %d dispatch jobs", event.id(), event.type(), dispatchJobs.size());

        return new EventCreateResult(event, dispatchJobs, false);
    }

    /**
     * Create multiple events and dispatch jobs in batch.
     *
     * <p>This method is optimized for batch operations:</p>
     * <ul>
     *   <li>Handles deduplication for each event</li>
     *   <li>Builds dispatch jobs for matching subscriptions</li>
     *   <li>Writes everything atomically via BatchEventWriter (single transaction)</li>
     *   <li>Queues dispatch jobs after persistence</li>
     * </ul>
     *
     * @param operations The create event operations
     * @return The result containing all events and dispatch jobs
     */
    public BatchEventCreateResult createBatch(List<CreateEvent> operations) {
        if (operations == null || operations.isEmpty()) {
            return new BatchEventCreateResult(List.of(), List.of(), 0);
        }

        List<Event> allEvents = new ArrayList<>();
        List<Event> newEvents = new ArrayList<>();
        int duplicateCount = 0;

        for (CreateEvent op : operations) {
            validateCreateEvent(op);

            // Check deduplication
            if (op.deduplicationId() != null) {
                Optional<Event> existing = eventRepository.findByDeduplicationId(op.deduplicationId());
                if (existing.isPresent()) {
                    allEvents.add(existing.get());
                    duplicateCount++;
                    continue;
                }
            }

            Event event = new Event(
                TsidGenerator.generate(),
                op.specVersion(),
                op.type(),
                op.source(),
                op.subject(),
                op.time() != null ? op.time() : Instant.now(),
                op.data(),
                op.correlationId(),
                op.causationId(),
                op.deduplicationId(),
                op.messageGroup(),
                op.contextData()
            );
            // clientId can be extracted from contextData if needed

            newEvents.add(event);
            allEvents.add(event);
        }

        List<DispatchJob> dispatchJobs = List.of();

        if (!newEvents.isEmpty()) {
            // Build dispatch jobs for matching subscriptions (no persistence yet)
            dispatchJobs = eventDispatchService.buildDispatchJobsForEvents(newEvents);

            // Write everything atomically: events + outbox + dispatch_jobs + metadata + headers + outbox
            batchEventWriter.writeBatch(newEvents, dispatchJobs);
            LOG.infof("Batch wrote %d events and %d dispatch jobs", newEvents.size(), dispatchJobs.size());

            // Queue dispatch jobs (after they're persisted)
            eventDispatchService.queueDispatchJobs(dispatchJobs);
        }

        LOG.infof("Batch created %d events (%d new, %d duplicates) with %d dispatch jobs",
            allEvents.size(), newEvents.size(), duplicateCount, dispatchJobs.size());

        return new BatchEventCreateResult(allEvents, dispatchJobs, duplicateCount);
    }

    /**
     * Find an event by its ID.
     *
     * @param id The event ID
     * @return The event if found
     */
    public Optional<Event> findById(String id) {
        return eventRepository.findByIdOptional(id);
    }

    /**
     * Find an event by deduplication ID.
     *
     * @param deduplicationId The deduplication ID
     * @return The event if found
     */
    public Optional<Event> findByDeduplicationId(String deduplicationId) {
        return eventRepository.findByDeduplicationId(deduplicationId);
    }

    /**
     * Validate the create event operation.
     */
    private void validateCreateEvent(CreateEvent operation) {
        if (operation.specVersion() == null || operation.specVersion().isBlank()) {
            throw new BadRequestException("specVersion is required");
        }
        if (operation.type() == null || operation.type().isBlank()) {
            throw new BadRequestException("type is required");
        }
        if (operation.source() == null || operation.source().isBlank()) {
            throw new BadRequestException("source is required");
        }
        if (operation.subject() == null || operation.subject().isBlank()) {
            throw new BadRequestException("subject is required");
        }
        if (operation.messageGroup() == null || operation.messageGroup().isBlank()) {
            throw new BadRequestException("messageGroup is required");
        }
    }

    // =========================================================================
    // Result Records
    // =========================================================================

    /**
     * Result of creating a single event.
     *
     * @param event The created event
     * @param dispatchJobs The dispatch jobs created for matching subscriptions
     * @param isDuplicate True if this was a deduplicated request (event already existed)
     */
    public record EventCreateResult(
        Event event,
        List<DispatchJob> dispatchJobs,
        boolean isDuplicate
    ) {}

    /**
     * Result of creating multiple events in batch.
     *
     * @param events All events (new and deduplicated)
     * @param dispatchJobs All dispatch jobs created
     * @param duplicateCount Number of events that were deduplicated
     */
    public record BatchEventCreateResult(
        List<Event> events,
        List<DispatchJob> dispatchJobs,
        int duplicateCount
    ) {}
}
