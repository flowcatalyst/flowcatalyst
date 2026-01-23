package tech.flowcatalyst.platform.dev;

import io.quarkus.arc.Arc;
import io.quarkus.runtime.LaunchMode;
import io.quarkus.runtime.StartupEvent;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.enterprise.context.control.ActivateRequestContext;
import jakarta.enterprise.event.Observes;
import jakarta.inject.Inject;
import org.eclipse.microprofile.config.inject.ConfigProperty;
import org.jboss.logging.Logger;
import tech.flowcatalyst.platform.authentication.AuthProvider;
import tech.flowcatalyst.platform.authentication.IdpType;
import tech.flowcatalyst.platform.principal.*;
import tech.flowcatalyst.platform.client.*;
import tech.flowcatalyst.platform.application.Application;
import tech.flowcatalyst.platform.application.ApplicationOperations;
import tech.flowcatalyst.platform.application.ApplicationRepository;
import tech.flowcatalyst.platform.application.operations.createapplication.CreateApplicationCommand;
import tech.flowcatalyst.platform.shared.EntityType;
import tech.flowcatalyst.platform.shared.TsidGenerator;
import tech.flowcatalyst.platform.audit.AuditContext;
import tech.flowcatalyst.platform.common.ExecutionContext;

import java.util.List;
import java.util.Optional;

/**
 * Seeds development data on application startup.
 *
 * Runs when any of:
 * - quarkus.launch-mode=DEV or TEST (and flowcatalyst.dev.seed-data=true)
 * - flowcatalyst.dev.force-seed=true (for dev-build native executables)
 *
 * Default credentials:
 *   Platform Admin: admin@flowcatalyst.local / DevPassword123!
 *   Client Admin:   alice@acme.com / DevPassword123!
 *   Regular User:   bob@acme.com / DevPassword123!
 */
@ApplicationScoped
public class DevDataSeeder {

    private static final Logger LOG = Logger.getLogger(DevDataSeeder.class);
    private static final String DEV_PASSWORD = "DevPassword123!";

    @Inject
    LaunchMode launchMode;

    @ConfigProperty(name = "flowcatalyst.dev.seed-data", defaultValue = "true")
    boolean seedDataEnabled;

    @ConfigProperty(name = "flowcatalyst.dev.force-seed", defaultValue = "false")
    boolean forceSeed;

    @Inject
    PasswordService passwordService;

    @Inject
    ClientRepository clientRepo;

    @Inject
    PrincipalRepository principalRepo;

    @Inject
    AnchorDomainRepository anchorDomainRepo;

    @Inject
    ClientAuthConfigRepository authConfigRepo;

    @Inject
    ClientAccessGrantRepository grantRepo;

    @Inject
    ApplicationRepository applicationRepo;

    // These are looked up lazily to avoid early bean resolution that can cause startup issues
    private AuditContext getAuditContext() {
        return Arc.container().instance(AuditContext.class).get();
    }

    private ApplicationOperations getApplicationOperations() {
        return Arc.container().instance(ApplicationOperations.class).get();
    }

    @ActivateRequestContext
    void onStart(@Observes StartupEvent event) {
        if (!shouldSeed()) {
            return;
        }

        LOG.info("=== DEV DATA SEEDER ===");
        LOG.info("Seeding development data...");

        try {
            seedData();
            LOG.info("Development data seeded successfully!");
            LOG.info("");
            LOG.info("Default logins:");
            LOG.infof("  Platform Admin: admin@flowcatalyst.local / %s", DEV_PASSWORD);
            LOG.infof("  Client Admin:   alice@acme.com / %s", DEV_PASSWORD);
            LOG.infof("  Regular User:   bob@acme.com / %s", DEV_PASSWORD);
            LOG.info("=======================");
        } catch (Exception e) {
            LOG.warn("Dev data seeding skipped (data may already exist): " + e.getMessage());
        }
    }

