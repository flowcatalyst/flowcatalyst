package tech.flowcatalyst.platform.common;

import jakarta.inject.Inject;
import jakarta.ws.rs.container.ContainerRequestContext;
import jakarta.ws.rs.container.ContainerRequestFilter;
import jakarta.ws.rs.container.ContainerResponseContext;
import jakarta.ws.rs.container.ContainerResponseFilter;
import jakarta.ws.rs.ext.Provider;

/**
 * JAX-RS filter that extracts tracing headers and populates {@link TracingContext}.
 *
 * <p>Recognized headers (case-insensitive):
 * <ul>
 *   <li>{@code X-Correlation-ID} - Correlation ID for distributed tracing</li>
 *   <li>{@code X-Request-ID} - Alternative correlation ID header</li>
 *   <li>{@code X-Causation-ID} - ID of the event that caused this request</li>
 * </ul>
 *
 * <p>The filter also adds the correlation ID to the response headers
 * for easier debugging and client-side correlation.
 */
@Provider
public class TracingFilter implements ContainerRequestFilter, ContainerResponseFilter {

    public static final String CORRELATION_ID_HEADER = "X-Correlation-ID";
    public static final String REQUEST_ID_HEADER = "X-Request-ID";
    public static final String CAUSATION_ID_HEADER = "X-Causation-ID";

    @Inject
    TracingContext tracingContext;

    @Override
    public void filter(ContainerRequestContext requestContext) {
        // Extract correlation ID from headers (try multiple common header names)
        String correlationId = requestContext.getHeaderString(CORRELATION_ID_HEADER);
        if (correlationId == null || correlationId.isBlank()) {
            correlationId = requestContext.getHeaderString(REQUEST_ID_HEADER);
        }

        if (correlationId != null && !correlationId.isBlank()) {
            tracingContext.setCorrelationId(correlationId);
        }

        // Extract causation ID
        String causationId = requestContext.getHeaderString(CAUSATION_ID_HEADER);
        if (causationId != null && !causationId.isBlank()) {
            tracingContext.setCausationId(causationId);
        }
    }

    @Override
    public void filter(ContainerRequestContext requestContext, ContainerResponseContext responseContext) {
        // Add correlation ID to response for client-side correlation
        if (tracingContext.hasCorrelationId()) {
            responseContext.getHeaders().putSingle(CORRELATION_ID_HEADER, tracingContext.getCorrelationId());
        }
    }
}
