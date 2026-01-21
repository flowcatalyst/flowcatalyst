package tech.flowcatalyst.platform.service;

import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.InjectMocks;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;
import tech.flowcatalyst.platform.authentication.IdpRoleMapping;
import tech.flowcatalyst.platform.authentication.IdpRoleMappingRepository;
import tech.flowcatalyst.platform.authentication.IdpType;
import tech.flowcatalyst.platform.authentication.OidcSyncService;
import tech.flowcatalyst.platform.authorization.PrincipalRole;
import tech.flowcatalyst.platform.authorization.RoleService;
import tech.flowcatalyst.platform.principal.*;
import tech.flowcatalyst.platform.shared.TsidGenerator;

import java.time.Instant;
import java.util.List;
import java.util.Optional;
import java.util.Set;

import static org.assertj.core.api.Assertions.*;
import static org.mockito.Mockito.*;

/**
 * Unit tests for OidcSyncService.
 * CRITICAL SECURITY TESTS: Tests IDP role authorization logic.
 *
 * This service prevents a critical attack vector:
 * - Compromised/misconfigured IDP granting unauthorized roles
 * - Only explicitly whitelisted IDP roles are accepted
 * - All unauthorized roles are rejected and logged
 *
 * These tests verify that the security control works correctly.
 */
@ExtendWith(MockitoExtension.class)
class OidcSyncServiceTest {

    @Mock
    private IdpRoleMappingRepository idpRoleMappingRepo;

    @Mock
    private RoleService roleService;

    @Mock
    private UserService userService;

    @InjectMocks
    private OidcSyncService service;

    // ========================================
    // syncOidcUser TESTS
    // ========================================

    @Test
    @DisplayName("syncOidcUser should create or update user and update last login")
    void syncOidcUser_shouldCreateOrUpdateUser_whenCalled() {
        // Arrange
        String email = "alice@customer.com";
        String name = "Alice Smith";
        String externalIdpId = "google-oauth2|123";
        String clientId = "0HZTEST00200";

        Principal principal = createOidcPrincipal("0HZTEST00001", email);
        when(userService.createOrUpdateOidcUser(email, name, externalIdpId, clientId, null))
            .thenReturn(principal);

        // Act
        Principal result = service.syncOidcUser(email, name, externalIdpId, clientId);

        // Assert
        assertThat(result).isEqualTo(principal);
        verify(userService).createOrUpdateOidcUser(email, name, externalIdpId, clientId, null);
        verify(userService).updateLastLogin(principal.id);
    }

    // ========================================
    // syncIdpRoles TESTS - CRITICAL SECURITY
    // ========================================

    @Test
    @DisplayName("SECURITY: syncIdpRoles should only accept whitelisted IDP roles")
    void syncIdpRoles_shouldOnlyAcceptWhitelistedRoles_whenIdpRolesProvided() {
        // Arrange
        Principal principal = createOidcPrincipal("0HZTEST00001", "alice@customer.com");

        // IDP provides 3 roles from token
        List<String> idpRoles = List.of(
            "customer-viewer",      // Authorized - mapped to internal role "platform:viewer"
            "customer-operator",    // Authorized - mapped to internal role "platform:operator"
            "super-admin"           // UNAUTHORIZED - attacker trying to inject
        );

        // Only 2 roles are in the whitelist (idp_role_mappings table)
        when(idpRoleMappingRepo.findByIdpRoleName("customer-viewer"))
            .thenReturn(Optional.of(createMapping("0HZMAPPING01", "customer-viewer", "platform:viewer")));
        when(idpRoleMappingRepo.findByIdpRoleName("customer-operator"))
            .thenReturn(Optional.of(createMapping("0HZMAPPING02", "customer-operator", "platform:operator")));
        when(idpRoleMappingRepo.findByIdpRoleName("super-admin"))
            .thenReturn(Optional.empty()); // NOT IN WHITELIST - REJECTED

        when(roleService.removeRolesBySource("0HZTEST00001", "IDP_SYNC")).thenReturn(0L);

        // Act
        Set<String> result = service.syncIdpRoles(principal, idpRoles);

        // Assert: Only 2 roles accepted (super-admin rejected)
        assertThat(result).containsExactlyInAnyOrder("platform:viewer", "platform:operator");
        assertThat(result).doesNotContain("super-admin"); // super-admin was not mapped

        // Verify role assignments
        verify(roleService).assignRole("0HZTEST00001", "platform:viewer", "IDP_SYNC");
        verify(roleService).assignRole("0HZTEST00001", "platform:operator", "IDP_SYNC");
        verify(roleService, never()).assignRole(eq("0HZTEST00001"), eq("super-admin"), anyString());

        // Verify old roles removed
        verify(roleService).removeRolesBySource("0HZTEST00001", "IDP_SYNC");
    }

