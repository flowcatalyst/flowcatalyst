package tech.flowcatalyst.event;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.ObjectMapper;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import org.eclipse.microprofile.config.inject.ConfigProperty;
import org.jboss.logging.Logger;
import tech.flowcatalyst.dispatch.DispatchMode;
import tech.flowcatalyst.dispatchjob.entity.DispatchJob;
import tech.flowcatalyst.dispatchjob.model.DispatchKind;
import tech.flowcatalyst.dispatchjob.model.DispatchStatus;
import tech.flowcatalyst.dispatchjob.model.MediationType;
import tech.flowcatalyst.dispatchjob.model.MessagePointer;
import tech.flowcatalyst.dispatchjob.queue.DispatchQueue;
import tech.flowcatalyst.dispatchjob.queue.DispatchQueueConfig;
import tech.flowcatalyst.dispatchjob.repository.DispatchJobRepository;
import tech.flowcatalyst.dispatchjob.security.DispatchAuthService;
import tech.flowcatalyst.platform.shared.EntityType;
import tech.flowcatalyst.platform.shared.TsidGenerator;
import tech.flowcatalyst.queue.QueueMessage;
import tech.flowcatalyst.queue.QueuePublishResult;
import tech.flowcatalyst.queue.QueuePublisher;
import tech.flowcatalyst.subscription.SubscriptionCache;
import tech.flowcatalyst.subscription.SubscriptionCache.CachedSubscription;

import java.time.Instant;
import java.util.ArrayList;
import java.util.HashMap;
import java.util.HashSet;
import java.util.List;
import java.util.Map;
import java.util.Set;

/**
 * Service for creating dispatch jobs from events.
 *
 * <p>This service orchestrates the synchronous flow of:</p>
 * <ol>
 *   <li>Looking up matching subscriptions (via cache)</li>
 *   <li>Creating dispatch jobs with status QUEUED</li>
 *   <li>Persisting jobs to MongoDB</li>
 *   <li>Sending message pointers to the queue</li>
 *   <li>Handling queue failures by setting status to PENDING</li>
 * </ol>
 *
 * <p>This service is optimized for batch operations to minimize database
 * round-trips and cache lookups.</p>
 */
@ApplicationScoped
public class EventDispatchService {

    private static final Logger LOG = Logger.getLogger(EventDispatchService.class);
    private static final MediationType MEDIATION_TYPE = MediationType.HTTP;

    @Inject
    SubscriptionCache subscriptionCache;

    @Inject
    DispatchJobRepository dispatchJobRepository;

    @Inject
    DispatchAuthService dispatchAuthService;

    @Inject
    @DispatchQueue
    QueuePublisher queuePublisher;

    @Inject
    DispatchQueueConfig dispatchQueueConfig;

    @Inject
    ObjectMapper objectMapper;

    @ConfigProperty(name = "flowcatalyst.features.messaging-enabled", defaultValue = "true")
    boolean messagingEnabled;

    /**
     * Create dispatch jobs for a single event and queue them.
     *
     * @param event The event to create dispatch jobs for
     * @return List of created dispatch jobs (may be empty if no subscriptions match)
     */
    public List<DispatchJob> createDispatchJobsForEvent(Event event) {
        return createDispatchJobsForEvents(List.of(event));
    }

    /**
     * Create dispatch jobs for multiple events and queue them.
     *
     * <p>This method is optimized for batch operations:</p>
     * <ul>
     *   <li>Groups events by (eventTypeCode, clientId) to minimize cache lookups</li>
     *   <li>Bulk inserts dispatch jobs</li>
     *   <li>Batch sends message pointers to the queue</li>
     * </ul>
     *
     * @param events The events to create dispatch jobs for
     * @return List of all created dispatch jobs
     * @deprecated Use {@link #buildDispatchJobsForEvents(List)} + BatchEventWriter + {@link #queueDispatchJobs(List)} instead
     */
    @Deprecated
    public List<DispatchJob> createDispatchJobsForEvents(List<Event> events) {
        List<DispatchJob> allJobs = buildDispatchJobsForEvents(events);

        if (allJobs.isEmpty()) {
            return allJobs;
        }

        // Bulk insert dispatch jobs with status QUEUED
        dispatchJobRepository.persistAll(allJobs);
        LOG.infof("Persisted %d dispatch jobs", allJobs.size());

        // Queue and handle failures
        queueDispatchJobs(allJobs);

        return allJobs;
    }

