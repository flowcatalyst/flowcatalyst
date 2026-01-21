package tech.flowcatalyst.platform.application;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.ws.rs.NotFoundException;
import tech.flowcatalyst.platform.application.events.ApplicationDisabledForClient;
import tech.flowcatalyst.platform.application.events.ApplicationEnabledForClient;
import tech.flowcatalyst.platform.application.operations.DisableApplicationForClientCommand;
import tech.flowcatalyst.platform.application.operations.EnableApplicationForClientCommand;
import tech.flowcatalyst.platform.authorization.PermissionRegistry;
import tech.flowcatalyst.platform.client.Client;
import tech.flowcatalyst.platform.client.ClientRepository;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.common.Result;
import tech.flowcatalyst.platform.common.UnitOfWork;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.platform.principal.PrincipalRepository;
import tech.flowcatalyst.platform.shared.TsidGenerator;

import java.util.*;

/**
 * Service for application management and access resolution.
 *
 * Application access is determined by roles:
 * - If a user has any role prefixed with the application code, they can access that app
 * - The client scope depends on user type:
 *   - Anchor users: roles apply to ALL clients
 *   - Partner users: roles apply to GRANTED clients only
 *   - Client users: roles apply to OWN client only
 */
@ApplicationScoped
public class ApplicationService {

    @Inject
    ApplicationRepository applicationRepo;

    @Inject
    ApplicationClientConfigRepository configRepo;

    @Inject
    PrincipalRepository principalRepo;

    @Inject
    ClientRepository clientRepo;

    @Inject
    UnitOfWork unitOfWork;

    // ========================================================================
    // Client Configuration
    // ========================================================================

    /**
     * Enable an application for a client with event sourcing.
     *
     * @param ctx Execution context for tracing
     * @param cmd The enable command
     * @return Result containing the event or error
     */
    public Result<ApplicationEnabledForClient> enableForClient(ExecutionContext ctx, EnableApplicationForClientCommand cmd) {
        Application app = applicationRepo.findByIdOptional(cmd.applicationId())
            .orElseThrow(() -> new NotFoundException("Application not found"));

        Client client = clientRepo.findByIdOptional(cmd.clientId())
            .orElseThrow(() -> new NotFoundException("Client not found"));

        ApplicationClientConfig config = configRepo
            .findByApplicationAndClient(cmd.applicationId(), cmd.clientId())
            .orElseGet(() -> {
                ApplicationClientConfig newConfig = new ApplicationClientConfig();
                newConfig.id = TsidGenerator.generate();
                newConfig.applicationId = app.id;
                newConfig.clientId = client.id;
                return newConfig;
            });

        config.enabled = true;
        if (cmd.baseUrlOverride() != null) {
            config.baseUrlOverride = cmd.baseUrlOverride();
        }
        if (cmd.websiteOverride() != null) {
            config.websiteOverride = cmd.websiteOverride();
        }
        if (cmd.configJson() != null) {
            config.configJson = cmd.configJson();
        }

        ApplicationEnabledForClient event = ApplicationEnabledForClient.fromContext(ctx)
            .configId(config.id)
            .applicationId(app.id)
            .applicationCode(app.code)
            .applicationName(app.name)
            .clientId(client.id)
            .clientIdentifier(client.identifier)
            .clientName(client.name)
            .baseUrlOverride(config.baseUrlOverride)
            .websiteOverride(config.websiteOverride)
            .build();

        return unitOfWork.commit(config, event, cmd);
    }

    /**
     * Disable an application for a client with event sourcing.
     *
     * @param ctx Execution context for tracing
     * @param cmd The disable command
     * @return Result containing the event or error
     */
    public Result<ApplicationDisabledForClient> disableForClient(ExecutionContext ctx, DisableApplicationForClientCommand cmd) {
        Application app = applicationRepo.findByIdOptional(cmd.applicationId())
            .orElseThrow(() -> new NotFoundException("Application not found"));

        Client client = clientRepo.findByIdOptional(cmd.clientId())
            .orElseThrow(() -> new NotFoundException("Client not found"));

        Optional<ApplicationClientConfig> existingConfig = configRepo.findByApplicationAndClient(cmd.applicationId(), cmd.clientId());

        if (existingConfig.isEmpty() || !existingConfig.get().enabled) {
            // Already disabled or never enabled - no-op but still emit event for idempotency
            ApplicationClientConfig config = existingConfig.orElseGet(() -> {
                ApplicationClientConfig newConfig = new ApplicationClientConfig();
                newConfig.id = TsidGenerator.generate();
                newConfig.applicationId = app.id;
                newConfig.clientId = client.id;
                newConfig.enabled = false;
                return newConfig;
            });

            ApplicationDisabledForClient event = ApplicationDisabledForClient.fromContext(ctx)
                .configId(config.id)
                .applicationId(app.id)
                .applicationCode(app.code)
                .applicationName(app.name)
                .clientId(client.id)
                .clientIdentifier(client.identifier)
                .clientName(client.name)
                .build();

            return unitOfWork.commit(config, event, cmd);
        }

        ApplicationClientConfig config = existingConfig.get();
        config.enabled = false;

        ApplicationDisabledForClient event = ApplicationDisabledForClient.fromContext(ctx)
            .configId(config.id)
            .applicationId(app.id)
            .applicationCode(app.code)
            .applicationName(app.name)
            .clientId(client.id)
            .clientIdentifier(client.identifier)
            .clientName(client.name)
            .build();

        return unitOfWork.commit(config, event, cmd);
    }

