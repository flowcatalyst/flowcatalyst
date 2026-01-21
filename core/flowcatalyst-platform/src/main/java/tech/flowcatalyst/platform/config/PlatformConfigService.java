package tech.flowcatalyst.platform.config;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.transaction.Transactional;
import tech.flowcatalyst.platform.security.secrets.SecretService;
import tech.flowcatalyst.platform.shared.EntityType;
import tech.flowcatalyst.platform.shared.TsidGenerator;

import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Optional;
import java.util.Set;

/**
 * Service for managing platform configuration entries.
 *
 * Provides methods for:
 * - Getting config values with optional secret resolution
 * - Setting config values (with automatic secret preparation)
 * - Deleting configs
 * - Access control checks
 */
@ApplicationScoped
public class PlatformConfigService {

    public static final String SUPER_ADMIN_ROLE = "platform:super-admin";
    public static final String ADMIN_ROLE = "platform:admin";

    @Inject
    SecretService secretService;

    @Inject
    PlatformConfigRepository configRepo;

    @Inject
    PlatformConfigAccessRepository accessRepo;

    /**
     * Get a single config value with optional secret resolution.
     *
     * @param applicationCode The application code
     * @param section The section name
     * @param property The property name
     * @param scope The scope (GLOBAL or CLIENT)
     * @param clientId The client ID (required for CLIENT scope, null for GLOBAL)
     * @param resolveSecrets If true, resolve SECRET type values via SecretService
     * @return The config value if found
     */
    public Optional<String> getValue(String applicationCode, String section, String property,
                                      ConfigScope scope, String clientId,
                                      boolean resolveSecrets) {
        return configRepo.findByKey(applicationCode, section, property, scope, clientId)
            .map(config -> resolveValue(config, resolveSecrets));
    }

    /**
     * Get a config value, falling back to GLOBAL if CLIENT scope not found.
     *
     * @param applicationCode The application code
     * @param section The section name
     * @param property The property name
     * @param clientId The client ID to check for CLIENT scope override
     * @param resolveSecrets If true, resolve SECRET type values via SecretService
     * @return The config value (client-specific if exists, otherwise global)
     */
    public Optional<String> getValueWithFallback(String applicationCode, String section, String property,
                                                  String clientId, boolean resolveSecrets) {
        // First try client-specific
        if (clientId != null) {
            Optional<String> clientValue = getValue(applicationCode, section, property,
                ConfigScope.CLIENT, clientId, resolveSecrets);
            if (clientValue.isPresent()) {
                return clientValue;
            }
        }
        // Fall back to global
        return getValue(applicationCode, section, property, ConfigScope.GLOBAL, null, resolveSecrets);
    }

    /**
     * Get all configs for a section as a property map.
     *
     * @param applicationCode The application code
     * @param section The section name
     * @param scope The scope (GLOBAL or CLIENT)
     * @param clientId The client ID (required for CLIENT scope, null for GLOBAL)
     * @param resolveSecrets If true, resolve SECRET type values via SecretService
     * @return Map of property name to value
     */
    public Map<String, String> getSection(String applicationCode, String section,
                                           ConfigScope scope, String clientId,
                                           boolean resolveSecrets) {
        List<PlatformConfig> configs = configRepo.findByApplicationAndSection(
            applicationCode, section, scope, clientId);

        Map<String, String> result = new LinkedHashMap<>();
        for (PlatformConfig config : configs) {
            result.put(config.property, resolveValue(config, resolveSecrets));
        }
        return result;
    }

    /**
     * Get section with client overrides merged over global values.
     *
     * @param applicationCode The application code
     * @param section The section name
     * @param clientId The client ID for override lookup
     * @param resolveSecrets If true, resolve SECRET type values via SecretService
     * @return Map of property name to value (client values override global)
     */
    public Map<String, String> getSectionWithFallback(String applicationCode, String section,
                                                       String clientId, boolean resolveSecrets) {
        // Start with global configs
        Map<String, String> result = getSection(applicationCode, section, ConfigScope.GLOBAL, null, resolveSecrets);

        // Overlay client-specific configs if clientId provided
        if (clientId != null) {
            Map<String, String> clientConfigs = getSection(applicationCode, section,
                ConfigScope.CLIENT, clientId, resolveSecrets);
            result.putAll(clientConfigs);
        }

        return result;
    }

