package tech.flowcatalyst.platform.security;

import io.quarkus.test.junit.QuarkusTest;
import jakarta.inject.Inject;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import tech.flowcatalyst.platform.authentication.IdpRoleMapping;
import tech.flowcatalyst.platform.authentication.IdpRoleMappingRepository;
import tech.flowcatalyst.platform.authentication.OidcSyncService;
import tech.flowcatalyst.platform.authorization.PrincipalRole;
import tech.flowcatalyst.platform.authorization.RoleService;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.platform.shared.TsidGenerator;

import java.util.List;

import java.util.UUID;

import static org.assertj.core.api.Assertions.*;

/**
 * CRITICAL SECURITY TESTS: IDP Role Authorization
 *
 * These tests validate the most important security control in the authentication system:
 * preventing unauthorized role grants via compromised or misconfigured Identity Providers.
 *
 * THREAT MODEL:
 * 1. Partner's IDP is compromised → Attacker grants themselves admin roles
 * 2. Partner's IDP is misconfigured → Accidentally grants wrong roles
 * 3. Partner modifies IDP configuration → Attempts privilege escalation
 *
 * SECURITY CONTROL:
 * Only IDP roles explicitly whitelisted in idp_role_mappings table are accepted.
 * All other roles are rejected and logged.
 *
 * BUSINESS IMPACT:
 * Failure of this control could allow:
 * - Unauthorized access to customer data
 * - Privilege escalation attacks
 * - Complete platform compromise
 *
 * Uses test role definitions from TestRoles factory:
 * - test:admin, test:editor, test:viewer
 * - platform:test-tenant-admin
 */
@Tag("integration")
@QuarkusTest
class IdpRoleAuthorizationSecurityTest {

    @Inject
    OidcSyncService oidcSyncService;

    @Inject
    RoleService roleService;

    @Inject
    IdpRoleMappingRepository idpRoleMappingRepo;

    // ========================================
    // ATTACK SCENARIO 1: Compromised IDP
    // ========================================

    @Test
    @DisplayName("SECURITY: Compromised IDP grants super-admin role → REJECTED")
    void shouldPreventUnauthorizedSuperAdminGrant_whenIdpCompromised() {
        // ATTACK SCENARIO:
        // Partner's Keycloak server is compromised by attacker.
        // Attacker modifies Keycloak configuration to grant all users "super-admin" role.
        // Attacker attempts to login and gain platform admin access.

        // ARRANGE: No IDP role mappings exist
        // (super-admin role is NOT in the whitelist)
        String email = uniqueEmail("attacker");
        String subject = uniqueSubject("compromised-idp");

        // ACT: Attacker logs in via compromised IDP
        Principal attacker = oidcSyncService.syncOidcLogin(
            email,
            "Malicious Actor",
            subject,
            null,
            List.of("super-admin", "platform-owner", "god-mode", "root")
        );

        // ASSERT: NO roles assigned - attack prevented
        List<PrincipalRole> assignments = roleService.findAssignmentsByPrincipal(attacker.id);
        assertThat(assignments).isEmpty();

        // VERIFY: User created but powerless
        assertThat(attacker).isNotNull();
        assertThat(attacker.active).isTrue();
        assertThat(attacker.userIdentity.email).isEqualTo(email);

        // Security layer prevented privilege escalation
        // Logs should contain warnings about unauthorized roles (not tested here)
    }

    @Test
    @DisplayName("SECURITY: Compromised IDP with mix of valid and malicious roles")
    void shouldFilterMaliciousRoles_whenIdpCompromisedPartially() {
        // ATTACK SCENARIO:
        // Sophisticated attacker compromises IDP and adds malicious roles
        // alongside legitimate roles to avoid detection.

        // ARRANGE: Only authorize one legitimate role (maps to code-defined test:viewer)
        String idpRole = uniqueIdpRole("partner-viewer");
        createIdpMapping(idpRole, "test:viewer");

        String email = uniqueEmail("attacker");
        String subject = uniqueSubject("idp");

        // ACT: Attacker logs in with mix of legitimate and malicious roles
        Principal attacker = oidcSyncService.syncOidcLogin(
            email,
            "Attacker",
            subject,
            null,
            List.of(
                idpRole,             // Legitimate (whitelisted)
                "super-admin",       // MALICIOUS
                "platform-admin",    // MALICIOUS
                "root"               // MALICIOUS
            )
        );

        // ASSERT: Only legitimate role granted, malicious roles rejected
        List<PrincipalRole> assignments = roleService.findAssignmentsByPrincipal(attacker.id);
        assertThat(assignments).hasSize(1);
        assertThat(assignments.get(0).roleName).isEqualTo("test:viewer");

        // Attack partially prevented - attacker only got intended access
    }

