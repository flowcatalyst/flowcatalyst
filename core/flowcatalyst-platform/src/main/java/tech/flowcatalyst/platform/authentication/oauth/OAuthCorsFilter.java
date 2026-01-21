package tech.flowcatalyst.platform.authentication.oauth;

import jakarta.annotation.Priority;
import jakarta.inject.Inject;
import jakarta.ws.rs.Priorities;
import jakarta.ws.rs.container.ContainerRequestContext;
import jakarta.ws.rs.container.ContainerRequestFilter;
import jakarta.ws.rs.container.ContainerResponseContext;
import jakarta.ws.rs.container.ContainerResponseFilter;
import jakarta.ws.rs.core.MultivaluedMap;
import jakarta.ws.rs.core.Response;
import jakarta.ws.rs.ext.Provider;
import org.jboss.logging.Logger;

import java.io.IOException;
import java.util.Optional;

/**
 * CORS filter for OAuth and OIDC endpoints that dynamically allows origins based on
 * the OAuthClient's configured allowedOrigins.
 *
 * <p>This filter handles:
 * <ul>
 *   <li>/oauth/* - OAuth endpoints (authorize, token, etc.)</li>
 *   <li>/.well-known/* - OIDC discovery endpoints</li>
 *   <li>/auth/* - Authentication endpoints</li>
 * </ul>
 *
 * <p>For discovery endpoints (/.well-known/*), the origin is checked against ALL
 * registered OAuth clients since these are public endpoints.
 *
 * <p>For /oauth/* endpoints, the client_id is extracted from:
 * <ul>
 *   <li>Query parameter (for /oauth/authorize and /oauth/token)</li>
 *   <li>Authorization header (Basic auth for /oauth/token)</li>
 * </ul>
 *
 * <p>For preflight OPTIONS requests to /oauth/token where client_id cannot be determined
 * from query params or headers, the filter checks if the origin is allowed by ANY active
 * OAuth client. The actual POST request will then validate the specific client.
 */
@Provider
@Priority(Priorities.HEADER_DECORATOR)
public class OAuthCorsFilter implements ContainerRequestFilter, ContainerResponseFilter {

    private static final Logger LOG = Logger.getLogger(OAuthCorsFilter.class);

    private static final String ORIGIN_HEADER = "Origin";
    private static final String ACCESS_CONTROL_ALLOW_ORIGIN = "Access-Control-Allow-Origin";
    private static final String ACCESS_CONTROL_ALLOW_METHODS = "Access-Control-Allow-Methods";
    private static final String ACCESS_CONTROL_ALLOW_HEADERS = "Access-Control-Allow-Headers";
    private static final String ACCESS_CONTROL_ALLOW_CREDENTIALS = "Access-Control-Allow-Credentials";
    private static final String ACCESS_CONTROL_MAX_AGE = "Access-Control-Max-Age";
    private static final String ACCESS_CONTROL_REQUEST_METHOD = "Access-Control-Request-Method";

    // Context property to store the validated origin for the response filter
    private static final String VALIDATED_ORIGIN_PROP = "oauth.cors.validatedOrigin";

    @Inject
    OAuthClientRepository clientRepo;

    @Override
    public void filter(ContainerRequestContext requestContext) throws IOException {
        String path = requestContext.getUriInfo().getPath();

        // Check if this is a path we handle
        boolean isOAuthPath = path.startsWith("oauth/") || path.startsWith("/oauth/");
        boolean isWellKnownPath = path.startsWith(".well-known/") || path.startsWith("/.well-known/");
        boolean isAuthPath = path.startsWith("auth/") || path.startsWith("/auth/");

        if (!isOAuthPath && !isWellKnownPath && !isAuthPath) {
            return;
        }

        String origin = requestContext.getHeaderString(ORIGIN_HEADER);
        if (origin == null || origin.isBlank()) {
            return; // No origin header, not a CORS request
        }

        boolean isPreflight = "OPTIONS".equalsIgnoreCase(requestContext.getMethod())
                && requestContext.getHeaderString(ACCESS_CONTROL_REQUEST_METHOD) != null;

        // For .well-known and /auth paths, check if ANY client allows this origin
        // These are public endpoints used by OIDC clients
        if (isWellKnownPath || isAuthPath) {
            if (clientRepo.isOriginAllowedByAnyClient(origin)) {
                LOG.debugf("CORS: Origin %s allowed for %s", origin, path);
                requestContext.setProperty(VALIDATED_ORIGIN_PROP, origin);
                if (isPreflight) {
                    requestContext.abortWith(buildPreflightResponse(origin));
                }
            } else {
                LOG.debugf("CORS: Origin %s not allowed by any client for %s", origin, path);
            }
            return;
        }

        // For /oauth/* paths, try to validate against specific client
        String clientId = extractClientId(requestContext);

        // If we have a client_id, validate against that specific client
        if (clientId != null) {
            Optional<OAuthClient> clientOpt = clientRepo.findByClientId(clientId);
            if (clientOpt.isEmpty()) {
                LOG.debugf("CORS: Unknown client_id %s", clientId);
                return;
            }

            OAuthClient client = clientOpt.get();
            if (!client.isOriginAllowed(origin)) {
                LOG.debugf("CORS: Origin %s not allowed for client %s", origin, clientId);
                return;
            }

            // Origin is valid - store it for the response filter
            requestContext.setProperty(VALIDATED_ORIGIN_PROP, origin);

            if (isPreflight) {
                LOG.debugf("CORS: Handling preflight for client %s, origin %s", clientId, origin);
                requestContext.abortWith(buildPreflightResponse(origin));
            }
            return;
        }

        // No client_id found - for preflight requests to /oauth/token, check if ANY client allows this origin
        // The actual POST request will have the client_id in the form body and be validated by the endpoint
        if (isPreflight && (path.contains("oauth/token") || path.contains("/oauth/token"))) {
            LOG.infof("CORS: Checking if origin %s is allowed by any client for preflight to %s", origin, path);
            try {
                boolean allowed = clientRepo.isOriginAllowedByAnyClient(origin);
                LOG.infof("CORS: isOriginAllowedByAnyClient returned %s for origin %s", allowed, origin);
                if (allowed) {
                    LOG.infof("CORS: Handling preflight for origin %s (client will be validated on actual request)", origin);
                    requestContext.setProperty(VALIDATED_ORIGIN_PROP, origin);
                    requestContext.abortWith(buildPreflightResponse(origin));
                } else {
                    LOG.warnf("CORS: Origin %s not allowed by any client", origin);
                }
            } catch (Exception e) {
                LOG.errorf(e, "CORS: Error checking origin %s", origin);
            }
            return;
        }

        LOG.debugf("CORS: No client_id found in request to %s", path);
    }

