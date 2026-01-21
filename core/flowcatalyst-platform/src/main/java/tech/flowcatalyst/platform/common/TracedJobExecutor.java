package tech.flowcatalyst.platform.common;

import jakarta.annotation.PreDestroy;
import jakarta.enterprise.context.ApplicationScoped;
import org.jboss.logging.Logger;

import java.util.Objects;
import java.util.concurrent.Callable;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.concurrent.TimeUnit;

/**
 * Executor for running background jobs on virtual threads with tracing context.
 *
 * <p>Uses Java 21 virtual threads for efficient, lightweight concurrency.
 * All jobs executed through this service will have tracing context available
 * via {@link TracingContext#current()} and {@link ExecutionContext#create(Long)}.
 *
 * <p>Example usage:
 * <pre>{@code
 * @Inject TracedJobExecutor jobExecutor;
 *
 * // Fire-and-forget from an HTTP request
 * jobExecutor.submit(tracingContext, () -> {
 *     ExecutionContext ctx = ExecutionContext.create(principalId);
 *     // ... do background work
 * });
 *
 * // Continue from an event
 * jobExecutor.submitFromEvent(parentEvent, () -> {
 *     // correlationId preserved, causationId = parent eventId
 * });
 *
 * // Get a result (blocks caller until complete)
 * Future<Result> future = jobExecutor.submit(tracingContext, () -> computeResult());
 * Result result = future.get();
 * }</pre>
 */
@ApplicationScoped
public class TracedJobExecutor {

    private static final Logger LOG = Logger.getLogger(TracedJobExecutor.class);

    private final ExecutorService executor = Executors.newVirtualThreadPerTaskExecutor();

    /**
     * Submit a fire-and-forget job with explicit tracing context.
     *
     * @param correlationId The correlation ID for distributed tracing (required)
     * @param causationId   The causation ID (may be null for root jobs)
     * @param job           The job to execute
     */
    public void submit(String correlationId, String causationId, Runnable job) {
        Objects.requireNonNull(correlationId, "correlationId is required for traced jobs");
        Objects.requireNonNull(job, "job cannot be null");

        executor.submit(() -> {
            TracingContext.runWithContext(correlationId, causationId, () -> {
                try {
                    job.run();
                } catch (Exception e) {
                    LOG.errorf(e, "Job failed [correlationId=%s, causationId=%s]", correlationId, causationId);
                }
            });
        });
    }

    /**
     * Submit a fire-and-forget job capturing context from TracingContext.
     *
     * <p>Use this when submitting a background job from within an HTTP request.
     *
     * @param tracingContext The current request's tracing context
     * @param job            The job to execute
     */
    public void submit(TracingContext tracingContext, Runnable job) {
        submit(
            tracingContext.getCorrelationId(),
            tracingContext.getCausationId(),
            job
        );
    }

    /**
     * Submit a fire-and-forget job continuing from a parent event.
     *
     * <p>The parent event's correlationId is preserved, and its eventId
     * becomes the causationId for the job.
     *
     * @param parentEvent The event that caused this job
     * @param job         The job to execute
     */
    public void submitFromEvent(DomainEvent parentEvent, Runnable job) {
        submit(
            parentEvent.correlationId(),
            String.valueOf(parentEvent.eventId()),
            job
        );
    }

    /**
     * Submit a job with explicit tracing context and return a Future for the result.
     *
     * @param correlationId The correlation ID for distributed tracing (required)
     * @param causationId   The causation ID (may be null for root jobs)
     * @param job           The job to execute
     * @param <T>           The return type
     * @return Future that completes with the job's result
     */
    public <T> Future<T> submit(String correlationId, String causationId, Callable<T> job) {
        Objects.requireNonNull(correlationId, "correlationId is required for traced jobs");
        Objects.requireNonNull(job, "job cannot be null");

        return executor.submit(() ->
            TracingContext.runWithContext(correlationId, causationId, () -> {
                try {
                    return job.call();
                } catch (RuntimeException e) {
                    LOG.errorf(e, "Job failed [correlationId=%s, causationId=%s]", correlationId, causationId);
                    throw e;
                } catch (Exception e) {
                    LOG.errorf(e, "Job failed [correlationId=%s, causationId=%s]", correlationId, causationId);
                    throw new RuntimeException("Job failed", e);
                }
            })
        );
    }

    /**
     * Submit a job capturing context from TracingContext and return a Future.
     *
     * @param tracingContext The current request's tracing context
     * @param job            The job to execute
     * @param <T>            The return type
     * @return Future that completes with the job's result
     */
    public <T> Future<T> submit(TracingContext tracingContext, Callable<T> job) {
        return submit(
            tracingContext.getCorrelationId(),
            tracingContext.getCausationId(),
            job
        );
    }

    /**
     * Submit a job continuing from a parent event and return a Future.
     *
     * @param parentEvent The event that caused this job
     * @param job         The job to execute
     * @param <T>         The return type
     * @return Future that completes with the job's result
     */
    public <T> Future<T> submitFromEvent(DomainEvent parentEvent, Callable<T> job) {
        return submit(
            parentEvent.correlationId(),
            String.valueOf(parentEvent.eventId()),
            job
        );
    }

    @PreDestroy
    void shutdown() {
        executor.shutdown();
        try {
            if (!executor.awaitTermination(10, TimeUnit.SECONDS)) {
                executor.shutdownNow();
            }
        } catch (InterruptedException e) {
            executor.shutdownNow();
            Thread.currentThread().interrupt();
        }
    }
}
