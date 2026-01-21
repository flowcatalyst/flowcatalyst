package tech.flowcatalyst.platform.authentication;

import jakarta.inject.Inject;
import jakarta.ws.rs.*;
import jakarta.ws.rs.core.MediaType;
import jakarta.ws.rs.core.Response;
import org.eclipse.microprofile.openapi.annotations.Operation;
import org.eclipse.microprofile.openapi.annotations.media.Content;
import org.eclipse.microprofile.openapi.annotations.media.Schema;
import org.eclipse.microprofile.openapi.annotations.responses.APIResponse;
import org.eclipse.microprofile.openapi.annotations.tags.Tag;
import org.jboss.logging.Logger;
import tech.flowcatalyst.platform.client.ClientAuthConfig;
import tech.flowcatalyst.platform.client.ClientAuthConfigRepository;

import java.util.Optional;

/**
 * Public authentication discovery endpoints.
 * Available in both embedded and remote modes.
 *
 * Used by frontends to determine the authentication method
 * for a given email domain before showing login UI.
 */
@Path("/auth")
@Tag(name = "Auth Discovery", description = "Authentication discovery endpoints")
@Produces(MediaType.APPLICATION_JSON)
@Consumes(MediaType.APPLICATION_JSON)
public class AuthDiscoveryResource {

    private static final Logger LOG = Logger.getLogger(AuthDiscoveryResource.class);

    @Inject
    ClientAuthConfigRepository authConfigRepository;

    /**
     * Check the authentication method for an email domain.
     * Returns 'internal' for password auth, 'external' with IDP URL for SSO.
     */
    @POST
    @Path("/check-domain")
    @Operation(summary = "Check authentication method for email domain",
            description = "Determines whether to show password login or redirect to SSO")
    @APIResponse(responseCode = "200", description = "Domain check result",
            content = @Content(schema = @Schema(implementation = DomainCheckResponse.class)))
    @APIResponse(responseCode = "400", description = "Invalid request")
    public Response checkDomain(DomainCheckRequest request) {
        if (request == null || request.email() == null || request.email().isBlank()) {
            return Response.status(Response.Status.BAD_REQUEST)
                    .entity(new ErrorResponse("Email is required"))
                    .build();
        }

        String email = request.email().toLowerCase().trim();
        int atIndex = email.indexOf('@');
        if (atIndex < 0) {
            return Response.status(Response.Status.BAD_REQUEST)
                    .entity(new ErrorResponse("Invalid email format"))
                    .build();
        }

        String domain = email.substring(atIndex + 1);
        LOG.debugf("Checking auth method for domain: %s", domain);

        // Look up auth config for this domain
        Optional<ClientAuthConfig> configOpt = authConfigRepository.findByEmailDomain(domain);

        if (configOpt.isEmpty()) {
            // Default to internal auth if no config
            LOG.debugf("No auth config for domain %s, defaulting to internal", domain);
            return Response.ok(new DomainCheckResponse("internal", null, null)).build();
        }

        ClientAuthConfig config = configOpt.get();

        if (config.authProvider == AuthProvider.OIDC && config.oidcIssuerUrl != null) {
            // Return the FlowCatalyst OIDC login URL (not the external IDP URL directly)
            String loginUrl = "/auth/oidc/login?domain=" + domain;
            LOG.debugf("Domain %s uses OIDC, login URL: %s", domain, loginUrl);
            return Response.ok(new DomainCheckResponse("external", loginUrl, config.oidcIssuerUrl)).build();
        }

        // Internal auth
        return Response.ok(new DomainCheckResponse("internal", null, null)).build();
    }

    // DTOs

    public record DomainCheckRequest(String email) {}

    /**
     * Response for domain authentication check.
     *
     * @param authMethod "internal" for password auth, "external" for OIDC
     * @param loginUrl The URL to redirect to for login (for external: /auth/oidc/login?domain=...)
     * @param idpIssuer The external IDP issuer URL (informational, for external auth only)
     */
    public record DomainCheckResponse(String authMethod, String loginUrl, String idpIssuer) {}

    public record ErrorResponse(String error) {}
}