    @Test
    @DisplayName("SECURITY: syncIdpRoles should reject all roles when none are whitelisted")
    void syncIdpRoles_shouldRejectAllRoles_whenNoneWhitelisted() {
        // Arrange: Compromised IDP sending unauthorized roles
        Principal principal = createOidcPrincipal("0HZTEST00001", "attacker@malicious.com");

        List<String> maliciousRoles = List.of(
            "super-admin",
            "platform-admin",
            "root"
        );

        // NONE of these roles are in the whitelist
        when(idpRoleMappingRepo.findByIdpRoleName(anyString()))
            .thenReturn(Optional.empty());

        when(roleService.removeRolesBySource("0HZTEST00001", "IDP_SYNC")).thenReturn(0L);

        // Act
        Set<String> result = service.syncIdpRoles(principal, maliciousRoles);

        // Assert: ALL roles rejected
        assertThat(result).isEmpty();

        // Verify NO roles assigned
        verify(roleService, never()).assignRole(eq("0HZTEST00001"), anyString(), anyString());

        // Verify old roles still removed (clean slate)
        verify(roleService).removeRolesBySource("0HZTEST00001", "IDP_SYNC");
    }

    @Test
    @DisplayName("SECURITY: syncIdpRoles should remove old IDP roles before assigning new ones")
    void syncIdpRoles_shouldRemoveOldIdpRoles_beforeAssigningNew() {
        // Arrange
        Principal principal = createOidcPrincipal("0HZTEST00001", "alice@customer.com");

        List<String> idpRoles = List.of("customer-viewer");

        when(idpRoleMappingRepo.findByIdpRoleName("customer-viewer"))
            .thenReturn(Optional.of(createMapping("0HZMAPPING01", "customer-viewer", "platform:viewer")));

        when(roleService.removeRolesBySource("0HZTEST00001", "IDP_SYNC")).thenReturn(3L); // 3 old roles removed

        // Act
        service.syncIdpRoles(principal, idpRoles);

        // Assert: Old roles removed BEFORE new ones assigned
        var inOrder = inOrder(roleService);
        inOrder.verify(roleService).removeRolesBySource("0HZTEST00001", "IDP_SYNC");
        inOrder.verify(roleService).assignRole("0HZTEST00001", "platform:viewer", "IDP_SYNC");
    }

    @Test
    @DisplayName("syncIdpRoles should handle null IDP roles list")
    void syncIdpRoles_shouldHandleNull_whenIdpRolesIsNull() {
        // Arrange
        Principal principal = createOidcPrincipal("0HZTEST00001", "alice@customer.com");
        when(roleService.removeRolesBySource("0HZTEST00001", "IDP_SYNC")).thenReturn(0L);

        // Act
        Set<String> result = service.syncIdpRoles(principal, null);

        // Assert: No crash, empty result
        assertThat(result).isEmpty();
        verify(roleService, never()).assignRole(anyString(), anyString(), anyString());

        // Still removes old roles (clean slate)
        verify(roleService).removeRolesBySource("0HZTEST00001", "IDP_SYNC");
    }

    @Test
    @DisplayName("syncIdpRoles should handle empty IDP roles list")
    void syncIdpRoles_shouldHandleEmpty_whenIdpRolesIsEmpty() {
        // Arrange
        Principal principal = createOidcPrincipal("0HZTEST00001", "alice@customer.com");
        when(roleService.removeRolesBySource("0HZTEST00001", "IDP_SYNC")).thenReturn(2L);

        // Act
        Set<String> result = service.syncIdpRoles(principal, List.of());

        // Assert: No crash, empty result, old roles removed
        assertThat(result).isEmpty();
        verify(roleService, never()).assignRole(anyString(), anyString(), anyString());
        verify(roleService).removeRolesBySource("0HZTEST00001", "IDP_SYNC");
    }

