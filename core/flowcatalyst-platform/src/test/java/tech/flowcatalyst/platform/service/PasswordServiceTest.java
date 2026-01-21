package tech.flowcatalyst.platform.service;

import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import tech.flowcatalyst.platform.principal.PasswordService;

import static org.assertj.core.api.Assertions.*;

/**
 * Unit tests for PasswordService.
 * Tests password hashing, verification, and complexity validation.
 * No dependencies to mock - standalone service.
 */
class PasswordServiceTest {

    private final PasswordService service = new PasswordService();

    // ========================================
    // HASHING TESTS
    // ========================================

    @Test
    @DisplayName("hashPassword should produce different hashes when called twice with same password")
    void hashPassword_shouldProduceDifferentHashes_whenCalledTwiceWithSamePassword() {
        // Arrange
        String password = "SecurePass123!";

        // Act
        String hash1 = service.hashPassword(password);
        String hash2 = service.hashPassword(password);

        // Assert: Hashes are different due to random salt
        assertThat(hash1).isNotEqualTo(hash2);
        assertThat(hash1).isNotNull();
        assertThat(hash2).isNotNull();
    }

    @Test
    @DisplayName("hashPassword should throw exception when password is null")
    void hashPassword_shouldThrowException_whenPasswordIsNull() {
        assertThatThrownBy(() -> service.hashPassword(null))
            .isInstanceOf(IllegalArgumentException.class)
            .hasMessageContaining("Password cannot be null or empty");
    }

    @Test
    @DisplayName("hashPassword should throw exception when password is empty")
    void hashPassword_shouldThrowException_whenPasswordIsEmpty() {
        assertThatThrownBy(() -> service.hashPassword(""))
            .isInstanceOf(IllegalArgumentException.class)
            .hasMessageContaining("Password cannot be null or empty");
    }

    @Test
    @DisplayName("hashPassword should produce Argon2id hash format")
    void hashPassword_shouldProduceArgon2idHashFormat_whenPasswordValid() {
        // Act
        String hash = service.hashPassword("ValidPass123!");

        // Assert: Argon2id hash starts with $argon2id$
        assertThat(hash).startsWith("$argon2id$");
        assertThat(hash).contains("m=65536"); // Memory cost
        assertThat(hash).contains("t=3");     // Time cost (iterations)
        assertThat(hash).contains("p=4");     // Parallelism
    }

    // ========================================
    // VERIFICATION TESTS
    // ========================================

    @Test
    @DisplayName("verifyPassword should return true when password is correct")
    void verifyPassword_shouldReturnTrue_whenPasswordIsCorrect() {
        // Arrange
        String password = "SecurePass123!";
        String hash = service.hashPassword(password);

        // Act & Assert
        assertThat(service.verifyPassword(password, hash)).isTrue();
    }

    @Test
    @DisplayName("verifyPassword should return false when password is incorrect")
    void verifyPassword_shouldReturnFalse_whenPasswordIsIncorrect() {
        // Arrange
        String correctPassword = "SecurePass123!";
        String wrongPassword = "WrongPass123!";
        String hash = service.hashPassword(correctPassword);

        // Act & Assert
        assertThat(service.verifyPassword(wrongPassword, hash)).isFalse();
    }

    @Test
    @DisplayName("verifyPassword should return false when password is null")
    void verifyPassword_shouldReturnFalse_whenPasswordIsNull() {
        // Arrange
        String hash = service.hashPassword("SecurePass123!");

        // Act & Assert
        assertThat(service.verifyPassword(null, hash)).isFalse();
    }

    @Test
    @DisplayName("verifyPassword should return false when hash is null")
    void verifyPassword_shouldReturnFalse_whenHashIsNull() {
        assertThat(service.verifyPassword("SecurePass123!", null)).isFalse();
    }

    @Test
    @DisplayName("verifyPassword should return false when hash is invalid")
    void verifyPassword_shouldReturnFalse_whenHashIsInvalid() {
        assertThat(service.verifyPassword("SecurePass123!", "invalid-hash-format")).isFalse();
    }

