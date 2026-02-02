package tech.flowcatalyst.platform.authentication.oidc;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import org.jboss.logging.Logger;
import tech.flowcatalyst.platform.authentication.idp.IdentityProvider;
import tech.flowcatalyst.platform.cache.CacheStore;

import java.math.BigInteger;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.nio.charset.StandardCharsets;
import java.security.KeyFactory;
import java.security.Signature;
import java.security.interfaces.RSAPublicKey;
import java.security.spec.RSAPublicKeySpec;
import java.time.Duration;
import java.util.Base64;
import java.util.Map;
import java.util.Optional;
import java.util.concurrent.ConcurrentHashMap;

/**
 * Service for fetching and caching JWKS (JSON Web Key Sets) from external identity providers.
 *
 * <p>Provides JWT signature verification using RSA public keys from OIDC providers.
 * JWKS are cached using the configured CacheStore backend for performance.
 *
 * <p>Key features:
 * <ul>
 *   <li>Automatic discovery of jwks_uri from OpenID Connect configuration</li>
 *   <li>Caching with configurable TTL (default 1 hour)</li>
 *   <li>Key rotation handling - refreshes cache on kid mismatch</li>
 *   <li>RS256 signature verification</li>
 * </ul>
 */
@ApplicationScoped
public class JwksService {

    private static final Logger LOG = Logger.getLogger(JwksService.class);
    private static final String CACHE_NAME = "jwks";
    private static final Duration CACHE_TTL = Duration.ofHours(1);
    private static final ObjectMapper MAPPER = new ObjectMapper();

    @Inject
    CacheStore cacheStore;

    private final HttpClient httpClient = HttpClient.newBuilder()
        .connectTimeout(Duration.ofSeconds(10))
        .build();

    // In-memory cache for parsed keys (CacheStore holds JSON, this holds parsed keys)
    private final Map<String, RSAPublicKey> parsedKeyCache = new ConcurrentHashMap<>();

    /**
     * Verify the signature of a JWT token using the identity provider's public keys.
     *
     * @param token The complete JWT token (header.payload.signature)
     * @param idp The identity provider configuration
     * @return true if the signature is valid
     * @throws JwksException if verification fails due to configuration or network issues
     */
    public boolean verifySignature(String token, IdentityProvider idp) throws JwksException {
        String[] parts = token.split("\\.");
        if (parts.length != 3) {
            throw new JwksException("Invalid JWT format - expected 3 parts");
        }

        try {
            // Decode header to get algorithm and key ID
            String headerJson = new String(Base64.getUrlDecoder().decode(parts[0]), StandardCharsets.UTF_8);
            JsonNode header = MAPPER.readTree(headerJson);

            String alg = header.path("alg").asText(null);
            String kid = header.path("kid").asText(null);

            // Only allow RS256 - prevent algorithm confusion attacks
            if (!"RS256".equals(alg)) {
                LOG.warnf("Rejecting token with unsupported algorithm: %s (only RS256 allowed)", alg);
                throw new JwksException("Unsupported JWT algorithm: " + alg + " (only RS256 allowed)");
            }

            if (kid == null || kid.isBlank()) {
                throw new JwksException("JWT missing key ID (kid) in header");
            }

            // Get the public key for this key ID
            RSAPublicKey publicKey = getPublicKey(idp, kid);

            // Verify signature
            String signedContent = parts[0] + "." + parts[1];
            byte[] signatureBytes = Base64.getUrlDecoder().decode(parts[2]);

            Signature sig = Signature.getInstance("SHA256withRSA");
            sig.initVerify(publicKey);
            sig.update(signedContent.getBytes(StandardCharsets.UTF_8));

            return sig.verify(signatureBytes);

        } catch (JwksException e) {
            throw e;
        } catch (Exception e) {
            throw new JwksException("Failed to verify JWT signature: " + e.getMessage(), e);
        }
    }

    /**
     * Get the RSA public key for a specific key ID from the identity provider.
     *
     * @param idp The identity provider
     * @param kid The key ID to look up
     * @return The RSA public key
     * @throws JwksException if the key cannot be found or fetched
     */
    public RSAPublicKey getPublicKey(IdentityProvider idp, String kid) throws JwksException {
        String cacheKey = idp.id + ":" + kid;

        // Check in-memory parsed key cache first
        RSAPublicKey cached = parsedKeyCache.get(cacheKey);
        if (cached != null) {
            return cached;
        }

        // Try to get JWKS from cache store
        String jwksJson = getJwksJson(idp, false);
        RSAPublicKey key = findKeyInJwks(jwksJson, kid);

        if (key == null) {
            // Key not found - might be key rotation, force refresh
            LOG.infof("Key ID %s not found in cached JWKS for IDP %s, refreshing", kid, idp.code);
            jwksJson = getJwksJson(idp, true);
            key = findKeyInJwks(jwksJson, kid);

            if (key == null) {
                throw new JwksException("Key ID " + kid + " not found in JWKS for identity provider: " + idp.code);
            }
        }

        // Cache the parsed key
        parsedKeyCache.put(cacheKey, key);
        return key;
    }