    // ========================================
    // ATTACK SCENARIO 2: Misconfigured IDP
    // ========================================

    @Test
    @DisplayName("SECURITY: Misconfigured IDP sends internal role names → REJECTED")
    void shouldRejectInternalRoleNames_whenIdpSendsWrongFormat() {
        // ATTACK SCENARIO:
        // Partner's IDP is misconfigured and sends FlowCatalyst internal role names
        // instead of IDP role names. This could be accidental or intentional.

        // ARRANGE: Internal roles exist in code (TestRoles), but no IDP mappings for internal names
        String email = uniqueEmail("user");
        String subject = uniqueSubject("idp");

        // ACT: Misconfigured IDP sends internal role names
        Principal user = oidcSyncService.syncOidcLogin(
            email,
            "User",
            subject,
            null,
            List.of("test:admin", "platform:test-tenant-admin")  // Internal names!
        );

        // ASSERT: Rejected - internal role names not in IDP whitelist
        List<PrincipalRole> assignments = roleService.findAssignmentsByPrincipal(user.id);
        assertThat(assignments).isEmpty();
    }

    @Test
    @DisplayName("SECURITY: IDP sends roles with similar names to internal roles")
    void shouldRejectSimilarRoleNames_whenNotExactMatch() {
        // ATTACK SCENARIO:
        // Attacker tries role names similar to internal roles, hoping for weak matching.

        // ARRANGE: Create IDP mapping for specific IDP role name (maps to test:admin)
        String idpRole = uniqueIdpRole("keycloak-admin");
        createIdpMapping(idpRole, "test:admin");

        String email = uniqueEmail("attacker");
        String subject = uniqueSubject("idp");

        // ACT: Attacker tries variations of the role name (none match the unique idpRole)
        Principal attacker = oidcSyncService.syncOidcLogin(
            email,
            "Attacker",
            subject,
            null,
            List.of(
                "admin",           // No mapping
                "Admin",           // Uppercase variant
                "ADMIN",           // All caps
                "admin ",          // With space
                " admin",          // Leading space
                "keycloak-Admin"   // Wrong case
            )
        );

        // ASSERT: All rejected - exact match required
        List<PrincipalRole> assignments = roleService.findAssignmentsByPrincipal(attacker.id);
        assertThat(assignments).isEmpty();
    }

    // ========================================
    // ATTACK SCENARIO 3: IDP Role Injection
    // ========================================

    @Test
    @DisplayName("SECURITY: Multiple compromised IDPs attack simultaneously")
    void shouldDefendAgainstMultipleCompromisedIdps_whenAttackersCoordinate() {
        // ATTACK SCENARIO:
        // Multiple partner IDPs are compromised.
        // Attackers coordinate to grant themselves maximum privileges.

        // ARRANGE: Only one legitimate mapping exists (with unique name so it doesn't conflict)
        String idpRole = uniqueIdpRole("partner-editor");
        createIdpMapping(idpRole, "test:editor");

        // ACT: Three different attackers from different compromised IDPs
        Principal attacker1 = oidcSyncService.syncOidcLogin(
            uniqueEmail("attacker1"), "Attacker 1", uniqueSubject("idp-a"), null,
            List.of("super-admin", "platform-owner"));

        Principal attacker2 = oidcSyncService.syncOidcLogin(
            uniqueEmail("attacker2"), "Attacker 2", uniqueSubject("idp-b"), null,
            List.of("root", "god-mode"));

        Principal attacker3 = oidcSyncService.syncOidcLogin(
            uniqueEmail("attacker3"), "Attacker 3", uniqueSubject("idp-c"), null,
            List.of("admin", "superuser"));

        // ASSERT: All attacks blocked - no roles granted
        assertThat(roleService.findAssignmentsByPrincipal(attacker1.id)).isEmpty();
        assertThat(roleService.findAssignmentsByPrincipal(attacker2.id)).isEmpty();
        assertThat(roleService.findAssignmentsByPrincipal(attacker3.id)).isEmpty();
    }

