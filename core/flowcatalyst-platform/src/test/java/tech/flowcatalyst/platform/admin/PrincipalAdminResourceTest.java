package tech.flowcatalyst.platform.admin;

import io.quarkus.test.junit.QuarkusTest;
import io.restassured.http.ContentType;
import jakarta.inject.Inject;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import tech.flowcatalyst.platform.authentication.JwtKeyService;
import tech.flowcatalyst.platform.authorization.RoleService;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.platform.principal.UserService;
import tech.flowcatalyst.platform.principal.UserScope;
import tech.flowcatalyst.platform.client.Client;
import tech.flowcatalyst.platform.client.ClientService;

import java.util.List;
import java.util.Set;

import static io.restassured.RestAssured.given;
import static org.hamcrest.Matchers.*;

/**
 * Integration tests for PrincipalAdminResource.
 */
@Tag("integration")
@QuarkusTest
class PrincipalAdminResourceTest {

    @Inject
    UserService userService;

    @Inject
    ClientService clientService;

    @Inject
    RoleService roleService;

    @Inject
    JwtKeyService jwtKeyService;

    private String adminToken;
    private Principal adminUser;
    private Client testClient;

    @BeforeEach
    void setUp() {
        // Create a test client
        String uniqueId = String.valueOf(System.currentTimeMillis());
        testClient = clientService.createClient("Principal Test Client", "principal-test-" + uniqueId);

        // Create an admin user and get a token
        adminUser = userService.createInternalUser(
            "principal-admin-" + uniqueId + "@test.com",
            "Password123!",
            "Admin User",
            null, UserScope.ANCHOR
        );

        // Assign the platform:platform-admin role to the user in the database
        roleService.assignRole(adminUser.id, "platform:platform-admin", "TEST");

        adminToken = jwtKeyService.issueSessionToken(adminUser.id, adminUser.userIdentity.email, Set.of("platform:platform-admin"), List.of("*"));
    }

    // ==================== List Principals ====================

    @Test
    @DisplayName("List principals should return list when authenticated")
    void listPrincipals_shouldReturnList_whenAuthenticated() {
        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
        .when()
            .get("/bff/admin/principals")
        .then()
            .statusCode(200)
            .body("principals", notNullValue())
            .body("total", greaterThanOrEqualTo(1));
    }

    @Test
    @DisplayName("List principals should filter by client")
    void listPrincipals_shouldFilterByClient_whenClientIdProvided() {
        // Create user in test client
        String uniqueId = String.valueOf(System.currentTimeMillis());
        userService.createInternalUser(
            "client-user-" + uniqueId + "@test.com",
            "Password123!",
            "Client User",
            testClient.id, UserScope.CLIENT
        );

        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
            .queryParam("clientId", testClient.id)
        .when()
            .get("/bff/admin/principals")
        .then()
            .statusCode(200)
            .body("principals.size()", greaterThanOrEqualTo(1))
            .body("principals.clientId", everyItem(equalTo(testClient.id)));
    }

    @Test
    @DisplayName("List principals should return 401 when not authenticated")
    void listPrincipals_shouldReturn401_whenNotAuthenticated() {
        given()
            .contentType(ContentType.JSON)
        .when()
            .get("/bff/admin/principals")
        .then()
            .statusCode(401);
    }

    // ==================== Create User ====================

    @Test
    @DisplayName("Create user should return 201 when valid request")
    void createUser_shouldReturn201_whenValidRequest() {
        String uniqueId = String.valueOf(System.currentTimeMillis());

        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
            .body("""
                {
                    "email": "newuser-%s@test.com",
                    "password": "SecurePass123!",
                    "name": "New User",
                    "clientId": "%s"
                }
                """.formatted(uniqueId, testClient.id))
        .when()
            .post("/bff/admin/principals/users")
        .then()
            .statusCode(201)
            .body("id", notNullValue())
            .body("email", equalTo("newuser-" + uniqueId + "@test.com"))
            .body("name", equalTo("New User"))
            .body("type", equalTo("USER"))
            .body("active", equalTo(true));
    }

