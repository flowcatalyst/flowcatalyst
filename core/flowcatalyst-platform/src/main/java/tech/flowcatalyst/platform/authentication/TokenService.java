package tech.flowcatalyst.platform.authentication;

import io.smallrye.jwt.build.Jwt;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import org.eclipse.microprofile.config.inject.ConfigProperty;
import tech.flowcatalyst.platform.principal.PrincipalType;

import java.time.Duration;
import java.time.Instant;
import java.util.Set;

/**
 * Service for JWT token generation and validation.
 * Used for service accounts and internal authentication tokens.
 *
 * Delegates to JwtKeyService for key management, ensuring all tokens
 * are signed with the same RSA key pair.
 */
@ApplicationScoped
public class TokenService {

    @Inject
    JwtKeyService jwtKeyService;

    @ConfigProperty(name = "flowcatalyst.auth.jwt.expiry", defaultValue = "365d")
    Duration defaultExpiry;

    /**
     * Issue a JWT token for a principal (user or service account).
     *
     * @param principalId The principal ID
     * @param principalType The principal type (USER or SERVICE)
     * @param expiry Token expiry duration (null for default)
     * @return JWT token string
     */
    public String issueToken(String principalId, PrincipalType principalType, Duration expiry) {
        if (expiry == null) {
            expiry = defaultExpiry;
        }

        return Jwt.issuer(jwtKeyService.getIssuer())
            .subject(String.valueOf(principalId))
            .claim("type", principalType.name())
            .issuedAt(Instant.now())
            .expiresAt(Instant.now().plus(expiry))
            .jws()
            .keyId(jwtKeyService.getKeyId())
            .sign(jwtKeyService.getPrivateKey());
    }

    /**
     * Issue a token with role claims for session management.
     *
     * @param principalId The principal ID
     * @param principalType The principal type
     * @param roles The set of role names
     * @param expiry Token expiry duration
     * @return JWT token string
     */
    public String issueTokenWithRoles(String principalId, PrincipalType principalType, Set<String> roles, Duration expiry) {
        if (expiry == null) {
            expiry = defaultExpiry;
        }

        return Jwt.issuer(jwtKeyService.getIssuer())
            .subject(String.valueOf(principalId))
            .claim("type", principalType.name())
            .groups(roles)
            .issuedAt(Instant.now())
            .expiresAt(Instant.now().plus(expiry))
            .jws()
            .keyId(jwtKeyService.getKeyId())
            .sign(jwtKeyService.getPrivateKey());
    }

    /**
     * Issue a service account token (long-lived).
     *
     * @param principalId The service account principal ID
     * @return JWT token string
     */
    public String issueServiceAccountToken(String principalId) {
        return issueToken(principalId, PrincipalType.SERVICE, defaultExpiry);
    }

    /**
     * Issue a session token (short-lived).
     *
     * @param principalId The user principal ID
     * @param roles User's roles
     * @return JWT token string
     */
    public String issueSessionToken(String principalId, Set<String> roles) {
        // Session tokens expire in 8 hours
        return issueTokenWithRoles(principalId, PrincipalType.USER, roles, Duration.ofHours(8));
    }
}