    // ========================================
    // ATTACK SCENARIO 4: Role Mapping Manipulation
    // ========================================

    @Test
    @DisplayName("SECURITY: Deleting IDP role mapping immediately revokes access")
    void shouldImmediatelyRevokeAccess_whenIdpRoleMappingDeleted() {
        // SCENARIO:
        // Platform admin discovers a partner IDP role was incorrectly authorized.
        // They delete the IDP role mapping to revoke access.
        // Users who already have the role should lose it on next login.

        // ARRANGE: Create IDP mapping to test:admin role
        String idpRole = uniqueIdpRole("partner-sensitive");
        IdpRoleMapping mapping = createIdpMapping(idpRole, "test:admin");

        String email = uniqueEmail("user");
        String subject = uniqueSubject("idp");

        // User logs in and gets role
        Principal user = oidcSyncService.syncOidcLogin(
            email, "User", subject, null,
            List.of(idpRole));

        assertThat(roleService.findAssignmentsByPrincipal(user.id)).hasSize(1);

        // ACT: Platform admin deletes mapping (removes from whitelist)
        idpRoleMappingRepo.delete(mapping);

        // User logs in again with same IDP role
        oidcSyncService.syncIdpRoles(user, List.of(idpRole));

        // ASSERT: Role immediately revoked
        assertThat(roleService.findAssignmentsByPrincipal(user.id)).isEmpty();
    }

    @Test
    @DisplayName("SECURITY: Cannot bypass whitelist by creating duplicate mapping")
    void shouldEnforceUniqueMappings_whenDuplicateAttempted() {
        // SCENARIO:
        // Attacker with platform admin access tries to create duplicate mapping
        // to bypass security controls.

        // ARRANGE: Create legitimate mapping with unique IDP role name
        String idpRole = uniqueIdpRole("partner-admin");
        createIdpMapping(idpRole, "test:admin");

        // ACT & ASSERT: Attempting to create duplicate should be prevented
        // (This would be tested in IdpRoleMappingRepository tests,
        // but we verify the behavior here as well)

        // The database unique constraint should prevent this
        assertThatCode(() -> {
            createIdpMapping(idpRole, "test:admin");
        }).isInstanceOf(Exception.class);
    }

    // ========================================
    // DEFENSE VERIFICATION TESTS
    // ========================================

    @Test
    @DisplayName("SECURITY: Empty IDP roles list removes all IDP-granted roles")
    void shouldRemoveAllIdpRoles_whenIdpSendsEmptyList() {
        // SCENARIO:
        // IDP revokes all roles for a user (e.g., user account suspended in IDP).
        // System should remove all IDP-granted roles on next login.

        // ARRANGE: User has IDP roles
        String idpRole = uniqueIdpRole("partner-editor");
        createIdpMapping(idpRole, "test:editor");

        String email = uniqueEmail("user");
        String subject = uniqueSubject("idp");

        Principal user = oidcSyncService.syncOidcLogin(
            email, "User", subject, null,
            List.of(idpRole));

        assertThat(roleService.findAssignmentsByPrincipal(user.id)).hasSize(1);

        // ACT: IDP sends empty roles (user suspended/revoked)
        oidcSyncService.syncIdpRoles(user, List.of());

        // ASSERT: All IDP roles removed
        assertThat(roleService.findAssignmentsByPrincipal(user.id)).isEmpty();
    }

    @Test
    @DisplayName("SECURITY: Manual role assignments preserved during IDP sync")
    void shouldPreserveManualRoles_whenIdpRolesSynced() {
        // SCENARIO:
        // Platform admin manually assigns sensitive role to user.
        // IDP role sync should NOT remove manually assigned roles.

        // ARRANGE: User with manual role
        String email = uniqueEmail("user");
        String subject = uniqueSubject("idp");
        Principal user = oidcSyncService.syncOidcLogin(
            email, "User", subject, null, List.of());

        roleService.assignRole(user.id, "test:admin", "MANUAL");

        // Create IDP mapping and grant IDP role
        String idpRole = uniqueIdpRole("partner-viewer");
        createIdpMapping(idpRole, "test:viewer");

        // ACT: Sync IDP roles
        oidcSyncService.syncIdpRoles(user, List.of(idpRole));

        // ASSERT: Both manual and IDP roles present
        assertThat(roleService.findAssignmentsByPrincipal(user.id))
            .extracting(a -> a.roleName)
            .containsExactlyInAnyOrder("test:admin", "test:viewer");
    }