    @Test
    @DisplayName("syncIdpRoles should continue when role assignment fails")
    void syncIdpRoles_shouldContinue_whenRoleAssignmentFails() {
        // Arrange
        Principal principal = createOidcPrincipal("0HZTEST00001", "alice@customer.com");

        List<String> idpRoles = List.of("role1", "role2", "role3");

        when(idpRoleMappingRepo.findByIdpRoleName("role1"))
            .thenReturn(Optional.of(createMapping("0HZMAPPING01", "role1", "platform:role1")));
        when(idpRoleMappingRepo.findByIdpRoleName("role2"))
            .thenReturn(Optional.of(createMapping("0HZMAPPING02", "role2", "platform:role2")));
        when(idpRoleMappingRepo.findByIdpRoleName("role3"))
            .thenReturn(Optional.of(createMapping("0HZMAPPING03", "role3", "platform:role3")));

        when(roleService.removeRolesBySource("0HZTEST00001", "IDP_SYNC")).thenReturn(0L);

        // Role assignment fails for role2 (e.g., already assigned from different source)
        PrincipalRole pr1 = createPrincipalRole("0HZPR00001", "0HZTEST00001", "platform:role1", "IDP_SYNC");
        PrincipalRole pr3 = createPrincipalRole("0HZPR00003", "0HZTEST00001", "platform:role3", "IDP_SYNC");

        when(roleService.assignRole("0HZTEST00001", "platform:role1", "IDP_SYNC")).thenReturn(pr1);
        when(roleService.assignRole("0HZTEST00001", "platform:role2", "IDP_SYNC"))
            .thenThrow(new RuntimeException("Role already assigned"));
        when(roleService.assignRole("0HZTEST00001", "platform:role3", "IDP_SYNC")).thenReturn(pr3);

        // Act: Should not throw exception
        Set<String> result = service.syncIdpRoles(principal, idpRoles);

        // Assert: All 3 roles authorized (even though 1 assignment failed)
        assertThat(result).containsExactlyInAnyOrder("platform:role1", "platform:role2", "platform:role3");

        // Verify attempts were made for all roles
        verify(roleService).assignRole("0HZTEST00001", "platform:role1", "IDP_SYNC");
        verify(roleService).assignRole("0HZTEST00001", "platform:role2", "IDP_SYNC");
        verify(roleService).assignRole("0HZTEST00001", "platform:role3", "IDP_SYNC");
    }

    @Test
    @DisplayName("syncIdpRoles should assign roles with IDP_SYNC source")
    void syncIdpRoles_shouldAssignRolesWithIdpSyncSource_whenAssigningRoles() {
        // Arrange
        Principal principal = createOidcPrincipal("0HZTEST00001", "alice@customer.com");

        List<String> idpRoles = List.of("customer-viewer");

        when(idpRoleMappingRepo.findByIdpRoleName("customer-viewer"))
            .thenReturn(Optional.of(createMapping("0HZMAPPING01", "customer-viewer", "platform:viewer")));

        when(roleService.removeRolesBySource("0HZTEST00001", "IDP_SYNC")).thenReturn(0L);

        // Act
        service.syncIdpRoles(principal, idpRoles);

        // Assert: Role assigned with correct source
        verify(roleService).assignRole("0HZTEST00001", "platform:viewer", "IDP_SYNC");
    }

    @Test
    @DisplayName("SECURITY: syncIdpRoles should map multiple IDP roles to same internal role")
    void syncIdpRoles_shouldMapMultipleIdpRoles_whenMappedToSameInternalRole() {
        // Arrange: 2 different IDP roles map to same internal role
        // (e.g., "admin" and "administrator" both map to internal role "platform:admin")
        Principal principal = createOidcPrincipal("0HZTEST00001", "alice@customer.com");

        List<String> idpRoles = List.of("admin", "administrator");

        when(idpRoleMappingRepo.findByIdpRoleName("admin"))
            .thenReturn(Optional.of(createMapping("0HZMAPPING01", "admin", "platform:admin")));
        when(idpRoleMappingRepo.findByIdpRoleName("administrator"))
            .thenReturn(Optional.of(createMapping("0HZMAPPING02", "administrator", "platform:admin")));

        when(roleService.removeRolesBySource("0HZTEST00001", "IDP_SYNC")).thenReturn(0L);

        // Act
        Set<String> result = service.syncIdpRoles(principal, idpRoles);

        // Assert: Only 1 internal role (deduplicated)
        assertThat(result).containsExactly("platform:admin");

        // Verify assignment attempted (may fail on second attempt due to duplicate, but that's ok)
        verify(roleService, atLeastOnce()).assignRole("0HZTEST00001", "platform:admin", "IDP_SYNC");
    }

