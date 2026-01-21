package tech.flowcatalyst.platform.security;

import io.quarkus.test.junit.QuarkusTest;
import jakarta.inject.Inject;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.platform.principal.PasswordService;
import tech.flowcatalyst.platform.principal.UserService;
import tech.flowcatalyst.platform.principal.UserScope;

import java.util.UUID;

import static org.assertj.core.api.Assertions.*;

/**
 * SECURITY TESTS: Password Security
 *
 * These tests verify that password security controls are properly enforced.
 *
 * THREAT MODEL:
 * 1. Weak passwords allow brute force attacks
 * 2. Rainbow table attacks via unsalted passwords
 * 3. Password reuse across users (same hash)
 * 4. OIDC users bypass password security
 * 5. Unauthorized password changes
 *
 * SECURITY CONTROLS:
 * - Password complexity requirements (12+ chars, upper, lower, digit, special)
 * - Argon2id with random salt (prevents rainbow tables, GPU/ASIC-resistant)
 * - Old password required for password change
 * - OIDC users cannot have passwords
 */
@Tag("integration")
@QuarkusTest
class PasswordSecurityTest {

    /** Generate a unique email for each test to avoid conflicts (MongoDB doesn't support JTA rollback) */
    private String uniqueEmail() {
        return "user-" + UUID.randomUUID().toString().substring(0, 8) + "@example.com";
    }

    /** Generate a unique email with a specific prefix */
    private String uniqueEmail(String prefix) {
        return prefix + "-" + UUID.randomUUID().toString().substring(0, 8) + "@example.com";
    }

    @Inject
    UserService userService;

    @Inject
    PasswordService passwordService;

    // ========================================
    // PASSWORD COMPLEXITY REQUIREMENTS
    // ========================================

    @Test
    @DisplayName("SECURITY: Weak passwords are rejected")
    void shouldRejectWeakPasswords_whenCreatingUser() {
        // THREAT: Weak passwords allow brute force attacks

        String[] weakPasswords = {
            "short",                    // Too short (< 12 chars)
            "nouppercase123!",         // No uppercase letter
            "NOLOWERCASE123!",         // No lowercase letter
            "NoDigitsHere!",           // No digits
            "NoSpecial123",            // No special character
            "weak",                    // Multiple violations
            "12345678901",             // Only digits
            "abcdefghijk"              // Only lowercase
        };

        for (String weakPassword : weakPasswords) {
            // Act & Assert: Each weak password is rejected
            assertThatThrownBy(() ->
                userService.createInternalUser(
                    uniqueEmail(),
                    weakPassword,
                    "User",
                    null,
                    UserScope.ANCHOR
                )
            )
            .isInstanceOf(IllegalArgumentException.class)
            .withFailMessage("Password '%s' should have been rejected", weakPassword);
        }
    }

    @Test
    @DisplayName("SECURITY: Strong passwords are accepted")
    void shouldAcceptStrongPasswords_whenCreatingUser() {
        // Valid passwords meeting all requirements

        String[] strongPasswords = {
            "ValidPass123!",           // Basic valid
            "Tr0ng#P@ssw0rd",         // Multiple special chars
            "MyP@ssw0rd2024!",        // With year
            "Secur3!Passw0rd#2024",   // Long and complex
            "P@ssW0rd!23456789"       // Extra long
        };

        for (String strongPassword : strongPasswords) {
            // Act: Should not throw exception
            assertThatCode(() ->
                userService.createInternalUser(
                    uniqueEmail(),
                    strongPassword,
                    "User",
                    null,
                    UserScope.ANCHOR
                )
            ).doesNotThrowAnyException();
        }
    }

    @Test
    @DisplayName("SECURITY: Minimum password length is enforced (12 characters)")
    void shouldEnforceMinimumLength_whenValidatingPasswords() {
        // 11 characters (too short)
        assertThatThrownBy(() ->
            passwordService.validatePasswordComplexity("ValidPass1!")
        )
        .hasMessageContaining("at least 12 characters");

        // 12 characters (minimum, should pass)
        assertThatCode(() ->
            passwordService.validatePasswordComplexity("ValidPass12!")
        ).doesNotThrowAnyException();
    }

    // ========================================
    // SALT AND HASH SECURITY
    // ========================================