    /**
     * Get the effective URL for an application and client.
     *
     * @param applicationId Application ID
     * @param clientId Client ID
     * @return The effective URL (client override or default)
     */
    public String getEffectiveUrl(String applicationId, String clientId) {
        Application app = applicationRepo.findByIdOptional(applicationId)
            .orElseThrow(() -> new NotFoundException("Application not found"));

        Optional<ApplicationClientConfig> config = configRepo.findByApplicationAndClient(applicationId, clientId);
        if (config.isPresent() && config.get().baseUrlOverride != null && !config.get().baseUrlOverride.isBlank()) {
            return config.get().baseUrlOverride;
        }

        return app.defaultBaseUrl;
    }

    /**
     * Get the effective website URL for an application and client.
     *
     * @param applicationId Application ID
     * @param clientId Client ID
     * @return The effective website URL (client override or default)
     */
    public String getEffectiveWebsite(String applicationId, String clientId) {
        Application app = applicationRepo.findByIdOptional(applicationId)
            .orElseThrow(() -> new NotFoundException("Application not found"));

        Optional<ApplicationClientConfig> config = configRepo.findByApplicationAndClient(applicationId, clientId);
        if (config.isPresent() && config.get().websiteOverride != null && !config.get().websiteOverride.isBlank()) {
            return config.get().websiteOverride;
        }

        return app.website;
    }

    // ========================================================================
    // Access Resolution
    // ========================================================================

    /**
     * Get all applications accessible by a principal.
     * Application access is determined by having any role prefixed with the application code.
     *
     * @param principalId Principal ID
     * @return List of accessible applications
     */
    public List<Application> getAccessibleApplications(String principalId) {
        // Get all roles from embedded Principal.roles
        Set<String> roleStrings = principalRepo.findByIdOptional(principalId)
            .map(Principal::getRoleNames)
            .orElse(Set.of());

        // Extract application codes from roles
        Set<String> appCodes = PermissionRegistry.extractApplicationCodes(roleStrings);

        if (appCodes.isEmpty()) {
            return List.of();
        }

        // Find applications by codes
        return applicationRepo.findByCodes(appCodes);
    }

    /**
     * Get roles for a specific application that a principal has.
     *
     * @param principalId Principal ID
     * @param applicationCode Application code
     * @return Set of role strings for that application
     */
    public Set<String> getRolesForApplication(String principalId, String applicationCode) {
        Set<String> roleStrings = principalRepo.findByIdOptional(principalId)
            .map(Principal::getRoleNames)
            .orElse(Set.of());

        return PermissionRegistry.filterRolesForApplication(roleStrings, applicationCode);
    }

    /**
     * Check if a principal can access an application.
     *
     * @param principalId Principal ID
     * @param applicationCode Application code
     * @return true if the principal has any roles for this application
     */
    public boolean canAccessApplication(String principalId, String applicationCode) {
        return !getRolesForApplication(principalId, applicationCode).isEmpty();
    }

    /**
     * Check if a principal can access an application for a specific client.
     * This considers:
     * 1. The principal must have roles for the application
     * 2. The application must be enabled for the client (if config exists)
     * 3. The principal must have access to the client (based on user type)
     *
     * @param principalId Principal ID
     * @param applicationCode Application code
     * @param clientId Client ID
     * @return true if access is allowed
     */
    public boolean canAccessApplicationForClient(String principalId, String applicationCode, String clientId) {
        // Check if principal has roles for this application
        if (!canAccessApplication(principalId, applicationCode)) {
            return false;
        }

        // Find the application
        Optional<Application> app = applicationRepo.findByCode(applicationCode);
        if (app.isEmpty() || !app.get().active) {
            return false;
        }

        // Check if application is enabled for client (if config exists and is disabled, deny)
        Optional<ApplicationClientConfig> config = configRepo.findByApplicationAndClient(app.get().id, clientId);
        if (config.isPresent() && !config.get().enabled) {
            return false;
        }

        // Client access is checked separately via ClientAccessService
        // This method only checks application-level access
        return true;
    }

    // ========================================================================
    // Query Methods
    // ========================================================================

    public Optional<Application> findById(String id) {
        return applicationRepo.findByIdOptional(id);
    }

    public Optional<Application> findByCode(String code) {
        return applicationRepo.findByCode(code);
    }

    public List<Application> findAllActive() {
        return applicationRepo.findAllActive();
    }

    public List<Application> findAll() {
        return applicationRepo.listAll();
    }

    public List<ApplicationClientConfig> getConfigsForApplication(String applicationId) {
        return configRepo.findByApplication(applicationId);
    }

    public List<ApplicationClientConfig> getConfigsForClient(String clientId) {
        return configRepo.findByClient(clientId);
    }
}
