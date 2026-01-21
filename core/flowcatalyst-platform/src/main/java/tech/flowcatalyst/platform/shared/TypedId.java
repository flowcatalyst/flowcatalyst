package tech.flowcatalyst.platform.shared;

import io.micrometer.core.instrument.Counter;
import io.micrometer.core.instrument.MeterRegistry;
import jakarta.annotation.PostConstruct;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import org.jboss.logging.Logger;

import java.util.Objects;
import java.util.regex.Pattern;

/**
 * Centralized utility for typed ID validation.
 *
 * IDs follow the Stripe pattern where the prefix is stored WITH the ID:
 * - Format: "{prefix}_{tsid}" (e.g., "clt_0HZXEQ5Y8JY5Z")
 * - Total length: 17 characters (3-char prefix + underscore + 13-char TSID)
 *
 * Since IDs are stored with prefixes in the database, no serialization/deserialization
 * is needed. This class provides validation utilities to ensure IDs have the correct
 * format and type.
 *
 * Features:
 * - Type validation with detailed error messages
 * - Metrics for monitoring invalid ID attempts
 * - Utility methods for extracting prefix/rawId
 *
 * Usage in Resources:
 * <pre>
 * {@code
 * @Inject TypedId typedId;
 *
 * // Validate an ID has the correct type
 * typedId.validate(EntityType.CLIENT, id);  // throws if invalid
 *
 * // Check if an ID is valid
 * if (typedId.isValid(EntityType.CLIENT, id)) { ... }
 * }
 * </pre>
 *
 * For static contexts, use the Ops inner class:
 * <pre>
 * {@code
 * TypedId.Ops.validate(EntityType.CLIENT, id);
 * }
 * </pre>
 */
@ApplicationScoped
public class TypedId {

    private static final Logger LOG = Logger.getLogger(TypedId.class);

    /**
     * Separator between prefix and ID.
     */
    public static final String SEPARATOR = "_";

    /**
     * Pattern for valid typed IDs (3-char prefix + underscore + 13-char TSID).
     * Example: "clt_0HZXEQ5Y8JY5Z"
     */
    private static final Pattern TYPED_ID_PATTERN = Pattern.compile(
        "^[a-z]{3}_[0-9A-HJKMNP-TV-Z]{13}$", Pattern.CASE_INSENSITIVE);

    /**
     * Pattern for valid TSID strings (Crockford Base32, 13 chars).
     * Crockford Base32 uses: 0-9, A-Z excluding I, L, O, U
     */
    private static final Pattern TSID_PATTERN = Pattern.compile(
        "^[0-9A-HJKMNP-TV-Z]{13}$", Pattern.CASE_INSENSITIVE);

    @Inject
    MeterRegistry registry;

    private Counter validationSuccessCounter;
    private Counter validationFailureCounter;

    @PostConstruct
    void init() {
        validationSuccessCounter = Counter.builder("flowcatalyst.typed_id.validation")
            .tag("result", "success")
            .description("Count of successful typed ID validations")
            .register(registry);

        validationFailureCounter = Counter.builder("flowcatalyst.typed_id.validation")
            .tag("result", "failure")
            .description("Count of failed typed ID validations")
            .register(registry);
    }

    // ========================================================================
    // Instance methods (with metrics)
    // ========================================================================

    /**
     * Validates that an ID has the correct format and entity type.
     * Records metrics for success/failure.
     *
     * @param expectedType the expected entity type
     * @param id the typed ID (e.g., "clt_0HZXEQ5Y8JY5Z")
     * @throws InvalidTypedIdException if the ID is malformed or wrong type
     */
    public void validate(EntityType expectedType, String id) {
        try {
            Ops.validate(expectedType, id);
            if (validationSuccessCounter != null) {
                validationSuccessCounter.increment();
            }
        } catch (InvalidTypedIdException e) {
            if (validationFailureCounter != null) {
                validationFailureCounter.increment();
            }
            recordValidationFailure(expectedType, id, e.getReason());
            throw e;
        }
    }

    /**
     * Validates an ID if present, returning null for null/blank inputs.
     * Useful for optional ID fields.
     *
     * @param expectedType the expected entity type
     * @param id the typed ID, may be null
     * @return the ID if valid, or null if input is null/blank
     * @throws InvalidTypedIdException if the ID is present but invalid
     */
    public String validateOrNull(EntityType expectedType, String id) {
        if (id == null || id.isBlank()) {
            return null;
        }
        validate(expectedType, id);
        return id;
    }

    /**
     * Validates a typed ID without throwing exceptions.
     *
     * @param expectedType the expected entity type
     * @param id the typed ID to validate
     * @return true if valid, false otherwise
     */
    public boolean isValid(EntityType expectedType, String id) {
        return Ops.isValid(expectedType, id);
    }

