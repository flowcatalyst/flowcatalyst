package tech.flowcatalyst.dispatchjob.service;

import com.fasterxml.jackson.databind.ObjectMapper;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import org.jboss.logging.Logger;
import software.amazon.awssdk.services.sqs.SqsClient;
import software.amazon.awssdk.services.sqs.model.SendMessageRequest;
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
import tech.flowcatalyst.dispatchjob.repository.DispatchJobRepository;
import tech.flowcatalyst.dispatchjob.security.DispatchAuthService;

import java.time.Duration;
import java.time.Instant;
import java.util.List;
import java.util.Optional;

/**
 * Service for managing and processing dispatch jobs.
 */
@ApplicationScoped
public class DispatchJobService {

    private static final Logger LOG = Logger.getLogger(DispatchJobService.class);
    private static final String DISPATCH_POOL_CODE = "DISPATCH-POOL";
    private static final MediationType MEDIATION_TYPE = MediationType.HTTP;
    private static final String PROCESSING_ENDPOINT = "http://localhost:8080/api/dispatch/process";

    @Inject
    DispatchJobRepository dispatchJobRepository;

    @Inject
    CredentialsService credentialsService;

    @Inject
    WebhookDispatcher webhookDispatcher;

    @Inject
    DispatchAuthService dispatchAuthService;

    @Inject
    SqsClient sqsClient;

    @Inject
    ObjectMapper objectMapper;

    public DispatchJob createDispatchJob(CreateDispatchJobRequest request) {
        // Validate service account exists
        credentialsService.validateServiceAccount(request.serviceAccountId());

        // Create via repository (handles TSID generation, status = QUEUED)
        DispatchJob job = dispatchJobRepository.create(request);

        LOG.infof("Created dispatch job [%s] kind=[%s] code=[%s] from source [%s]", job.id, job.kind, job.code, job.source);

        // Send to SQS queue
        boolean queued = sendToQueue(job, request.queueUrl());

        // If queue send fails, update status to PENDING for safety net polling
        if (!queued) {
            LOG.warnf("Queue send failed for dispatch job [%s], updating to PENDING status", job.id);
            dispatchJobRepository.updateStatus(job.id, DispatchStatus.PENDING, null, null, null);
            job.status = DispatchStatus.PENDING;
        }

        return job;
    }

    /**
     * Send a dispatch job to the queue.
     *
     * @param job The dispatch job to queue
     * @param queueUrl The SQS queue URL
     * @return true if successfully queued, false otherwise
     */
    private boolean sendToQueue(DispatchJob job, String queueUrl) {
        try {
            // Generate HMAC auth token for this dispatch job
            String authToken = dispatchAuthService.generateAuthToken(job.id);

            // Create MessagePointer for the dispatch job
            MessagePointer messagePointer = new MessagePointer(
                job.id,
                DISPATCH_POOL_CODE,
                authToken,
                MEDIATION_TYPE,
                PROCESSING_ENDPOINT,
                null,  // No message group ordering needed for dispatch jobs (each job is independent)
                null   // batchId is populated by message router during routing
            );

            String messageBody = objectMapper.writeValueAsString(messagePointer);

            SendMessageRequest sendRequest = SendMessageRequest.builder()
                .queueUrl(queueUrl)
                .messageBody(messageBody)
                .messageGroupId("dispatch-" + job.code) // Group by code
                .messageDeduplicationId(job.id)
                .build();

            sqsClient.sendMessage(sendRequest);
            LOG.infof("Sent dispatch job [%s] to queue [%s]", job.id, queueUrl);
            return true;

        } catch (Exception e) {
            LOG.errorf(e, "Failed to send dispatch job [%s] to queue", job.id);
            return false;
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
