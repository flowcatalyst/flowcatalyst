package tech.flowcatalyst.platform.authentication.oidc;

import jakarta.inject.Inject;
import jakarta.transaction.Transactional;
import jakarta.ws.rs.*;
import jakarta.ws.rs.core.*;
import org.eclipse.microprofile.openapi.annotations.Operation;
import org.eclipse.microprofile.openapi.annotations.parameters.Parameter;
import org.eclipse.microprofile.openapi.annotations.tags.Tag;
import org.jboss.logging.Logger;
import tech.flowcatalyst.platform.authentication.AuthConfig;
import tech.flowcatalyst.platform.authentication.AuthProvider;
import tech.flowcatalyst.platform.authentication.EmbeddedModeOnly;
import tech.flowcatalyst.platform.authentication.JwtKeyService;
import tech.flowcatalyst.platform.client.Client;
import tech.flowcatalyst.platform.client.ClientAuthConfig;
import tech.flowcatalyst.platform.client.ClientAuthConfigService;
import tech.flowcatalyst.platform.client.ClientRepository;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.platform.principal.UserService;

import java.net.URI;
import java.net.URLEncoder;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.security.SecureRandom;
import java.time.Duration;
import java.util.*;
import java.util.stream.Collectors;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;

/**
 * OIDC Federation Login Resource.
 *
 * Handles login flows where FlowCatalyst acts as an OIDC client,
 * federating authentication to external identity providers (Entra ID, Keycloak, etc.)
 *
 * Flow:
 * 1. GET /auth/oidc/login?domain=example.com - Redirects to external IDP
 * 2. User authenticates at external IDP
 * 3. GET /auth/oidc/callback?code=...&state=... - Handles callback, creates session
 */
@Path("/auth/oidc")
@Tag(name = "OIDC Federation", description = "External identity provider login endpoints")
@Produces(MediaType.APPLICATION_JSON)
@EmbeddedModeOnly
public class OidcLoginResource {

    private static final Logger LOG = Logger.getLogger(OidcLoginResource.class);
    private static final SecureRandom SECURE_RANDOM = new SecureRandom();
    private static final ObjectMapper MAPPER = new ObjectMapper();

    @Inject
    ClientAuthConfigService authConfigService;

    @Inject
    OidcLoginStateRepository stateRepository;

    @Inject
    UserService userService;

    @Inject
    JwtKeyService jwtKeyService;

    @Inject
    AuthConfig authConfig;

    @Inject
    ClientRepository clientRepository;

    @Context
    UriInfo uriInfo;

    private final HttpClient httpClient = HttpClient.newBuilder()
        .connectTimeout(Duration.ofSeconds(10))
        .build();

    // ==================== Login Initiation ====================

    /**
     * Initiate OIDC login for a domain.
     * Redirects the user to the external identity provider.
     */
    @GET
    @Path("/login")
    @Operation(summary = "Start OIDC login", description = "Redirects to external IDP for authentication")
    @Transactional
    public Response login(
            @Parameter(description = "Email domain to authenticate", required = true)
            @QueryParam("domain") String domain,

            @Parameter(description = "URL to return to after login")
            @QueryParam("return_url") String returnUrl,

            // OAuth flow parameters (if login was triggered by /oauth/authorize)
            @QueryParam("oauth_client_id") String oauthClientId,
            @QueryParam("oauth_redirect_uri") String oauthRedirectUri,
            @QueryParam("oauth_scope") String oauthScope,
            @QueryParam("oauth_state") String oauthState,
            @QueryParam("oauth_code_challenge") String oauthCodeChallenge,
            @QueryParam("oauth_code_challenge_method") String oauthCodeChallengeMethod,
            @QueryParam("oauth_nonce") String oauthNonce
    ) {
        if (domain == null || domain.isBlank()) {
            return errorResponse(Response.Status.BAD_REQUEST, "domain parameter is required");
        }

        domain = domain.toLowerCase().trim();

        // Look up auth config for this domain
        Optional<ClientAuthConfig> configOpt = authConfigService.findByEmailDomain(domain);
        if (configOpt.isEmpty()) {
            return errorResponse(Response.Status.NOT_FOUND,
                "No authentication configuration found for domain: " + domain);
        }

        ClientAuthConfig config = configOpt.get();

        if (config.authProvider != AuthProvider.OIDC) {
            return errorResponse(Response.Status.BAD_REQUEST,
                "Domain " + domain + " uses internal authentication, not OIDC");
        }

        if (config.oidcIssuerUrl == null || config.oidcClientId == null) {
            return errorResponse(Response.Status.INTERNAL_SERVER_ERROR,
                "OIDC configuration incomplete for domain: " + domain);
        }

        // Generate state and nonce
        String state = generateRandomString(32);
        String nonce = generateRandomString(32);
        String codeVerifier = generateCodeVerifier();
        String codeChallenge = generateCodeChallenge(codeVerifier);

        // Store state for callback validation
        OidcLoginState loginState = new OidcLoginState();
        loginState.state = state;
        loginState.emailDomain = domain;
        loginState.authConfigId = config.id;
        loginState.nonce = nonce;
        loginState.codeVerifier = codeVerifier;
        loginState.returnUrl = returnUrl;
        loginState.oauthClientId = oauthClientId;
        loginState.oauthRedirectUri = oauthRedirectUri;
        loginState.oauthScope = oauthScope;
        loginState.oauthState = oauthState;
        loginState.oauthCodeChallenge = oauthCodeChallenge;
        loginState.oauthCodeChallengeMethod = oauthCodeChallengeMethod;
        loginState.oauthNonce = oauthNonce;

        stateRepository.persist(loginState);

        // Build authorization URL
        String authorizationUrl = buildAuthorizationUrl(config, state, nonce, codeChallenge);

        LOG.infof("Redirecting to OIDC provider for domain %s: %s", domain, config.oidcIssuerUrl);

        return Response.seeOther(URI.create(authorizationUrl)).build();
    }

