package tech.flowcatalyst.platform.authentication;

import jakarta.inject.Inject;
import jakarta.json.JsonObject;
import jakarta.ws.rs.GET;
import jakarta.ws.rs.Path;
import jakarta.ws.rs.Produces;
import jakarta.ws.rs.core.Context;
import jakarta.ws.rs.core.MediaType;
import jakarta.ws.rs.core.UriInfo;
import org.eclipse.microprofile.openapi.annotations.Operation;
import org.eclipse.microprofile.openapi.annotations.responses.APIResponse;
import org.eclipse.microprofile.openapi.annotations.tags.Tag;

/**
 * Well-known endpoints for OAuth2/OIDC discovery.
 * Enables other services to discover and validate tokens.
 *
 * This resource is only available in embedded mode.
 * In remote mode, discovery endpoints should be fetched from the external IdP.
 */
@Path("/.well-known")
@Tag(name = "Discovery", description = "OAuth2/OIDC discovery endpoints")
@Produces(MediaType.APPLICATION_JSON)
@EmbeddedModeOnly
public class WellKnownResource {

    @Inject
    JwtKeyService jwtKeyService;

    @Context
    UriInfo uriInfo;

    /**
     * JSON Web Key Set (JWKS) endpoint.
     * Returns the public keys used to verify tokens.
     */
    @GET
    @Path("/jwks.json")
    @Operation(summary = "Get JSON Web Key Set for token verification")
    @APIResponse(responseCode = "200", description = "JWKS document")
    public JsonObject jwks() {
        return jwtKeyService.getJwks();
    }

    /**
     * OpenID Connect Discovery endpoint.
     * Returns metadata about the authorization server.
     */
    @GET
    @Path("/openid-configuration")
    @Operation(summary = "Get OpenID Connect discovery document")
    @APIResponse(responseCode = "200", description = "OpenID configuration")
    public JsonObject openIdConfiguration() {
        String baseUrl = getBaseUrl();
        return jwtKeyService.getOpenIdConfiguration(baseUrl);
    }

    private String getBaseUrl() {
        return uriInfo.getBaseUri().toString().replaceAll("/$", "");
    }
}
