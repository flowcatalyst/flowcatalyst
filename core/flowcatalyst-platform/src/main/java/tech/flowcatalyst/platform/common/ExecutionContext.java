package tech.flowcatalyst.platform.common;

import tech.flowcatalyst.platform.shared.TsidGenerator;

import java.time.Instant;

/**
 * Context for a use case execution.
 *
 * <p>Carries tracing IDs and principal information through the execution
 * of a use case. This context is used to populate domain event metadata.
 *
 * <p>The execution context enables:
 * <ul>
 *   <li>Distributed tracing via correlationId</li>
 *   <li>Causal chain tracking via causationId</li>
 *   <li>Process/saga tracking via executionId</li>
 *   <li>Audit trail via principalId</li>
 *   <li>Authorization context via authz</li>
 * </ul>
 *
 * @param executionId   Unique ID for this execution (generated)
 * @param correlationId ID for distributed tracing (usually from original request)
 * @param causationId   ID of the parent event that caused this execution (if any)
 * @param principalId   ID of the principal performing the action
 * @param initiatedAt   When the execution was initiated
 * @param authz         Authorization context (may be null if not yet resolved)
 */
public record ExecutionContext(
    String executionId,
    String correlationId,
    String causationId,
    String principalId,
    Instant initiatedAt,
    AuthorizationContext authz
) {

    /**
     * Create a new execution context for a fresh request.
     *
     * <p>The executionId and correlationId are both set to a new TSID.
     * Use this for API-initiated requests when no tracing context is available.
     *
     * <p><b>Prefer using {@link #from(TracingContext, Long)} when a TracingContext
     * is available</b>, as it will preserve correlation/causation from HTTP headers
     * or background job context.
     *
     * @param principalId The principal performing the action
     * @return A new execution context
     */
    public static ExecutionContext create(String principalId) {
        // Check if there's a thread-local TracingContext (for background jobs)
        TracingContext threadLocalCtx = TracingContext.current();
        if (threadLocalCtx != null) {
            return fromTracingContext(threadLocalCtx, principalId);
        }

        String execId = "exec-" + TsidGenerator.generateRaw();
        return new ExecutionContext(
            execId,
            execId,  // correlation starts as execution ID
            null,    // no causation for fresh requests
            principalId,
            Instant.now(),
            null     // authz not yet resolved
        );
    }

    /**
     * Create an execution context from a TracingContext.
     *
     * <p>This is the preferred method when running within an HTTP request
     * where TracingContext has been populated from headers by {@link TracingFilter}.
     *
     * @param tracingContext The tracing context (from CDI injection or thread-local)
     * @param principalId    The principal performing the action
     * @return A new execution context with correlation/causation from tracing context
     */
    public static ExecutionContext from(TracingContext tracingContext, String principalId) {
        return fromTracingContext(tracingContext, principalId);
    }

    private static ExecutionContext fromTracingContext(TracingContext tracingContext, String principalId) {
        String execId = "exec-" + TsidGenerator.generateRaw();
        return new ExecutionContext(
            execId,
            tracingContext.getCorrelationId(),  // uses existing or generates new
            tracingContext.getCausationId(),     // may be null
            principalId,
            Instant.now(),
            null     // authz not yet resolved
        );
    }

    /**
     * Create a new execution context with a specific correlation ID.
     *
     * <p>Use this when you have an existing correlation ID from an
     * upstream system or request header.
     *
     * @param principalId   The principal performing the action
     * @param correlationId The correlation ID to use
     * @return A new execution context
     */
    public static ExecutionContext withCorrelation(String principalId, String correlationId) {
        return new ExecutionContext(
            "exec-" + TsidGenerator.generateRaw(),
            correlationId,
            null,
            principalId,
            Instant.now(),
            null     // authz not yet resolved
        );
    }

    /**
     * Create a new execution context from a parent event.
     *
     * <p>Use this when reacting to an event and creating a new execution.
     * The parent event's ID becomes the causationId, and the correlationId
     * is preserved.
     *
     * @param parent      The parent event that caused this execution
     * @param principalId The principal performing the action
     * @return A new execution context linked to the parent event
     */
    public static ExecutionContext fromParentEvent(DomainEvent parent, String principalId) {
        return new ExecutionContext(
            "exec-" + TsidGenerator.generateRaw(),
            parent.correlationId(),
            parent.eventId(),
            principalId,
            Instant.now(),
            null     // authz not yet resolved
        );
    }

    /**
     * Create a child context within the same execution.
     *
     * <p>Use this when an execution needs to perform sub-operations
     * that should share the same executionId but have different causation.
     *
     * @param causingEventId The event ID that caused this sub-operation
     * @return A new context with the same executionId but new causationId
     */
    public ExecutionContext withCausation(String causingEventId) {
        return new ExecutionContext(
            this.executionId,
            this.correlationId,
            causingEventId,
            this.principalId,
            Instant.now(),
            this.authz     // preserve authz from parent context
        );
    }

    /**
     * Create a new execution context with an authorization context attached.
     *
     * <p>Use this to add authorization information to an existing context.
     *
     * @param authz The authorization context
     * @return A new context with the same tracing info but with authz attached
     */
    public ExecutionContext withAuthz(AuthorizationContext authz) {
        return new ExecutionContext(
            this.executionId,
            this.correlationId,
            this.causationId,
            this.principalId,
            this.initiatedAt,
            authz
        );
    }
}