    // ==================== Callback Handler ====================

    /**
     * Handle OIDC callback from external IDP.
     * Exchanges authorization code for tokens and creates local session.
     */
    @GET
    @Path("/callback")
    @Operation(summary = "OIDC callback", description = "Handles callback from external IDP")
    @Transactional
    public Response callback(
            @QueryParam("code") String code,
            @QueryParam("state") String state,
            @QueryParam("error") String error,
            @QueryParam("error_description") String errorDescription
    ) {
        // Handle IDP errors
        if (error != null) {
            LOG.warnf("OIDC callback error: %s - %s", error, errorDescription);
            return errorRedirect(errorDescription != null ? errorDescription : error);
        }

        if (code == null || code.isBlank()) {
            return errorRedirect("No authorization code received");
        }

        if (state == null || state.isBlank()) {
            return errorRedirect("No state parameter received");
        }

        // Validate state
        Optional<OidcLoginState> stateOpt = stateRepository.findValidState(state);
        if (stateOpt.isEmpty()) {
            LOG.warnf("Invalid or expired OIDC state: %s", state);
            return errorRedirect("Invalid or expired login session. Please try again.");
        }

        OidcLoginState loginState = stateOpt.get();

        // Delete state immediately (single use)
        stateRepository.deleteByState(state);

        // Look up auth config
        Optional<ClientAuthConfig> configOpt = authConfigService.findById(loginState.authConfigId);
        if (configOpt.isEmpty()) {
            return errorRedirect("Authentication configuration no longer exists");
        }

        ClientAuthConfig config = configOpt.get();

        try {
            // Exchange code for tokens
            TokenResponse tokens = exchangeCodeForTokens(config, code, loginState.codeVerifier);

            // Validate and parse ID token
            IdTokenClaims claims = parseAndValidateIdToken(tokens.idToken, config, loginState.nonce);

            // Find or create user
            Principal principal = findOrCreateUser(claims, config);

            // Load roles from embedded Principal.roles
            Set<String> roles = loadRoles(principal);

            // Determine accessible clients based on scope and config type
            List<String> clients = determineAccessibleClients(principal, roles, config);

            // Issue session token
            String sessionToken = jwtKeyService.issueSessionToken(
                principal.id,
                claims.email,
                roles,
                clients
            );

            // Build response with session cookie
            NewCookie sessionCookie = new NewCookie.Builder(authConfig.session().cookieName())
                .value(sessionToken)
                .path("/")
                .secure(authConfig.session().secure())
                .httpOnly(true)
                .sameSite(NewCookie.SameSite.valueOf(authConfig.session().sameSite().toUpperCase()))
                .maxAge((int) authConfig.jwt().sessionTokenExpiry().toSeconds())
                .build();

            // Determine redirect URL
            String redirectUrl = determineRedirectUrl(loginState);

            LOG.infof("OIDC login successful for %s (principal %s) from %s",
                claims.email, principal.id, config.oidcIssuerUrl);

            return Response.seeOther(URI.create(redirectUrl))
                .cookie(sessionCookie)
                .build();

        } catch (OidcException e) {
            LOG.errorf(e, "OIDC token exchange failed for domain %s", loginState.emailDomain);
            return errorRedirect(e.getMessage());
        } catch (Exception e) {
            LOG.errorf(e, "OIDC callback processing failed");
            return errorRedirect("Authentication failed. Please try again.");
        }
    }

