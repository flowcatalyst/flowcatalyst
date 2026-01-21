package tech.flowcatalyst.platform.common;

import jakarta.enterprise.context.RequestScoped;
import tech.flowcatalyst.platform.shared.TsidGenerator;

/**
 * Request-scoped context for distributed tracing.
 *
 * <p>This context holds correlation and causation IDs for the current request.
 * It can be populated from:
 * <ul>
 *   <li>HTTP headers via {@link TracingFilter}</li>
 *   <li>Background job context via {@link #runWithContext}</li>
 *   <li>Event-driven context when processing domain events</li>
 * </ul>
 *
 * <p>Standard HTTP headers:
 * <ul>
 *   <li>{@code X-Correlation-ID} - Traces a request across services</li>
 *   <li>{@code X-Causation-ID} - References the event that caused this request</li>
 * </ul>
 */
@RequestScoped
public class TracingContext {

    /**
     * Thread-local fallback for contexts outside of CDI request scope
     * (e.g., background jobs on virtual threads).
     */
    private static final ThreadLocal<TracingContext> THREAD_LOCAL = new ThreadLocal<>();

    private String correlationId;
    private String causationId;

    /**
     * Get the correlation ID for the current context.
     * If not set, generates a new one.
     */
    public String getCorrelationId() {
        if (correlationId == null) {
            correlationId = generateId();
        }
        return correlationId;
    }

    /**
     * Set the correlation ID (typically from HTTP header).
     */
    public void setCorrelationId(String correlationId) {
        this.correlationId = correlationId;
    }

    /**
     * Get the causation ID for the current context.
     * May be null if this is a fresh request (not caused by an event).
     */
    public String getCausationId() {
        return causationId;
    }

    /**
     * Set the causation ID (typically from HTTP header or parent event).
     */
    public void setCausationId(String causationId) {
        this.causationId = causationId;
    }

    /**
     * Check if a correlation ID has been explicitly set.
     */
    public boolean hasCorrelationId() {
        return correlationId != null;
    }

    /**
     * Check if a causation ID has been set.
     */
    public boolean hasCausationId() {
        return causationId != null;
    }

    // ========================================================================
    // Static methods for thread-local access (background jobs)
    // ========================================================================

    /**
     * Get the current tracing context from thread-local storage.
     * Returns null if not in a tracing context.
     */
    public static TracingContext current() {
        return THREAD_LOCAL.get();
    }

    /**
     * Get the current tracing context, throwing if not available.
     *
     * <p>Use this in background jobs to enforce that tracing context was set up.
     *
     * @return The current tracing context
     * @throws IllegalStateException if no tracing context is available
     */
    public static TracingContext requireCurrent() {
        TracingContext ctx = THREAD_LOCAL.get();
        if (ctx == null) {
            throw new IllegalStateException(
                "No TracingContext available. Background jobs must be executed via " +
                "TracedJobExecutor or TracingContext.runWithContext()");
        }
        return ctx;
    }

    /**
     * Run a task with a specific tracing context on the current thread.
     *
     * <p>This is useful for background jobs running on virtual threads
     * that need to propagate tracing context.
     *
     * <p>Example:
     * <pre>{@code
     * TracingContext.runWithContext(correlationId, causationId, () -> {
     *     // Background job code here
     *     // TracingContext.current() will return the context
     * });
     * }</pre>
     *
     * @param correlationId The correlation ID to use
     * @param causationId   The causation ID to use (may be null)
     * @param task          The task to run
     */
    public static void runWithContext(String correlationId, String causationId, Runnable task) {
        TracingContext ctx = new TracingContext();
        ctx.correlationId = correlationId;
        ctx.causationId = causationId;

        THREAD_LOCAL.set(ctx);
        try {
            task.run();
        } finally {
            THREAD_LOCAL.remove();
        }
    }

    /**
     * Run a task with a specific tracing context and return a result.
     *
     * @param correlationId The correlation ID to use
     * @param causationId   The causation ID to use (may be null)
     * @param task          The task to run
     * @param <T>           The return type
     * @return The result of the task
     */
    public static <T> T runWithContext(String correlationId, String causationId, java.util.concurrent.Callable<T> task) {
        TracingContext ctx = new TracingContext();
        ctx.correlationId = correlationId;
        ctx.causationId = causationId;

        THREAD_LOCAL.set(ctx);
        try {
            return task.call();
        } catch (RuntimeException e) {
            throw e;
        } catch (Exception e) {
            throw new RuntimeException("Task failed", e);
        } finally {
            THREAD_LOCAL.remove();
        }
    }

    /**
     * Run a task continuing from a parent event's context.
     *
     * <p>The parent event's correlationId is preserved, and its eventId
     * becomes the causationId.
     *
     * @param parentEvent The event that caused this task
     * @param task        The task to run
     */
    public static void runFromEvent(DomainEvent parentEvent, Runnable task) {
        runWithContext(
            parentEvent.correlationId(),
            String.valueOf(parentEvent.eventId()),
            task
        );
    }

    /**
     * Run a task continuing from a parent event's context and return a result.
     *
     * @param parentEvent The event that caused this task
     * @param task        The task to run
     * @param <T>         The return type
     * @return The result of the task
     */
    public static <T> T runFromEvent(DomainEvent parentEvent, java.util.concurrent.Callable<T> task) {
        return runWithContext(
            parentEvent.correlationId(),
            String.valueOf(parentEvent.eventId()),
            task
        );
    }

    private static String generateId() {
        return "trace-" + TsidGenerator.generateRaw();
    }
}
