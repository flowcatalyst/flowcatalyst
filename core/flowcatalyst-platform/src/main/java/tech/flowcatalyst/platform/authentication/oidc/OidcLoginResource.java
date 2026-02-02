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
import tech.flowcatalyst.platform.authentication.EmbeddedModeOnly;
import tech.flowcatalyst.platform.authentication.JwtKeyService;
import tech.flowcatalyst.platform.authentication.domain.EmailDomainMapping;
import tech.flowcatalyst.platform.authentication.idp.IdentityProvider;
import tech.flowcatalyst.platform.authentication.idp.IdentityProviderService;
import tech.flowcatalyst.platform.authentication.idp.IdentityProviderType;
import tech.flowcatalyst.platform.client.Client;
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
    IdentityProviderService idpService;

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

    @Inject
    JwksService jwksService;

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

        // Look up authentication config (IDP + domain mapping) for this domain
        var authConfigOpt = idpService.findAuthConfigByEmailDomain(domain);
        if (authConfigOpt.isEmpty()) {
            return errorResponse(Response.Status.NOT_FOUND,
                "No authentication configuration found for domain: " + domain);
        }

        var authenticationConfig = authConfigOpt.get();
        var idp = authenticationConfig.identityProvider();
        var mapping = authenticationConfig.emailDomainMapping();

        if (idp.type != IdentityProviderType.OIDC) {
            return errorResponse(Response.Status.BAD_REQUEST,
                "Domain " + domain + " uses internal authentication, not OIDC");
        }

        if (idp.oidcIssuerUrl == null || idp.oidcClientId == null) {
            return errorResponse(Response.Status.INTERNAL_SERVER_ERROR,
                "OIDC configuration incomplete for domain: " + domain);
        }

        // Validate email domain is allowed by IDP
        if (!authenticationConfig.isEmailDomainAllowed()) {
            return errorResponse(Response.Status.FORBIDDEN,
                "Email domain " + domain + " is not allowed by this identity provider");
        }

        // Generate state and nonce
        String state = generateRandomString(32);
        String nonce = generateRandomString(32);
        String codeVerifier = generateCodeVerifier();
        String codeChallenge = generateCodeChallenge(codeVerifier);

        // Store state for callback validation
        var loginState = new OidcLoginState();
        loginState.state = state;
        loginState.emailDomain = domain;
        loginState.identityProviderId = idp.id;
        loginState.emailDomainMappingId = mapping.id;
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
        String authorizationUrl = buildAuthorizationUrl(idp, state, nonce, codeChallenge);

        LOG.infof("Redirecting to OIDC provider for domain %s: %s", domain, idp.oidcIssuerUrl);

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

        // Look up identity provider
        Optional<IdentityProvider> idpOpt = idpService.findById(loginState.identityProviderId);
        if (idpOpt.isEmpty()) {
            return errorRedirect("Identity provider no longer exists");
        }

        IdentityProvider idp = idpOpt.get();

        // Look up email domain mapping
        Optional<EmailDomainMapping> mappingOpt = idpService.findMappingById(loginState.emailDomainMappingId);
        if (mappingOpt.isEmpty()) {
            return errorRedirect("Email domain mapping no longer exists");
        }

        EmailDomainMapping mapping = mappingOpt.get();

        try {
            // Exchange code for tokens
            TokenResponse tokens = exchangeCodeForTokens(idp, code, loginState.codeVerifier);

            // Validate and parse ID token
            IdTokenClaims claims = parseAndValidateIdToken(tokens.idToken, idp, loginState.nonce);

            // Validate email domain matches and is allowed by IDP
            String emailDomain = extractEmailDomain(claims.email);
            if (!emailDomain.equals(loginState.emailDomain)) {
                LOG.warnf("Email domain mismatch: expected %s, got %s", loginState.emailDomain, emailDomain);
                return errorRedirect("Email domain does not match the login request");
            }

            if (!idp.isEmailDomainAllowed(emailDomain)) {
                LOG.warnf("Email domain %s not allowed by IDP %s", emailDomain, idp.code);
                return errorRedirect("Email domain is not allowed by this identity provider");
            }

            // Verify mapping IDP matches the one we used
            if (!mapping.identityProviderId.equals(idp.id)) {
                LOG.warnf("IDP mismatch: mapping points to %s, but used %s", mapping.identityProviderId, idp.id);
                return errorRedirect("Identity provider configuration mismatch");
            }

            // Validate OIDC tenant ID for multi-tenant identity providers
            // For multi-tenant IDPs, tenant ID validation is MANDATORY
            if (idp.oidcMultiTenant) {
                if (mapping.requiredOidcTenantId == null || mapping.requiredOidcTenantId.isBlank()) {
                    LOG.errorf("SECURITY: Multi-tenant IDP %s missing requiredOidcTenantId for domain %s",
                        idp.code, emailDomain);
                    return errorRedirect("Configuration error: tenant ID required for this identity provider");
                }
                if (claims.tenantId() == null) {
                    LOG.warnf("Tenant ID required but not in token for domain %s", emailDomain);
                    return errorRedirect("Authentication failed: tenant information not provided");
                }
                if (!mapping.requiredOidcTenantId.equals(claims.tenantId())) {
                    LOG.warnf("Tenant ID mismatch for %s: expected %s, got %s",
                        emailDomain, mapping.requiredOidcTenantId, claims.tenantId());
                    return errorRedirect("Authentication failed: unauthorized tenant");
                }
            } else if (mapping.requiredOidcTenantId != null && !mapping.requiredOidcTenantId.isBlank()) {
                // Single-tenant IDP with optional tenant ID check
                if (claims.tenantId() != null && !mapping.requiredOidcTenantId.equals(claims.tenantId())) {
                    LOG.warnf("Tenant ID mismatch for %s: expected %s, got %s",
                        emailDomain, mapping.requiredOidcTenantId, claims.tenantId());
                    return errorRedirect("Authentication failed: unauthorized tenant");
                }
            }

            // Find or create user
            Principal principal = findOrCreateUser(claims, mapping);

            // Load roles from embedded Principal.roles
            Set<String> roles = loadRoles(principal);

            // Determine accessible clients based on scope and mapping
            List<String> clients = determineAccessibleClients(principal, roles, mapping);

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

            LOG.infof("OIDC login successful for %s (principal %s) from IDP %s",
                claims.email, principal.id, idp.code);

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

    private TokenResponse exchangeCodeForTokens(IdentityProvider idp, String code, String codeVerifier)
            throws OidcException {

        String tokenEndpoint = getTokenEndpoint(idp.oidcIssuerUrl);
        String callbackUrl = getCallbackUrl();

        // Build token request
        StringBuilder body = new StringBuilder();
        body.append("grant_type=authorization_code");
        body.append("&code=").append(urlEncode(code));
        body.append("&redirect_uri=").append(urlEncode(callbackUrl));
        body.append("&client_id=").append(urlEncode(idp.oidcClientId));
        body.append("&code_verifier=").append(urlEncode(codeVerifier));

        // Add client secret if configured
        Optional<String> clientSecret = idpService.resolveClientSecret(idp);
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

    private IdTokenClaims parseAndValidateIdToken(String idToken, IdentityProvider idp, String expectedNonce)
            throws OidcException {

        try {
            // Split JWT
            String[] parts = idToken.split("\\.");
            if (parts.length != 3) {
                throw new OidcException("Invalid ID token format");
            }

            // Verify JWT signature using JWKS from the identity provider
            try {
                if (!jwksService.verifySignature(idToken, idp)) {
                    LOG.warnf("JWT signature verification failed for IDP %s", idp.code);
                    throw new OidcException("Invalid token signature");
                }
            } catch (JwksService.JwksException e) {
                LOG.errorf(e, "Failed to verify JWT signature for IDP %s", idp.code);
                throw new OidcException("Failed to verify token signature: " + e.getMessage());
            }

            // Decode and parse payload
            String payloadJson = new String(Base64.getUrlDecoder().decode(parts[1]), StandardCharsets.UTF_8);
            JsonNode payload = MAPPER.readTree(payloadJson);

            // Extract claims
            String issuer = payload.path("iss").asText(null);
            String subject = payload.path("sub").asText(null);
            String email = payload.path("email").asText(null);
            String name = payload.path("name").asText(null);
            String nonce = payload.path("nonce").asText(null);
            String tenantId = payload.path("tid").asText(null); // Azure AD/Entra tenant ID
            long exp = payload.path("exp").asLong(0);

            // Validate issuer
            if (!idp.isValidIssuer(issuer)) {
                LOG.warnf("Invalid issuer: expected pattern matching %s, got %s",
                    idp.getEffectiveIssuerPattern(), issuer);
                throw new OidcException("Invalid token issuer");
            }

            // Validate audience - aud can be string or array per OIDC spec
            JsonNode audNode = payload.path("aud");
            if (!isValidAudience(audNode, idp.oidcClientId)) {
                LOG.warnf("Invalid audience: expected %s, got %s", idp.oidcClientId, audNode);
                throw new OidcException("Invalid token audience");
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

            return new IdTokenClaims(issuer, subject, email.toLowerCase(), name, tenantId);

        } catch (OidcException e) {
            throw e;
        } catch (Exception e) {
            throw new OidcException("Failed to parse ID token: " + e.getMessage(), e);
        }
    }

    // ==================== User Management ====================

    private Principal findOrCreateUser(IdTokenClaims claims, EmailDomainMapping mapping) throws OidcException {
        try {
            // Determine scope based on mapping scope type
            var scope = mapping.scopeType.toUserScope();

            // Determine client ID to associate with user
            // - CLIENT type: Use the primary client as user's home client
            // - PARTNER type: No home client (access determined by mapping's grantedClientIds)
            // - ANCHOR type: No home client (access to all clients)
            String userClientId = switch (mapping.scopeType) {
                case CLIENT -> mapping.primaryClientId;
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

    private String buildAuthorizationUrl(IdentityProvider idp, String state, String nonce, String codeChallenge) {
        String authEndpoint = getAuthorizationEndpoint(idp.oidcIssuerUrl);
        String callbackUrl = getCallbackUrl();

        StringBuilder url = new StringBuilder(authEndpoint);
        url.append("?response_type=code");
        url.append("&client_id=").append(urlEncode(idp.oidcClientId));
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

    private String extractEmailDomain(String email) {
        if (email == null || !email.contains("@")) {
            return "";
        }
        return email.substring(email.indexOf("@") + 1).toLowerCase();
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
     * Determine which clients the user can access based on their scope and email domain mapping.
     *
     * @param principal The authenticated user principal
     * @param roles The user's roles
     * @param mapping The email domain mapping used for authentication
     * @return List of client entries as "id:identifier" strings, or ["*"] for anchor users
     */
    private List<String> determineAccessibleClients(Principal principal, Set<String> roles, EmailDomainMapping mapping) {
        // Use mapping scope type to determine accessible clients
        switch (mapping.scopeType) {
            case ANCHOR:
                return List.of("*");
            case CLIENT:
                // CLIENT type: primary client + additional clients
                return formatClientEntries(mapping.getAllAccessibleClientIds());
            case PARTNER:
                // PARTNER type: granted clients from mapping
                return formatClientEntries(mapping.getAllAccessibleClientIds());
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

    /**
     * Validate audience claim - can be string or array per OIDC spec.
     */
    private boolean isValidAudience(JsonNode audNode, String expectedClientId) {
        if (audNode == null || audNode.isMissingNode()) {
            return false;
        }
        if (audNode.isTextual()) {
            return audNode.asText().equals(expectedClientId);
        }
        if (audNode.isArray()) {
            for (JsonNode aud : audNode) {
                if (aud.asText().equals(expectedClientId)) {
                    return true;
                }
            }
        }
        return false;
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

    private record IdTokenClaims(String issuer, String subject, String email, String name, String tenantId) {}

    public static class OidcException extends Exception {
        public OidcException(String message) {
            super(message);
        }
        public OidcException(String message, Throwable cause) {
            super(message, cause);
        }
    }
}
