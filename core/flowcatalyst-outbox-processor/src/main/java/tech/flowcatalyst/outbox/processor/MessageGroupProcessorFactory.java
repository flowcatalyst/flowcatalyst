package tech.flowcatalyst.outbox.processor;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.outbox.api.FlowCatalystApiClient;
import tech.flowcatalyst.outbox.config.OutboxProcessorConfig;
import tech.flowcatalyst.outbox.model.OutboxItemType;
import tech.flowcatalyst.outbox.repository.OutboxRepository;

import java.util.concurrent.Semaphore;

/**
 * Factory for creating MessageGroupProcessor instances.
 */
@ApplicationScoped
public class MessageGroupProcessorFactory {

    @Inject
    OutboxProcessorConfig config;

    @Inject
    OutboxRepository repository;

    @Inject
    FlowCatalystApiClient apiClient;

    @Inject
    OutboxPoller outboxPoller;

    /**
     * Create a new MessageGroupProcessor for the given type and message group.
     *
     * @param type             The type of items this processor will handle
     * @param messageGroup     The message group for FIFO ordering
     * @param globalSemaphore  Semaphore to limit concurrent group processing
     * @return A new MessageGroupProcessor instance
     */
    public MessageGroupProcessor create(OutboxItemType type, String messageGroup, Semaphore globalSemaphore) {
        return new MessageGroupProcessor(type, messageGroup, globalSemaphore, config, repository, apiClient, outboxPoller);
    }
}
