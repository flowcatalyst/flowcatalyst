package tech.flowcatalyst.app;

import io.quarkus.vertx.http.runtime.filters.Filters;
import io.vertx.core.http.HttpServerRequest;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.enterprise.event.Observes;
import org.jboss.logging.Logger;

/**
 * Filter to add cache control headers to API and BFF endpoints.
 *
 * Prevents browsers from caching API responses, which can cause issues
 * when cached error responses (like redirects) are returned instead of
 * fresh data.
 *
 * API endpoints should:
 * 1. Never be cached by browsers
 * 2. Always return JSON (not HTML redirects)
 * 3. Return proper status codes (401/403) for auth failures
 */
@ApplicationScoped
public class ApiCacheControlFilter {

    private static final Logger LOG = Logger.getLogger(ApiCacheControlFilter.class);

    public void registerFilter(@Observes Filters filters) {
        filters.register(rc -> {
            HttpServerRequest request = rc.request();
            String path = request.path();

            // Add cache control headers for API and BFF endpoints
            if (path.startsWith("/api/") || path.startsWith("/bff/")) {
                rc.response().putHeader("Cache-Control", "no-store, no-cache, must-revalidate");
                rc.response().putHeader("Pragma", "no-cache");
                rc.response().putHeader("Expires", "0");
            }

            rc.next();
        }, 9000); // Run before SPA fallback filter (which is 10000)

        LOG.info("API cache control filter registered - API/BFF responses will not be cached");
    }
}
