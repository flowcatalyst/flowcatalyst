package tech.flowcatalyst.platform.shared;

import io.micrometer.core.instrument.MeterRegistry;
import io.micrometer.core.instrument.Timer;
import jakarta.annotation.Priority;
import jakarta.inject.Inject;
import jakarta.interceptor.AroundInvoke;
import jakarta.interceptor.Interceptor;
import jakarta.interceptor.InvocationContext;
import org.jboss.logging.Logger;

/**
 * CDI Interceptor that instruments repository methods with metrics.
 *
 * Automatically records:
 * - Operation duration (histogram with percentiles)
 * - Operation count (success/error)
 * - Error count by type
 * - Slow query warnings (>100ms)
 */
@Instrumented
@Interceptor
@Priority(Interceptor.Priority.APPLICATION)
public class InstrumentedInterceptor {

    private static final Logger LOG = Logger.getLogger(InstrumentedInterceptor.class);
    private static final long SLOW_QUERY_THRESHOLD_MS = 100;

    @Inject
    MeterRegistry registry;

    @AroundInvoke
    public Object instrument(InvocationContext ctx) throws Exception {
        String collection = resolveCollection(ctx);
        String operation = ctx.getMethod().getName();

        Timer.Sample sample = Timer.start(registry);
        String result = "success";

        try {
            return ctx.proceed();
        } catch (Exception e) {
            result = "error";
            registry.counter("flowcatalyst.db.operation.errors",
                "collection", collection,
                "operation", operation,
                "error_type", classifyError(e)
            ).increment();
            throw e;
        } finally {
            long durationNanos = sample.stop(Timer.builder("flowcatalyst.db.operation.duration")
                .tag("collection", collection)
                .tag("operation", operation)
                .tag("result", result)
                .publishPercentileHistogram()
                .register(registry));

            long durationMs = durationNanos / 1_000_000;

            registry.counter("flowcatalyst.db.operations",
                "collection", collection,
                "operation", operation,
                "result", result
            ).increment();

            if (durationMs > SLOW_QUERY_THRESHOLD_MS) {
                LOG.warnf("Slow database operation: %s.%s took %dms",
                    collection, operation, durationMs);
            }
        }
    }

    private String resolveCollection(InvocationContext ctx) {
        // Check method annotation first
        Instrumented methodAnnotation = ctx.getMethod().getAnnotation(Instrumented.class);
        if (methodAnnotation != null && !methodAnnotation.collection().isEmpty()) {
            return methodAnnotation.collection();
        }

        // Check class annotation
        Class<?> targetClass = ctx.getTarget().getClass();
        Instrumented classAnnotation = targetClass.getAnnotation(Instrumented.class);
        if (classAnnotation != null && !classAnnotation.collection().isEmpty()) {
            return classAnnotation.collection();
        }

        // Check superclass annotations (for CDI proxies)
        Class<?> superClass = targetClass.getSuperclass();
        if (superClass != null) {
            classAnnotation = superClass.getAnnotation(Instrumented.class);
            if (classAnnotation != null && !classAnnotation.collection().isEmpty()) {
                return classAnnotation.collection();
            }
        }

        // Default: derive from class name (MongoPrincipalRepository -> principals)
        String className = targetClass.getSimpleName();
        // Handle CDI proxy class names like MongoPrincipalRepository_ClientProxy
        if (className.contains("_")) {
            className = className.substring(0, className.indexOf("_"));
        }
        return className
            .replace("Mongo", "")
            .replace("Repository", "")
            .toLowerCase() + "s";
    }

    private String classifyError(Exception e) {
        String name = e.getClass().getSimpleName();
        if (name.contains("NotFound") || name.contains("NoResult") || name.contains("NoDocument")) {
            return "not_found";
        }
        if (name.contains("Duplicate") || name.contains("Constraint") || name.contains("WriteConflict")) {
            return "duplicate_key";
        }
        if (name.contains("Timeout") || name.contains("TimeoutException")) {
            return "timeout";
        }
        if (name.contains("Connection") || name.contains("Network")) {
            return "connection";
        }
        return "internal";
    }
}