    /**
     * Extracts the raw TSID portion from a typed ID.
     *
     * @param id the typed ID (e.g., "clt_0HZXEQ5Y8JY5Z")
     * @return the raw TSID (e.g., "0HZXEQ5Y8JY5Z")
     */
    public String extractRawId(String id) {
        return Ops.extractRawId(id);
    }

    /**
     * Extracts the entity type from a typed ID.
     *
     * @param id the typed ID
     * @return the entity type, or null if not recognized
     */
    public EntityType extractType(String id) {
        return Ops.extractType(id);
    }

    // ========================================================================
    // Deprecated instance methods (for backwards compatibility)
    // ========================================================================

    /**
     * Serializes an internal ID to external format.
     *
     * @param type the entity type
     * @param internalId the internal ID (may be raw TSID or already prefixed)
     * @return the prefixed typed ID, or null if input is null
     * @deprecated IDs are now stored with prefixes. No serialization needed.
     *             Use the ID directly from the database.
     */
    @Deprecated
    public String serialize(EntityType type, String internalId) {
        return Ops.serialize(type, internalId);
    }

    /**
     * Deserializes an external typed ID to internal format.
     *
     * @param expectedType the expected entity type
     * @param externalId the external typed ID
     * @return the raw TSID portion
     * @throws InvalidTypedIdException if the ID is invalid
     * @deprecated IDs are now stored with prefixes. Use validate() to check
     *             the ID format and use the ID directly.
     */
    @Deprecated
    public String deserialize(EntityType expectedType, String externalId) {
        try {
            String result = Ops.deserialize(expectedType, externalId);
            if (validationSuccessCounter != null) {
                validationSuccessCounter.increment();
            }
            return result;
        } catch (InvalidTypedIdException e) {
            if (validationFailureCounter != null) {
                validationFailureCounter.increment();
            }
            recordValidationFailure(expectedType, externalId, e.getReason());
            throw e;
        }
    }

    private void recordValidationFailure(EntityType expectedType, String id, String reason) {
        LOG.warnf("Invalid typed ID: expected=%s, value='%s', reason=%s",
            expectedType, maskId(id), reason);

        if (registry != null) {
            registry.counter("flowcatalyst.typed_id.validation.errors",
                "expected_type", expectedType.name(),
                "reason", reason
            ).increment();
        }
    }

    private String maskId(String id) {
        if (id == null) return "null";
        if (id.length() <= 8) return id;
        return id.substring(0, 8) + "...";
    }

    // ========================================================================
    // Static operations (for use in records/DTOs without injection)
    // ========================================================================

    /**
     * Static operations for typed IDs.
     * Use these in contexts where dependency injection is not available.
     *
     * Note: These methods do not record metrics. For monitored operations,
     * inject TypedId and use instance methods.
     */
    public static final class Ops {

        private Ops() {
            // Static utility class
        }

        /**
         * Validates that an ID has the correct format and entity type.
         *
         * @param expectedType the expected entity type
         * @param id the typed ID (e.g., "clt_0HZXEQ5Y8JY5Z")
         * @throws InvalidTypedIdException if the ID is malformed or wrong type
         */
        public static void validate(EntityType expectedType, String id) {
            Objects.requireNonNull(expectedType, "EntityType must not be null");

            if (id == null || id.isBlank()) {
                throw new InvalidTypedIdException(
                    "ID is required",
                    expectedType,
                    id,
                    "empty"
                );
            }

            int separatorIndex = id.indexOf(SEPARATOR);
            if (separatorIndex == -1) {
                throw new InvalidTypedIdException(
                    String.format("Invalid ID format. Expected '%s_<id>' but got '%s'",
                        expectedType.prefix(), id),
                    expectedType,
                    id,
                    "missing_separator"
                );
            }

            String prefix = id.substring(0, separatorIndex);
            String rawId = id.substring(separatorIndex + 1);

            // Check prefix matches expected type
            EntityType actualType = EntityType.fromPrefix(prefix);
            if (actualType == null) {
                throw new InvalidTypedIdException(
                    String.format("Unknown ID prefix '%s'", prefix),
                    expectedType,
                    id,
                    "unknown_prefix"
                );
            }

            if (actualType != expectedType) {
                throw new InvalidTypedIdException(
                    String.format("ID type mismatch. Expected '%s' but got '%s'",
                        expectedType.prefix(), actualType.prefix()),
                    expectedType,
                    id,
                    "type_mismatch"
                );
            }

            // Validate TSID format
            if (!isValidTsid(rawId)) {
                throw new InvalidTypedIdException(
                    String.format("Invalid TSID format in ID '%s'", id),
                    expectedType,
                    id,
                    "invalid_tsid"
                );
            }
        }