    @Test
    @DisplayName("SECURITY: IDP role changes reflected on every login")
    void shouldUpdateRolesOnEveryLogin_whenIdpRolesChange() {
        // SCENARIO:
        // User's roles in IDP change between logins.
        // System should reflect current IDP state.

        // ARRANGE: Create 3 IDP role mappings to code-defined roles
        String idpRole1 = uniqueIdpRole("idp-role-1");
        String idpRole2 = uniqueIdpRole("idp-role-2");
        String idpRole3 = uniqueIdpRole("idp-role-3");
        createIdpMapping(idpRole1, "test:viewer");
        createIdpMapping(idpRole2, "test:editor");
        createIdpMapping(idpRole3, "test:admin");

        String email = uniqueEmail("user");
        String subject = uniqueSubject("idp");

        // Login 1: User has test:viewer role
        Principal user = oidcSyncService.syncOidcLogin(
            email, "User", subject, null,
            List.of(idpRole1));

        assertThat(roleService.findAssignmentsByPrincipal(user.id))
            .extracting(a -> a.roleName)
            .containsExactly("test:viewer");

        // Login 2: User promoted to viewer + editor
        oidcSyncService.syncIdpRoles(user, List.of(idpRole1, idpRole2));

        assertThat(roleService.findAssignmentsByPrincipal(user.id))
            .extracting(a -> a.roleName)
            .containsExactlyInAnyOrder("test:viewer", "test:editor");

        // Login 3: User changed to admin only
        oidcSyncService.syncIdpRoles(user, List.of(idpRole3));

        assertThat(roleService.findAssignmentsByPrincipal(user.id))
            .extracting(a -> a.roleName)
            .containsExactly("test:admin");
    }

    // ========================================
    // COMMON IDP DEFAULT ROLES TESTS
    // ========================================

    @Test
    @DisplayName("SECURITY: Keycloak default roles are filtered out")
    void shouldFilterOutKeycloakDefaultRoles_whenNotWhitelisted() {
        // SCENARIO:
        // Keycloak sends default roles like "offline_access", "uma_authorization".
        // These should be filtered unless explicitly whitelisted.

        // ARRANGE: Create legitimate mapping
        String idpRole = uniqueIdpRole("partner-viewer");
        createIdpMapping(idpRole, "test:viewer");

        String email = uniqueEmail("user");
        String subject = uniqueSubject("keycloak");

        // ACT: Login with Keycloak defaults + legitimate role
        Principal user = oidcSyncService.syncOidcLogin(
            email, "User", subject, null,
            List.of(
                idpRole,                    // Legitimate
                "offline_access",           // Keycloak default
                "uma_authorization",        // Keycloak default
                "default-roles-keycloak"    // Keycloak default
            )
        );

        // ASSERT: Only legitimate role granted
        assertThat(roleService.findAssignmentsByPrincipal(user.id))
            .hasSize(1)
            .extracting(a -> a.roleName)
            .containsExactly("test:viewer");
    }

    // ========================================
    // HELPER METHODS
    // ========================================

    private String uniqueEmail(String prefix) {
        return prefix + "-" + UUID.randomUUID().toString().substring(0, 8) + "@test.com";
    }

    private String uniqueIdpRole(String prefix) {
        return prefix + "-" + UUID.randomUUID().toString().substring(0, 8);
    }

    private String uniqueSubject(String prefix) {
        return prefix + "-" + UUID.randomUUID().toString().substring(0, 8);
    }

    private IdpRoleMapping createIdpMapping(String idpRoleName, String internalRoleName) {
        IdpRoleMapping mapping = new IdpRoleMapping();
        mapping.id = TsidGenerator.generate();
        mapping.idpRoleName = idpRoleName;
        mapping.internalRoleName = internalRoleName;
        idpRoleMappingRepo.persist(mapping);
        // MongoDB doesn't need flush - writes are immediate
        return mapping;
    }
}
