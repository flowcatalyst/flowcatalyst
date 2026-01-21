package tech.flowcatalyst.platform.authentication;

import jakarta.inject.Inject;
import jakarta.transaction.Transactional;
import jakarta.ws.rs.*;
import jakarta.ws.rs.core.*;
import org.eclipse.microprofile.openapi.annotations.Operation;
import org.eclipse.microprofile.openapi.annotations.media.Content;
import org.eclipse.microprofile.openapi.annotations.media.Schema;
import org.eclipse.microprofile.openapi.annotations.responses.APIResponse;
import org.eclipse.microprofile.openapi.annotations.tags.Tag;
import org.jboss.logging.Logger;
import tech.flowcatalyst.platform.principal.PasswordService;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.platform.principal.PrincipalRepository;
import tech.flowcatalyst.platform.principal.PrincipalType;

import java.time.Instant;
import java.util.List;
import java.util.Optional;
import java.util.Set;

/**
 * Authentication endpoints for human users.
 * Handles login/logout with session cookies.
 *
 * This resource is only available in embedded mode.
 * In remote mode, auth requests are redirected to the external IdP.
 */
@Path("/auth")
@Tag(name = "Authentication", description = "User authentication endpoints")
@Produces(MediaType.APPLICATION_JSON)
@Consumes(MediaType.APPLICATION_JSON)
@EmbeddedModeOnly
public class AuthResource {

    private static final Logger LOG = Logger.getLogger(AuthResource.class);
    private static final String SESSION_COOKIE_NAME = "fc_session";

    @Inject
    AuthConfig authConfig;

    @Inject
    PrincipalRepository principalRepository;

    @Inject
    PasswordService passwordService;

    @Inject
    JwtKeyService jwtKeyService;


    @Context
    UriInfo uriInfo;

    /**
     * Login with email and password.
     * Returns a session cookie on success.
     */
    @POST
    @Path("/login")
    @Operation(summary = "Login with email and password")
    @APIResponse(responseCode = "200", description = "Login successful",
            content = @Content(schema = @Schema(implementation = LoginResponse.class)))
    @APIResponse(responseCode = "401", description = "Invalid credentials")
    @Transactional
    public Response login(LoginRequest request) {
        LOG.debugf("Login attempt for email: %s", request.email());

        // Find user by email
        Optional<Principal> principalOpt = principalRepository.findByEmail(request.email());
        if (principalOpt.isEmpty()) {
            LOG.infof("Login failed: user not found for email %s", request.email());
            return Response.status(Response.Status.UNAUTHORIZED)
                    .entity(new ErrorResponse("Invalid email or password"))
                    .build();
        }

        Principal principal = principalOpt.get();

        // Verify it's a user (not service account)
        if (principal.type != PrincipalType.USER) {
            LOG.warnf("Login attempt for non-user principal: %s", request.email());
            return Response.status(Response.Status.UNAUTHORIZED)
                    .entity(new ErrorResponse("Invalid email or password"))
                    .build();
        }

        // Verify user is active
        if (!principal.active) {
            LOG.infof("Login failed: user is inactive %s", request.email());
            return Response.status(Response.Status.UNAUTHORIZED)
                    .entity(new ErrorResponse("Account is disabled"))
                    .build();
        }

        // Verify password
        if (principal.userIdentity == null || principal.userIdentity.passwordHash == null) {
            LOG.warnf("Login failed: no password set for user %s", request.email());
            return Response.status(Response.Status.UNAUTHORIZED)
                    .entity(new ErrorResponse("Invalid email or password"))
                    .build();
        }

        if (!passwordService.verifyPassword(request.password(), principal.userIdentity.passwordHash)) {
            LOG.infof("Login failed: invalid password for %s", request.email());
            return Response.status(Response.Status.UNAUTHORIZED)
                    .entity(new ErrorResponse("Invalid email or password"))
                    .build();
        }

        // Load roles from embedded Principal.roles
        Set<String> roles = principal.getRoleNames();

        // Determine accessible clients
        List<String> clients = determineAccessibleClients(principal, roles);

        // Issue session token
        String token = jwtKeyService.issueSessionToken(principal.id, request.email(), roles, clients);

        // Update last login (don't touch roles)
        principal.userIdentity.lastLoginAt = Instant.now();
        principalRepository.updateOnly(principal);

        // Create session cookie
        NewCookie sessionCookie = createSessionCookie(token);

        LOG.infof("Login successful for user: %s", request.email());

        return Response.ok(new LoginResponse(
                        principal.id,
                        principal.name,
                        request.email(),
                        roles,
                        principal.clientId
                ))
                .cookie(sessionCookie)
                .build();
    }

    /**
     * Logout - clears the session cookie.
     */
    @POST
    @Path("/logout")
    @Operation(summary = "Logout and clear session")
    @APIResponse(responseCode = "200", description = "Logout successful")
    public Response logout() {
        NewCookie expiredCookie = new NewCookie.Builder(authConfig.session().cookieName())
                .value("")
                .path("/")
                .maxAge(0)
                .httpOnly(true)
                .secure(authConfig.session().secure())
                .sameSite(NewCookie.SameSite.valueOf(authConfig.session().sameSite().toUpperCase()))
                .build();

        return Response.ok(new MessageResponse("Logged out successfully"))
                .cookie(expiredCookie)
                .build();
    }

    // Note: /auth/me endpoint is now in SessionResource (available in both embedded and OIDC modes)

    private NewCookie createSessionCookie(String token) {
        long maxAgeSeconds = jwtKeyService.getSessionTokenExpiry().toSeconds();

        return new NewCookie.Builder(authConfig.session().cookieName())
                .value(token)
                .path("/")
                .maxAge((int) maxAgeSeconds)
                .httpOnly(true)
                .secure(authConfig.session().secure())
                .sameSite(NewCookie.SameSite.valueOf(authConfig.session().sameSite().toUpperCase()))
                .build();
    }

    /**
     * Determine which clients the user can access based on their scope.
     *
     * @return List of client IDs as strings, or ["*"] for anchor users
     */
    private List<String> determineAccessibleClients(Principal principal, Set<String> roles) {
        // Check explicit scope first
        if (principal.scope != null) {
            switch (principal.scope) {
                case ANCHOR:
                    return List.of("*");
                case CLIENT:
                    if (principal.clientId != null) {
                        return List.of(String.valueOf(principal.clientId));
                    }
                    return List.of();
                case PARTNER:
                    if (principal.clientId != null) {
                        return List.of(String.valueOf(principal.clientId));
                    }
                    return List.of();
            }
        }

        // Fallback: check roles for platform admins
        if (roles.stream().anyMatch(r -> r.contains("platform:admin") || r.contains("super-admin"))) {
            return List.of("*");
        }

        // User is bound to a specific client
        if (principal.clientId != null) {
            return List.of(String.valueOf(principal.clientId));
        }

        return List.of();
    }

    // DTOs

    public record LoginRequest(String email, String password) {}

    public record LoginResponse(
            String principalId,
            String name,
            String email,
            Set<String> roles,
            String clientId
    ) {}

    public record ErrorResponse(String error) {}

    public record MessageResponse(String message) {}
}