    /**
     * Get the JWKS JSON for an identity provider.
     */
    private String getJwksJson(IdentityProvider idp, boolean forceRefresh) throws JwksException {
        String cacheKey = idp.id;

        if (!forceRefresh) {
            Optional<String> cached = cacheStore.get(CACHE_NAME, cacheKey);
            if (cached.isPresent()) {
                LOG.debugf("JWKS cache hit for IDP: %s", idp.code);
                return cached.get();
            }
        }

        // Fetch fresh JWKS
        String jwksUri = getJwksUri(idp);
        LOG.infof("Fetching JWKS from %s for IDP %s", jwksUri, idp.code);

        try {
            HttpRequest request = HttpRequest.newBuilder()
                .uri(URI.create(jwksUri))
                .GET()
                .timeout(Duration.ofSeconds(30))
                .build();

            HttpResponse<String> response = httpClient.send(request, HttpResponse.BodyHandlers.ofString());

            if (response.statusCode() != 200) {
                throw new JwksException("Failed to fetch JWKS: HTTP " + response.statusCode());
            }

            String jwksJson = response.body();

            // Validate it's valid JWKS JSON
            JsonNode jwks = MAPPER.readTree(jwksJson);
            if (!jwks.has("keys") || !jwks.get("keys").isArray()) {
                throw new JwksException("Invalid JWKS format - missing 'keys' array");
            }

            // Cache the JWKS
            cacheStore.put(CACHE_NAME, cacheKey, jwksJson, CACHE_TTL);

            // Clear parsed key cache for this IDP (keys might have changed)
            parsedKeyCache.entrySet().removeIf(e -> e.getKey().startsWith(idp.id + ":"));

            return jwksJson;

        } catch (JwksException e) {
            throw e;
        } catch (Exception e) {
            throw new JwksException("Failed to fetch JWKS: " + e.getMessage(), e);
        }
    }

    /**
     * Get the JWKS URI for an identity provider.
     * First tries explicit jwksUri, then discovers from OpenID configuration.
     */
    private String getJwksUri(IdentityProvider idp) throws JwksException {
        // If explicit JWKS URI is configured, use it
        if (idp.oidcJwksUri != null && !idp.oidcJwksUri.isBlank()) {
            return idp.oidcJwksUri;
        }

        // Otherwise, discover from OpenID configuration
        return discoverJwksUri(idp.oidcIssuerUrl);
    }

    /**
     * Discover the JWKS URI from the OpenID configuration endpoint.
     */
    private String discoverJwksUri(String issuerUrl) throws JwksException {
        String configUrl = issuerUrl;
        if (!configUrl.endsWith("/")) {
            configUrl += "/";
        }
        configUrl += ".well-known/openid-configuration";

        try {
            HttpRequest request = HttpRequest.newBuilder()
                .uri(URI.create(configUrl))
                .GET()
                .timeout(Duration.ofSeconds(30))
                .build();

            HttpResponse<String> response = httpClient.send(request, HttpResponse.BodyHandlers.ofString());

            if (response.statusCode() != 200) {
                throw new JwksException("Failed to fetch OpenID configuration: HTTP " + response.statusCode());
            }

            JsonNode config = MAPPER.readTree(response.body());
            String jwksUri = config.path("jwks_uri").asText(null);

            if (jwksUri == null || jwksUri.isBlank()) {
                throw new JwksException("OpenID configuration missing jwks_uri");
            }

            return jwksUri;

        } catch (JwksException e) {
            throw e;
        } catch (Exception e) {
            throw new JwksException("Failed to discover JWKS URI: " + e.getMessage(), e);
        }
    }

    /**
     * Find and parse a specific key from JWKS JSON.
     */
    private RSAPublicKey findKeyInJwks(String jwksJson, String kid) throws JwksException {
        try {
            JsonNode jwks = MAPPER.readTree(jwksJson);
            JsonNode keys = jwks.get("keys");

            for (JsonNode key : keys) {
                String keyId = key.path("kid").asText(null);
                if (!kid.equals(keyId)) {
                    continue;
                }

                String kty = key.path("kty").asText(null);
                if (!"RSA".equals(kty)) {
                    LOG.warnf("Skipping non-RSA key with kid %s (kty=%s)", kid, kty);
                    continue;
                }

                String use = key.path("use").asText(null);
                if (use != null && !"sig".equals(use)) {
                    LOG.warnf("Skipping key with kid %s not intended for signing (use=%s)", kid, use);
                    continue;
                }

                return parseRsaPublicKey(key);
            }

            return null; // Key not found

        } catch (JwksException e) {
            throw e;
        } catch (Exception e) {
            throw new JwksException("Failed to parse JWKS: " + e.getMessage(), e);
        }
    }

    /**
     * Parse an RSA public key from a JWK JSON node.
     */
    private RSAPublicKey parseRsaPublicKey(JsonNode keyNode) throws JwksException {
        try {
            String n = keyNode.path("n").asText(null);
            String e = keyNode.path("e").asText(null);

            if (n == null || e == null) {
                throw new JwksException("RSA key missing modulus (n) or exponent (e)");
            }

            // Decode Base64URL encoded values
            BigInteger modulus = new BigInteger(1, Base64.getUrlDecoder().decode(n));
            BigInteger exponent = new BigInteger(1, Base64.getUrlDecoder().decode(e));

            RSAPublicKeySpec spec = new RSAPublicKeySpec(modulus, exponent);
            KeyFactory factory = KeyFactory.getInstance("RSA");
            return (RSAPublicKey) factory.generatePublic(spec);

        } catch (JwksException ex) {
            throw ex;
        } catch (Exception ex) {
            throw new JwksException("Failed to parse RSA public key: " + ex.getMessage(), ex);
        }
    }

    /**
     * Invalidate cached JWKS for an identity provider.
     * Useful when IDP configuration changes.
     */
    public void invalidateCache(String idpId) {
        cacheStore.invalidate(CACHE_NAME, idpId);
        parsedKeyCache.entrySet().removeIf(e -> e.getKey().startsWith(idpId + ":"));
        LOG.infof("Invalidated JWKS cache for IDP: %s", idpId);
    }

    /**
     * Exception for JWKS-related errors.
     */
    public static class JwksException extends Exception {
        public JwksException(String message) {
            super(message);
        }
        public JwksException(String message, Throwable cause) {
            super(message, cause);
        }
    }
}