    /**
     * Set a config value.
     *
     * @param applicationCode The application code
     * @param section The section name
     * @param property The property name
     * @param scope The scope (GLOBAL or CLIENT)
     * @param clientId The client ID (required for CLIENT scope, null for GLOBAL)
     * @param value The value to store
     * @param valueType The value type (PLAIN or SECRET)
     * @param description Optional description
     * @return The created or updated config
     */
    @Transactional
    public PlatformConfig setValue(String applicationCode, String section, String property,
                                    ConfigScope scope, String clientId,
                                    String value, ConfigValueType valueType,
                                    String description) {
        // Validate scope/clientId consistency
        if (scope == ConfigScope.GLOBAL && clientId != null) {
            throw new IllegalArgumentException("clientId must be null for GLOBAL scope");
        }
        if (scope == ConfigScope.CLIENT && (clientId == null || clientId.isBlank())) {
            throw new IllegalArgumentException("clientId is required for CLIENT scope");
        }

        // Prepare secret value for storage if needed
        String storedValue = valueType == ConfigValueType.SECRET
            ? secretService.prepareForStorage(value)
            : value;

        // Check if config already exists
        Optional<PlatformConfig> existing = configRepo.findByKey(
            applicationCode, section, property, scope, clientId);

        if (existing.isPresent()) {
            // Update existing
            PlatformConfig config = existing.get();
            config.value = storedValue;
            config.valueType = valueType;
            config.description = description;
            configRepo.update(config);
            return config;
        } else {
            // Create new
            PlatformConfig config = new PlatformConfig();
            config.id = TsidGenerator.generate(EntityType.PLATFORM_CONFIG);
            config.applicationCode = applicationCode;
            config.section = section;
            config.property = property;
            config.scope = scope;
            config.clientId = clientId;
            config.value = storedValue;
            config.valueType = valueType;
            config.description = description;
            configRepo.persist(config);
            return config;
        }
    }

    /**
     * Delete a config.
     *
     * @param applicationCode The application code
     * @param section The section name
     * @param property The property name
     * @param scope The scope (GLOBAL or CLIENT)
     * @param clientId The client ID (required for CLIENT scope, null for GLOBAL)
     * @return true if deleted, false if not found
     */
    @Transactional
    public boolean delete(String applicationCode, String section, String property,
                          ConfigScope scope, String clientId) {
        return configRepo.findByKey(applicationCode, section, property, scope, clientId)
            .map(config -> {
                configRepo.delete(config);
                return true;
            })
            .orElse(false);
    }

    /**
     * Check if a user with the given roles can access configs for an application.
     *
     * @param applicationCode The application code
     * @param userRoles The user's roles
     * @param isWrite True for write access, false for read access
     * @return true if access is granted
     */
    public boolean canAccess(String applicationCode, Set<String> userRoles, boolean isWrite) {
        var log = org.jboss.logging.Logger.getLogger(PlatformConfigService.class);
        log.infof("canAccess: appCode=%s, roles=%s, isWrite=%s", applicationCode, userRoles, isWrite);

        // Platform admins can access everything
        if (userRoles != null) {
            for (String role : userRoles) {
                String lowerRole = role.toLowerCase();
                log.infof("Checking role: %s (lower: %s)", role, lowerRole);
                // Check for any platform admin role (handles various formats)
                if (lowerRole.contains("platform") && lowerRole.contains("admin")) {
                    log.infof("Access granted via role: %s", role);
                    return true;
                }
            }
        }

        // Check application-specific access
        if (isWrite) {
            boolean hasAccess = accessRepo.hasWriteAccess(applicationCode, userRoles);
            log.infof("Write access from repo: %s", hasAccess);
            return hasAccess;
        } else {
            boolean hasAccess = accessRepo.hasReadAccess(applicationCode, userRoles);
            log.infof("Read access from repo: %s", hasAccess);
            return hasAccess;
        }
    }

    /**
     * Get all configs for an application.
     *
     * @param applicationCode The application code
     * @param scope The scope (GLOBAL or CLIENT)
     * @param clientId The client ID (required for CLIENT scope, null for GLOBAL)
     * @return List of configs
     */
    public List<PlatformConfig> getConfigs(String applicationCode, ConfigScope scope, String clientId) {
        return configRepo.findByApplication(applicationCode, scope, clientId);
    }

    /**
     * Get config by ID.
     */
    public Optional<PlatformConfig> getById(String id) {
        return configRepo.findByIdOptional(id);
    }

    private String resolveValue(PlatformConfig config, boolean resolveSecrets) {
        if (resolveSecrets && config.valueType == ConfigValueType.SECRET) {
            return secretService.resolve(config.value);
        }
        return config.value;
    }
}
