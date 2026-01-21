package tech.flowcatalyst.messagerouter.config;

import com.fasterxml.jackson.annotation.JsonProperty;

import java.util.List;

public record MessageRouterConfig(
        List<QueueConfig> queues,
        int connections,
        List<ProcessingPool> processingPools
) {
}
