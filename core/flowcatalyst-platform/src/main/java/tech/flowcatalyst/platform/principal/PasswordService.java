package tech.flowcatalyst.platform.principal;

import de.mkammerer.argon2.Argon2;
import de.mkammerer.argon2.Argon2Factory;
import jakarta.enterprise.context.ApplicationScoped;

/**
 * Service for password hashing and validation using Argon2id.
 * Argon2id is the OWASP-recommended password hashing algorithm,
 * providing resistance against both GPU and side-channel attacks.
 *
 * Configuration matches Rust implementation:
 * - Memory: 65536 KiB (64 MiB)
 * - Iterations: 3
 * - Parallelism: 4
 * - Hash length: 32 bytes
 */
@ApplicationScoped
public class PasswordService {

    // Argon2id parameters (matching Rust fc-platform)
    private static final int MEMORY_COST = 65536;  // 64 MiB in KiB
    private static final int ITERATIONS = 3;       // Time cost
    private static final int PARALLELISM = 4;      // Parallel threads
    private static final int HASH_LENGTH = 32;     // Output length in bytes
    private static final int SALT_LENGTH = 16;     // Salt length in bytes

    // Password policy
    private static final int MIN_PASSWORD_LENGTH = 12;
    private static final int MAX_PASSWORD_LENGTH = 128;
    private static final String SPECIAL_CHARS = "!@#$%^&*()_+-=[]{}|;':\",./<>?`~";

    private final Argon2 argon2;

    public PasswordService() {
        // Create Argon2id instance (the "id" variant combines Argon2i and Argon2d)
        this.argon2 = Argon2Factory.create(
            Argon2Factory.Argon2Types.ARGON2id,
            SALT_LENGTH,
            HASH_LENGTH
        );
    }

    /**
     * Hash a password using Argon2id.
     *
     * @param plainPassword The plain text password
     * @return The hashed password in PHC format (e.g., $argon2id$v=19$m=65536,t=3,p=4$...)
     */
    public String hashPassword(String plainPassword) {
        if (plainPassword == null || plainPassword.isEmpty()) {
            throw new IllegalArgumentException("Password cannot be null or empty");
        }

        try {
            // Hash with Argon2id - returns PHC format string
            return argon2.hash(ITERATIONS, MEMORY_COST, PARALLELISM, plainPassword.toCharArray());
        } finally {
            // Clear password from memory (Argon2 library handles internal cleanup)
        }
    }

    /**
     * Verify a password against a hash.
     *
     * @param plainPassword The plain text password to verify
     * @param passwordHash  The hash to verify against (PHC format)
     * @return true if password matches the hash
     */
    public boolean verifyPassword(String plainPassword, String passwordHash) {
        if (plainPassword == null || passwordHash == null) {
            return false;
        }

        try {
            // Constant-time verification
            return argon2.verify(passwordHash, plainPassword.toCharArray());
        } catch (Exception e) {
            // Invalid hash format or verification error
            return false;
        }
    }

    /**
     * Check if a password hash needs to be rehashed (e.g., algorithm upgrade).
     * Currently checks if the hash uses Argon2id with current parameters.
     *
     * @param passwordHash The hash to check
     * @return true if the hash should be regenerated with current parameters
     */
    public boolean needsRehash(String passwordHash) {
        if (passwordHash == null || passwordHash.isEmpty()) {
            return true;
        }

        // Check if it's an Argon2id hash with our current parameters
        // PHC format: $argon2id$v=19$m=65536,t=3,p=4$<salt>$<hash>
        if (!passwordHash.startsWith("$argon2id$")) {
            return true; // Not Argon2id, needs rehash
        }

        // Parse parameters from hash
        try {
            String[] parts = passwordHash.split("\\$");
            if (parts.length < 4) {
                return true;
            }

            String params = parts[3]; // m=65536,t=3,p=4
            boolean hasCorrectMemory = params.contains("m=" + MEMORY_COST);
            boolean hasCorrectIterations = params.contains("t=" + ITERATIONS);
            boolean hasCorrectParallelism = params.contains("p=" + PARALLELISM);

            return !(hasCorrectMemory && hasCorrectIterations && hasCorrectParallelism);
        } catch (Exception e) {
            return true; // Parse error, needs rehash
        }
    }

    /**
     * Validate password complexity requirements.
     *
     * @param password The password to validate
     * @throws IllegalArgumentException if password doesn't meet requirements
     */
    public void validatePasswordComplexity(String password) {
        if (password == null || password.length() < MIN_PASSWORD_LENGTH) {
            throw new IllegalArgumentException(
                "Password must be at least " + MIN_PASSWORD_LENGTH + " characters long"
            );
        }

        if (password.length() > MAX_PASSWORD_LENGTH) {
            throw new IllegalArgumentException(
                "Password must be at most " + MAX_PASSWORD_LENGTH + " characters long"
            );
        }

        boolean hasUpper = password.chars().anyMatch(Character::isUpperCase);
        boolean hasLower = password.chars().anyMatch(Character::isLowerCase);
        boolean hasDigit = password.chars().anyMatch(Character::isDigit);
        boolean hasSpecial = password.chars().anyMatch(ch -> SPECIAL_CHARS.indexOf(ch) >= 0);

        if (!hasUpper) {
            throw new IllegalArgumentException("Password must contain at least one uppercase letter");
        }

        if (!hasLower) {
            throw new IllegalArgumentException("Password must contain at least one lowercase letter");
        }

        if (!hasDigit) {
            throw new IllegalArgumentException("Password must contain at least one digit");
        }

        if (!hasSpecial) {
            throw new IllegalArgumentException(
                "Password must contain at least one special character (" + SPECIAL_CHARS + ")"
            );
        }
    }

    /**
     * Validate and hash a password.
     *
     * @param plainPassword The password to validate and hash
     * @return The hashed password
     * @throws IllegalArgumentException if password doesn't meet complexity requirements
     */
    public String validateAndHashPassword(String plainPassword) {
        validatePasswordComplexity(plainPassword);
        return hashPassword(plainPassword);
    }
}
