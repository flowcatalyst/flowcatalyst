package tech.flowcatalyst.platform.audit;

import jakarta.annotation.Priority;
import jakarta.inject.Inject;
import jakarta.ws.rs.Priorities;
import jakarta.ws.rs.container.ContainerRequestContext;
import jakarta.ws.rs.container.ContainerRequestFilter;
import jakarta.ws.rs.core.Cookie;
import jakarta.ws.rs.ext.Provider;
import tech.flowcatalyst.platform.authentication.AuthConfig;
import tech.flowcatalyst.platform.authentication.JwtKeyService;

/**
 * JAX-RS filter that auto-populates AuditContext from session cookie or Bearer token.
 *
 * Runs after authentication filters to extract the principal ID and make it
 * available for audit logging throughout the request.
 */
@Provider
@Priority(Priorities.AUTHENTICATION + 10)
public class AuditContextFilter implements ContainerRequestFilter {

    private static final org.jboss.logging.Logger LOG = org.jboss.logging.Logger.getLogger(AuditContextFilter.class);

    @Inject
    AuditContext auditContext;

    @Inject
    JwtKeyService jwtKeyService;

    @Inject
    AuthConfig authConfig;

    @Override
    public void filter(ContainerRequestContext ctx) {
        String path = ctx.getUriInfo().getPath();

        // Skip logging for health/metrics endpoints
        boolean isHealthPath = path.startsWith("q/") || path.startsWith("health") || path.startsWith("metrics");

        String principalId = null;

        // Try session cookie first - use configured cookie name
        String cookieName = authConfig.session().cookieName();
        Cookie sessionCookie = ctx.getCookies().get(cookieName);
        if (sessionCookie != null && sessionCookie.getValue() != null) {
            principalId = jwtKeyService.validateAndGetPrincipalId(sessionCookie.getValue());
            if (principalId == null && !isHealthPath) {
                LOG.warnf("Session cookie '%s' present but INVALID for path: %s", cookieName, path);
            }
        } else if (!isHealthPath) {
            LOG.warnf("No session cookie '%s' for path: %s, cookies present: %s", cookieName, path, ctx.getCookies().keySet());
        }

        // Fall back to Bearer token
        if (principalId == null) {
            String authHeader = ctx.getHeaderString("Authorization");
            if (authHeader != null && authHeader.startsWith("Bearer ")) {
                String token = authHeader.substring("Bearer ".length());
                principalId = jwtKeyService.validateAndGetPrincipalId(token);
            }
        }

        if (principalId != null) {
            auditContext.setPrincipalId(principalId);
            LOG.debugf("Audit context set for principal: %s on path: %s", principalId, path);
        }
    }
}
