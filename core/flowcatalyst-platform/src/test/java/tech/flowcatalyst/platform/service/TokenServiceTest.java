package tech.flowcatalyst.platform.service;

import io.quarkus.test.junit.QuarkusTest;
import io.smallrye.jwt.auth.principal.DefaultJWTParser;
import io.smallrye.jwt.auth.principal.ParseException;
import jakarta.inject.Inject;
import org.eclipse.microprofile.jwt.JsonWebToken;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import tech.flowcatalyst.platform.authentication.JwtKeyService;
import tech.flowcatalyst.platform.authentication.TokenService;
import tech.flowcatalyst.platform.principal.PrincipalType;

import java.time.Duration;
import java.time.Instant;
import java.util.Set;

import static org.assertj.core.api.Assertions.*;

/**
 * Unit tests for TokenService.
 * Tests JWT token generation and validation.
 *
 * Uses @QuarkusTest because the service relies on Quarkus JWT configuration
 * and SmallRye JWT implementation.
 */
@Tag("integration")
@QuarkusTest
class TokenServiceTest {

    @Inject
    TokenService service;

    @Inject
    JwtKeyService jwtKeyService;

    /**
     * Parse a token using the JwtKeyService's public key.
     */
    private JsonWebToken parseToken(String token) throws ParseException {
        DefaultJWTParser parser = new DefaultJWTParser();
        return parser.verify(token, jwtKeyService.getPublicKey());
    }

    // ========================================
    // issueToken TESTS
    // ========================================

    @Test
    @DisplayName("issueToken should include subject when token is issued")
    void issueToken_shouldIncludeSubject_whenTokenIssued() throws ParseException {
        // Arrange
        String principalId = "0HZTEST12345";
        PrincipalType principalType = PrincipalType.USER;

        // Act
        String token = service.issueToken(principalId, principalType, Duration.ofHours(8));

        // Assert
        JsonWebToken jwt = parseToken(token);
        assertThat(jwt.getSubject()).isEqualTo("0HZTEST12345");
    }

    @Test
    @DisplayName("issueToken should include type claim when token is issued")
    void issueToken_shouldIncludeType_whenTokenIssued() throws ParseException {
        // Arrange
        String principalId = "0HZTEST12345";
        PrincipalType principalType = PrincipalType.USER;

        // Act
        String token = service.issueToken(principalId, principalType, Duration.ofHours(8));

        // Assert
        JsonWebToken jwt = parseToken(token);
        assertThat((String) jwt.getClaim("type")).isEqualTo("USER");
    }

    @Test
    @DisplayName("issueToken should set expiry when expiry is provided")
    void issueToken_shouldSetExpiry_whenExpiryProvided() throws ParseException {
        // Arrange
        Instant beforeIssuance = Instant.now();

        // Act
        String token = service.issueToken("0HZTEST12345", PrincipalType.USER, Duration.ofHours(8));

        Instant afterIssuance = Instant.now();

        // Assert
        JsonWebToken jwt = parseToken(token);
        Instant expiry = Instant.ofEpochSecond(jwt.getExpirationTime());

        // Expiry should be approximately 8 hours from now (allowing for test execution time)
        assertThat(expiry).isAfter(beforeIssuance.plus(Duration.ofHours(7).plus(Duration.ofMinutes(50))));
        assertThat(expiry).isBefore(afterIssuance.plus(Duration.ofHours(8).plus(Duration.ofMinutes(10))));
    }

    @Test
    @DisplayName("issueToken should use default expiry when expiry is null")
    void issueToken_shouldUseDefaultExpiry_whenExpiryIsNull() throws ParseException {
        // Act
        String token = service.issueToken("0HZTEST12345", PrincipalType.SERVICE, null);

        // Assert
        JsonWebToken jwt = parseToken(token);
        Instant expiry = Instant.ofEpochSecond(jwt.getExpirationTime());

        // Default expiry is 365 days - should expire in roughly a year
        assertThat(expiry).isAfter(Instant.now().plus(Duration.ofDays(364)));
        assertThat(expiry).isBefore(Instant.now().plus(Duration.ofDays(366)));
    }

