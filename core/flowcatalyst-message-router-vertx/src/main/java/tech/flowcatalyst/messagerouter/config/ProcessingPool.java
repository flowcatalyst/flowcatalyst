package tech.flowcatalyst.messagerouter.config;

import com.fasterxml.jackson.annotation.JsonProperty;

public record ProcessingPool(
    String code,
    int concurrency,
    Integer rateLimitPerMinute
) {
}