    @Test
    @DisplayName("Create user should return 400 when email exists")
    void createUser_shouldReturn400_whenEmailExists() {
        String uniqueId = String.valueOf(System.currentTimeMillis());
        String email = "duplicate-" + uniqueId + "@test.com";

        // Create first user
        userService.createInternalUser(email, "Password123!", "First User", testClient.id, UserScope.CLIENT);

        // Try to create with same email
        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
            .body("""
                {
                    "email": "%s",
                    "password": "SecurePass123!",
                    "name": "Second User",
                    "clientId": "%s"
                }
                """.formatted(email, testClient.id))
        .when()
            .post("/bff/admin/principals/users")
        .then()
            .statusCode(409)
            .body("message", containsString("already exists"));
    }

    // ==================== Get Principal ====================

    @Test
    @DisplayName("Get principal should return principal with roles when exists")
    void getPrincipal_shouldReturnPrincipal_whenExists() {
        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
        .when()
            .get("/bff/admin/principals/" + adminUser.id)
        .then()
            .statusCode(200)
            .body("id", equalTo(adminUser.id))
            .body("name", equalTo("Admin User"))
            .body("roles", notNullValue())
            .body("grantedClientIds", notNullValue());
    }

    @Test
    @DisplayName("Get principal should return 404 when not found")
    void getPrincipal_shouldReturn404_whenNotFound() {
        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
        .when()
            .get("/bff/admin/principals/999999999")
        .then()
            .statusCode(404);
    }

    // ==================== Update Principal ====================

    @Test
    @DisplayName("Update principal should update name")
    void updatePrincipal_shouldUpdateName_whenValid() {
        String uniqueId = String.valueOf(System.currentTimeMillis());
        Principal user = userService.createInternalUser(
            "update-" + uniqueId + "@test.com",
            "Password123!",
            "Original Name",
            testClient.id, UserScope.CLIENT
        );

        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
            .body("""
                {
                    "name": "Updated Name"
                }
                """)
        .when()
            .put("/bff/admin/principals/" + user.id)
        .then()
            .statusCode(200)
            .body("name", equalTo("Updated Name"));
    }

    // ==================== Activate/Deactivate ====================

    @Test
    @DisplayName("Deactivate principal should set active to false")
    void deactivatePrincipal_shouldSetActiveFalse() {
        String uniqueId = String.valueOf(System.currentTimeMillis());
        Principal user = userService.createInternalUser(
            "deactivate-" + uniqueId + "@test.com",
            "Password123!",
            "Deactivate User",
            testClient.id, UserScope.CLIENT
        );

        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
        .when()
            .post("/bff/admin/principals/" + user.id + "/deactivate")
        .then()
            .statusCode(200)
            .body("message", equalTo("Principal deactivated"));

        // Verify deactivated
        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
        .when()
            .get("/bff/admin/principals/" + user.id)
        .then()
            .statusCode(200)
            .body("active", equalTo(false));
    }

    @Test
    @DisplayName("Activate principal should set active to true")
    void activatePrincipal_shouldSetActiveTrue() {
        String uniqueId = String.valueOf(System.currentTimeMillis());
        Principal user = userService.createInternalUser(
            "activate-" + uniqueId + "@test.com",
            "Password123!",
            "Activate User",
            testClient.id, UserScope.CLIENT
        );
        userService.deactivateUser(user.id);

        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
        .when()
            .post("/bff/admin/principals/" + user.id + "/activate")
        .then()
            .statusCode(200)
            .body("message", equalTo("Principal activated"));
    }

    // ==================== Password Reset ====================

    @Test
    @DisplayName("Reset password should succeed for internal user")
    void resetPassword_shouldSucceed_forInternalUser() {
        String uniqueId = String.valueOf(System.currentTimeMillis());
        Principal user = userService.createInternalUser(
            "reset-pwd-" + uniqueId + "@test.com",
            "OldPassword123!",
            "Reset Password User",
            testClient.id, UserScope.CLIENT
        );

        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
            .body("""
                {
                    "newPassword": "NewPassword456!"
                }
                """)
        .when()
            .post("/bff/admin/principals/" + user.id + "/reset-password")
        .then()
            .statusCode(200)
            .body("message", equalTo("Password reset successfully"));
    }

    // ==================== Role Management ====================

    @Test
    @DisplayName("Get principal roles should return empty list initially")
    void getPrincipalRoles_shouldReturnEmptyList_initially() {
        String uniqueId = String.valueOf(System.currentTimeMillis());
        Principal user = userService.createInternalUser(
            "roles-" + uniqueId + "@test.com",
            "Password123!",
            "Roles User",
            testClient.id, UserScope.CLIENT
        );

        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
        .when()
            .get("/bff/admin/principals/" + user.id + "/roles")
        .then()
            .statusCode(200)
            .body("roles", hasSize(0));
    }