    @Test
    @DisplayName("SECURITY: syncIdpRoles should handle mixed authorized and unauthorized roles")
    void syncIdpRoles_shouldHandleMixed_whenSomeAuthorizedSomeNot() {
        // Arrange: Realistic scenario - some roles authorized, others not
        Principal principal = createOidcPrincipal("0HZTEST00001", "alice@customer.com");

        List<String> idpRoles = List.of(
            "customer-viewer",      // Authorized
            "hacker-role",          // UNAUTHORIZED
            "customer-operator",    // Authorized
            "default-roles-keycloak", // UNAUTHORIZED (Keycloak default)
            "offline_access"        // UNAUTHORIZED (Keycloak default)
        );

        when(idpRoleMappingRepo.findByIdpRoleName("customer-viewer"))
            .thenReturn(Optional.of(createMapping("0HZMAPPING01", "customer-viewer", "platform:viewer")));
        when(idpRoleMappingRepo.findByIdpRoleName("customer-operator"))
            .thenReturn(Optional.of(createMapping("0HZMAPPING02", "customer-operator", "platform:operator")));

        // Unauthorized roles not in whitelist
        when(idpRoleMappingRepo.findByIdpRoleName("hacker-role"))
            .thenReturn(Optional.empty());
        when(idpRoleMappingRepo.findByIdpRoleName("default-roles-keycloak"))
            .thenReturn(Optional.empty());
        when(idpRoleMappingRepo.findByIdpRoleName("offline_access"))
            .thenReturn(Optional.empty());

        when(roleService.removeRolesBySource("0HZTEST00001", "IDP_SYNC")).thenReturn(0L);

        // Act
        Set<String> result = service.syncIdpRoles(principal, idpRoles);

        // Assert: Only 2 authorized roles accepted
        assertThat(result).containsExactlyInAnyOrder("platform:viewer", "platform:operator");
        assertThat(result).hasSize(2);

        // Verify only authorized roles assigned
        verify(roleService).assignRole("0HZTEST00001", "platform:viewer", "IDP_SYNC");
        verify(roleService).assignRole("0HZTEST00001", "platform:operator", "IDP_SYNC");
        verify(roleService, times(2)).assignRole(anyString(), anyString(), eq("IDP_SYNC"));
    }

    // ========================================
    // syncOidcLogin TESTS
    // ========================================

    @Test
    @DisplayName("syncOidcLogin should sync both user and roles")
    void syncOidcLogin_shouldSyncUserAndRoles_whenCalled() {
        // Arrange
        String email = "alice@customer.com";
        String name = "Alice Smith";
        String externalIdpId = "google-oauth2|123";
        String clientId = TsidGenerator.generate();
        List<String> idpRoles = List.of("customer-viewer");

        String principalId = TsidGenerator.generate();
        Principal principal = createOidcPrincipal(principalId, email);

        when(userService.createOrUpdateOidcUser(email, name, externalIdpId, clientId, null))
            .thenReturn(principal);

        when(idpRoleMappingRepo.findByIdpRoleName("customer-viewer"))
            .thenReturn(Optional.of(createMapping("0HZMAPPING01", "customer-viewer", "platform:viewer")));

        when(roleService.removeRolesBySource(principalId, "IDP_SYNC")).thenReturn(0L);

        // Act
        Principal result = service.syncOidcLogin(email, name, externalIdpId, clientId, idpRoles);

        // Assert
        assertThat(result).isEqualTo(principal);

        // Verify user sync
        verify(userService).createOrUpdateOidcUser(email, name, externalIdpId, clientId, null);
        verify(userService).updateLastLogin(principal.id);

        // Verify role sync
        verify(idpRoleMappingRepo).findByIdpRoleName("customer-viewer");
        verify(roleService).removeRolesBySource(principalId, "IDP_SYNC");
        verify(roleService).assignRole(principalId, "platform:viewer", "IDP_SYNC");
    }

    @Test
    @DisplayName("syncOidcLogin should handle null IDP roles")
    void syncOidcLogin_shouldHandleNullRoles_whenIdpRolesIsNull() {
        // Arrange
        String email = "alice@customer.com";
        Principal principal = createOidcPrincipal("0HZTEST00001", email);

        when(userService.createOrUpdateOidcUser(anyString(), anyString(), anyString(), anyString(), any()))
            .thenReturn(principal);
        when(roleService.removeRolesBySource("0HZTEST00001", "IDP_SYNC")).thenReturn(0L);

        // Act: Should not crash
        Principal result = service.syncOidcLogin(email, "Alice", "idp-123", "0HZTEST00200", null);

        // Assert
        assertThat(result).isEqualTo(principal);
        verify(userService).updateLastLogin(principal.id);
        verify(roleService).removeRolesBySource("0HZTEST00001", "IDP_SYNC");
    }