    @Test
    @DisplayName("issueToken should include issued-at timestamp")
    void issueToken_shouldIncludeIssuedAt_whenTokenIssued() throws ParseException {
        // Arrange
        Instant beforeIssuance = Instant.now().truncatedTo(java.time.temporal.ChronoUnit.SECONDS);

        // Act
        String token = service.issueToken("0HZTEST12345", PrincipalType.USER, Duration.ofHours(8));

        Instant afterIssuance = Instant.now().truncatedTo(java.time.temporal.ChronoUnit.SECONDS);

        // Assert
        JsonWebToken jwt = parseToken(token);
        Long issuedAt = jwt.getIssuedAtTime();
        assertThat(issuedAt).isNotNull();

        Instant issuedAtInstant = Instant.ofEpochSecond(issuedAt);
        assertThat(issuedAtInstant).isBetween(beforeIssuance, afterIssuance);
    }

    @Test
    @DisplayName("issueToken should include issuer claim")
    void issueToken_shouldIncludeIssuer_whenTokenIssued() throws ParseException {
        // Act
        String token = service.issueToken("0HZTEST12345", PrincipalType.USER, Duration.ofHours(8));

        // Assert
        JsonWebToken jwt = parseToken(token);
        assertThat(jwt.getIssuer()).isEqualTo("flowcatalyst");
    }

    @Test
    @DisplayName("issueToken should handle SERVICE type correctly")
    void issueToken_shouldHandleServiceType_whenTypeIsService() throws ParseException {
        // Act
        String token = service.issueToken("0HZTEST99999", PrincipalType.SERVICE, Duration.ofDays(365));

        // Assert
        JsonWebToken jwt = parseToken(token);
        assertThat((String) jwt.getClaim("type")).isEqualTo("SERVICE");
        assertThat(jwt.getSubject()).isEqualTo("0HZTEST99999");
    }

    // ========================================
    // issueTokenWithRoles TESTS
    // ========================================

    @Test
    @DisplayName("issueTokenWithRoles should include groups claim when roles provided")
    void issueTokenWithRoles_shouldIncludeGroups_whenRolesProvided() throws ParseException {
        // Arrange
        Set<String> roles = Set.of("admin", "operator", "viewer");

        // Act
        String token = service.issueTokenWithRoles("0HZTEST12345", PrincipalType.USER, roles, Duration.ofHours(8));

        // Assert
        JsonWebToken jwt = parseToken(token);
        assertThat(jwt.getGroups()).containsExactlyInAnyOrder("admin", "operator", "viewer");
    }

    @Test
    @DisplayName("issueTokenWithRoles should include subject and type")
    void issueTokenWithRoles_shouldIncludeSubjectAndType_whenCalled() throws ParseException {
        // Arrange
        Set<String> roles = Set.of("user");

        // Act
        String token = service.issueTokenWithRoles("0HZTEST12345", PrincipalType.USER, roles, Duration.ofHours(8));

        // Assert
        JsonWebToken jwt = parseToken(token);
        assertThat(jwt.getSubject()).isEqualTo("0HZTEST12345");
        assertThat((String) jwt.getClaim("type")).isEqualTo("USER");
    }

    @Test
    @DisplayName("issueTokenWithRoles should handle empty roles set")
    void issueTokenWithRoles_shouldHandleEmptyRoles_whenRolesEmpty() throws ParseException {
        // Arrange
        Set<String> emptyRoles = Set.of();

        // Act
        String token = service.issueTokenWithRoles("0HZTEST12345", PrincipalType.USER, emptyRoles, Duration.ofHours(8));

        // Assert
        JsonWebToken jwt = parseToken(token);
        assertThat(jwt.getGroups()).isEmpty();
    }

    @Test
    @DisplayName("issueTokenWithRoles should use default expiry when null")
    void issueTokenWithRoles_shouldUseDefaultExpiry_whenExpiryIsNull() throws ParseException {
        // Arrange
        Set<String> roles = Set.of("admin");

        // Act
        String token = service.issueTokenWithRoles("0HZTEST12345", PrincipalType.USER, roles, null);

        // Assert
        JsonWebToken jwt = parseToken(token);
        Instant expiry = Instant.ofEpochSecond(jwt.getExpirationTime());

        // Should use default expiry (365 days)
        assertThat(expiry).isAfter(Instant.now().plus(Duration.ofDays(364)));
    }