    @Test
    @DisplayName("SECURITY: Same password produces different hashes (salt protection)")
    void shouldProduceDifferentHashes_whenSamePasswordUsed() {
        // THREAT: Rainbow table attacks if passwords not salted

        // Arrange: Create 2 users with same password
        Principal user1 = userService.createInternalUser(
            uniqueEmail("user1"),
            "SamePass123!",
            "User 1",
            null, UserScope.ANCHOR
        );

        Principal user2 = userService.createInternalUser(
            uniqueEmail("user2"),
            "SamePass123!",
            "User 2",
            null, UserScope.ANCHOR
        );

        // Assert: Password hashes are DIFFERENT (due to random salt)
        assertThat(user1.userIdentity.passwordHash)
            .isNotEqualTo(user2.userIdentity.passwordHash);

        // Assert: Both passwords still verify correctly
        assertThat(passwordService.verifyPassword(
            "SamePass123!",
            user1.userIdentity.passwordHash
        )).isTrue();

        assertThat(passwordService.verifyPassword(
            "SamePass123!",
            user2.userIdentity.passwordHash
        )).isTrue();
    }

    @Test
    @DisplayName("SECURITY: Password hashes are using Argon2id format")
    void shouldUseArgon2idFormat_whenHashingPasswords() {
        // THREAT: Weak hashing algorithm (MD5, SHA1, etc.)

        // Act
        String hash = passwordService.hashPassword("SecurePass123!");

        // Assert: Argon2id format ($argon2id$ prefix)
        assertThat(hash).startsWith("$argon2id$");
        assertThat(hash).contains("m=65536"); // Memory cost
        assertThat(hash).contains("t=3");     // Time cost
        assertThat(hash).contains("p=4");     // Parallelism

        // Argon2id format: $argon2id$v=19$m=65536,t=3,p=4$[salt]$[hash]
        String[] parts = hash.split("\\$");
        assertThat(parts).hasSizeGreaterThanOrEqualTo(5);
    }

    @Test
    @DisplayName("SECURITY: Multiple hashing operations are consistent")
    void shouldProduceConsistentResults_whenHashingAndVerifying() {
        // Verify that hash/verify operations are deterministic

        String password = "TestPass123!";

        // Hash the password 10 times
        for (int i = 0; i < 10; i++) {
            String hash = passwordService.hashPassword(password);

            // Each hash should verify correctly
            assertThat(passwordService.verifyPassword(password, hash))
                .as("Iteration %d", i)
                .isTrue();

            // Wrong password should not verify
            assertThat(passwordService.verifyPassword("WrongPass123!", hash))
                .as("Iteration %d (wrong password)", i)
                .isFalse();
        }
    }

    // ========================================
    // PASSWORD CHANGE SECURITY
    // ========================================

    @Test
    @DisplayName("SECURITY: Cannot change password without old password")
    void shouldRejectPasswordChange_whenOldPasswordIncorrect() {
        // THREAT: Unauthorized password change (session hijacking, XSS, etc.)

        // Arrange: Create user
        Principal user = userService.createInternalUser(
            uniqueEmail(),
            "CurrentPass123!",
            "User",
            null, UserScope.ANCHOR
        );

        String oldHash = user.userIdentity.passwordHash;

        // Act & Assert: Attempt password change with wrong old password
        assertThatThrownBy(() ->
            userService.changePassword(user.id, "WrongOldPass!", "NewPass123!")
        )
        .hasMessageContaining("incorrect");

        // Verify password unchanged
        Principal unchangedUser = userService.findById(user.id).get();
        assertThat(unchangedUser.userIdentity.passwordHash).isEqualTo(oldHash);
    }

    @Test
    @DisplayName("SECURITY: Password change requires old password verification")
    void shouldRequireOldPassword_whenChangingPassword() {
        // SCENARIO: Legitimate password change

        // Arrange
        String oldPassword = "OldPass123!x";
        String newPassword = "NewPass456!x";

        Principal user = userService.createInternalUser(
            uniqueEmail(),
            oldPassword,
            "User",
            null, UserScope.ANCHOR
        );

        // Act: Change password with correct old password
        userService.changePassword(user.id, oldPassword, newPassword);

        // Refresh user
        Principal updatedUser = userService.findById(user.id).get();

        // Assert: Old password no longer works
        assertThat(passwordService.verifyPassword(
            oldPassword,
            updatedUser.userIdentity.passwordHash
        )).isFalse();

        // Assert: New password works
        assertThat(passwordService.verifyPassword(
            newPassword,
            updatedUser.userIdentity.passwordHash
        )).isTrue();
    }

