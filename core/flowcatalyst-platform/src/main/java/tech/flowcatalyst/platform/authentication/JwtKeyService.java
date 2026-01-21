package tech.flowcatalyst.platform.authentication;

import io.smallrye.jwt.build.Jwt;
import jakarta.annotation.PostConstruct;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.json.Json;
import jakarta.json.JsonArrayBuilder;
import jakarta.json.JsonObject;
import org.eclipse.microprofile.config.inject.ConfigProperty;
import org.jboss.logging.Logger;

import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Path;
import tech.flowcatalyst.platform.authorization.PermissionRegistry;

import java.security.*;
import java.security.interfaces.RSAPrivateKey;
import java.security.interfaces.RSAPublicKey;
import java.security.spec.PKCS8EncodedKeySpec;
import java.security.spec.X509EncodedKeySpec;
import java.time.Duration;
import java.time.Instant;
import java.util.Base64;
import java.util.List;
import java.util.Optional;
import java.util.Set;
import java.util.UUID;

/**
 * Service for managing JWT signing keys and generating tokens.
 *
 * Supports two modes:
 * 1. Auto-generated keys (development) - generates RSA key pair on startup
 * 2. File-based keys (production) - loads keys from configured paths
 *
 * Provides JWKS (JSON Web Key Set) for token verification by other services.
 */
@ApplicationScoped
public class JwtKeyService {

    private static final Logger LOG = Logger.getLogger(JwtKeyService.class);
    private static final String ALGORITHM = "RS256";
    private static final int KEY_SIZE = 2048;

    @ConfigProperty(name = "flowcatalyst.auth.jwt.issuer", defaultValue = "flowcatalyst")
    String issuer;

    @ConfigProperty(name = "flowcatalyst.auth.jwt.private-key-path")
    Optional<String> privateKeyPath;

    @ConfigProperty(name = "flowcatalyst.auth.jwt.public-key-path")
    Optional<String> publicKeyPath;

    @ConfigProperty(name = "flowcatalyst.auth.jwt.access-token-expiry", defaultValue = "PT1H")
    Duration accessTokenExpiry;

    @ConfigProperty(name = "flowcatalyst.auth.jwt.session-token-expiry", defaultValue = "PT24H")
    Duration sessionTokenExpiry;

    @ConfigProperty(name = "flowcatalyst.auth.jwt.dev-key-dir", defaultValue = ".jwt-keys")
    String devKeyDir;

    private RSAPrivateKey privateKey;
    private RSAPublicKey publicKey;
    private String keyId;

    @PostConstruct
    void init() {
        try {
            if (privateKeyPath.isPresent() && publicKeyPath.isPresent()) {
                loadKeysFromFiles();
            } else {
                // In dev mode, try to load persisted keys or generate new ones
                loadOrGenerateDevKeys();
            }
            // Generate a stable key ID based on public key
            this.keyId = generateKeyId(publicKey);
            LOG.infof("JWT key service initialized with key ID: %s", keyId);
        } catch (Exception e) {
            throw new RuntimeException("Failed to initialize JWT keys", e);
        }
    }

    /**
     * Load dev keys from local directory, or generate and persist new ones.
     * This ensures sessions survive backend restarts during development.
     */
    private void loadOrGenerateDevKeys() throws Exception {
        Path keyDir = Path.of(devKeyDir);
        Path privateKeyFile = keyDir.resolve("private.key");
        Path publicKeyFile = keyDir.resolve("public.key");

        if (Files.exists(privateKeyFile) && Files.exists(publicKeyFile)) {
            LOG.info("Loading persisted dev JWT keys from " + devKeyDir);
            loadKeysFromPaths(privateKeyFile, publicKeyFile);
        } else {
            LOG.info("Generating new dev JWT keys (will be persisted to " + devKeyDir + ")");
            generateKeyPair();
            persistDevKeys(keyDir, privateKeyFile, publicKeyFile);
        }
        LOG.warn("Using dev JWT keys. Configure flowcatalyst.auth.jwt.private-key-path and flowcatalyst.auth.jwt.public-key-path for production.");
    }

