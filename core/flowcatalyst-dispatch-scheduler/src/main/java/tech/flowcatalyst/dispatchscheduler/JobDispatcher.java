package tech.flowcatalyst.dispatchscheduler;

import com.fasterxml.jackson.databind.ObjectMapper;
import jakarta.annotation.PostConstruct;
import jakarta.annotation.PreDestroy;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import org.jboss.logging.Logger;
import tech.flowcatalyst.dispatchjob.entity.DispatchJob;
import tech.flowcatalyst.dispatchjob.model.DispatchStatus;
import tech.flowcatalyst.dispatchjob.model.MediationType;
import tech.flowcatalyst.dispatchjob.model.MessagePointer;
import tech.flowcatalyst.dispatchjob.repository.DispatchJobRepository;
import tech.flowcatalyst.dispatchjob.security.DispatchAuthService;
import tech.flowcatalyst.queue.*;

/**
 * Dispatches jobs to the external queue.
 * Updates job status to QUEUED on successful dispatch.
 */
@ApplicationScoped
public class JobDispatcher {

    private static final Logger LOG = Logger.getLogger(JobDispatcher.class);

    @Inject
    DispatchSchedulerConfig config;

    @Inject
    QueuePublisherFactory queuePublisherFactory;

    @Inject
    DispatchJobRepository dispatchJobRepository;

    @Inject
    DispatchAuthService dispatchAuthService;

    @Inject
    ObjectMapper objectMapper;

    private QueuePublisher queuePublisher;

    @PostConstruct
    void init() {
        if (!config.enabled()) {
            LOG.info("Dispatch scheduler is disabled");
            return;
        }

        QueueConfig queueConfig = switch (config.queueType()) {
            case SQS -> QueueConfig.sqsFifo(config.queueUrl().orElseThrow(
                () -> new IllegalStateException("Queue URL required for SQS")));
            case EMBEDDED -> QueueConfig.embedded(config.embeddedDbPath());
            case ACTIVEMQ -> QueueConfig.activeMq(config.queueUrl().orElse("dispatch-queue"));
            case NATS ->  QueueConfig.nats(config.queueUrl().orElse("dispatch-queue"));
        };

        queuePublisher = queuePublisherFactory.create(queueConfig);
        LOG.infof("Initialized JobDispatcher with queue type: %s", config.queueType());
    }

    @PreDestroy
    void cleanup() {
        if (queuePublisher != null) {
            queuePublisher.close();
        }
    }

    /**
     * Dispatch a single job to the external queue.
     * Updates status to QUEUED on success.
     *
     * @param job The dispatch job to send
     * @return true if successfully dispatched
     */
    public boolean dispatch(DispatchJob job) {
        if (queuePublisher == null) {
            LOG.warn("Queue publisher not initialized, skipping dispatch");
            return false;
        }

        try {
            // Generate HMAC auth token for this dispatch job
            String authToken = dispatchAuthService.generateAuthToken(job.id);

            // Create MessagePointer for the dispatch job
            MessagePointer pointer = new MessagePointer(
                job.id,
                job.dispatchPoolId != null ? job.dispatchPoolId : config.defaultDispatchPoolCode(),
                authToken,
                MediationType.HTTP,
                config.processingEndpoint(),
                job.messageGroup,
                null  // batchId populated by message router
            );

            String messageBody = objectMapper.writeValueAsString(pointer);

            QueueMessage message = new QueueMessage(
                job.id,
                job.messageGroup,
                job.id,  // Use job ID as deduplication ID
                messageBody
            );

            QueuePublishResult result = queuePublisher.publish(message);

            if (result.allPublished()) {
                // Update status to QUEUED
                dispatchJobRepository.updateStatus(job.id, DispatchStatus.QUEUED, null, null, null);
                LOG.debugf("Dispatched job [%s] to queue, status updated to QUEUED", job.id);
                return true;

            } else if (result.errorMessage().map(e -> e.contains("Deduplicated")).orElse(false)) {
                // Already dispatched (deduplicated), still mark as QUEUED
                LOG.debugf("Job [%s] was deduplicated (already dispatched)", job.id);
                dispatchJobRepository.updateStatus(job.id, DispatchStatus.QUEUED, null, null, null);
                return true;

            } else {
                LOG.warnf("Failed to dispatch job [%s]: %s",
                    job.id, result.errorMessage().orElse("Unknown error"));
                return false;
            }

        } catch (Exception e) {
            LOG.errorf(e, "Error dispatching job [%s]", job.id);
            return false;
        }
    }

    /**
     * Check if the queue publisher is healthy.
     */
    public boolean isHealthy() {
        return queuePublisher != null && queuePublisher.isHealthy();
    }
}
