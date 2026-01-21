package tech.flowcatalyst.platform.config;

import com.fasterxml.jackson.databind.ObjectMapper;
import jakarta.inject.Inject;
import jakarta.ws.rs.*;
import jakarta.ws.rs.core.MediaType;
import jakarta.ws.rs.core.Response;
import org.eclipse.microprofile.openapi.annotations.Operation;
import org.eclipse.microprofile.openapi.annotations.responses.APIResponse;
import org.eclipse.microprofile.openapi.annotations.responses.APIResponses;
import org.eclipse.microprofile.openapi.annotations.tags.Tag;
import org.jboss.logging.Logger;

/**
 * Public (unauthenticated) API for retrieving login theme configuration.
 *
 * This endpoint returns the login page theming configuration that the UI
 * needs before the user authenticates. It does not require authentication.
 *
 * The theme is stored as a single JSON value at: platform.login.theme
 */
@Path("/api/public")
@Produces(MediaType.APPLICATION_JSON)
@Tag(name = "Public Config", description = "Public configuration endpoints")
public class PublicConfigResource {

    private static final Logger LOG = Logger.getLogger(PublicConfigResource.class);

    private static final String APP_CODE = "platform";
    private static final String SECTION = "login";
    private static final String PROPERTY = "theme";

    private static final LoginThemeResponse DEFAULT_THEME = new LoginThemeResponse(
        "FlowCatalyst",
        "Platform Administration",
        null,
        null,
        "#102a43",
        "#0967d2",
        "#0a1929",
        "linear-gradient(135deg, #102a43 0%, #0a1929 100%)",
        "Secure access to your FlowCatalyst platform",
        null
    );

    @Inject
    PlatformConfigService configService;

    @Inject
    ObjectMapper objectMapper;

    @GET
    @Path("/login-theme")
    @Operation(
        summary = "Get login page theme",
        description = "Returns theme configuration for the login page. No authentication required."
    )
    @APIResponses({
        @APIResponse(responseCode = "200", description = "Login theme configuration")
    })
    public Response getLoginTheme(@QueryParam("clientId") String clientId) {
        // Try to get the theme JSON from config (client-specific falls back to global)
        var themeJson = configService.getValueWithFallback(
            APP_CODE, SECTION, PROPERTY, clientId, false
        );

        if (themeJson.isEmpty()) {
            return Response.ok(DEFAULT_THEME).build();
        }

        try {
            // Parse the JSON and merge with defaults
            LoginThemeResponse stored = objectMapper.readValue(themeJson.get(), LoginThemeResponse.class);
            LoginThemeResponse merged = mergeWithDefaults(stored);
            return Response.ok(merged).build();
        } catch (Exception e) {
            LOG.warnf("Failed to parse login theme JSON, using defaults: %s", e.getMessage());
            return Response.ok(DEFAULT_THEME).build();
        }
    }

    private LoginThemeResponse mergeWithDefaults(LoginThemeResponse stored) {
        return new LoginThemeResponse(
            stored.brandName() != null ? stored.brandName() : DEFAULT_THEME.brandName(),
            stored.brandSubtitle() != null ? stored.brandSubtitle() : DEFAULT_THEME.brandSubtitle(),
            stored.logoUrl(),
            stored.logoSvg(),
            stored.primaryColor() != null ? stored.primaryColor() : DEFAULT_THEME.primaryColor(),
            stored.accentColor() != null ? stored.accentColor() : DEFAULT_THEME.accentColor(),
            stored.backgroundColor() != null ? stored.backgroundColor() : DEFAULT_THEME.backgroundColor(),
            stored.backgroundGradient() != null ? stored.backgroundGradient() : DEFAULT_THEME.backgroundGradient(),
            stored.footerText() != null ? stored.footerText() : DEFAULT_THEME.footerText(),
            stored.customCss()
        );
    }

    // ========================================================================
    // DTOs
    // ========================================================================

    public record LoginThemeResponse(
        String brandName,
        String brandSubtitle,
        String logoUrl,
        String logoSvg,
        String primaryColor,
        String accentColor,
        String backgroundColor,
        String backgroundGradient,
        String footerText,
        String customCss
    ) {}
}
