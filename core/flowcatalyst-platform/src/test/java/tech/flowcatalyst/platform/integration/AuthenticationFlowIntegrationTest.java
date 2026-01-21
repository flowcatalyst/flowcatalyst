package tech.flowcatalyst.platform.integration;

import io.quarkus.test.junit.QuarkusTest;
import jakarta.inject.Inject;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import tech.flowcatalyst.platform.authorization.AuthorizationService;
import tech.flowcatalyst.platform.authorization.RoleService;
import tech.flowcatalyst.platform.principal.PasswordService;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.platform.principal.UserService;
import tech.flowcatalyst.platform.principal.UserScope;

import java.util.UUID;

import static org.assertj.core.api.Assertions.*;

/**
 * Integration tests for end-to-end authentication flows.
 * Tests complete workflows with real database interactions.
 */
@Tag("integration")
@QuarkusTest
class AuthenticationFlowIntegrationTest {

    @Inject
    UserService userService;

    @Inject
    PasswordService passwordService;

    @Inject
    AuthorizationService authzService;

    @Inject
    RoleService roleService;

    // ========================================
    // HELPER METHODS
    // ========================================

    private String uniqueEmail(String prefix) {
        return prefix + "-" + UUID.randomUUID().toString().substring(0, 8) + "@test.com";
    }

    private String uniqueSubject(String prefix) {
        return prefix + "-" + UUID.randomUUID().toString().substring(0, 8);
    }

    // ========================================
    // INTERNAL USER AUTHENTICATION TESTS
    // ========================================

    @Test
    @DisplayName("Internal user authentication flow should work end-to-end")
    void internalAuth_shouldAuthenticateUser_whenCredentialsCorrect() {
        // Arrange: Create user with password
        String email = uniqueEmail("john");
        String password = "SecurePass123!";

        Principal user = userService.createInternalUser(
            email,
            password,
            "John Doe",
            null, UserScope.ANCHOR
        );

        // Act: Simulate login - verify password
        boolean passwordValid = passwordService.verifyPassword(
            password,
            user.userIdentity.passwordHash
        );

        // Assert: Password verification successful
        assertThat(passwordValid).isTrue();
        assertThat(user.active).isTrue();
        assertThat(user.userIdentity.email).isEqualTo(email);
    }

    @Test
    @DisplayName("Internal user authentication should fail when password is wrong")
    void internalAuth_shouldRejectUser_whenPasswordWrong() {
        // Arrange
        Principal user = userService.createInternalUser(
            uniqueEmail("john"),
            "CorrectPass123!",
            "John Doe",
            null, UserScope.ANCHOR
        );

        // Act: Try to authenticate with wrong password
        boolean passwordValid = passwordService.verifyPassword(
            "WrongPass123!",
            user.userIdentity.passwordHash
        );

        // Assert: Authentication fails
        assertThat(passwordValid).isFalse();
    }

    @Test
    @DisplayName("Internal user authentication should fail when user is deactivated")
    void internalAuth_shouldRejectUser_whenDeactivated() {
        // Arrange: Create and deactivate user
        Principal user = userService.createInternalUser(
            uniqueEmail("john"),
            "SecurePass123!",
            "John Doe",
            null, UserScope.ANCHOR
        );

        userService.deactivateUser(user.id);

        // Act: Verify password (would succeed)
        boolean passwordValid = passwordService.verifyPassword(
            "SecurePass123!",
            user.userIdentity.passwordHash
        );

        // Refresh user from DB
        Principal deactivatedUser = userService.findById(user.id).get();

        // Assert: Password is valid BUT user is deactivated
        assertThat(passwordValid).isTrue();
        assertThat(deactivatedUser.active).isFalse();
        // Authentication layer should check active flag and reject
    }

    // ========================================
    // ROLE-BASED ACCESS CONTROL TESTS
    // ========================================

    @Test
    @DisplayName("User should have permissions after role is assigned")
    void roleBasedAccess_shouldGrantPermissions_whenRoleAssigned() {
        // Arrange: Create user
        Principal user = userService.createInternalUser(
            uniqueEmail("operator"),
            "SecurePass123!",
            "Operator User",
            null, UserScope.ANCHOR
        );

        // Act: Assign test editor role (has create, view, update permissions)
        roleService.assignRole(user.id, "test:editor", "MANUAL");

        // Assert: User has permissions from the role
        assertThat(authzService.hasPermission(user.id, "test:context:resource:create")).isTrue();
        assertThat(authzService.hasPermission(user.id, "test:context:resource:view")).isTrue();
        assertThat(authzService.hasPermission(user.id, "test:context:resource:update")).isTrue();
        assertThat(authzService.hasPermission(user.id, "test:context:resource:delete")).isFalse();
    }

