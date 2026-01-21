package tech.flowcatalyst.platform.common;

import jakarta.annotation.Priority;
import jakarta.ws.rs.Priorities;
import jakarta.ws.rs.container.ContainerRequestContext;
import jakarta.ws.rs.container.ContainerResponseContext;
import jakarta.ws.rs.container.ContainerResponseFilter;
import jakarta.ws.rs.core.HttpHeaders;
import jakarta.ws.rs.ext.Provider;

/**
 * JAX-RS filter that adds no-cache headers to API responses.
 *
 * Prevents browsers from caching API responses, ensuring clients
 * always receive fresh data.
 *
 * Excludes static assets and Quarkus built-in endpoints.
 */
@Provider
@Priority(Priorities.HEADER_DECORATOR)
public class NoCacheFilter implements ContainerResponseFilter {

    @Override
    public void filter(ContainerRequestContext requestContext, ContainerResponseContext responseContext) {
        String path = requestContext.getUriInfo().getPath();

        // Skip static assets and Quarkus built-in endpoints
        if (isStaticAsset(path)) {
            return;
        }

        // Add no-cache headers for API responses
        responseContext.getHeaders().putSingle(HttpHeaders.CACHE_CONTROL, "no-store, no-cache, must-revalidate");
        responseContext.getHeaders().putSingle("Pragma", "no-cache");
        responseContext.getHeaders().putSingle("Expires", "0");
    }

    private boolean isStaticAsset(String path) {
        return path.startsWith("q/")           // Quarkus dev UI, health, metrics
            || path.startsWith("assets/")      // Static assets
            || path.startsWith("static/")      // Static files
            || path.startsWith("favicon")      // Favicon
            || path.endsWith(".js")
            || path.endsWith(".css")
            || path.endsWith(".png")
            || path.endsWith(".jpg")
            || path.endsWith(".jpeg")
            || path.endsWith(".gif")
            || path.endsWith(".svg")
            || path.endsWith(".ico")
            || path.endsWith(".woff")
            || path.endsWith(".woff2")
            || path.endsWith(".ttf")
            || path.endsWith(".eot");
    }
}
