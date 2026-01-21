package tech.flowcatalyst.platform.bootstrap;

import io.quarkus.runtime.StartupEvent;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.enterprise.context.control.ActivateRequestContext;
import jakarta.enterprise.event.Observes;
import jakarta.inject.Inject;
import org.eclipse.microprofile.config.inject.ConfigProperty;
import org.jboss.logging.Logger;
import tech.flowcatalyst.platform.authentication.IdpType;
import tech.flowcatalyst.platform.principal.*;
import tech.flowcatalyst.platform.shared.TsidGenerator;

import java.util.List;
import java.util.Optional;

/**
 * Bootstrap service for first-run setup.
 *
 * Creates an initial admin user if:
 * 1. No ANCHOR scope users exist in the system
 * 2. Bootstrap environment variables are configured
 *
 * Environment variables:
 * - FLOWCATALYST_BOOTSTRAP_ADMIN_EMAIL: Email for the bootstrap admin
 * - FLOWCATALYST_BOOTSTRAP_ADMIN_PASSWORD: Password for the bootstrap admin
 * - FLOWCATALYST_BOOTSTRAP_ADMIN_NAME: Display name (optional, defaults to "Bootstrap Admin")
 *
 * The bootstrap admin will:
 * - Have ANCHOR scope (platform-wide access)
 * - Have platform:super-admin role
 * - Use INTERNAL authentication (password-based)
 *
 * The email domain will automatically be registered as an anchor domain.
 */
@ApplicationScoped
public class BootstrapService {

    private static final Logger LOG = Logger.getLogger(BootstrapService.class);

    @ConfigProperty(name = "flowcatalyst.bootstrap.admin.email", defaultValue = "")
    String bootstrapEmail;

    @ConfigProperty(name = "flowcatalyst.bootstrap.admin.password", defaultValue = "")
    String bootstrapPassword;

    @ConfigProperty(name = "flowcatalyst.bootstrap.admin.name", defaultValue = "Bootstrap Admin")
    String bootstrapName;

    @Inject
    PrincipalRepository principalRepository;

    @Inject
    AnchorDomainRepository anchorDomainRepository;

    @Inject
    PasswordService passwordService;

    @ActivateRequestContext
    void onStart(@Observes StartupEvent event) {
        if (!shouldBootstrap()) {
            return;
        }

        LOG.info("=== BOOTSTRAP SERVICE ===");
        LOG.info("No anchor users found - checking for bootstrap configuration...");

        if (bootstrapEmail.isBlank() || bootstrapPassword.isBlank()) {
            LOG.warn("No bootstrap admin configured. Set FLOWCATALYST_BOOTSTRAP_ADMIN_EMAIL and FLOWCATALYST_BOOTSTRAP_ADMIN_PASSWORD to create an initial admin.");
            LOG.warn("=========================");
            return;
        }

        try {
            bootstrap();
            LOG.info("Bootstrap completed successfully!");
            LOG.info("=========================");
        } catch (Exception e) {
            LOG.error("Bootstrap failed: " + e.getMessage(), e);
            LOG.info("=========================");
        }
    }

    private boolean shouldBootstrap() {
        // Check if any ANCHOR scope users exist
        List<Principal> principals = principalRepository.listAll();
        boolean hasAnchorUser = principals.stream()
            .anyMatch(p -> p.type == PrincipalType.USER && p.scope == UserScope.ANCHOR);

        if (hasAnchorUser) {
            LOG.debug("Anchor users already exist - skipping bootstrap");
            return false;
        }

        return true;
    }

    private void bootstrap() {
        // Validate email format
        if (!bootstrapEmail.contains("@")) {
            throw new IllegalArgumentException("Invalid bootstrap email format: " + bootstrapEmail);
        }

        String emailDomain = bootstrapEmail.substring(bootstrapEmail.indexOf('@') + 1);

        // Check if user already exists (idempotency)
        Optional<Principal> existing = principalRepository.findByEmail(bootstrapEmail);
        if (existing.isPresent()) {
            LOG.infof("Bootstrap user already exists: %s", bootstrapEmail);
            return;
        }

        // Create anchor domain if it doesn't exist
        if (!anchorDomainRepository.existsByDomain(emailDomain)) {
            AnchorDomain anchorDomain = new AnchorDomain();
            anchorDomain.id = TsidGenerator.generate();
            anchorDomain.domain = emailDomain;
            anchorDomainRepository.persist(anchorDomain);
            LOG.infof("Created anchor domain: %s", emailDomain);
        }

        // Validate and hash password
        String passwordHash;
        try {
            passwordHash = passwordService.validateAndHashPassword(bootstrapPassword);
        } catch (IllegalArgumentException e) {
            LOG.warnf("Bootstrap password does not meet complexity requirements: %s", e.getMessage());
            LOG.warn("Hashing password anyway for bootstrap (consider changing it after login)");
            passwordHash = passwordService.hashPassword(bootstrapPassword);
        }

        // Create the bootstrap admin user
        Principal admin = new Principal();
        admin.id = TsidGenerator.generate();
        admin.type = PrincipalType.USER;
        admin.scope = UserScope.ANCHOR;
        admin.clientId = null; // Anchor users don't have a home client
        admin.name = bootstrapName;
        admin.active = true;

        admin.userIdentity = new UserIdentity();
        admin.userIdentity.email = bootstrapEmail;
        admin.userIdentity.emailDomain = emailDomain;
        admin.userIdentity.idpType = IdpType.INTERNAL;
        admin.userIdentity.passwordHash = passwordHash;

        // Add super-admin role
        admin.roles.add(new Principal.RoleAssignment("platform:super-admin", "BOOTSTRAP"));

        principalRepository.persist(admin);

        LOG.infof("Created bootstrap admin: %s (%s)", bootstrapName, bootstrapEmail);
        LOG.info("This user has platform:super-admin role and ANCHOR scope");
    }
}