    /**
     * Build dispatch jobs for events WITHOUT persisting.
     *
     * <p>Use this with {@link tech.flowcatalyst.platform.batch.BatchEventWriter}
     * to write events and dispatch jobs atomically.</p>
     *
     * @param events The events to create dispatch jobs for
     * @return List of dispatch job objects (not yet persisted)
     */
    public List<DispatchJob> buildDispatchJobsForEvents(List<Event> events) {
        if (!messagingEnabled) {
            LOG.debug("Messaging disabled - skipping event dispatch");
            return List.of();
        }

        if (events == null || events.isEmpty()) {
            return List.of();
        }

        List<DispatchJob> allJobs = new ArrayList<>();

        // Group events by (eventTypeCode, clientId) to minimize cache lookups
        Map<String, List<Event>> eventsByTypeAndClient = groupEventsByTypeAndClient(events);

        for (var entry : eventsByTypeAndClient.entrySet()) {
            String[] parts = entry.getKey().split(":", 2);
            String eventTypeCode = parts[0];
            String clientId = parts.length > 1 && !parts[1].equals("anchor") ? parts[1] : null;
            List<Event> eventsInGroup = entry.getValue();

            // Single cache lookup per (eventTypeCode, clientId)
            List<CachedSubscription> subscriptions = subscriptionCache.getByEventTypeCode(eventTypeCode, clientId);

            if (subscriptions.isEmpty()) {
                LOG.debugf("No subscriptions found for eventTypeCode=%s, clientId=%s", eventTypeCode, clientId);
                continue;
            }

            // Create dispatch jobs for each event × subscription combination
            for (Event event : eventsInGroup) {
                for (CachedSubscription sub : subscriptions) {
                    DispatchJob job = createDispatchJob(event, sub);
                    allJobs.add(job);
                }
            }
        }

        if (allJobs.isEmpty()) {
            LOG.debug("No dispatch jobs created - no matching subscriptions");
        }

        return allJobs;
    }

    /**
     * Queue already-persisted dispatch jobs and handle failures.
     *
     * @param jobs The persisted dispatch jobs to queue
     */
    public void queueDispatchJobs(List<DispatchJob> jobs) {
        if (jobs.isEmpty()) {
            return;
        }

        Set<String> failedJobIds = publishBatch(jobs);

        // Handle failures: update failed jobs to PENDING
        if (!failedJobIds.isEmpty()) {
            LOG.warnf("Queue send failed for %d jobs, updating to PENDING status", failedJobIds.size());
            dispatchJobRepository.updateStatusBatch(new ArrayList<>(failedJobIds), DispatchStatus.PENDING);
        }
    }

    /**
     * Group events by their type code and client ID for efficient cache lookups.
     */
    private Map<String, List<Event>> groupEventsByTypeAndClient(List<Event> events) {
        Map<String, List<Event>> grouped = new HashMap<>();
        for (Event event : events) {
            // Extract clientId from event context data or use null for anchor-level
            String clientId = extractClientIdFromEvent(event);
            String key = event.type() + ":" + (clientId != null ? clientId : "anchor");
            grouped.computeIfAbsent(key, k -> new ArrayList<>()).add(event);
        }
        return grouped;
    }

    /**
     * Extract client ID from event context data if present.
     */
    private String extractClientIdFromEvent(Event event) {
        if (event.contextData() == null) {
            return null;
        }
        return event.contextData().stream()
            .filter(cd -> "clientId".equals(cd.key()))
            .map(ContextData::value)
            .findFirst()
            .orElse(null);
    }

