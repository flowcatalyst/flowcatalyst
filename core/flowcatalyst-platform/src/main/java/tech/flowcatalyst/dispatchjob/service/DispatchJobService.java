package tech.flowcatalyst.dispatchjob.service;

import com.fasterxml.jackson.core.JsonProcessingException;
import com.fasterxml.jackson.databind.ObjectMapper;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import org.jboss.logging.Logger;
import tech.flowcatalyst.dispatchjob.dto.CreateDispatchJobRequest;
import tech.flowcatalyst.dispatchjob.dto.DispatchJobFilter;
import tech.flowcatalyst.dispatchjob.entity.DispatchAttempt;
import tech.flowcatalyst.dispatchjob.entity.DispatchJob;
import tech.flowcatalyst.dispatchjob.service.CredentialsService.ResolvedCredentials;
import tech.flowcatalyst.dispatchjob.model.DispatchAttemptStatus;
import tech.flowcatalyst.dispatchjob.model.DispatchStatus;
import tech.flowcatalyst.dispatchjob.model.ErrorType;
import tech.flowcatalyst.dispatchjob.model.MediationType;
import tech.flowcatalyst.dispatchjob.model.MessagePointer;
import tech.flowcatalyst.dispatchjob.queue.DispatchQueue;
import tech.flowcatalyst.dispatchjob.queue.DispatchQueueConfig;
import tech.flowcatalyst.dispatchjob.repository.DispatchJobRepository;
import tech.flowcatalyst.dispatchjob.security.DispatchAuthService;
import tech.flowcatalyst.queue.QueueMessage;
import tech.flowcatalyst.queue.QueuePublishResult;
import tech.flowcatalyst.queue.QueuePublisher;

import java.time.Duration;
import java.time.Instant;
import java.util.ArrayList;
import java.util.HashSet;
import java.util.List;
import java.util.Optional;
import java.util.Set;

/**
 * Service for managing and processing dispatch jobs.
 */
@ApplicationScoped
public class DispatchJobService {

    private static final Logger LOG = Logger.getLogger(DispatchJobService.class);
    private static final MediationType MEDIATION_TYPE = MediationType.HTTP;

    @Inject
    DispatchJobRepository dispatchJobRepository;

    @Inject
    CredentialsService credentialsService;

    @Inject
    WebhookDispatcher webhookDispatcher;

    @Inject
    DispatchAuthService dispatchAuthService;

    @Inject
    @DispatchQueue
    QueuePublisher queuePublisher;

    @Inject
    DispatchQueueConfig dispatchQueueConfig;

    @Inject
    ObjectMapper objectMapper;

    public DispatchJob createDispatchJob(CreateDispatchJobRequest request) {
        // Validate service account exists
        credentialsService.validateServiceAccount(request.serviceAccountId());

        // Create via repository (handles TSID generation, status = QUEUED)
        DispatchJob job = dispatchJobRepository.create(request);

        LOG.infof("Created dispatch job [%s] kind=[%s] code=[%s] from source [%s]", job.id, job.kind, job.code, job.source);

        // Publish to queue
        boolean queued = publishSingle(job);

        // If queue send fails, update status to PENDING for safety net polling
        if (!queued) {
            LOG.warnf("Queue send failed for dispatch job [%s], updating to PENDING status", job.id);
            dispatchJobRepository.updateStatus(job.id, DispatchStatus.PENDING, null, null, null);
            job.status = DispatchStatus.PENDING;
        }

        return job;
    }

    /**
     * Create multiple dispatch jobs and publish them in a single batch.
     *
     * @param requests The dispatch job requests
     * @return List of created dispatch jobs
     */
    public List<DispatchJob> createDispatchJobs(List<CreateDispatchJobRequest> requests) {
        // Validate all service accounts first
        for (var request : requests) {
            credentialsService.validateServiceAccount(request.serviceAccountId());
        }

        // Create all jobs
        List<DispatchJob> jobs = new ArrayList<>(requests.size());
        for (var request : requests) {
            var job = dispatchJobRepository.create(request);
            LOG.infof("Created dispatch job [%s] kind=[%s] code=[%s] from source [%s]", job.id, job.kind, job.code, job.source);
            jobs.add(job);
        }

        // Batch publish to queue
        Set<String> failedJobIds = publishBatch(jobs);

        // Update failed jobs to PENDING
        if (!failedJobIds.isEmpty()) {
            LOG.warnf("Queue send failed for %d dispatch jobs, updating to PENDING status", failedJobIds.size());
            for (var job : jobs) {
                if (failedJobIds.contains(job.id)) {
                    dispatchJobRepository.updateStatus(job.id, DispatchStatus.PENDING, null, null, null);
                    job.status = DispatchStatus.PENDING;
                }
            }
        }

        return jobs;
    }