    // ========================================
    // issueSessionToken TESTS
    // ========================================

    @Test
    @DisplayName("issueSessionToken should have 8-hour expiry")
    void issueSessionToken_shouldHave8HourExpiry_whenCalled() throws ParseException {
        // Arrange
        Set<String> roles = Set.of("user", "operator");
        Instant beforeIssuance = Instant.now();

        // Act
        String token = service.issueSessionToken("0HZTEST12345", roles);

        Instant afterIssuance = Instant.now();

        // Assert
        JsonWebToken jwt = parseToken(token);
        Instant expiry = Instant.ofEpochSecond(jwt.getExpirationTime());

        // Should expire in ~8 hours (allowing for test execution time)
        assertThat(expiry).isAfter(beforeIssuance.plus(Duration.ofHours(7).plus(Duration.ofMinutes(50))));
        assertThat(expiry).isBefore(afterIssuance.plus(Duration.ofHours(8).plus(Duration.ofMinutes(10))));
    }

    @Test
    @DisplayName("issueSessionToken should include roles in groups claim")
    void issueSessionToken_shouldIncludeRolesInGroups_whenCalled() throws ParseException {
        // Arrange
        Set<String> roles = Set.of("admin", "operator");

        // Act
        String token = service.issueSessionToken("0HZTEST12345", roles);

        // Assert
        JsonWebToken jwt = parseToken(token);
        assertThat(jwt.getGroups()).containsExactlyInAnyOrder("admin", "operator");
    }

    @Test
    @DisplayName("issueSessionToken should have USER type")
    void issueSessionToken_shouldHaveUserType_whenCalled() throws ParseException {
        // Arrange
        Set<String> roles = Set.of("user");

        // Act
        String token = service.issueSessionToken("0HZTEST12345", roles);

        // Assert
        JsonWebToken jwt = parseToken(token);
        assertThat((String) jwt.getClaim("type")).isEqualTo("USER");
    }

    @Test
    @DisplayName("issueSessionToken should include subject")
    void issueSessionToken_shouldIncludeSubject_whenCalled() throws ParseException {
        // Arrange
        Set<String> roles = Set.of("user");

        // Act
        String token = service.issueSessionToken("0HZTEST12345", roles);

        // Assert
        JsonWebToken jwt = parseToken(token);
        assertThat(jwt.getSubject()).isEqualTo("0HZTEST12345");
    }

    // ========================================
    // issueServiceAccountToken TESTS
    // ========================================

    @Test
    @DisplayName("issueServiceAccountToken should have long expiry")
    void issueServiceAccountToken_shouldHaveLongExpiry_whenCalled() throws ParseException {
        // Act
        String token = service.issueServiceAccountToken("0HZTEST99999");

        // Assert
        JsonWebToken jwt = parseToken(token);
        Instant expiry = Instant.ofEpochSecond(jwt.getExpirationTime());

        // Should expire in ~365 days
        assertThat(expiry).isAfter(Instant.now().plus(Duration.ofDays(364)));
        assertThat(expiry).isBefore(Instant.now().plus(Duration.ofDays(366)));
    }

    @Test
    @DisplayName("issueServiceAccountToken should have SERVICE type")
    void issueServiceAccountToken_shouldHaveServiceType_whenCalled() throws ParseException {
        // Act
        String token = service.issueServiceAccountToken("0HZTEST99999");

        // Assert
        JsonWebToken jwt = parseToken(token);
        assertThat((String) jwt.getClaim("type")).isEqualTo("SERVICE");
    }

    @Test
    @DisplayName("issueServiceAccountToken should include subject")
    void issueServiceAccountToken_shouldIncludeSubject_whenCalled() throws ParseException {
        // Act
        String token = service.issueServiceAccountToken("0HZTEST99999");

        // Assert
        JsonWebToken jwt = parseToken(token);
        assertThat(jwt.getSubject()).isEqualTo("0HZTEST99999");
    }

    // ========================================
    // TOKEN FORMAT AND STRUCTURE TESTS
    // ========================================

    @Test
    @DisplayName("Token should be valid JWT format")
    void token_shouldBeValidJwtFormat_whenGenerated() {
        // Act
        String token = service.issueToken("0HZTEST12345", PrincipalType.USER, Duration.ofHours(8));

        // Assert: JWT format is header.payload.signature (3 parts separated by dots)
        assertThat(token).contains(".");
        String[] parts = token.split("\\.");
        assertThat(parts).hasSize(3);
    }

