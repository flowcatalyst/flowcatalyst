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

import java.util.List;
import java.util.Set;

import static io.restassured.RestAssured.given;
import static org.hamcrest.Matchers.*;

/**
 * Integration tests for RoleAdminResource.
 * Tests the read-only role and permission listing endpoints.
 */
@Tag("integration")
@QuarkusTest
class RoleAdminResourceTest {

    @Inject
    UserService userService;

    @Inject
    JwtKeyService jwtKeyService;

    @Inject
    RoleService roleService;

    private String adminToken;

    @BeforeEach
    void setUp() {
        // Create an admin user and get a token
        String uniqueId = String.valueOf(System.currentTimeMillis());
        Principal adminUser = userService.createInternalUser(
            "role-admin-" + uniqueId + "@test.com",
            "Password123!",
            "Admin User",
            null, UserScope.ANCHOR
        );

        // Assign the platform:platform-admin role to the user in the database
        roleService.assignRole(adminUser.id, "platform:platform-admin", "TEST");

        adminToken = jwtKeyService.issueSessionToken(adminUser.id, adminUser.userIdentity.email, Set.of("platform:platform-admin"), List.of("*"));
    }

    // ==================== List Roles ====================

    @Test
    @DisplayName("List roles should return all defined roles")
    void listRoles_shouldReturnAllRoles_whenAuthenticated() {
        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
        .when()
            .get("/bff/admin/roles")
        .then()
            .statusCode(200)
            .body("roles", notNullValue())
            .body("total", greaterThanOrEqualTo(1))
            .body("roles.name", hasItem("platform:platform-admin"));
    }

    @Test
    @DisplayName("List roles should return 401 when not authenticated")
    void listRoles_shouldReturn401_whenNotAuthenticated() {
        given()
            .contentType(ContentType.JSON)
        .when()
            .get("/bff/admin/roles")
        .then()
            .statusCode(401);
    }

    // ==================== Get Role ====================

    @Test
    @DisplayName("Get role should return role with permissions when exists")
    void getRole_shouldReturnRole_whenExists() {
        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
        .when()
            .get("/bff/admin/roles/platform:platform-admin")
        .then()
            .statusCode(200)
            .body("name", equalTo("platform:platform-admin"))
            .body("applicationCode", equalTo("platform"))
            .body("shortName", equalTo("platform-admin"))
            .body("permissions", notNullValue())
            .body("permissions.size()", greaterThanOrEqualTo(1));
    }

    @Test
    @DisplayName("Get role should return 404 when not found")
    void getRole_shouldReturn404_whenNotFound() {
        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
        .when()
            .get("/bff/admin/roles/nonexistent:role")
        .then()
            .statusCode(404)
            .body("message", containsString("not found"));
    }

    // ==================== List Permissions ====================

    @Test
    @DisplayName("List permissions should return all defined permissions")
    void listPermissions_shouldReturnAllPermissions_whenAuthenticated() {
        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
        .when()
            .get("/bff/admin/roles/permissions")
        .then()
            .statusCode(200)
            .body("permissions", notNullValue())
            .body("total", greaterThanOrEqualTo(1));
    }

    @Test
    @DisplayName("List permissions should return 401 when not authenticated")
    void listPermissions_shouldReturn401_whenNotAuthenticated() {
        given()
            .contentType(ContentType.JSON)
        .when()
            .get("/bff/admin/roles/permissions")
        .then()
            .statusCode(401);
    }

    // ==================== Get Permission ====================

    @Test
    @DisplayName("Get permission should return permission when exists")
    void getPermission_shouldReturnPermission_whenExists() {
        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
        .when()
            .get("/bff/admin/roles/permissions/platform:iam:user:create")
        .then()
            .statusCode(200)
            .body("permission", equalTo("platform:iam:user:create"))
            .body("application", equalTo("platform"))
            .body("context", equalTo("iam"))
            .body("aggregate", equalTo("user"))
            .body("action", equalTo("create"));
    }

    @Test
    @DisplayName("Get permission should return 404 when not found")
    void getPermission_shouldReturn404_whenNotFound() {
        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
        .when()
            .get("/bff/admin/roles/permissions/nonexistent:permission")
        .then()
            .statusCode(404)
            .body("message", containsString("not found"));
    }
}