    private void loadKeysFromPaths(Path privateKeyFile, Path publicKeyFile) throws Exception {
        byte[] privateKeyBytes = Files.readAllBytes(privateKeyFile);
        byte[] publicKeyBytes = Files.readAllBytes(publicKeyFile);

        KeyFactory keyFactory = KeyFactory.getInstance("RSA");
        this.privateKey = (RSAPrivateKey) keyFactory.generatePrivate(new PKCS8EncodedKeySpec(privateKeyBytes));
        this.publicKey = (RSAPublicKey) keyFactory.generatePublic(new X509EncodedKeySpec(publicKeyBytes));
    }

    private void persistDevKeys(Path keyDir, Path privateKeyFile, Path publicKeyFile) throws IOException {
        Files.createDirectories(keyDir);
        Files.write(privateKeyFile, privateKey.getEncoded());
        Files.write(publicKeyFile, publicKey.getEncoded());
        LOG.infof("Dev JWT keys persisted to %s", keyDir);
    }

    private void loadKeysFromFiles() throws Exception {
        LOG.info("Loading JWT keys from files");

        byte[] privateKeyBytes = Files.readAllBytes(Path.of(privateKeyPath.get()));
        byte[] publicKeyBytes = Files.readAllBytes(Path.of(publicKeyPath.get()));

        // Handle PEM format
        String privateKeyPem = new String(privateKeyBytes);
        String publicKeyPem = new String(publicKeyBytes);

        privateKeyBytes = parsePemKey(privateKeyPem, "PRIVATE KEY");
        publicKeyBytes = parsePemKey(publicKeyPem, "PUBLIC KEY");

        KeyFactory keyFactory = KeyFactory.getInstance("RSA");
        this.privateKey = (RSAPrivateKey) keyFactory.generatePrivate(new PKCS8EncodedKeySpec(privateKeyBytes));
        this.publicKey = (RSAPublicKey) keyFactory.generatePublic(new X509EncodedKeySpec(publicKeyBytes));
    }

    private byte[] parsePemKey(String pem, String type) {
        String base64 = pem
                .replace("-----BEGIN " + type + "-----", "")
                .replace("-----END " + type + "-----", "")
                .replaceAll("\\s", "");
        return Base64.getDecoder().decode(base64);
    }

    private void generateKeyPair() throws NoSuchAlgorithmException {
        LOG.info("Generating JWT key pair");
        KeyPairGenerator keyGen = KeyPairGenerator.getInstance("RSA");
        keyGen.initialize(KEY_SIZE, new SecureRandom());
        KeyPair keyPair = keyGen.generateKeyPair();
        this.privateKey = (RSAPrivateKey) keyPair.getPrivate();
        this.publicKey = (RSAPublicKey) keyPair.getPublic();
    }

    private String generateKeyId(RSAPublicKey key) {
        try {
            MessageDigest digest = MessageDigest.getInstance("SHA-256");
            byte[] hash = digest.digest(key.getEncoded());
            return Base64.getUrlEncoder().withoutPadding().encodeToString(hash).substring(0, 8);
        } catch (NoSuchAlgorithmException e) {
            return UUID.randomUUID().toString().substring(0, 8);
        }
    }

    /**
     * Issue an access token for a service account (client credentials).
     */
    public String issueAccessToken(String principalId, String clientId, Set<String> roles) {
        return Jwt.issuer(issuer)
                .subject(String.valueOf(principalId))
                .claim("client_id", clientId)
                .claim("type", "SERVICE")
                .claim("roles", roles)
                .issuedAt(Instant.now())
                .expiresAt(Instant.now().plus(accessTokenExpiry))
                .jws()
                .keyId(keyId)
                .sign(privateKey);
    }

    /**
     * Issue a session token for a human user.
     *
     * @param principalId The principal ID
     * @param email The user's email
     * @param roles The user's role strings
     * @param clients List of client IDs the user can access, or ["*"] for all
     */
    public String issueSessionToken(String principalId, String email, Set<String> roles, List<String> clients) {
        // Extract application codes from roles
        Set<String> applications = PermissionRegistry.extractApplicationCodes(roles);

        // Filter out any null values from collections (JWT builder doesn't handle nulls)
        Set<String> safeRoles = roles != null ? roles.stream().filter(java.util.Objects::nonNull).collect(java.util.stream.Collectors.toSet()) : Set.of();
        Set<String> safeApplications = applications != null ? applications.stream().filter(java.util.Objects::nonNull).collect(java.util.stream.Collectors.toSet()) : Set.of();
        List<String> safeClients = clients != null ? clients.stream().filter(java.util.Objects::nonNull).toList() : List.of();

        return Jwt.issuer(issuer)
                .subject(String.valueOf(principalId))
                .claim("email", email)
                .claim("type", "USER")
                .claim("clients", safeClients)
                .claim("applications", safeApplications)
                .claim("roles", safeRoles)
                .issuedAt(Instant.now())
                .expiresAt(Instant.now().plus(sessionTokenExpiry))
                .jws()
                .keyId(keyId)
                .sign(privateKey);
    }