    private boolean shouldSeed() {
        // Force seed bypasses launch mode check (for dev-build native executables)
        if (forceSeed) {
            LOG.info("Force seed enabled - seeding regardless of launch mode");
            return true;
        }

        if (launchMode != LaunchMode.DEVELOPMENT && launchMode != LaunchMode.TEST) {
            LOG.debug("Skipping dev seeder - not in dev/test mode (use flowcatalyst.dev.force-seed=true to override)");
            return false;
        }

        if (!seedDataEnabled) {
            LOG.debug("Skipping dev seeder - disabled via config");
            return false;
        }

        return true;
    }

    void seedData() {
        // Set up SYSTEM principal for audit context since we're outside HTTP request
        // Must be inside @Transactional because it may need to persist the SYSTEM principal
        getAuditContext().setSystemPrincipal();

        seedAnchorDomain();
        seedClients();
        seedAuthConfig();
        seedUsers();
        seedApplications();
        // Note: Platform event types are synced via PlatformSyncStartupObserver, not DevDataSeeder
    }

    private void seedAnchorDomain() {
        if (anchorDomainRepo.existsByDomain("flowcatalyst.local")) {
            return;
        }

        AnchorDomain anchor = new AnchorDomain();
        anchor.id = TsidGenerator.generate(EntityType.ANCHOR_DOMAIN);
        anchor.domain = "flowcatalyst.local";
        anchorDomainRepo.persist(anchor);
        LOG.info("Created anchor domain: flowcatalyst.local");
    }

    private void seedClients() {
        createClientIfNotExists("Acme Corporation", "acme", ClientStatus.ACTIVE);
        createClientIfNotExists("Globex Industries", "globex", ClientStatus.ACTIVE);
        createClientIfNotExists("Initech Solutions", "initech", ClientStatus.ACTIVE);
        createClientIfNotExists("Umbrella Corp", "umbrella", ClientStatus.SUSPENDED);
    }

    private Client createClientIfNotExists(String name, String identifier, ClientStatus status) {
        Optional<Client> existing = clientRepo.findByIdentifier(identifier);
        if (existing.isPresent()) {
            return existing.get();
        }

        Client client = new Client();
        client.id = TsidGenerator.generate(EntityType.CLIENT);
        client.name = name;
        client.identifier = identifier;
        client.status = status;
        clientRepo.persist(client);
        LOG.infof("Created client: %s (%s)", name, identifier);
        return client;
    }

    private void seedAuthConfig() {
        createAuthConfigIfNotExists("flowcatalyst.local", AuthProvider.INTERNAL);
        createAuthConfigIfNotExists("acme.com", AuthProvider.INTERNAL);
        createAuthConfigIfNotExists("globex.com", AuthProvider.INTERNAL);
        createAuthConfigIfNotExists("initech.com", AuthProvider.INTERNAL);
        createAuthConfigIfNotExists("partner.io", AuthProvider.INTERNAL);
    }

    private void createAuthConfigIfNotExists(String domain, AuthProvider provider) {
        if (authConfigRepo.findByEmailDomain(domain).isPresent()) {
            return;
        }

        ClientAuthConfig config = new ClientAuthConfig();
        config.id = TsidGenerator.generate(EntityType.CLIENT_AUTH_CONFIG);
        config.emailDomain = domain;
        config.authProvider = provider;
        authConfigRepo.persist(config);
    }