    @Test
    @DisplayName("syncOidcLogin should handle empty IDP roles")
    void syncOidcLogin_shouldHandleEmptyRoles_whenIdpRolesIsEmpty() {
        // Arrange
        String email = "alice@customer.com";
        Principal principal = createOidcPrincipal("0HZTEST00001", email);

        when(userService.createOrUpdateOidcUser(anyString(), anyString(), anyString(), anyString(), any()))
            .thenReturn(principal);
        when(roleService.removeRolesBySource("0HZTEST00001", "IDP_SYNC")).thenReturn(0L);

        // Act
        Principal result = service.syncOidcLogin(email, "Alice", "idp-123", "0HZTEST00200", List.of());

        // Assert
        assertThat(result).isEqualTo(principal);
        verify(userService).updateLastLogin(principal.id);
        verify(roleService).removeRolesBySource("0HZTEST00001", "IDP_SYNC");
        verify(roleService, never()).assignRole(anyString(), anyString(), anyString());
    }

    // ========================================
    // auditIdpRoles TESTS
    // ========================================

    @Test
    @DisplayName("auditIdpRoles should return audit log of IDP-sourced roles")
    void auditIdpRoles_shouldReturnAuditLog_whenPrincipalHasIdpRoles() {
        // Arrange
        String principalId = "0HZTEST00001";

        PrincipalRole idpRole1 = createPrincipalRole("0HZPR00001", principalId, "platform:viewer", "IDP_SYNC");
        PrincipalRole idpRole2 = createPrincipalRole("0HZPR00002", principalId, "platform:operator", "IDP_SYNC");
        PrincipalRole manualRole = createPrincipalRole("0HZPR00003", principalId, "platform:admin", "MANUAL");

        when(roleService.findAssignmentsByPrincipal(principalId))
            .thenReturn(List.of(idpRole1, idpRole2, manualRole));

        // Act
        String audit = service.auditIdpRoles(principalId);

        // Assert
        assertThat(audit).contains("Principal " + principalId + " has 2 IDP-sourced roles");
        assertThat(audit).contains("platform:viewer");
        assertThat(audit).contains("platform:operator");
        assertThat(audit).doesNotContain("platform:admin"); // Manual role excluded
    }

    @Test
    @DisplayName("auditIdpRoles should handle principal with no IDP roles")
    void auditIdpRoles_shouldReturnEmptyAudit_whenPrincipalHasNoIdpRoles() {
        // Arrange
        String principalId = "0HZTEST00001";

        PrincipalRole manualRole = createPrincipalRole("0HZPR00001", principalId, "platform:admin", "MANUAL");

        when(roleService.findAssignmentsByPrincipal(principalId))
            .thenReturn(List.of(manualRole));

        // Act
        String audit = service.auditIdpRoles(principalId);

        // Assert
        assertThat(audit).contains("Principal " + principalId + " has 0 IDP-sourced roles");
    }

    // ========================================
    // HELPER METHODS
    // ========================================

    private Principal createOidcPrincipal(String id, String email) {
        Principal p = new Principal();
        p.id = id;
        p.type = PrincipalType.USER;
        p.name = "Test OIDC User";
        p.active = true;

        UserIdentity identity = new UserIdentity();
        identity.email = email;
        identity.idpType = IdpType.OIDC;
        identity.externalIdpId = "idp-" + id;

        p.userIdentity = identity;
        return p;
    }

    private IdpRoleMapping createMapping(String id, String idpRoleName, String internalRoleName) {
        IdpRoleMapping mapping = new IdpRoleMapping();
        mapping.id = id;
        mapping.idpRoleName = idpRoleName;
        mapping.internalRoleName = internalRoleName;
        mapping.createdAt = Instant.now();
        return mapping;
    }

    private PrincipalRole createPrincipalRole(String id, String principalId, String roleName, String source) {
        PrincipalRole pr = new PrincipalRole();
        // Note: PrincipalRole no longer has an id field (it's now a DTO, not an entity)
        pr.principalId = principalId;
        pr.roleName = roleName;
        pr.assignmentSource = source;
        pr.assignedAt = Instant.now();
        return pr;
    }
}