    /**
     * Issue an OIDC ID token.
     *
     * ID tokens are meant for the client application to verify the user's identity.
     * They contain user identity claims but not authorization claims like roles.
     *
     * @param principalId The principal ID (sub claim)
     * @param email The user's email
     * @param name The user's display name
     * @param audience The client_id of the requesting application (aud claim)
     * @param nonce The nonce from the authorization request (for replay protection)
     * @param clients List of client IDs the user can access
     * @return Signed ID token
     */
    public String issueIdToken(String principalId, String email, String name,
            String audience, String nonce, List<String> clients) {
        var builder = Jwt.issuer(issuer)
                .subject(String.valueOf(principalId))
                .audience(audience)
                .claim("email", email)
                .claim("clients", clients)
                .issuedAt(Instant.now())
                .expiresAt(Instant.now().plus(sessionTokenExpiry));

        if (name != null) {
            builder.claim("name", name);
        }

        if (nonce != null) {
            builder.claim("nonce", nonce);
        }

        return builder.jws()
                .keyId(keyId)
                .sign(privateKey);
    }

    /**
     * Issue a session token with client context.
     *
     * This token includes:
     * - client_id: The selected client context
     * - roles: User's role strings
     * - permissions: Resolved permissions from roles
     * - applications: Application codes extracted from roles
     *
     * @param principalId The principal ID
     * @param email The user's email
     * @param roles The user's role strings
     * @param permissions The resolved permission strings
     * @param clientId The client context
     * @return Signed JWT token
     */
    public String issueSessionTokenWithClient(String principalId, String email,
            Set<String> roles, Set<String> permissions, String clientId) {
        // Extract application codes from roles
        Set<String> applications = PermissionRegistry.extractApplicationCodes(roles);

        var jwtBuilder = Jwt.issuer(issuer)
                .subject(String.valueOf(principalId))
                .claim("email", email)
                .claim("type", "USER")
                .claim("applications", applications)
                .claim("roles", roles);

        // Add client context
        if (clientId != null) {
            jwtBuilder.claim("client_id", clientId);
        }

        // Add permissions as a claim
        if (permissions != null && !permissions.isEmpty()) {
            jwtBuilder.claim("permissions", permissions);
        }

        return jwtBuilder
                .issuedAt(Instant.now())
                .expiresAt(Instant.now().plus(sessionTokenExpiry))
                .jws()
                .keyId(keyId)
                .sign(privateKey);
    }

    /**
     * Issue a session token with full context (client + permissions).
     * Convenience method that resolves permissions from roles using PermissionRegistry.
     */
    public String issueSessionTokenWithClientAndPermissions(String principalId, String email,
            Set<String> roles, String clientId,
            tech.flowcatalyst.platform.authorization.PermissionRegistry permissionRegistry) {
        Set<String> permissions = permissionRegistry.getPermissionsForRoles(roles);
        return issueSessionTokenWithClient(principalId, email, roles, permissions, clientId);
    }

    /**
     * Get the JWKS (JSON Web Key Set) for token verification.
     */
    public JsonObject getJwks() {
        JsonArrayBuilder keysArray = Json.createArrayBuilder();
        keysArray.add(getJwk());
        return Json.createObjectBuilder()
                .add("keys", keysArray)
                .build();
    }

    /**
     * Get the JWK (JSON Web Key) for the current public key.
     */
    public JsonObject getJwk() {
        byte[] nBytes = publicKey.getModulus().toByteArray();
        byte[] eBytes = publicKey.getPublicExponent().toByteArray();

        // Remove leading zero byte if present (BigInteger sign bit)
        if (nBytes[0] == 0) {
            byte[] tmp = new byte[nBytes.length - 1];
            System.arraycopy(nBytes, 1, tmp, 0, tmp.length);
            nBytes = tmp;
        }

        return Json.createObjectBuilder()
                .add("kty", "RSA")
                .add("alg", ALGORITHM)
                .add("use", "sig")
                .add("kid", keyId)
                .add("n", Base64.getUrlEncoder().withoutPadding().encodeToString(nBytes))
                .add("e", Base64.getUrlEncoder().withoutPadding().encodeToString(eBytes))
                .build();
    }