    private void seedUsers() {
        String passwordHash = passwordService.hashPassword(DEV_PASSWORD);

        // Platform admin (anchor domain - access to all clients)
        createUserIfNotExists(
            "admin@flowcatalyst.local",
            "Platform Administrator",
            null,  // No home client
            passwordHash,
            List.of("platform:super-admin")
        );

        // Get Acme client
        Client acme = clientRepo.findByIdentifier("acme").orElse(null);
        Client globex = clientRepo.findByIdentifier("globex").orElse(null);

        if (acme != null) {
            // Acme client admin
            createUserIfNotExists(
                "alice@acme.com",
                "Alice Johnson",
                acme.id,
                passwordHash,
                List.of("acme:client-admin", "dispatch:admin")
            );

            // Acme regular user
            createUserIfNotExists(
                "bob@acme.com",
                "Bob Smith",
                acme.id,
                passwordHash,
                List.of("dispatch:user")
            );
        }

        if (globex != null) {
            // Globex user
            createUserIfNotExists(
                "charlie@globex.com",
                "Charlie Brown",
                globex.id,
                passwordHash,
                List.of("dispatch:admin")
            );
        }

        // Partner user (cross-client access)
        Principal partner = createUserIfNotExists(
            "diana@partner.io",
            "Diana Partner",
            null,  // No home client
            passwordHash,
            List.of("dispatch:viewer")
        );

        // Grant partner access to Acme and Globex
        if (partner != null && acme != null) {
            createGrantIfNotExists(partner.id, acme.id);
        }
        if (partner != null && globex != null) {
            createGrantIfNotExists(partner.id, globex.id);
        }
    }

    private Principal createUserIfNotExists(String email, String name, String clientId,
                                            String passwordHash, List<String> roles) {
        Optional<Principal> existing = principalRepo.findByEmail(email);
        if (existing.isPresent()) {
            return existing.get();
        }

        Principal user = new Principal();
        user.id = TsidGenerator.generate(EntityType.PRINCIPAL);
        user.type = PrincipalType.USER;
        user.clientId = clientId;
        user.name = name;
        user.active = true;

        user.userIdentity = new UserIdentity();
        user.userIdentity.email = email;
        user.userIdentity.emailDomain = email.substring(email.indexOf('@') + 1);
        user.userIdentity.idpType = IdpType.INTERNAL;
        user.userIdentity.passwordHash = passwordHash;

        // Add roles (embedded in principal)
        for (String roleName : roles) {
            user.roles.add(new Principal.RoleAssignment(roleName, "DEV_SEEDER"));
        }

        principalRepo.persist(user);

        LOG.infof("Created user: %s (%s)", name, email);
        return user;
    }

    private void createGrantIfNotExists(String principalId, String clientId) {
        if (grantRepo.existsByPrincipalIdAndClientId(principalId, clientId)) {
            return;
        }

        ClientAccessGrant grant = new ClientAccessGrant();
        grant.id = TsidGenerator.generate(EntityType.CLIENT_ACCESS_GRANT);
        grant.principalId = principalId;
        grant.clientId = clientId;
        grantRepo.persist(grant);
    }

    // ========================================================================
    // Applications
    // ========================================================================

    private void seedApplications() {
        createApplicationIfNotExists("tms", "Transport Management System",
            "End-to-end transportation planning, execution, and optimization");
        createApplicationIfNotExists("wms", "Warehouse Management System",
            "Inventory control, picking, packing, and warehouse operations");
        createApplicationIfNotExists("oms", "Order Management System",
            "Order processing, fulfillment orchestration, and customer service");
        createApplicationIfNotExists("track", "Shipment Tracking",
            "Real-time visibility and tracking for shipments and assets");
        createApplicationIfNotExists("yard", "Yard Management System",
            "Dock scheduling, trailer tracking, and yard operations");
        createApplicationIfNotExists("platform", "Platform Services",
            "Core platform infrastructure and shared services");
    }

    private void createApplicationIfNotExists(String code, String name, String description) {
        if (applicationRepo.findByCode(code).isPresent()) {
            return;
        }

        ExecutionContext context = ExecutionContext.create(getAuditContext().requirePrincipalId());
        CreateApplicationCommand command = new CreateApplicationCommand(code, name, description, null, null);
        getApplicationOperations().createApplication(command, context);
        LOG.infof("Created application: %s (%s)", name, code);
    }
}