    @Test
    @DisplayName("verifyPassword should work with very long passwords")
    void verifyPassword_shouldWork_whenPasswordIsVeryLong() {
        // Arrange: 100 character password
        String longPassword = "A".repeat(50) + "b".repeat(30) + "1".repeat(15) + "!".repeat(5);
        String hash = service.hashPassword(longPassword);

        // Act & Assert
        assertThat(service.verifyPassword(longPassword, hash)).isTrue();

        // NOTE: BCrypt has a 72-byte effective password length limit
        // Very long passwords beyond 72 bytes might not differentiate properly
        // This is expected BCrypt behavior, not a bug
    }

    // ========================================
    // COMPLEXITY VALIDATION TESTS
    // ========================================

    @Test
    @DisplayName("validatePasswordComplexity should throw exception when password too short")
    void validatePasswordComplexity_shouldThrowException_whenPasswordTooShort() {
        // Arrange: 11 characters (needs 12)
        String shortPassword = "Short123!";

        // Act & Assert
        assertThatThrownBy(() -> service.validatePasswordComplexity(shortPassword))
            .isInstanceOf(IllegalArgumentException.class)
            .hasMessageContaining("at least 12 characters long");
    }

    @Test
    @DisplayName("validatePasswordComplexity should throw exception when no uppercase")
    void validatePasswordComplexity_shouldThrowException_whenNoUppercase() {
        assertThatThrownBy(() -> service.validatePasswordComplexity("lowercase123!"))
            .isInstanceOf(IllegalArgumentException.class)
            .hasMessageContaining("uppercase");
    }

    @Test
    @DisplayName("validatePasswordComplexity should throw exception when no lowercase")
    void validatePasswordComplexity_shouldThrowException_whenNoLowercase() {
        assertThatThrownBy(() -> service.validatePasswordComplexity("UPPERCASE123!"))
            .isInstanceOf(IllegalArgumentException.class)
            .hasMessageContaining("lowercase");
    }

    @Test
    @DisplayName("validatePasswordComplexity should throw exception when no digit")
    void validatePasswordComplexity_shouldThrowException_whenNoDigit() {
        assertThatThrownBy(() -> service.validatePasswordComplexity("NoDigitsHere!"))
            .isInstanceOf(IllegalArgumentException.class)
            .hasMessageContaining("digit");
    }

    @Test
    @DisplayName("validatePasswordComplexity should throw exception when no special character")
    void validatePasswordComplexity_shouldThrowException_whenNoSpecialChar() {
        assertThatThrownBy(() -> service.validatePasswordComplexity("NoSpecial123"))
            .isInstanceOf(IllegalArgumentException.class)
            .hasMessageContaining("special character");
    }

    @Test
    @DisplayName("validatePasswordComplexity should not throw when password meets all requirements")
    void validatePasswordComplexity_shouldNotThrow_whenPasswordMeetsAllRequirements() {
        // Valid: 12+ chars, upper, lower, digit, special
        assertThatCode(() -> service.validatePasswordComplexity("ValidPass123!"))
            .doesNotThrowAnyException();
    }

    @Test
    @DisplayName("validatePasswordComplexity should accept all special characters")
    void validatePasswordComplexity_shouldAcceptAllSpecialCharacters_whenPresent() {
        String[] validPasswords = {
            "ValidPass123!",
            "ValidPass123@",
            "ValidPass123#",
            "ValidPass123$",
            "ValidPass123%",
            "ValidPass123^",
            "ValidPass123&",
            "ValidPass123*",
            "ValidPass123(",
            "ValidPass123)",
            "ValidPass123_",
            "ValidPass123+",
            "ValidPass123-",
            "ValidPass123=",
            "ValidPass123[",
            "ValidPass123]",
            "ValidPass123{",
            "ValidPass123}",
            "ValidPass123|",
            "ValidPass123;",
            "ValidPass123:",
            "ValidPass123,",
            "ValidPass123.",
            "ValidPass123<",
            "ValidPass123>",
            "ValidPass123?"
        };

        for (String password : validPasswords) {
            assertThatCode(() -> service.validatePasswordComplexity(password))
                .doesNotThrowAnyException();
        }
    }