    /**
     * Get the OpenID Connect discovery document.
     */
    public JsonObject getOpenIdConfiguration(String baseUrl) {
        return Json.createObjectBuilder()
                .add("issuer", issuer)
                .add("authorization_endpoint", baseUrl + "/oauth/authorize")
                .add("token_endpoint", baseUrl + "/oauth/token")
                .add("jwks_uri", baseUrl + "/.well-known/jwks.json")
                .add("response_types_supported", Json.createArrayBuilder()
                        .add("code")
                        .add("token")
                        .add("id_token")
                        .add("code id_token"))
                .add("grant_types_supported", Json.createArrayBuilder()
                        .add("authorization_code")
                        .add("refresh_token")
                        .add("client_credentials")
                        .add("password"))
                .add("scopes_supported", Json.createArrayBuilder()
                        .add("openid")
                        .add("profile")
                        .add("email"))
                .add("token_endpoint_auth_methods_supported", Json.createArrayBuilder()
                        .add("client_secret_basic")
                        .add("client_secret_post"))
                .add("code_challenge_methods_supported", Json.createArrayBuilder()
                        .add("S256")
                        .add("plain"))
                .add("subject_types_supported", Json.createArrayBuilder().add("public"))
                .add("id_token_signing_alg_values_supported", Json.createArrayBuilder().add(ALGORITHM))
                .add("claims_supported", Json.createArrayBuilder()
                        .add("sub")
                        .add("iss")
                        .add("aud")
                        .add("exp")
                        .add("iat")
                        .add("nonce")
                        .add("email")
                        .add("name")
                        .add("clients"))
                .build();
    }

    public String getIssuer() {
        return issuer;
    }

    public String getKeyId() {
        return keyId;
    }

    public Duration getAccessTokenExpiry() {
        return accessTokenExpiry;
    }

    public Duration getSessionTokenExpiry() {
        return sessionTokenExpiry;
    }

    public RSAPublicKey getPublicKey() {
        return publicKey;
    }

    public RSAPrivateKey getPrivateKey() {
        return privateKey;
    }

    /**
     * Extract token from session cookie or Authorization header, then validate and return principal ID.
     * This is the preferred method for resources to use - consolidates the common pattern of
     * checking cookie first, then Bearer token.
     *
     * @param sessionToken The session token from cookie (may be null)
     * @param authHeader The Authorization header value (may be null)
     * @return Optional containing the principal ID if valid, empty otherwise
     */
    public Optional<String> extractAndValidatePrincipalId(String sessionToken, String authHeader) {
        String token = sessionToken;
        if (token == null && authHeader != null && authHeader.startsWith("Bearer ")) {
            token = authHeader.substring("Bearer ".length());
        }
        if (token == null) {
            return Optional.empty();
        }
        return Optional.ofNullable(validateAndGetPrincipalId(token));
    }

    /**
     * Validate a token and extract the principal ID.
     * Returns null if the token is invalid or expired.
     */
    public String validateAndGetPrincipalId(String token) {
        try {
            io.smallrye.jwt.auth.principal.JWTParser parser = new io.smallrye.jwt.auth.principal.DefaultJWTParser();
            org.eclipse.microprofile.jwt.JsonWebToken jwt = parser.verify(token, publicKey);

            // Verify issuer
            if (!issuer.equals(jwt.getIssuer())) {
                LOG.debugf("Token issuer mismatch: expected %s, got %s", issuer, jwt.getIssuer());
                return null;
            }

            // Verify not expired
            if (jwt.getExpirationTime() < System.currentTimeMillis() / 1000) {
                LOG.debug("Token expired");
                return null;
            }

            return jwt.getSubject();
        } catch (Exception e) {
            LOG.debugf("Token validation failed: %s", e.getMessage());
            return null;
        }
    }