    @Test
    @DisplayName("SECURITY: New password must meet complexity requirements when changing")
    void shouldEnforceComplexity_whenChangingPassword() {
        // THREAT: Changing to weak password

        // Arrange
        Principal user = userService.createInternalUser(
            uniqueEmail(),
            "StrongOldPass123!",
            "User",
            null, UserScope.ANCHOR
        );

        // Act & Assert: Weak new password rejected
        assertThatThrownBy(() ->
            userService.changePassword(user.id, "StrongOldPass123!", "weak")
        )
        .isInstanceOf(IllegalArgumentException.class);
    }

    // ========================================
    // PASSWORD RESET SECURITY
    // ========================================

    @Test
    @DisplayName("SECURITY: Admin password reset requires proper authorization")
    void shouldAllowAdminReset_whenAuthorized() {
        // SCENARIO: Platform admin resets user's forgotten password

        // Arrange
        Principal user = userService.createInternalUser(
            uniqueEmail(),
            "OldPass123!x",
            "User",
            null, UserScope.ANCHOR
        );

        String oldHash = user.userIdentity.passwordHash;

        // Act: Admin resets password (doesn't need old password)
        String newPassword = "NewPass123!x";
        userService.resetPassword(user.id, newPassword);

        // Refresh user
        Principal updatedUser = userService.findById(user.id).get();

        // Assert: Password changed
        assertThat(updatedUser.userIdentity.passwordHash).isNotEqualTo(oldHash);
        assertThat(passwordService.verifyPassword(
            newPassword,
            updatedUser.userIdentity.passwordHash
        )).isTrue();
    }

    @Test
    @DisplayName("SECURITY: Reset password must meet complexity requirements")
    void shouldEnforceComplexity_whenResettingPassword() {
        // THREAT: Admin resets to weak password

        // Arrange
        Principal user = userService.createInternalUser(
            uniqueEmail(),
            "OldPass123!x",
            "User",
            null, UserScope.ANCHOR
        );

        // Act & Assert: Weak password rejected
        assertThatThrownBy(() ->
            userService.resetPassword(user.id, "weak")
        )
        .isInstanceOf(IllegalArgumentException.class);
    }

    // ========================================
    // OIDC USER PASSWORD SECURITY
    // ========================================

    @Test
    @DisplayName("SECURITY: OIDC users cannot have passwords")
    void shouldPreventPasswordOperations_whenUserIsOIDC() {
        // THREAT: OIDC authentication bypass via password

        // Arrange: Create OIDC user (via SSO login)
        Principal oidcUser = userService.createOrUpdateOidcUser(
            "alice@customer.com",
            "Alice Smith",
            "google-oauth2|123456",
            null,
            null
        );

        // Assert: No password hash
        assertThat(oidcUser.userIdentity.passwordHash).isNull();

        // Act & Assert: Cannot reset password for OIDC user
        assertThatThrownBy(() ->
            userService.resetPassword(oidcUser.id, "NewPass123!x")
        )
        .hasMessageContaining("OIDC");

        // Act & Assert: Cannot change password for OIDC user
        assertThatThrownBy(() ->
            userService.changePassword(oidcUser.id, "OldPass123!x", "NewPass123!x")
        )
        .hasMessageContaining("OIDC");
    }

    @Test
    @DisplayName("SECURITY: OIDC users remain password-less after multiple logins")
    void shouldRemainsPasswordLess_whenOidcUserLoginsMultipleTimes() {
        // SCENARIO: OIDC user logs in multiple times

        // First login
        Principal firstLogin = userService.createOrUpdateOidcUser(
            "alice@customer.com",
            "Alice Smith",
            "google-oauth2|123",
            null,
            null
        );

        assertThat(firstLogin.userIdentity.passwordHash).isNull();

        // Second login (user info update)
        Principal secondLogin = userService.createOrUpdateOidcUser(
            "alice@customer.com",
            "Alice Johnson",  // Name changed
            "google-oauth2|456",
            null,
            null
        );

        // Assert: Same user, still no password
        assertThat(secondLogin.id).isEqualTo(firstLogin.id);
        assertThat(secondLogin.userIdentity.passwordHash).isNull();
    }

    // ========================================
    // PASSWORD VERIFICATION SECURITY
    // ========================================

