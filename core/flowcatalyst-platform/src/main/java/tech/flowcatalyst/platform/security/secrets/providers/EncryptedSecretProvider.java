package tech.flowcatalyst.platform.security.secrets.providers;

import jakarta.annotation.PostConstruct;
import jakarta.enterprise.context.ApplicationScoped;
import org.eclipse.microprofile.config.inject.ConfigProperty;
import org.jboss.logging.Logger;
import tech.flowcatalyst.platform.security.secrets.SecretProvider;
import tech.flowcatalyst.platform.security.secrets.SecretResolutionException;

import javax.crypto.Cipher;
import javax.crypto.SecretKey;
import javax.crypto.SecretKeyFactory;
import javax.crypto.spec.GCMParameterSpec;
import javax.crypto.spec.PBEKeySpec;
import javax.crypto.spec.SecretKeySpec;
import java.nio.ByteBuffer;
import java.nio.charset.StandardCharsets;
import java.security.SecureRandom;
import java.security.spec.KeySpec;
import java.util.Base64;

/**
 * Secret provider that encrypts/decrypts locally stored secrets using AES-256-GCM.
 *
 * Reference formats:
 * - encrypted:BASE64_ENCODED_CIPHERTEXT - Pre-encrypted ciphertext
 * - encrypt:PLAINTEXT_SECRET - Plaintext to be encrypted on storage
 *
 * When saving a secret with "encrypt:" prefix, the service will encrypt it
 * and convert to "encrypted:" format for storage.
 *
 * The ciphertext format: IV (12 bytes) + encrypted data + auth tag (16 bytes)
 *
 * Configuration (in order of precedence):
 * 1. FLOWCATALYST_APP_KEY env var or flowcatalyst.app-key property
 *    - Base64-encoded 256-bit (32 byte) key
 *    - Generate with: openssl rand -base64 32
 * 2. flowcatalyst.secrets.encryption.key (legacy, same format)
 * 3. flowcatalyst.secrets.encryption.passphrase + salt (derives key from passphrase)
 */
@ApplicationScoped
public class EncryptedSecretProvider implements SecretProvider {

    private static final Logger LOG = Logger.getLogger(EncryptedSecretProvider.class);

    private static final String PREFIX = "encrypted:";
    private static final String PLAINTEXT_PREFIX = "encrypt:";
    private static final String ALGORITHM = "AES/GCM/NoPadding";
    private static final int GCM_IV_LENGTH = 12;
    private static final int GCM_TAG_LENGTH = 128; // bits
    private static final SecureRandom SECURE_RANDOM = new SecureRandom();

    // Primary: APP_KEY (like Laravel)
    @ConfigProperty(name = "flowcatalyst.app-key")
    java.util.Optional<String> appKey;

    // Legacy: explicit encryption key
    @ConfigProperty(name = "flowcatalyst.secrets.encryption.key")
    java.util.Optional<String> encryptionKey;

    // Legacy: passphrase-based key derivation
    @ConfigProperty(name = "flowcatalyst.secrets.encryption.passphrase")
    java.util.Optional<String> passphrase;

    @ConfigProperty(name = "flowcatalyst.secrets.encryption.salt", defaultValue = "flowcatalyst-default-salt")
    String salt;

    private SecretKey secretKey;

    @PostConstruct
    void init() {
        // 1. Try APP_KEY first (recommended)
        if (appKey.isPresent() && !appKey.get().isBlank()) {
            secretKey = parseKey(appKey.get(), "flowcatalyst.app-key");
            LOG.info("Encrypted secret provider initialized with APP_KEY");
        }
        // 2. Try legacy encryption key
        else if (encryptionKey.isPresent() && !encryptionKey.get().isBlank()) {
            secretKey = parseKey(encryptionKey.get(), "flowcatalyst.secrets.encryption.key");
            LOG.info("Encrypted secret provider initialized with encryption key");
        }
        // 3. Try passphrase derivation
        else if (passphrase.isPresent() && !passphrase.get().isBlank()) {
            secretKey = deriveKey(passphrase.get(), salt);
            LOG.info("Encrypted secret provider initialized with derived key from passphrase");
        }
        // 4. No key configured
        else {
            LOG.warn("No APP_KEY configured. Local secret encryption unavailable. " +
                "Set FLOWCATALYST_APP_KEY env var or flowcatalyst.app-key property. " +
                "Generate with: openssl rand -base64 32");
            secretKey = null;
        }
    }

    private SecretKey parseKey(String base64Key, String configName) {
        try {
            byte[] keyBytes = Base64.getDecoder().decode(base64Key);
            if (keyBytes.length != 32) {
                throw new IllegalStateException(
                    configName + " must be 256 bits (32 bytes) Base64-encoded. Got: " + keyBytes.length + " bytes. " +
                    "Generate with: openssl rand -base64 32");
            }
            return new SecretKeySpec(keyBytes, "AES");
        } catch (IllegalArgumentException e) {
            throw new IllegalStateException(
                configName + " must be valid Base64. Generate with: openssl rand -base64 32", e);
        }
    }

