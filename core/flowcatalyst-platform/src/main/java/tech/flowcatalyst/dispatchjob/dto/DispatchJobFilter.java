package tech.flowcatalyst.dispatchjob.dto;

import tech.flowcatalyst.dispatchjob.model.DispatchKind;
import tech.flowcatalyst.dispatchjob.model.DispatchStatus;

import java.time.Instant;

public record DispatchJobFilter(
    DispatchStatus status,
    String source,
    DispatchKind kind,
    String code,
    String clientId,
    String subscriptionId,
    String dispatchPoolId,
    String messageGroup,
    Instant createdAfter,
    Instant createdBefore,
    Integer page,
    Integer size
) {
    public DispatchJobFilter {
        if (page == null || page < 0) {
            page = 0;
        }
        if (size == null || size < 1 || size > 100) {
            size = 20;
        }
    }
}