    /**
     * Publish a single dispatch job to the queue.
     *
     * @return true if successfully queued
     */
    private boolean publishSingle(DispatchJob job) {
        try {
            var message = toQueueMessage(job);
            QueuePublishResult result = queuePublisher.publish(message);

            if (result.allPublished()) {
                LOG.infof("Sent dispatch job [%s] to queue", job.id);
                return true;
            } else {
                LOG.warnf("Failed to publish dispatch job [%s]: %s",
                    job.id, result.errorMessage().orElse("Unknown error"));
                return false;
            }

        } catch (Exception e) {
            LOG.errorf(e, "Failed to send dispatch job [%s] to queue", job.id);
            return false;
        }
    }

    /**
     * Publish multiple dispatch jobs to the queue in batch.
     *
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
            return new QueueMessage(job.id, "dispatch-" + job.code, job.id, messageBody);

        } catch (JsonProcessingException e) {
            throw new RuntimeException("Failed to serialize MessagePointer for job " + job.id, e);
        }
    }

    public DispatchJobProcessResult processDispatchJob(String dispatchJobId) {
        // Load the dispatch job (single document read - includes metadata and attempts)
        DispatchJob job = dispatchJobRepository.findByIdOptional(dispatchJobId)
            .orElseThrow(() -> new IllegalArgumentException("Job not found: " + dispatchJobId));

        LOG.infof("Processing dispatch job [%s], attempt %d/%d", job.id, job.attemptCount + 1, job.maxRetries);

        // Update status to IN_PROGRESS
        dispatchJobRepository.updateStatus(job.id, DispatchStatus.IN_PROGRESS, null, null, null);

        // Resolve credentials from ServiceAccount
        ResolvedCredentials credentials = credentialsService.resolveCredentials(job)
            .orElseThrow(() -> new IllegalArgumentException("Credentials not found for job: " + job.id));

        // Dispatch webhook
        DispatchAttempt attempt = webhookDispatcher.sendWebhook(job, credentials);

        // Add attempt atomically (single MongoDB operation)
        dispatchJobRepository.addAttempt(job.id, attempt);

        // Update job based on attempt result
        int newAttemptCount = job.attemptCount + 1;

        if (attempt.status == DispatchAttemptStatus.SUCCESS) {
            // Success - mark as completed
            Instant completedAt = Instant.now();
            Long duration = Duration.between(job.createdAt, completedAt).toMillis();

            dispatchJobRepository.updateStatus(
                job.id, DispatchStatus.COMPLETED, completedAt, duration, null);

            LOG.infof("Dispatch job [%s] completed successfully", job.id);
            return DispatchJobProcessResult.success("");

        } else {
            // Failure - check if we should retry based on error type and retry count
            boolean isNotTransient = attempt.errorType == ErrorType.NOT_TRANSIENT;
            boolean retriesExhausted = newAttemptCount >= job.maxRetries;

            if (isNotTransient || retriesExhausted) {
                // Permanent error - either non-transient or max attempts exhausted
                // ACK to remove from queue
                Instant completedAt = Instant.now();
                Long duration = Duration.between(job.createdAt, completedAt).toMillis();

                dispatchJobRepository.updateStatus(
                    job.id, DispatchStatus.ERROR, completedAt, duration, attempt.errorMessage);

                if (isNotTransient) {
                    LOG.warnf("Dispatch job [%s] failed with non-transient error, marking as ERROR", job.id);
                    return DispatchJobProcessResult.permanentError("Non-transient error");
                } else {
                    LOG.warnf("Dispatch job [%s] failed after %d attempts, marking as ERROR", job.id, newAttemptCount);
                    return DispatchJobProcessResult.permanentError("Max attempts exhausted");
                }

            } else {
                // More attempts available and error is transient - NACK for retry with backoff
                dispatchJobRepository.updateStatus(
                    job.id, DispatchStatus.QUEUED, null, null, attempt.errorMessage);

                // Calculate exponential backoff delay based on attempt count
                int backoffDelay = DispatchJobProcessResult.calculateBackoffDelay(newAttemptCount);

                LOG.warnf("Dispatch job [%s] failed, attempt %d/%d, will retry in %ds",
                    job.id, newAttemptCount, job.maxRetries, backoffDelay);
                return DispatchJobProcessResult.transientError("Error but retries not exhausted.", backoffDelay);
            }
        }
    }

    public Optional<DispatchJob> findById(String id) {
        return dispatchJobRepository.findByIdOptional(id);
    }

    public List<DispatchJob> findWithFilter(DispatchJobFilter filter) {
        return dispatchJobRepository.findWithFilter(filter);
    }

    public long countWithFilter(DispatchJobFilter filter) {
        return dispatchJobRepository.countWithFilter(filter);
    }

    /**
     * Result of processing a dispatch job.
     *
     * <p>This is used to build the response to the message router:</p>
     * <ul>
     *   <li><b>ack: true</b> - Remove from queue (success OR permanent error like max retries reached)</li>
     *   <li><b>ack: false</b> - Keep on queue, retry later (transient errors)</li>
     * </ul>
     *
     * @param ack Whether to acknowledge (true) or nack (false) the message
     * @param message Human-readable status message for the message router
     * @param delaySeconds Optional delay in seconds before the message becomes visible again (for transient errors)
     */
    public record DispatchJobProcessResult(
        boolean ack,
        String message,
        Integer delaySeconds
    ) {
        /** Success - ack the message, remove from queue */
        public static DispatchJobProcessResult success(String message) {
            return new DispatchJobProcessResult(true, message, null);
        }

        /** Transient error - nack for retry with calculated backoff delay */
        public static DispatchJobProcessResult transientError(String message, int delaySeconds) {
            return new DispatchJobProcessResult(false, message, delaySeconds);
        }

        /** Permanent error - ack to prevent retry (e.g., 4xx or max retries exhausted) */
        public static DispatchJobProcessResult permanentError(String message) {
            return new DispatchJobProcessResult(true, message, null);
        }

        /**
         * Calculate exponential backoff delay based on attempt count.
         *
         * <p>Formula: min(baseDelay * (multiplier ^ attemptCount), maxDelay)</p>
         *
         * @param attemptCount The current attempt number (1-based, clamped to >= 1)
         * @param baseDelaySeconds Base delay in seconds
         * @param multiplier Backoff multiplier
         * @param maxDelaySeconds Maximum delay cap in seconds
         * @return Delay in seconds
         */
        public static int calculateBackoffDelay(int attemptCount, int baseDelaySeconds, double multiplier, int maxDelaySeconds) {
            // Ensure attemptCount is at least 1
            int safeAttemptCount = Math.max(1, attemptCount);
            // Calculate exponential backoff: base * (multiplier ^ (attemptCount - 1))
            double delay = baseDelaySeconds * Math.pow(multiplier, safeAttemptCount - 1);
            return (int) Math.min(delay, maxDelaySeconds);
        }

        /**
         * Calculate exponential backoff with default values:
         * - Base delay: 3 seconds
         * - Multiplier: 2.0
         * - Max delay: 600 seconds (10 minutes)
         *
         * <p>Backoff schedule:
         * <ul>
         *   <li>Attempt 1: 3s</li>
         *   <li>Attempt 2: 6s</li>
         *   <li>Attempt 3: 12s</li>
         *   <li>Attempt 4: 24s</li>
         *   <li>Attempt 5: 48s</li>
         *   <li>Attempt 6: 96s</li>
         *   <li>Attempt 7: 192s (~3min)</li>
         *   <li>Attempt 8: 384s (~6min)</li>
         *   <li>Attempt 9+: capped at 600s (10 min)</li>
         * </ul>
         */
        public static int calculateBackoffDelay(int attemptCount) {
            return calculateBackoffDelay(attemptCount, 3, 2.0, 600);
        }
    }
}