    /**
     * Create a dispatch job from an event and subscription.
     */
    private DispatchJob createDispatchJob(Event event, CachedSubscription sub) {
        Instant now = Instant.now();

        DispatchJob job = new DispatchJob();
        job.id = TsidGenerator.generate(EntityType.DISPATCH_JOB);
        job.kind = DispatchKind.EVENT;
        job.code = event.type();
        job.source = event.source();
        job.subject = event.subject();
        job.eventId = event.id();
        job.correlationId = event.correlationId();
        job.targetUrl = sub.target();
        job.payload = formatPayload(event, sub);
        job.payloadContentType = "application/json";
        job.serviceAccountId = sub.serviceAccountId();
        job.clientId = sub.clientId();
        job.subscriptionId = sub.id();
        job.idempotencyKey = event.id() + ":" + sub.id();
        job.dispatchPoolId = sub.dispatchPoolId();
        job.mode = sub.mode();
        job.messageGroup = computeMessageGroup(sub.code(), event.messageGroup());
        job.sequence = sub.sequence();
        job.timeoutSeconds = sub.timeoutSeconds();
        job.dataOnly = sub.dataOnly();
        job.maxRetries = sub.maxRetries();
        job.status = DispatchStatus.QUEUED;
        job.attemptCount = 0;
        job.createdAt = now;
        job.updatedAt = now;

        // Apply delay if configured
        if (sub.delaySeconds() > 0) {
            job.scheduledFor = now.plusSeconds(sub.delaySeconds());
        }

        // Apply expiry if configured
        if (sub.maxAgeSeconds() > 0) {
            job.expiresAt = now.plusSeconds(sub.maxAgeSeconds());
        }

        return job;
    }

    /**
     * Format the payload based on dataOnly setting.
     */
    private String formatPayload(Event event, CachedSubscription sub) {
        if (sub.dataOnly()) {
            // Return raw event data
            return event.data();
        }

        // Wrap in envelope
        try {
            var envelope = Map.of(
                "id", event.id(),
                "type", event.type(),
                "source", event.source(),
                "subject", event.subject(),
                "time", event.time().toString(),
                "data", event.data() != null ? objectMapper.readTree(event.data()) : null
            );
            return objectMapper.writeValueAsString(envelope);
        } catch (JsonProcessingException e) {
            LOG.warnf("Failed to create envelope for event %s, using raw data", event.id());
            return event.data();
        }
    }

    /**
     * Compute message group for FIFO ordering.
     *
     * <p>Format: "{subscriptionCode}:{eventMessageGroup}"</p>
     */
    private String computeMessageGroup(String subscriptionCode, String eventMessageGroup) {
        if (eventMessageGroup == null || eventMessageGroup.isBlank()) {
            return subscriptionCode;
        }
        return subscriptionCode + ":" + eventMessageGroup;
    }

    /**
     * Publish dispatch jobs to the queue in batch.
     *
     * @param jobs The dispatch jobs to queue
     * @return Set of job IDs that failed to queue
     */
    private Set<String> publishBatch(List<DispatchJob> jobs) {
        try {
            List<QueueMessage> messages = jobs.stream()
                .map(this::toQueueMessage)
                .toList();

            QueuePublishResult result = queuePublisher.publishBatch(messages);

            return new HashSet<>(result.failedMessageIds());

        } catch (Exception e) {
            LOG.errorf(e, "Failed to publish batch of %d dispatch jobs to queue", jobs.size());
            var failedIds = new HashSet<String>();
            jobs.forEach(j -> failedIds.add(j.id));
            return failedIds;
        }
    }

    /**
     * Convert a dispatch job to a QueueMessage with a serialized MessagePointer body.
     */
    private QueueMessage toQueueMessage(DispatchJob job) {
        try {
            var authToken = dispatchAuthService.generateAuthToken(job.id);
            var pointer = new MessagePointer(
                job.id,
                job.dispatchPoolId != null ? job.dispatchPoolId : dispatchQueueConfig.defaultPoolCode(),
                authToken,
                MEDIATION_TYPE,
                dispatchQueueConfig.processingEndpoint(),
                job.messageGroup,
                null
            );

            var messageBody = objectMapper.writeValueAsString(pointer);
            return new QueueMessage(job.id, job.messageGroup, job.id, messageBody);

        } catch (JsonProcessingException e) {
            throw new RuntimeException("Failed to serialize MessagePointer for job " + job.id, e);
        }
    }

    /**
     * Result of creating dispatch jobs for events.
     */
    public record EventDispatchResult(
        List<DispatchJob> dispatchJobs,
        int queuedCount,
        int pendingCount
    ) {
        public static EventDispatchResult of(List<DispatchJob> jobs) {
            int queued = (int) jobs.stream()
                .filter(j -> j.status == DispatchStatus.QUEUED)
                .count();
            return new EventDispatchResult(jobs, queued, jobs.size() - queued);
        }
    }
}
