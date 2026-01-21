package tech.flowcatalyst.messagerouter.security;

import jakarta.annotation.Priority;
import jakarta.enterprise.inject.Instance;
import jakarta.inject.Inject;
import jakarta.ws.rs.container.ContainerRequestContext;
import jakarta.ws.rs.container.ContainerRequestFilter;
import jakarta.ws.rs.core.Response;
import jakarta.ws.rs.ext.Provider;
import tech.flowcatalyst.messagerouter.config.AuthenticationConfig;

import java.io.IOException;
import java.util.logging.Logger;

/**
 * Filter for handling authentication on protected endpoints.
 * Supports both BasicAuth and OIDC authentication modes.
 */
@Provider
@Priority(2)
public class AuthenticationFilter implements ContainerRequestFilter {

    private static final Logger LOG = Logger.getLogger(AuthenticationFilter.class.getName());

    @Inject
    Instance<AuthenticationConfig> authConfigInstance;

    @Inject
    BasicAuthIdentityProvider basicAuthProvider;

    @Override
    public void filter(ContainerRequestContext requestContext) throws IOException {
        // If authentication config is not available or not enabled, allow all requests
        if (!authConfigInstance.isResolvable()) {
            return;
        }

        AuthenticationConfig authConfig = authConfigInstance.get();
        if (!authConfig.enabled()) {
            return;
        }

        // If OIDC is enabled, let Quarkus OIDC extension handle everything
        // Our filter should not interfere with OIDC flow
        if (authConfig.isOidcEnabled()) {
            return;
        }

        // Health checks should never require authentication
        String path = requestContext.getUriInfo().getPath();
        if (isHealthCheckEndpoint(path)) {
            return;
        }

        // Check if endpoint requires authentication
        if (!requiresAuthentication(path)) {
            return;
        }

        // Handle BasicAuth only (OIDC handled by Quarkus extension)
        if (authConfig.isBasicAuthEnabled()) {
            handleBasicAuth(requestContext);
        }
    }

    /**
     * Health check and metrics endpoints should always be open for k8s probes and Prometheus
     */
    private boolean isHealthCheckEndpoint(String path) {
        return path.startsWith("/health") ||
               path.startsWith("/q/health") ||
               path.startsWith("/q/live") ||
               path.startsWith("/q/ready") ||
               path.startsWith("/q/startup") ||
               path.startsWith("/q/metrics") ||    // Prometheus metrics
               path.startsWith("/metrics") ||       // Alternative metrics endpoint
               path.equals("/monitoring/health");   // Dashboard health check
    }

    /**
     * Determine which endpoints require authentication.
     * Protects API endpoints only.
     * Note: /dashboard.html is protected by HTTP auth permissions (for static files)
     * Note: /monitoring/health is excluded (handled in isHealthCheckEndpoint)
     */
    private boolean requiresAuthentication(String path) {
        return (path.startsWith("/monitoring") && !path.equals("/monitoring/health")) ||
               path.startsWith("/api/seed") ||
               path.startsWith("/api/config");
    }

    private void handleBasicAuth(ContainerRequestContext requestContext) {
        String authHeader = requestContext.getHeaderString("Authorization");

        if (authHeader == null || authHeader.isEmpty()) {
            // Return redirect instruction to login modal
            requestContext.abortWith(
                    Response.status(Response.Status.UNAUTHORIZED)
                            .header("X-Auth-Mode", "BASIC")
                            .header("X-Login-Redirect", "/dashboard.html?loginRequired=true")
                            .entity("{\"error\": \"Authentication required\", \"authMode\": \"BASIC\", \"loginUrl\": \"/dashboard.html?loginRequired=true\"}")
                            .build()
            );
            return;
        }

        // Extract and validate BasicAuth credentials
        BasicAuthRequest basicAuthRequest = BasicAuthIdentityProvider.extractBasicAuth(authHeader);

        if (basicAuthRequest == null) {
            requestContext.abortWith(
                    Response.status(Response.Status.BAD_REQUEST)
                            .entity("Invalid Authorization header format")
                            .build()
            );
            return;
        }

        // Validate credentials using provider
        if (!basicAuthProvider.validateCredentials(basicAuthRequest.getUsername(), basicAuthRequest.getPassword())) {
            LOG.warning("Failed BasicAuth attempt for user: " + basicAuthRequest.getUsername());
            requestContext.abortWith(
                    Response.status(Response.Status.UNAUTHORIZED)
                            .header("X-Auth-Mode", "BASIC")
                            .header("X-Login-Redirect", "/dashboard.html?loginRequired=true")
                            .entity("{\"error\": \"Invalid credentials\", \"authMode\": \"BASIC\", \"loginUrl\": \"/dashboard.html?loginRequired=true\"}")
                            .build()
            );
            return;
        }

        LOG.fine("BasicAuth authentication successful for user: " + basicAuthRequest.getUsername());
    }
}