    @Test
    @DisplayName("User should accumulate permissions from multiple roles")
    void roleBasedAccess_shouldAccumulatePermissions_whenMultipleRoles() {
        // Arrange: Create user
        Principal user = userService.createInternalUser(
            uniqueEmail("poweruser"),
            "SecurePass123!",
            "Power User",
            null, UserScope.ANCHOR
        );

        // Act: Assign multiple roles - editor (create, view, update) and viewer (view only)
        roleService.assignRole(user.id, "test:viewer", "MANUAL");
        roleService.assignRole(user.id, "test:editor", "MANUAL");

        // Assert: User has permissions from both roles (editor superset of viewer)
        assertThat(authzService.hasPermission(user.id, "test:context:resource:create")).isTrue();
        assertThat(authzService.hasPermission(user.id, "test:context:resource:view")).isTrue();
        assertThat(authzService.hasPermission(user.id, "test:context:resource:update")).isTrue();
        // Still no delete permission (only admin has that)
        assertThat(authzService.hasPermission(user.id, "test:context:resource:delete")).isFalse();
    }

    @Test
    @DisplayName("Removing role should revoke permissions")
    void roleBasedAccess_shouldRevokePermissions_whenRoleRemoved() {
        // Arrange: Create user and assign role
        Principal user = userService.createInternalUser(
            uniqueEmail("operator"),
            "SecurePass123!",
            "Operator",
            null, UserScope.ANCHOR
        );

        roleService.assignRole(user.id, "test:editor", "MANUAL");

        // Verify permission granted
        assertThat(authzService.hasPermission(user.id, "test:context:resource:create")).isTrue();

        // Act: Remove role
        roleService.removeRole(user.id, "test:editor");

        // Assert: Permission revoked
        assertThat(authzService.hasPermission(user.id, "test:context:resource:create")).isFalse();
    }

    // ========================================
    // PASSWORD MANAGEMENT TESTS
    // ========================================

    @Test
    @DisplayName("User should be able to change password")
    void passwordManagement_shouldUpdatePassword_whenOldPasswordCorrect() {
        // Arrange: Create user
        String oldPassword = "OldSecurePass123!";
        String newPassword = "NewSecurePass456!";

        Principal user = userService.createInternalUser(
            uniqueEmail("john"),
            oldPassword,
            "John Doe",
            null, UserScope.ANCHOR
        );

        // Act: Change password
        userService.changePassword(user.id, oldPassword, newPassword);

        // Refresh user
        Principal updatedUser = userService.findById(user.id).get();

        // Assert: Old password no longer works, new password works
        assertThat(passwordService.verifyPassword(oldPassword, updatedUser.userIdentity.passwordHash))
            .isFalse();
        assertThat(passwordService.verifyPassword(newPassword, updatedUser.userIdentity.passwordHash))
            .isTrue();
    }

    @Test
    @DisplayName("Password change should fail when old password is wrong")
    void passwordManagement_shouldRejectChange_whenOldPasswordWrong() {
        // Arrange: Create user
        Principal user = userService.createInternalUser(
            uniqueEmail("john"),
            "CorrectPass123!",
            "John Doe",
            null, UserScope.ANCHOR
        );

        // Act & Assert: Attempt password change with wrong old password
        assertThatThrownBy(() ->
            userService.changePassword(user.id, "WrongOldPass!", "NewPass123!")
        )
        .hasMessageContaining("incorrect");
    }

    @Test
    @DisplayName("Admin should be able to reset user password")
    void passwordManagement_shouldResetPassword_whenAdminResets() {
        // Arrange: Create user
        Principal user = userService.createInternalUser(
            uniqueEmail("john"),
            "OldPass123!x",
            "John Doe",
            null, UserScope.ANCHOR
        );

        // Act: Admin resets password (doesn't need old password)
        String newPassword = "ResetPass123!x";
        userService.resetPassword(user.id, newPassword);

        // Refresh user
        Principal updatedUser = userService.findById(user.id).get();

        // Assert: New password works
        assertThat(passwordService.verifyPassword(newPassword, updatedUser.userIdentity.passwordHash))
            .isTrue();
    }

    // ========================================
    // OIDC USER TESTS
    // ========================================

    @Test
    @DisplayName("OIDC user should be created without password")
    void oidcAuth_shouldCreateUser_whenUserLogsInViaOidc() {
        // Act: Simulate OIDC login
        String email = uniqueEmail("alice");
        String subject = uniqueSubject("google-oauth2");
        Principal oidcUser = userService.createOrUpdateOidcUser(
            email,
            "Alice Smith",
            subject,
            null,
            null
        );

        // Assert: User created without password
        assertThat(oidcUser.userIdentity.passwordHash).isNull();
        assertThat(oidcUser.userIdentity.externalIdpId).isEqualTo(subject);
        assertThat(oidcUser.active).isTrue();
    }

