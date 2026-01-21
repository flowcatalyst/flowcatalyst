package tech.flowcatalyst.app;

import io.quarkus.vertx.http.runtime.filters.Filters;
import io.vertx.core.http.HttpServerRequest;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.enterprise.event.Observes;
import org.jboss.logging.Logger;

/**
 * SPA Fallback Filter for Vue Router history mode.
 *
 * Routes that don't match API endpoints, auth endpoints, static files,
 * or Quarkus dev endpoints will be rewritten to /index.html so Vue Router
 * can handle client-side routing.
 */
@ApplicationScoped
public class SpaFallbackFilter {

    private static final Logger LOG = Logger.getLogger(SpaFallbackFilter.class);

    public void registerRoutes(@Observes Filters filters) {
        filters.register(rc -> {
            HttpServerRequest request = rc.request();
            String path = request.path();

            // Skip API and backend endpoints
            // Exception: GET /auth/login should serve the Vue login page
            boolean isAuthLoginPage = path.equals("/auth/login") && "GET".equalsIgnoreCase(request.method().name());

            if (!isAuthLoginPage && (
                path.startsWith("/api/") ||
                path.startsWith("/bff/") ||
                path.startsWith("/auth/") ||
                path.startsWith("/oauth/") ||
                path.startsWith("/q/") ||
                path.startsWith("/.well-known/"))) {
                rc.next();
                return;
            }

            // Skip requests for static files (have file extensions like .js, .css, .png)
            if (path.contains(".") && !path.endsWith("/")) {
                rc.next();
                return;
            }

            // Skip root path (already serves index.html)
            if (path.equals("/")) {
                rc.next();
                return;
            }

            // Rewrite SPA routes to /index.html
            // This lets Vue Router handle the route client-side
            rc.reroute("/index.html");
        }, 10000); // High priority number = runs early

        LOG.info("SPA fallback filter registered for Vue Router history mode");
    }
}