    @Test
    @DisplayName("Assign role should return 201 when valid")
    void assignRole_shouldReturn201_whenValid() {
        String uniqueId = String.valueOf(System.currentTimeMillis());
        Principal user = userService.createInternalUser(
            "assign-role-" + uniqueId + "@test.com",
            "Password123!",
            "Assign Role User",
            testClient.id, UserScope.CLIENT
        );

        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
            .body("""
                {
                    "roleName": "platform:test-client-admin"
                }
                """)
        .when()
            .post("/bff/admin/principals/" + user.id + "/roles")
        .then()
            .statusCode(201)
            .body("roleName", equalTo("platform:test-client-admin"))
            .body("assignmentSource", equalTo("MANUAL"));
    }

    @Test
    @DisplayName("Assign role should return 400 when role not defined")
    void assignRole_shouldReturn400_whenRoleNotDefined() {
        String uniqueId = String.valueOf(System.currentTimeMillis());
        Principal user = userService.createInternalUser(
            "bad-role-" + uniqueId + "@test.com",
            "Password123!",
            "Bad Role User",
            testClient.id, UserScope.CLIENT
        );

        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
            .body("""
                {
                    "roleName": "nonexistent:role"
                }
                """)
        .when()
            .post("/bff/admin/principals/" + user.id + "/roles")
        .then()
            .statusCode(400)
            .body("message", containsString("not defined"));
    }

    @Test
    @DisplayName("Remove role should return 204 when exists")
    void removeRole_shouldReturn204_whenExists() {
        String uniqueId = String.valueOf(System.currentTimeMillis());
        Principal user = userService.createInternalUser(
            "remove-role-" + uniqueId + "@test.com",
            "Password123!",
            "Remove Role User",
            testClient.id, UserScope.CLIENT
        );

        // First assign the role
        roleService.assignRole(user.id, "platform:test-client-admin", "MANUAL");

        // Then remove it
        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
        .when()
            .delete("/bff/admin/principals/" + user.id + "/roles/platform:test-client-admin")
        .then()
            .statusCode(204);
    }

    // ==================== Client Access Grants ====================

    @Test
    @DisplayName("Get client access grants should return empty list initially")
    void getClientAccessGrants_shouldReturnEmptyList_initially() {
        String uniqueId = String.valueOf(System.currentTimeMillis());
        Principal user = userService.createInternalUser(
            "grants-" + uniqueId + "@test.com",
            "Password123!",
            "Grants User",
            null,
            UserScope.ANCHOR
        );

        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
        .when()
            .get("/bff/admin/principals/" + user.id + "/client-access")
        .then()
            .statusCode(200)
            .body("grants", hasSize(0));
    }

    @Test
    @DisplayName("Grant client access should return 201 when valid")
    void grantClientAccess_shouldReturn201_whenValid() {
        String uniqueId = String.valueOf(System.currentTimeMillis());
        Principal partnerUser = userService.createInternalUser(
            "grant-access-" + uniqueId + "@test.com",
            "Password123!",
            "Partner User",
            null,
            UserScope.ANCHOR
        );

        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
            .body("""
                {
                    "clientId": "%s"
                }
                """.formatted(testClient.id))
        .when()
            .post("/bff/admin/principals/" + partnerUser.id + "/client-access")
        .then()
            .statusCode(201)
            .body("clientId", equalTo(testClient.id));
    }

    @Test
    @DisplayName("Revoke client access should return 204 when exists")
    void revokeClientAccess_shouldReturn204_whenExists() {
        String uniqueId = String.valueOf(System.currentTimeMillis());
        Principal partnerUser = userService.createInternalUser(
            "revoke-access-" + uniqueId + "@test.com",
            "Password123!",
            "Partner User",
            null, UserScope.ANCHOR
        );

        // Grant access first
        clientService.grantClientAccess(partnerUser.id, testClient.id);

        // Then revoke it
        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
        .when()
            .delete("/bff/admin/principals/" + partnerUser.id + "/client-access/" + testClient.id)
        .then()
            .statusCode(204);
    }
}
