package tech.flowcatalyst.platform.authentication;

import jakarta.inject.Inject;
import jakarta.ws.rs.container.ContainerRequestContext;
import jakarta.ws.rs.container.ContainerRequestFilter;
import jakarta.ws.rs.core.Response;
import jakarta.ws.rs.ext.Provider;
import org.jboss.logging.Logger;

import java.io.IOException;

/**
 * JAX-RS filter that blocks requests to {@link EmbeddedModeOnly} annotated
 * resources when the auth module is running in REMOTE mode.
 *
 * When blocked, returns 404 Not Found with an explanation message.
 *
 * This filter is bound to resources using the {@link EmbeddedModeOnly} annotation
 * via JAX-RS name binding.
 *
 * @see EmbeddedModeOnly
 * @see AuthMode#REMOTE
 */
@Provider
@EmbeddedModeOnly
public class EmbeddedModeFilter implements ContainerRequestFilter {

    private static final Logger LOG = Logger.getLogger(EmbeddedModeFilter.class);

    @Inject
    AuthConfig authConfig;

    @Override
    public void filter(ContainerRequestContext requestContext) throws IOException {
        if (authConfig.mode() == AuthMode.REMOTE) {
            String path = requestContext.getUriInfo().getPath();
            LOG.debugf("Blocking request to %s - embedded mode required but running in remote mode", path);

            // Check if there's a configured redirect URL for this type of request
            String redirectUrl = getRedirectUrl(path);
            if (redirectUrl != null) {
                requestContext.abortWith(
                    Response.status(Response.Status.TEMPORARY_REDIRECT)
                        .header("Location", redirectUrl)
                        .build()
                );
                return;
            }

            requestContext.abortWith(
                Response.status(Response.Status.NOT_FOUND)
                    .entity(new ErrorResponse(
                        "not_found",
                        "This endpoint is not available. Auth is handled by an external service.",
                        getHelpMessage()
                    ))
                    .type("application/json")
                    .build()
            );
        }
    }

    /**
     * Get redirect URL for certain paths when in remote mode.
     */
    private String getRedirectUrl(String path) {
        // Redirect /platform/* to the remote platform service
        if (path.startsWith("platform") || path.startsWith("/platform")) {
            return authConfig.remote().platformUrl()
                .map(url -> url + "/" + path)
                .orElse(null);
        }

        // Redirect /auth/login to remote login URL
        if (path.equals("auth/login") || path.equals("/auth/login")) {
            return authConfig.remote().loginUrl().orElse(null);
        }

        // Redirect /auth/logout to remote logout URL
        if (path.equals("auth/logout") || path.equals("/auth/logout")) {
            return authConfig.remote().logoutUrl().orElse(null);
        }

        return null;
    }

    private String getHelpMessage() {
        return authConfig.remote().platformUrl()
            .map(url -> "The platform admin UI is available at: " + url + "/platform")
            .orElse("Configure flowcatalyst.auth.remote.platform-url to enable redirects.");
    }

    /**
     * Error response DTO.
     */
    public record ErrorResponse(String error, String message, String help) {
    }
}
