package tech.flowcatalyst.platform.bootstrap;

import io.quarkus.runtime.StartupEvent;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.enterprise.context.control.ActivateRequestContext;
import jakarta.enterprise.event.Observes;
import jakarta.inject.Inject;
import jakarta.transaction.Transactional;
import org.eclipse.microprofile.config.inject.ConfigProperty;
import org.jboss.logging.Logger;
import tech.flowcatalyst.platform.authentication.IdpType;
import tech.flowcatalyst.platform.authentication.domain.EmailDomainMapping;
import tech.flowcatalyst.platform.authentication.domain.EmailDomainMappingRepository;
import tech.flowcatalyst.platform.authentication.domain.ScopeType;
import tech.flowcatalyst.platform.authentication.idp.IdentityProvider;
import tech.flowcatalyst.platform.authentication.idp.IdentityProviderRepository;
import tech.flowcatalyst.platform.authentication.idp.IdentityProviderType;
import tech.flowcatalyst.platform.principal.*;
import tech.flowcatalyst.platform.shared.EntityType;
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
    IdentityProviderRepository identityProviderRepository;

    @Inject
    EmailDomainMappingRepository emailDomainMappingRepository;

    @Inject
    PasswordService passwordService;

    @ActivateRequestContext
    @Transactional
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

        // Create anchor domain mapping if it doesn't exist
        if (!emailDomainMappingRepository.isAnchorDomain(emailDomain)) {
            // Ensure we have an internal identity provider
            String internalIdpId = ensureInternalIdentityProvider();

            // Create email domain mapping with ANCHOR scope
            EmailDomainMapping mapping = new EmailDomainMapping();
            mapping.id = TsidGenerator.generate(EntityType.EMAIL_DOMAIN_MAPPING);
            mapping.emailDomain = emailDomain;
            mapping.identityProviderId = internalIdpId;
            mapping.scopeType = ScopeType.ANCHOR;
            emailDomainMappingRepository.persist(mapping);
            LOG.infof("Created anchor domain mapping: %s", emailDomain);
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
        admin.id = TsidGenerator.generate(EntityType.PRINCIPAL);
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

    /**
     * Ensure an internal identity provider exists for password-based authentication.
     * Creates one if it doesn't exist.
     *
     * @return the ID of the internal identity provider
     */
    private String ensureInternalIdentityProvider() {
        // Check if internal IDP already exists
        Optional<IdentityProvider> existing = identityProviderRepository.findByCode("internal");
        if (existing.isPresent()) {
            return existing.get().id;
        }

        // Create internal IDP
        IdentityProvider idp = new IdentityProvider();
        idp.id = TsidGenerator.generate(EntityType.IDENTITY_PROVIDER);
        idp.code = "internal";
        idp.name = "Internal Authentication";
        idp.type = IdentityProviderType.INTERNAL;
        identityProviderRepository.persist(idp);

        LOG.info("Created internal identity provider");
        return idp.id;
    }
}