    // ==================== Token Exchange ====================

    private TokenResponse exchangeCodeForTokens(ClientAuthConfig config, String code, String codeVerifier)
            throws OidcException {

        String tokenEndpoint = getTokenEndpoint(config.oidcIssuerUrl);
        String callbackUrl = getCallbackUrl();

        // Build token request
        StringBuilder body = new StringBuilder();
        body.append("grant_type=authorization_code");
        body.append("&code=").append(urlEncode(code));
        body.append("&redirect_uri=").append(urlEncode(callbackUrl));
        body.append("&client_id=").append(urlEncode(config.oidcClientId));
        body.append("&code_verifier=").append(urlEncode(codeVerifier));

        // Add client secret if configured
        Optional<String> clientSecret = authConfigService.resolveClientSecret(config);
        if (clientSecret.isPresent()) {
            body.append("&client_secret=").append(urlEncode(clientSecret.get()));
        }

        try {
            HttpRequest request = HttpRequest.newBuilder()
                .uri(URI.create(tokenEndpoint))
                .header("Content-Type", "application/x-www-form-urlencoded")
                .POST(HttpRequest.BodyPublishers.ofString(body.toString()))
                .timeout(Duration.ofSeconds(30))
                .build();

            HttpResponse<String> response = httpClient.send(request, HttpResponse.BodyHandlers.ofString());

            if (response.statusCode() != 200) {
                LOG.errorf("Token endpoint returned %d: %s", response.statusCode(), response.body());
                throw new OidcException("Failed to exchange authorization code");
            }

            JsonNode json = MAPPER.readTree(response.body());

            String accessToken = json.path("access_token").asText(null);
            String idToken = json.path("id_token").asText(null);
            String refreshToken = json.path("refresh_token").asText(null);

            if (idToken == null) {
                throw new OidcException("No ID token received from identity provider");
            }

            return new TokenResponse(accessToken, idToken, refreshToken);

        } catch (OidcException e) {
            throw e;
        } catch (Exception e) {
            throw new OidcException("Token exchange failed: " + e.getMessage(), e);
        }
    }

    // ==================== ID Token Validation ====================

    private IdTokenClaims parseAndValidateIdToken(String idToken, ClientAuthConfig config, String expectedNonce)
            throws OidcException {

        try {
            // Split JWT
            String[] parts = idToken.split("\\.");
            if (parts.length != 3) {
                throw new OidcException("Invalid ID token format");
            }

            // Decode payload (we're not validating signature here - that should be done with JWKS)
            // TODO: Implement proper signature validation using JWKS
            String payloadJson = new String(Base64.getUrlDecoder().decode(parts[1]), StandardCharsets.UTF_8);
            JsonNode payload = MAPPER.readTree(payloadJson);

            // Extract claims
            String issuer = payload.path("iss").asText(null);
            String subject = payload.path("sub").asText(null);
            String email = payload.path("email").asText(null);
            String name = payload.path("name").asText(null);
            String nonce = payload.path("nonce").asText(null);
            long exp = payload.path("exp").asLong(0);

            // Validate issuer
            if (!config.isValidIssuer(issuer)) {
                LOG.warnf("Invalid issuer: expected pattern matching %s, got %s",
                    config.getEffectiveIssuerPattern(), issuer);
                throw new OidcException("Invalid token issuer");
            }

            // Validate expiration
            if (exp * 1000 < System.currentTimeMillis()) {
                throw new OidcException("ID token has expired");
            }

            // Validate nonce
            if (expectedNonce != null && !expectedNonce.equals(nonce)) {
                throw new OidcException("Invalid nonce in ID token");
            }

            // Email is required
            if (email == null || email.isBlank()) {
                // Try preferred_username as fallback (common in Entra ID)
                email = payload.path("preferred_username").asText(null);
                if (email == null || email.isBlank()) {
                    throw new OidcException("No email claim in ID token");
                }
            }

            return new IdTokenClaims(issuer, subject, email.toLowerCase(), name);

        } catch (OidcException e) {
            throw e;
        } catch (Exception e) {
            throw new OidcException("Failed to parse ID token: " + e.getMessage(), e);
        }
    }