        /**
         * Validates a typed ID without throwing exceptions.
         *
         * @param expectedType the expected entity type
         * @param id the typed ID to validate
         * @return true if valid, false otherwise
         */
        public static boolean isValid(EntityType expectedType, String id) {
            if (id == null || id.isBlank()) {
                return false;
            }

            int separatorIndex = id.indexOf(SEPARATOR);
            if (separatorIndex == -1) {
                return false;
            }

            String prefix = id.substring(0, separatorIndex);
            String rawId = id.substring(separatorIndex + 1);

            EntityType actualType = EntityType.fromPrefix(prefix);
            if (actualType != expectedType) {
                return false;
            }

            return isValidTsid(rawId);
        }

        /**
         * Checks if a string is a valid typed ID (any type).
         *
         * @param id the potential typed ID
         * @return true if valid format, false otherwise
         */
        public static boolean isValidFormat(String id) {
            return id != null && TYPED_ID_PATTERN.matcher(id).matches();
        }

        /**
         * Extracts the entity type from a typed ID.
         *
         * @param id the typed ID
         * @return the entity type, or null if not recognized
         */
        public static EntityType extractType(String id) {
            if (id == null || id.isBlank()) {
                return null;
            }

            int separatorIndex = id.indexOf(SEPARATOR);
            if (separatorIndex == -1) {
                return null;
            }

            String prefix = id.substring(0, separatorIndex);
            return EntityType.fromPrefix(prefix);
        }

        /**
         * Extracts the raw TSID portion from a typed ID.
         *
         * @param id the typed ID (e.g., "clt_0HZXEQ5Y8JY5Z")
         * @return the raw TSID (e.g., "0HZXEQ5Y8JY5Z")
         * @throws IllegalArgumentException if the ID format is invalid
         */
        public static String extractRawId(String id) {
            if (id == null || id.isBlank()) {
                throw new IllegalArgumentException("ID cannot be null or blank");
            }
            int separatorIndex = id.indexOf(SEPARATOR);
            if (separatorIndex == -1) {
                throw new IllegalArgumentException("Invalid typed ID format: missing separator");
            }
            return id.substring(separatorIndex + 1);
        }

        /**
         * Extracts the prefix from a typed ID.
         *
         * @param id the typed ID (e.g., "clt_0HZXEQ5Y8JY5Z")
         * @return the prefix (e.g., "clt")
         * @throws IllegalArgumentException if the ID format is invalid
         */
        public static String extractPrefix(String id) {
            if (id == null || id.isBlank()) {
                throw new IllegalArgumentException("ID cannot be null or blank");
            }
            int separatorIndex = id.indexOf(SEPARATOR);
            if (separatorIndex == -1) {
                throw new IllegalArgumentException("Invalid typed ID format: missing separator");
            }
            return id.substring(0, separatorIndex);
        }

        // ====================================================================
        // Deprecated methods (for backwards compatibility during migration)
        // ====================================================================

        /**
         * Serializes an internal ID to external format.
         *
         * @param type the entity type
         * @param internalId the internal ID (may be raw TSID or already prefixed)
         * @return the prefixed typed ID, or null if input is null
         * @deprecated IDs are now stored with prefixes. No serialization needed.
         *             Use the ID directly from the database.
         */
        @Deprecated
        public static String serialize(EntityType type, String internalId) {
            if (internalId == null) return null;
            // If already prefixed, return as-is
            if (internalId.contains(SEPARATOR)) return internalId;
            return type.prefix() + SEPARATOR + internalId;
        }

        /**
         * Deserializes an external typed ID to internal format.
         *
         * @param expectedType the expected entity type
         * @param externalId the external typed ID
         * @return the raw TSID portion
         * @throws InvalidTypedIdException if the ID is invalid
         * @deprecated IDs are now stored with prefixes. Use validate() to check
         *             the ID format and use the ID directly.
         */
        @Deprecated
        public static String deserialize(EntityType expectedType, String externalId) {
            validate(expectedType, externalId);
            int separatorIndex = externalId.indexOf(SEPARATOR);
            return externalId.substring(separatorIndex + 1);
        }

        private static boolean isValidTsid(String id) {
            return id != null && TSID_PATTERN.matcher(id).matches();
        }
    }

    // ========================================================================
    // Exception
    // ========================================================================

    /**
     * Exception thrown when a typed ID is invalid.
     */
    public static class InvalidTypedIdException extends IllegalArgumentException {

        private final EntityType expectedType;
        private final String providedValue;
        private final String reason;

        public InvalidTypedIdException(String message, EntityType expectedType, String providedValue, String reason) {
            super(message);
            this.expectedType = expectedType;
            this.providedValue = providedValue;
            this.reason = reason;
        }

        public EntityType getExpectedType() {
            return expectedType;
        }

        public String getProvidedValue() {
            return providedValue;
        }

        public String getReason() {
            return reason;
        }
    }
}
