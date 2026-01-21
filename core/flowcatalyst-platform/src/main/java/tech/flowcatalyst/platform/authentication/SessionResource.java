package tech.flowcatalyst.platform.authentication;

import jakarta.inject.Inject;
import jakarta.ws.rs.*;
import jakarta.ws.rs.core.*;
import org.eclipse.microprofile.openapi.annotations.Operation;
import org.eclipse.microprofile.openapi.annotations.media.Content;
import org.eclipse.microprofile.openapi.annotations.media.Schema;
import org.eclipse.microprofile.openapi.annotations.responses.APIResponse;
import org.eclipse.microprofile.openapi.annotations.tags.Tag;
import org.jboss.logging.Logger;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.platform.principal.PrincipalRepository;

import java.util.Optional;
import java.util.Set;

/**
 * Session endpoints available in both embedded and OIDC modes.
 * Provides user info for authenticated sessions regardless of how authentication occurred.
 */
@Path("/auth")
@Tag(name = "Session", description = "Session management endpoints")
@Produces(MediaType.APPLICATION_JSON)
@Consumes(MediaType.APPLICATION_JSON)
public class SessionResource {

    private static final Logger LOG = Logger.getLogger(SessionResource.class);
    private static final String SESSION_COOKIE_NAME = "fc_session";

    @Inject
    JwtKeyService jwtKeyService;

    @Inject
    PrincipalRepository principalRepository;

    /**
     * Get current user info from session.
     * Works in both embedded and OIDC modes.
     */
    @GET
    @Path("/me")
    @Operation(summary = "Get current authenticated user")
    @APIResponse(responseCode = "200", description = "User info",
            content = @Content(schema = @Schema(implementation = SessionUserResponse.class)))
    @APIResponse(responseCode = "401", description = "Not authenticated")
    public Response me(@CookieParam(SESSION_COOKIE_NAME) String sessionToken) {
        if (sessionToken == null || sessionToken.isEmpty()) {
            return Response.status(Response.Status.UNAUTHORIZED)
                    .entity(new ErrorResponse("Not authenticated"))
                    .build();
        }

        try {
            // Use JwtKeyService to validate and parse the token
            String principalId = jwtKeyService.validateAndGetPrincipalId(sessionToken);
            if (principalId == null) {
                return Response.status(Response.Status.UNAUTHORIZED)
                        .entity(new ErrorResponse("Invalid session"))
                        .build();
            }

            Optional<Principal> principalOpt = principalRepository.findByIdOptional(principalId);

            if (principalOpt.isEmpty()) {
                return Response.status(Response.Status.UNAUTHORIZED)
                        .entity(new ErrorResponse("User not found"))
                        .build();
            }

            Principal principal = principalOpt.get();
            Set<String> roles = principal.getRoleNames();

            LOG.debugf("Session /me for principal %s, roles: %s", principalId, roles);

            return Response.ok(new SessionUserResponse(
                    principal.id,
                    principal.name,
                    principal.userIdentity != null ? principal.userIdentity.email : null,
                    roles,
                    principal.clientId
            )).build();

        } catch (Exception e) {
            LOG.debug("Failed to parse session token", e);
            return Response.status(Response.Status.UNAUTHORIZED)
                    .entity(new ErrorResponse("Invalid session"))
                    .build();
        }
    }

    // DTOs

    public record SessionUserResponse(
            String principalId,
            String name,
            String email,
            Set<String> roles,
            String clientId
    ) {}

    public record ErrorResponse(String error) {}
}