    // ==================== User Management ====================

    private Principal findOrCreateUser(IdTokenClaims claims, ClientAuthConfig config) throws OidcException {
        try {
            // Determine scope based on IDP config type
            // - ANCHOR: Platform-wide access, no client binding
            // - PARTNER: Partner IDP, access to granted clients stored on config
            // - CLIENT: Client-specific, bound to primary client
            tech.flowcatalyst.platform.principal.UserScope scope = config.getEffectiveConfigType().toUserScope();

            // Determine client ID to associate with user
            // - CLIENT type: Use the primary client as user's home client
            // - PARTNER type: No home client (access determined by IDP's grantedClientIds)
            // - ANCHOR type: No home client (access to all clients)
            String userClientId = switch (config.getEffectiveConfigType()) {
                case CLIENT -> config.getEffectivePrimaryClientId();
                case PARTNER, ANCHOR -> null;
            };

            // Use existing service method which handles both create and update
            return userService.createOrUpdateOidcUser(
                claims.email,
                claims.name,
                claims.subject,
                userClientId,
                scope
            );
        } catch (Exception e) {
            throw new OidcException("Failed to create or update user account: " + e.getMessage(), e);
        }
    }

    // ==================== Helper Methods ====================

    private String buildAuthorizationUrl(ClientAuthConfig config, String state, String nonce, String codeChallenge) {
        String authEndpoint = getAuthorizationEndpoint(config.oidcIssuerUrl);
        String callbackUrl = getCallbackUrl();

        StringBuilder url = new StringBuilder(authEndpoint);
        url.append("?response_type=code");
        url.append("&client_id=").append(urlEncode(config.oidcClientId));
        url.append("&redirect_uri=").append(urlEncode(callbackUrl));
        url.append("&scope=").append(urlEncode("openid profile email"));
        url.append("&state=").append(urlEncode(state));
        url.append("&nonce=").append(urlEncode(nonce));
        url.append("&code_challenge=").append(urlEncode(codeChallenge));
        url.append("&code_challenge_method=S256");

        return url.toString();
    }

    private String getAuthorizationEndpoint(String issuerUrl) {
        // For well-known IDPs, construct the endpoint
        if (issuerUrl.contains("login.microsoftonline.com")) {
            // Entra ID
            return issuerUrl.replace("/v2.0", "/oauth2/v2.0/authorize");
        }
        // Generic: append /authorize
        return issuerUrl + (issuerUrl.endsWith("/") ? "" : "/") + "authorize";
    }

    private String getTokenEndpoint(String issuerUrl) {
        if (issuerUrl.contains("login.microsoftonline.com")) {
            return issuerUrl.replace("/v2.0", "/oauth2/v2.0/token");
        }
        return issuerUrl + (issuerUrl.endsWith("/") ? "" : "/") + "token";
    }

    private String getExternalBaseUrl() {
        // Use configured external base URL if set, otherwise fall back to request context
        String baseUrl = authConfig.externalBaseUrl()
            .orElseGet(() -> uriInfo.getBaseUri().toString());

        // Ensure no trailing slash
        if (baseUrl.endsWith("/")) {
            baseUrl = baseUrl.substring(0, baseUrl.length() - 1);
        }

        return baseUrl;
    }

    private String getCallbackUrl() {
        return getExternalBaseUrl() + "/auth/oidc/callback";
    }

    private String determineRedirectUrl(OidcLoginState loginState) {
        String baseUrl = getExternalBaseUrl();

        // If this was part of an OAuth flow, redirect back to authorize endpoint
        if (loginState.oauthClientId != null) {
            StringBuilder url = new StringBuilder(baseUrl + "/oauth/authorize?");
            url.append("response_type=code");
            url.append("&client_id=").append(urlEncode(loginState.oauthClientId));
            if (loginState.oauthRedirectUri != null) {
                url.append("&redirect_uri=").append(urlEncode(loginState.oauthRedirectUri));
            }
            if (loginState.oauthScope != null) {
                url.append("&scope=").append(urlEncode(loginState.oauthScope));
            }
            if (loginState.oauthState != null) {
                url.append("&state=").append(urlEncode(loginState.oauthState));
            }
            if (loginState.oauthCodeChallenge != null) {
                url.append("&code_challenge=").append(urlEncode(loginState.oauthCodeChallenge));
            }
            if (loginState.oauthCodeChallengeMethod != null) {
                url.append("&code_challenge_method=").append(urlEncode(loginState.oauthCodeChallengeMethod));
            }
            if (loginState.oauthNonce != null) {
                url.append("&nonce=").append(urlEncode(loginState.oauthNonce));
            }
            return url.toString();
        }

        // Return to specified URL or default to dashboard
        if (loginState.returnUrl != null && !loginState.returnUrl.isBlank()) {
            // If returnUrl is relative, prepend base URL
            if (loginState.returnUrl.startsWith("/")) {
                return baseUrl + loginState.returnUrl;
            }
            return loginState.returnUrl;
        }

        return baseUrl + "/dashboard";
    }