    @Override
    public void filter(ContainerRequestContext requestContext, ContainerResponseContext responseContext)
            throws IOException {
        String path = requestContext.getUriInfo().getPath();

        // Check if this is a path we handle
        boolean isOAuthPath = path.startsWith("oauth/") || path.startsWith("/oauth/");
        boolean isWellKnownPath = path.startsWith(".well-known/") || path.startsWith("/.well-known/");
        boolean isAuthPath = path.startsWith("auth/") || path.startsWith("/auth/");

        if (!isOAuthPath && !isWellKnownPath && !isAuthPath) {
            return;
        }

        // Add CORS headers if origin was validated in the request filter
        String validatedOrigin = (String) requestContext.getProperty(VALIDATED_ORIGIN_PROP);
        if (validatedOrigin != null) {
            addCorsHeaders(responseContext.getHeaders(), validatedOrigin);
            return;
        }

        // For POST /oauth/token where we couldn't validate in request filter (client_id in form body),
        // check if the origin is allowed by any client. The endpoint has already validated the client.
        // This is safe because the token endpoint validates the client credentials.
        String origin = requestContext.getHeaderString(ORIGIN_HEADER);
        if (origin != null && !origin.isBlank()
                && "POST".equalsIgnoreCase(requestContext.getMethod())
                && (path.contains("oauth/token") || path.contains("/oauth/token"))
                && responseContext.getStatus() < 400) { // Only for successful responses
            if (clientRepo.isOriginAllowedByAnyClient(origin)) {
                LOG.debugf("CORS: Adding headers to token response for origin %s", origin);
                addCorsHeaders(responseContext.getHeaders(), origin);
            }
        }
    }

    private static final String CLIENT_ID_PREFIX = "oauth_";

    /**
     * Convert external client ID (with prefix) to internal format (raw TSID).
     * Strips "oauth_" or legacy "fc_" prefix.
     */
    private static String toInternalClientId(String externalId) {
        if (externalId == null) return null;
        if (externalId.startsWith(CLIENT_ID_PREFIX)) {
            return externalId.substring(CLIENT_ID_PREFIX.length());
        }
        // Also support legacy fc_ prefix for backwards compatibility
        if (externalId.startsWith("fc_")) {
            return externalId.substring(3);
        }
        return externalId;
    }

    /**
     * Extract client_id from the request and convert to internal format.
     * Checks query params first, then Basic auth header.
     */
    private String extractClientId(ContainerRequestContext requestContext) {
        String clientId = null;

        // Try query parameter first (used by /oauth/authorize)
        clientId = requestContext.getUriInfo().getQueryParameters().getFirst("client_id");
        if (clientId != null && !clientId.isBlank()) {
            return toInternalClientId(clientId);
        }

        // For POST requests, try to get from form params
        // Note: We can't easily read form params without consuming the entity
        // For /oauth/token, the client_id should also be in query params or we rely on
        // the Authorization header's Basic auth which we'd need to parse

        // Try to extract from Basic auth header
        String authHeader = requestContext.getHeaderString("Authorization");
        if (authHeader != null && authHeader.startsWith("Basic ")) {
            try {
                String base64 = authHeader.substring("Basic ".length()).trim();
                String decoded = new String(java.util.Base64.getDecoder().decode(base64));
                int colonIdx = decoded.indexOf(':');
                if (colonIdx > 0) {
                    return toInternalClientId(decoded.substring(0, colonIdx));
                }
            } catch (Exception e) {
                // Ignore parsing errors
            }
        }

        return null;
    }

    /**
     * Build a preflight response with CORS headers.
     */
    private Response buildPreflightResponse(String origin) {
        return Response.ok()
            .header(ACCESS_CONTROL_ALLOW_ORIGIN, origin)
            .header(ACCESS_CONTROL_ALLOW_METHODS, "GET, POST, OPTIONS")
            .header(ACCESS_CONTROL_ALLOW_HEADERS, "Content-Type, Authorization")
            .header(ACCESS_CONTROL_ALLOW_CREDENTIALS, "true")
            .header(ACCESS_CONTROL_MAX_AGE, "86400") // 24 hours
            .build();
    }

    /**
     * Add CORS headers to the response.
     */
    private void addCorsHeaders(MultivaluedMap<String, Object> headers, String origin) {
        headers.putSingle(ACCESS_CONTROL_ALLOW_ORIGIN, origin);
        headers.putSingle(ACCESS_CONTROL_ALLOW_CREDENTIALS, "true");
    }
}