    @Test
    @DisplayName("validatePasswordComplexity should accept exactly 12 characters")
    void validatePasswordComplexity_shouldAccept_whenExactly12Characters() {
        // 12 characters exactly: V-a-l-i-d-P-a-s-s-1-2-!
        String password = "ValidPass12!";
        assertThat(password).hasSize(12);

        assertThatCode(() -> service.validatePasswordComplexity(password))
            .doesNotThrowAnyException();
    }

    @Test
    @DisplayName("validatePasswordComplexity should throw exception when null")
    void validatePasswordComplexity_shouldThrowException_whenNull() {
        assertThatThrownBy(() -> service.validatePasswordComplexity(null))
            .isInstanceOf(IllegalArgumentException.class);
    }

    // ========================================
    // VALIDATE AND HASH TESTS
    // ========================================

    @Test
    @DisplayName("validateAndHashPassword should hash password when valid")
    void validateAndHashPassword_shouldHashPassword_whenPasswordIsValid() {
        // Act
        String hash = service.validateAndHashPassword("ValidPass123!");

        // Assert
        assertThat(hash).isNotNull();
        assertThat(hash).startsWith("$argon2id$");
        assertThat(service.verifyPassword("ValidPass123!", hash)).isTrue();
    }

    @Test
    @DisplayName("validateAndHashPassword should throw exception when password invalid")
    void validateAndHashPassword_shouldThrowException_whenPasswordInvalid() {
        // All these should fail validation
        String[] invalidPasswords = {
            "weak",                 // Too short
            "nouppercase123!",     // No uppercase
            "NOLOWERCASE123!",     // No lowercase
            "NoDigitsHere!",       // No digits
            "NoSpecial123"         // No special char
        };

        for (String password : invalidPasswords) {
            assertThatThrownBy(() -> service.validateAndHashPassword(password))
                .isInstanceOf(IllegalArgumentException.class);
        }
    }

    // ========================================
    // EDGE CASES
    // ========================================

    @Test
    @DisplayName("Password with unicode characters should work")
    void password_shouldWork_whenContainsUnicodeCharacters() {
        // Password with unicode characters (but still meets requirements)
        String unicodePassword = "ValidPass123!こんにちは";

        String hash = service.hashPassword(unicodePassword);
        assertThat(service.verifyPassword(unicodePassword, hash)).isTrue();
    }

    @Test
    @DisplayName("Password with whitespace should work")
    void password_shouldWork_whenContainsWhitespace() {
        String passwordWithSpaces = "Valid Pass 123!";

        // This should pass complexity if it meets requirements
        String hash = service.hashPassword(passwordWithSpaces);
        assertThat(service.verifyPassword(passwordWithSpaces, hash)).isTrue();
    }

    @Test
    @DisplayName("Case sensitivity in verification")
    void verifyPassword_shouldBeCaseSensitive_whenVerifying() {
        // Arrange
        String password = "SecurePass123!";
        String hash = service.hashPassword(password);

        // Act & Assert: Different case should fail
        assertThat(service.verifyPassword("securepass123!", hash)).isFalse();
        assertThat(service.verifyPassword("SECUREPASS123!", hash)).isFalse();
    }

    @Test
    @DisplayName("Multiple calls to hashPassword should all be verifiable")
    void hashPassword_shouldProduceVerifiableHashes_whenCalledMultipleTimes() {
        String password = "SecurePass123!";

        // Hash same password 10 times
        for (int i = 0; i < 10; i++) {
            String hash = service.hashPassword(password);
            assertThat(service.verifyPassword(password, hash))
                .as("Iteration " + i)
                .isTrue();
        }
    }
}