    private String generateRandomString(int length) {
        byte[] bytes = new byte[length];
        SECURE_RANDOM.nextBytes(bytes);
        return Base64.getUrlEncoder().withoutPadding().encodeToString(bytes);
    }

    private String generateCodeVerifier() {
        byte[] bytes = new byte[32];
        SECURE_RANDOM.nextBytes(bytes);
        return Base64.getUrlEncoder().withoutPadding().encodeToString(bytes);
    }

    private String generateCodeChallenge(String verifier) {
        try {
            MessageDigest digest = MessageDigest.getInstance("SHA-256");
            byte[] hash = digest.digest(verifier.getBytes(StandardCharsets.US_ASCII));
            return Base64.getUrlEncoder().withoutPadding().encodeToString(hash);
        } catch (Exception e) {
            throw new RuntimeException("Failed to generate code challenge", e);
        }
    }

    private Set<String> loadRoles(Principal principal) {
        return principal.getRoleNames();
    }

    /**
     * Determine which clients the user can access based on their scope and IDP config.
     *
     * @param principal The authenticated user principal
     * @param roles The user's roles
     * @param config The IDP auth config used for authentication
     * @return List of client entries as "id:identifier" strings, or ["*"] for anchor users
     */
    private List<String> determineAccessibleClients(Principal principal, Set<String> roles, ClientAuthConfig config) {
        // Use config type to determine accessible clients
        switch (config.getEffectiveConfigType()) {
            case ANCHOR:
                return List.of("*");
            case CLIENT:
                // CLIENT type: primary client + additional clients
                return formatClientEntries(config.getAllAccessibleClientIds());
            case PARTNER:
                // PARTNER type: granted clients from IDP config
                return formatClientEntries(config.getAllAccessibleClientIds());
        }

        // Fallback: check roles for platform admins
        if (roles.stream().anyMatch(r -> r.contains("platform:admin") || r.contains("super-admin"))) {
            return List.of("*");
        }

        // User is bound to a specific client
        if (principal.clientId != null) {
            return formatClientEntries(List.of(principal.clientId));
        }

        // User has no specific client - could be a partner or unassigned
        return List.of();
    }

    /**
     * Format client IDs as "id:identifier" entries for the clients claim.
     * Falls back to just the ID if client not found or has no identifier.
     */
    private List<String> formatClientEntries(List<String> clientIds) {
        if (clientIds == null || clientIds.isEmpty()) {
            return List.of();
        }
        var clients = clientRepository.findByIds(Set.copyOf(clientIds));
        var clientMap = clients.stream()
            .collect(Collectors.toMap(c -> c.id, c -> c));

        return clientIds.stream()
            .map(id -> {
                Client client = clientMap.get(id);
                if (client != null && client.identifier != null) {
                    return id + ":" + client.identifier;
                }
                return id;
            })
            .toList();
    }

    private String urlEncode(String value) {
        return URLEncoder.encode(value, StandardCharsets.UTF_8);
    }

    private Response errorResponse(Response.Status status, String message) {
        return Response.status(status)
            .entity(Map.of("error", message))
            .type(MediaType.APPLICATION_JSON)
            .build();
    }

    private Response errorRedirect(String message) {
        // Redirect to frontend error page with message
        String errorUrl = "/?error=" + urlEncode(message);
        return Response.seeOther(URI.create(errorUrl)).build();
    }

    // ==================== Inner Classes ====================

    private record TokenResponse(String accessToken, String idToken, String refreshToken) {}

    private record IdTokenClaims(String issuer, String subject, String email, String name) {}

    public static class OidcException extends Exception {
        public OidcException(String message) {
            super(message);
        }
        public OidcException(String message, Throwable cause) {
            super(message, cause);
        }
    }
}