    @Test
    @DisplayName("OIDC user should be updated on subsequent logins")
    void oidcAuth_shouldUpdateUser_whenUserLogsInAgain() {
        // Arrange: First login
        String email = uniqueEmail("alice");
        String subject1 = uniqueSubject("google-oauth2");
        String subject2 = uniqueSubject("google-oauth2");

        Principal firstLogin = userService.createOrUpdateOidcUser(
            email,
            "Alice Smith",
            subject1,
            null,
            null
        );

        // Act: Second login with updated name and external ID
        Principal secondLogin = userService.createOrUpdateOidcUser(
            email,
            "Alice Johnson",  // Name changed (married)
            subject2,  // External ID changed
            null,
            null
        );

        // Assert: Same user, updated info
        assertThat(secondLogin.id).isEqualTo(firstLogin.id);
        assertThat(secondLogin.name).isEqualTo("Alice Johnson");
        assertThat(secondLogin.userIdentity.externalIdpId).isEqualTo(subject2);
    }

    @Test
    @DisplayName("OIDC user cannot have password operations")
    void oidcAuth_shouldRejectPasswordOps_whenUserIsOidc() {
        // Arrange: Create OIDC user
        Principal oidcUser = userService.createOrUpdateOidcUser(
            uniqueEmail("alice"),
            "Alice Smith",
            uniqueSubject("google-oauth2"),
            null,
            null
        );

        // Act & Assert: Cannot reset password for OIDC user
        assertThatThrownBy(() ->
            userService.resetPassword(oidcUser.id, "NewPass123!")
        )
        .hasMessageContaining("OIDC");

        // Act & Assert: Cannot change password for OIDC user
        assertThatThrownBy(() ->
            userService.changePassword(oidcUser.id, "OldPass123!", "NewPass123!")
        )
        .hasMessageContaining("OIDC");
    }

    // ========================================
    // COMPLETE LOGIN FLOW TESTS
    // ========================================

    @Test
    @DisplayName("Complete login flow: user creation -> role assignment -> permission check")
    void completeFlow_shouldWork_endToEnd() {
        // Step 1: Create user (registration)
        Principal user = userService.createInternalUser(
            uniqueEmail("newuser"),
            "SecurePass123!",
            "New User",
            null, UserScope.ANCHOR
        );

        // Step 2: Authenticate (login)
        boolean authenticated = passwordService.verifyPassword(
            "SecurePass123!",
            user.userIdentity.passwordHash
        );
        assertThat(authenticated).isTrue();

        // Step 3: Assign viewer role (limited permissions)
        roleService.assignRole(user.id, "test:viewer", "MANUAL");

        // Step 4: Check permissions (authorization)
        assertThat(authzService.hasPermission(user.id, "test:context:resource:view")).isTrue();

        // Step 5: Verify no admin permissions (can't create, update, delete)
        assertThat(authzService.hasPermission(user.id, "test:context:resource:create")).isFalse();
        assertThat(authzService.hasPermission(user.id, "test:context:resource:update")).isFalse();
        assertThat(authzService.hasPermission(user.id, "test:context:resource:delete")).isFalse();
    }

    @Test
    @DisplayName("Complete flow with role upgrade: user gets promoted to admin")
    void completeFlow_shouldWork_whenUserPromotedToAdmin() {
        // Arrange: Create user with viewer role (limited permissions)
        Principal user = userService.createInternalUser(
            uniqueEmail("john"),
            "SecurePass123!",
            "John Doe",
            null, UserScope.ANCHOR
        );

        roleService.assignRole(user.id, "test:viewer", "MANUAL");

        // Verify basic permissions
        assertThat(authzService.hasPermission(user.id, "test:context:resource:view")).isTrue();
        assertThat(authzService.hasPermission(user.id, "test:context:resource:delete")).isFalse();

        // Act: Promote to admin (full permissions)
        roleService.assignRole(user.id, "test:admin", "MANUAL");

        // Assert: Now has both viewer and admin permissions (all CRUD operations)
        assertThat(authzService.hasPermission(user.id, "test:context:resource:view")).isTrue();
        assertThat(authzService.hasPermission(user.id, "test:context:resource:create")).isTrue();
        assertThat(authzService.hasPermission(user.id, "test:context:resource:update")).isTrue();
        assertThat(authzService.hasPermission(user.id, "test:context:resource:delete")).isTrue();
    }
}