    /**
     * Extract the client ID from a token.
     * Returns null if token is invalid or has no client claim.
     */
    public String extractClientId(String token) {
        if (token == null) {
            return null;
        }
        try {
            io.smallrye.jwt.auth.principal.JWTParser parser = new io.smallrye.jwt.auth.principal.DefaultJWTParser();
            org.eclipse.microprofile.jwt.JsonWebToken jwt = parser.verify(token, publicKey);

            Object clientClaim = jwt.getClaim("client_id");
            if (clientClaim == null) {
                return null;
            }
            if (clientClaim instanceof Long) {
                return String.valueOf(clientClaim);
            }
            if (clientClaim instanceof Number) {
                return String.valueOf((Number) clientClaim);
            }
            return clientClaim.toString();
        } catch (Exception e) {
            LOG.debugf("Failed to extract client_id from token: %s", e.getMessage());
            return null;
        }
    }

    /**
     * Extract permissions from a token.
     * Returns empty set if token is invalid or has no permissions claim.
     */
    @SuppressWarnings("unchecked")
    public Set<String> extractPermissions(String token) {
        if (token == null) {
            return Set.of();
        }
        try {
            io.smallrye.jwt.auth.principal.JWTParser parser = new io.smallrye.jwt.auth.principal.DefaultJWTParser();
            org.eclipse.microprofile.jwt.JsonWebToken jwt = parser.verify(token, publicKey);

            Object permissionsClaim = jwt.getClaim("permissions");
            if (permissionsClaim == null) {
                return Set.of();
            }
            if (permissionsClaim instanceof Set) {
                return (Set<String>) permissionsClaim;
            }
            if (permissionsClaim instanceof java.util.Collection) {
                return new java.util.HashSet<>((java.util.Collection<String>) permissionsClaim);
            }
            return Set.of();
        } catch (Exception e) {
            LOG.debugf("Failed to extract permissions from token: %s", e.getMessage());
            return Set.of();
        }
    }

    /**
     * Extract applications from a token.
     * Returns empty set if token is invalid or has no applications claim.
     */
    @SuppressWarnings("unchecked")
    public Set<String> extractApplications(String token) {
        if (token == null) {
            return Set.of();
        }
        try {
            io.smallrye.jwt.auth.principal.JWTParser parser = new io.smallrye.jwt.auth.principal.DefaultJWTParser();
            org.eclipse.microprofile.jwt.JsonWebToken jwt = parser.verify(token, publicKey);

            Object applicationsClaim = jwt.getClaim("applications");
            if (applicationsClaim == null) {
                return Set.of();
            }
            if (applicationsClaim instanceof Set) {
                return (Set<String>) applicationsClaim;
            }
            if (applicationsClaim instanceof java.util.Collection) {
                return new java.util.HashSet<>((java.util.Collection<String>) applicationsClaim);
            }
            return Set.of();
        } catch (Exception e) {
            LOG.debugf("Failed to extract applications from token: %s", e.getMessage());
            return Set.of();
        }
    }

    /**
     * Extract clients from a token.
     * Returns list of client IDs, or ["*"] for all clients access.
     * Returns empty list if token is invalid or has no clients claim.
     */
    public List<String> extractClients(String token) {
        if (token == null) {
            return List.of();
        }
        try {
            io.smallrye.jwt.auth.principal.JWTParser parser = new io.smallrye.jwt.auth.principal.DefaultJWTParser();
            org.eclipse.microprofile.jwt.JsonWebToken jwt = parser.verify(token, publicKey);

            Object clientsClaim = jwt.getClaim("clients");
            if (clientsClaim == null) {
                return List.of();
            }
            // Convert elements to actual Strings to handle JsonStringImpl from JWT parser
            if (clientsClaim instanceof java.util.Collection<?> collection) {
                return collection.stream()
                    .map(Object::toString)
                    .toList();
            }
            return List.of();
        } catch (Exception e) {
            LOG.debugf("Failed to extract clients from token: %s", e.getMessage());
            return List.of();
        }
    }

    /**
     * Extract clients from session cookie or Authorization header.
     */
    public List<String> extractClients(String sessionToken, String authHeader) {
        String token = sessionToken;
        if (token == null && authHeader != null && authHeader.startsWith("Bearer ")) {
            token = authHeader.substring("Bearer ".length());
        }
        if (token == null) {
            return List.of();
        }
        return extractClients(token);
    }
}