    @Test
    @DisplayName("Token should be signed and verifiable")
    void token_shouldBeVerifiable_whenGenerated() {
        // Act
        String token = service.issueToken("0HZTEST12345", PrincipalType.USER, Duration.ofHours(8));

        // Assert: Should be able to parse without exception (validates signature)
        assertThatCode(() -> parseToken(token))
            .doesNotThrowAnyException();
    }

    @Test
    @DisplayName("Multiple tokens for same principal should be different")
    void tokens_shouldBeDifferent_whenGeneratedMultipleTimes() {
        // Act: Generate 3 tokens for same principal
        String token1 = service.issueToken("0HZTEST12345", PrincipalType.USER, Duration.ofHours(8));
        String token2 = service.issueToken("0HZTEST12345", PrincipalType.USER, Duration.ofHours(8));
        String token3 = service.issueToken("0HZTEST12345", PrincipalType.USER, Duration.ofHours(8));

        // Assert: Tokens should be different (different issued-at timestamps)
        assertThat(token1).isNotEqualTo(token2);
        assertThat(token2).isNotEqualTo(token3);
        assertThat(token1).isNotEqualTo(token3);
    }

    // ========================================
    // EXPIRY EDGE CASES
    // ========================================

    @Test
    @DisplayName("issueToken should handle very short expiry")
    void issueToken_shouldHandleVeryShortExpiry_whenExpiryIsVeryShort() throws ParseException {
        // Act: 1 minute expiry
        String token = service.issueToken("0HZTEST12345", PrincipalType.USER, Duration.ofMinutes(1));

        // Assert
        JsonWebToken jwt = parseToken(token);
        Instant expiry = Instant.ofEpochSecond(jwt.getExpirationTime());

        assertThat(expiry).isBefore(Instant.now().plus(Duration.ofMinutes(2)));
        assertThat(expiry).isAfter(Instant.now());
    }

    @Test
    @DisplayName("issueToken should handle very long expiry")
    void issueToken_shouldHandleVeryLongExpiry_whenExpiryIsVeryLong() throws ParseException {
        // Act: 10 year expiry
        String token = service.issueToken("0HZTEST12345", PrincipalType.SERVICE, Duration.ofDays(3650));

        // Assert
        JsonWebToken jwt = parseToken(token);
        Instant expiry = Instant.ofEpochSecond(jwt.getExpirationTime());

        assertThat(expiry).isAfter(Instant.now().plus(Duration.ofDays(3649)));
    }

    // ========================================
    // EDGE CASES
    // ========================================

    @Test
    @DisplayName("issueToken should handle various TSID principal IDs")
    void issueToken_shouldHandleVariousTsidPrincipalIds_whenIdIsValid() throws ParseException {
        // Arrange: A typical TSID string (13 chars, Crockford Base32)
        String tsidPrincipalId = "0HZABCDEFGHIJ";

        // Act
        String token = service.issueToken(tsidPrincipalId, PrincipalType.USER, Duration.ofHours(8));

        // Assert
        JsonWebToken jwt = parseToken(token);
        assertThat(jwt.getSubject()).isEqualTo("0HZABCDEFGHIJ");
    }

    @Test
    @DisplayName("issueToken should handle zero expiry duration")
    void issueToken_shouldHandleZeroExpiry_whenExpiryIsZero() throws ParseException {
        // Act: Zero duration (token expires immediately)
        String token = service.issueToken("0HZTEST12345", PrincipalType.USER, Duration.ZERO);

        // Assert: Should still generate token (even if expired)
        JsonWebToken jwt = parseToken(token);
        assertThat(jwt.getSubject()).isEqualTo("0HZTEST12345");

        Instant expiry = Instant.ofEpochSecond(jwt.getExpirationTime());
        Instant issuedAt = Instant.ofEpochSecond(jwt.getIssuedAtTime());

        // Expiry should be approximately equal to issued-at (or very close)
        assertThat(expiry).isBetween(issuedAt.minus(Duration.ofSeconds(1)), issuedAt.plus(Duration.ofSeconds(1)));
    }
}
