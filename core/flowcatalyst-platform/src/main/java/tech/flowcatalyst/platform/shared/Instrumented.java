package tech.flowcatalyst.platform.shared;

import jakarta.interceptor.InterceptorBinding;
import java.lang.annotation.*;

/**
 * Marks a repository class for automatic metrics instrumentation.
 * Records duration, count, and errors via Micrometer.
 *
 * Usage:
 * <pre>
 * {@code @Instrumented(collection = "principals")}
 * class MongoPrincipalRepository implements PrincipalRepository { ... }
 * </pre>
 *
 * Metrics produced:
 * - flowcatalyst_db_operation_duration_seconds (histogram)
 * - flowcatalyst_db_operations_total (counter)
 * - flowcatalyst_db_operation_errors_total (counter)
 */
@Inherited
@InterceptorBinding
@Retention(RetentionPolicy.RUNTIME)
@Target({ElementType.METHOD, ElementType.TYPE})
public @interface Instrumented {

    /**
     * Override collection name for metrics.
     * If empty, derived from class name (e.g., MongoPrincipalRepository → principals).
     */
    String collection() default "";
}