    @Test
    @DisplayName("SECURITY: Password verification is case-sensitive")
    void shouldBeCaseSensitive_whenVerifyingPasswords() {
        // THREAT: Case-insensitive comparison weakens security

        // Arrange
        String password = "SecurePass123!";
        Principal user = userService.createInternalUser(
            uniqueEmail(),
            password,
            "User",
            null, UserScope.ANCHOR
        );

        String hash = user.userIdentity.passwordHash;

        // Assert: Different cases should fail
        assertThat(passwordService.verifyPassword("securepass123!", hash)).isFalse();
        assertThat(passwordService.verifyPassword("SECUREPASS123!", hash)).isFalse();
        assertThat(passwordService.verifyPassword("SecurePass123!", hash)).isTrue();
    }

    @Test
    @DisplayName("SECURITY: Null or empty password verification returns false")
    void shouldReturnFalse_whenPasswordIsNullOrEmpty() {
        // THREAT: Null/empty password bypass

        // Arrange
        String hash = passwordService.hashPassword("ValidPass123!");

        // Assert: Null and empty passwords always fail
        assertThat(passwordService.verifyPassword(null, hash)).isFalse();
        assertThat(passwordService.verifyPassword("", hash)).isFalse();
        assertThat(passwordService.verifyPassword("   ", hash)).isFalse();
    }

    @Test
    @DisplayName("SECURITY: Invalid hash format returns false")
    void shouldReturnFalse_whenHashIsInvalid() {
        // THREAT: Hash injection attacks

        // Assert: Invalid hashes don't crash, just return false
        assertThat(passwordService.verifyPassword("ValidPass123!", "invalid-hash"))
            .isFalse();
        assertThat(passwordService.verifyPassword("ValidPass123!", ""))
            .isFalse();
        assertThat(passwordService.verifyPassword("ValidPass123!", null))
            .isFalse();
        assertThat(passwordService.verifyPassword("ValidPass123!", "short"))
            .isFalse();
    }

    // ========================================
    // EDGE CASES AND ATTACK SCENARIOS
    // ========================================

    @Test
    @DisplayName("SECURITY: Very long passwords are handled safely")
    void shouldHandleLongPasswords_safely() {
        // THREAT: Buffer overflow via very long passwords

        // Note: Argon2id handles long passwords better than BCrypt (no 72-byte limit)
        String veryLongPassword = "A".repeat(100) + "b1!";

        // Should not crash
        assertThatCode(() -> {
            Principal user = userService.createInternalUser(
                uniqueEmail(),
                veryLongPassword,
                "User",
                null, UserScope.ANCHOR
            );

            // Should verify correctly
            assertThat(passwordService.verifyPassword(
                veryLongPassword,
                user.userIdentity.passwordHash
            )).isTrue();
        }).doesNotThrowAnyException();
    }

    @Test
    @DisplayName("SECURITY: Unicode characters in passwords are supported")
    void shouldSupportUnicodeCharacters_inPasswords() {
        // Support for international users

        String[] unicodePasswords = {
            "Pässwörd123!xxx",     // German umlauts (15 chars)
            "Contraseña123!x",     // Spanish (15 chars)
            "ПарольPass123!",      // Russian (14 chars)
            "密码Password123!",    // Chinese (15 chars)
            "パスワードPass123!"   // Japanese (15 chars)
        };

        for (String unicodePassword : unicodePasswords) {
            assertThatCode(() -> {
                Principal user = userService.createInternalUser(
                    uniqueEmail(),
                    unicodePassword,
                    "User",
                    null, UserScope.ANCHOR
                );

                assertThat(passwordService.verifyPassword(
                    unicodePassword,
                    user.userIdentity.passwordHash
                )).isTrue();
            }).doesNotThrowAnyException();
        }
    }

    @Test
    @DisplayName("SECURITY: Password with whitespace is handled correctly")
    void shouldHandleWhitespace_inPasswords() {
        // Passwords can contain spaces

        String passwordWithSpaces = "My Secure Pass 123!";

        Principal user = userService.createInternalUser(
            uniqueEmail(),
            passwordWithSpaces,
            "User",
            null, UserScope.ANCHOR
        );

        // Exact match required (spaces matter)
        assertThat(passwordService.verifyPassword(
            passwordWithSpaces,
            user.userIdentity.passwordHash
        )).isTrue();

        assertThat(passwordService.verifyPassword(
            "MySecurePass123!",  // Without spaces
            user.userIdentity.passwordHash
        )).isFalse();
    }
}