    @Override
    public String resolve(String reference) throws SecretResolutionException {
        if (!canHandle(reference)) {
            throw new SecretResolutionException("Invalid reference format for encrypted provider");
        }

        if (secretKey == null) {
            throw new SecretResolutionException(
                "Encryption key not configured. Cannot decrypt secrets.");
        }

        try {
            String ciphertext = reference.substring(PREFIX.length());
            byte[] cipherBytes = Base64.getDecoder().decode(ciphertext);

            // Extract IV and encrypted data
            ByteBuffer buffer = ByteBuffer.wrap(cipherBytes);
            byte[] iv = new byte[GCM_IV_LENGTH];
            buffer.get(iv);
            byte[] encrypted = new byte[buffer.remaining()];
            buffer.get(encrypted);

            // Decrypt
            Cipher cipher = Cipher.getInstance(ALGORITHM);
            GCMParameterSpec spec = new GCMParameterSpec(GCM_TAG_LENGTH, iv);
            cipher.init(Cipher.DECRYPT_MODE, secretKey, spec);
            byte[] plaintext = cipher.doFinal(encrypted);

            return new String(plaintext, StandardCharsets.UTF_8);
        } catch (Exception e) {
            throw new SecretResolutionException("Failed to decrypt secret", e);
        }
    }

    @Override
    public ValidationResult validate(String reference) {
        if (!canHandle(reference)) {
            return ValidationResult.failure("Invalid reference format for encrypted provider");
        }

        if (secretKey == null) {
            return ValidationResult.failure("Encryption key not configured");
        }

        try {
            String ciphertext = reference.substring(PREFIX.length());
            byte[] cipherBytes = Base64.getDecoder().decode(ciphertext);

            // Check minimum length (IV + at least some data)
            if (cipherBytes.length < GCM_IV_LENGTH + 16) {
                return ValidationResult.failure("Invalid ciphertext: too short");
            }

            // Try to decrypt to verify it's valid
            resolve(reference);

            return ValidationResult.success("Encrypted secret is valid and decryptable");
        } catch (IllegalArgumentException e) {
            return ValidationResult.failure("Invalid Base64 encoding");
        } catch (SecretResolutionException e) {
            return ValidationResult.failure("Decryption failed: " + e.getMessage());
        } catch (Exception e) {
            return ValidationResult.failure("Validation failed: " + e.getMessage());
        }
    }

    @Override
    public boolean canHandle(String reference) {
        return reference != null &&
            (reference.startsWith(PREFIX) || reference.startsWith(PLAINTEXT_PREFIX));
    }

    @Override
    public String getType() {
        return "encrypted";
    }

    /**
     * Check if a reference is plaintext that needs encryption.
     *
     * @param reference The secret reference
     * @return true if this is a plaintext reference (encrypt: prefix)
     */
    public boolean isPlaintextReference(String reference) {
        return reference != null && reference.startsWith(PLAINTEXT_PREFIX);
    }

    /**
     * Encrypt a plaintext secret and return the encrypted reference.
     *
     * @param plaintext The plaintext secret value
     * @return Encrypted reference in format "encrypted:BASE64_CIPHERTEXT"
     * @throws SecretResolutionException if encryption fails or key not configured
     */
    public String encrypt(String plaintext) throws SecretResolutionException {
        if (secretKey == null) {
            throw new SecretResolutionException(
                "Encryption key not configured. Cannot encrypt secrets.");
        }

        try {
            // Generate random IV
            byte[] iv = new byte[GCM_IV_LENGTH];
            SECURE_RANDOM.nextBytes(iv);

            // Encrypt
            Cipher cipher = Cipher.getInstance(ALGORITHM);
            GCMParameterSpec spec = new GCMParameterSpec(GCM_TAG_LENGTH, iv);
            cipher.init(Cipher.ENCRYPT_MODE, secretKey, spec);
            byte[] ciphertext = cipher.doFinal(plaintext.getBytes(StandardCharsets.UTF_8));

            // Combine IV + ciphertext
            ByteBuffer buffer = ByteBuffer.allocate(iv.length + ciphertext.length);
            buffer.put(iv);
            buffer.put(ciphertext);

            // Return as encrypted reference
            return PREFIX + Base64.getEncoder().encodeToString(buffer.array());
        } catch (Exception e) {
            throw new SecretResolutionException("Failed to encrypt secret", e);
        }
    }

    /**
     * Convert a plaintext reference (encrypt:SECRET) to encrypted reference.
     *
     * @param reference The plaintext reference
     * @return Encrypted reference
     * @throws SecretResolutionException if encryption fails
     */
    public String encryptReference(String reference) throws SecretResolutionException {
        if (!isPlaintextReference(reference)) {
            throw new SecretResolutionException("Reference is not a plaintext reference");
        }
        String plaintext = reference.substring(PLAINTEXT_PREFIX.length());
        return encrypt(plaintext);
    }

    /**
     * Check if encryption is available (key is configured).
     *
     * @return true if encryption/decryption is available
     */
    public boolean isEncryptionAvailable() {
        return secretKey != null;
    }

    private SecretKey deriveKey(String passphrase, String salt) {
        try {
            SecretKeyFactory factory = SecretKeyFactory.getInstance("PBKDF2WithHmacSHA256");
            KeySpec spec = new PBEKeySpec(
                passphrase.toCharArray(),
                salt.getBytes(StandardCharsets.UTF_8),
                65536,  // iterations
                256     // key length in bits
            );
            return new SecretKeySpec(factory.generateSecret(spec).getEncoded(), "AES");
        } catch (Exception e) {
            throw new IllegalStateException("Failed to derive encryption key", e);
        }
    }
}
