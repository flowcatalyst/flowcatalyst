package tech.flowcatalyst.messagerouter.factory;

import tech.flowcatalyst.messagerouter.config.QueueConfig;
import tech.flowcatalyst.messagerouter.consumer.QueueConsumer;

public interface QueueConsumerFactory {

    /**
     * Creates a queue consumer based on the queue configuration
     *
     * @param queueConfig the queue configuration
     * @param connections number of connections/pollers for this queue
     * @return a queue consumer instance
     */
    QueueConsumer createConsumer(QueueConfig queueConfig, int connections);
}
