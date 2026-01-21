package tech.flowcatalyst.app;

import io.quarkus.vertx.http.runtime.filters.Filters;
import io.vertx.core.http.HttpMethod;
import io.vertx.core.http.HttpServerRequest;
import io.vertx.core.http.HttpServerResponse;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.enterprise.event.Observes;
import jakarta.inject.Inject;
import org.jboss.logging.Logger;
import tech.flowcatalyst.platform.cors.CorsService;

/**
 * Dynamic CORS filter that checks origins against database-managed allowed list.
 *
 * Features:
 * - Origins are managed via Platform IAM API
 * - Allowed origins are cached for 5 minutes
 * - Supports preflight (OPTIONS) requests
 * - Only applies to /api/ and /bff/ endpoints
 */
@ApplicationScoped
public class DynamicCorsFilter {

    private static final Logger LOG = Logger.getLogger(DynamicCorsFilter.class);

    @Inject
    CorsService corsService;

    public void registerFilter(@Observes Filters filters) {
        filters.register(rc -> {
            HttpServerRequest request = rc.request();
            HttpServerResponse response = rc.response();
            String path = request.path();

            // Only apply CORS to API endpoints
            // Note: /oauth/ and /auth/ paths are handled by OAuthCorsFilter
            if (!path.startsWith("/api/") && !path.startsWith("/bff/")) {
                rc.next();
                return;
            }

            String origin = request.getHeader("Origin");

            // No Origin header = same-origin request, allow it
            if (origin == null || origin.isBlank()) {
                rc.next();
                return;
            }

            // Check if origin is allowed
            boolean isAllowed = corsService.isOriginAllowed(origin);

            if (!isAllowed) {
                LOG.debugf("CORS rejected origin: %s for path: %s", origin, path);
                // Don't add CORS headers - browser will block the request
                rc.next();
                return;
            }

            // Add CORS headers for allowed origins
            response.putHeader("Access-Control-Allow-Origin", origin);
            response.putHeader("Access-Control-Allow-Credentials", "true");
            response.putHeader("Vary", "Origin");

            // Handle preflight request
            if (request.method() == HttpMethod.OPTIONS) {
                response.putHeader("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS");
                response.putHeader("Access-Control-Allow-Headers",
                    "Content-Type, Authorization, Accept, X-Requested-With, X-Correlation-Id");
                response.putHeader("Access-Control-Max-Age", "86400"); // 24 hours

                response.setStatusCode(204).end();
                return;
            }

            // Expose headers that JavaScript can read
            response.putHeader("Access-Control-Expose-Headers",
                "Content-Type, X-Correlation-Id, X-Total-Count");

            rc.next();
        }, 8000); // Run before SPA fallback (10000) and after cache control (9000)

        LOG.info("Dynamic CORS filter registered - origins managed via /api/admin/platform/cors");
    }
}
