package tech.flowcatalyst.platform.admin;

import io.quarkus.narayana.jta.QuarkusTransaction;
import io.quarkus.test.junit.QuarkusTest;
import io.restassured.http.ContentType;
import jakarta.inject.Inject;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import tech.flowcatalyst.platform.application.Application;
import tech.flowcatalyst.platform.application.ApplicationRepository;
import tech.flowcatalyst.platform.application.ApplicationService;
import tech.flowcatalyst.platform.application.operations.EnableApplicationForClientCommand;
import tech.flowcatalyst.platform.authentication.JwtKeyService;
import tech.flowcatalyst.platform.authorization.RoleService;
import tech.flowcatalyst.platform.common.ExecutionContext;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.platform.principal.UserService;
import tech.flowcatalyst.platform.principal.UserScope;
import tech.flowcatalyst.platform.client.Client;
import tech.flowcatalyst.platform.client.ClientService;
import tech.flowcatalyst.platform.shared.TsidGenerator;

import java.util.List;
import java.util.Set;

import static io.restassured.RestAssured.given;
import static org.hamcrest.Matchers.*;

/**
 * Integration tests for ApplicationAdminResource.
 */
@Tag("integration")
@QuarkusTest
class ApplicationAdminResourceTest {

    @Inject
    UserService userService;

    @Inject
    ClientService clientService;

    @Inject
    ApplicationService applicationService;

    @Inject
    ApplicationRepository applicationRepo;

    @Inject
    JwtKeyService jwtKeyService;

    @Inject
    RoleService roleService;

    private String adminToken;
    private Client testClient;

    @BeforeEach
    void setUp() {
        // Create a test client
        String uniqueId = String.valueOf(System.currentTimeMillis());
        testClient = clientService.createClient("App Test Client", "app-test-" + uniqueId);

        // Create an admin user and get a token
        Principal adminUser = userService.createInternalUser(
            "app-admin-" + uniqueId + "@test.com",
            "Password123!",
            "Admin User",
            null, UserScope.ANCHOR
        );

        // Assign the platform:platform-admin role to the user in the database
        roleService.assignRole(adminUser.id, "platform:platform-admin", "TEST");

        adminToken = jwtKeyService.issueSessionToken(adminUser.id, adminUser.userIdentity.email, Set.of("platform:platform-admin"), List.of("*"));
    }

    // ==================== List Applications ====================

    @Test
    @DisplayName("List applications should return list when authenticated")
    void listApplications_shouldReturnList_whenAuthenticated() {
        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
        .when()
            .get("/bff/admin/applications")
        .then()
            .statusCode(200)
            .body("applications", notNullValue())
            .body("total", greaterThanOrEqualTo(0));
    }

    @Test
    @DisplayName("List applications should return 401 when not authenticated")
    void listApplications_shouldReturn401_whenNotAuthenticated() {
        given()
            .contentType(ContentType.JSON)
        .when()
            .get("/bff/admin/applications")
        .then()
            .statusCode(401);
    }

    // ==================== Create Application ====================

    @Test
    @DisplayName("Create application should return 201 when valid")
    void createApplication_shouldReturn201_whenValid() {
        String uniqueId = String.valueOf(System.currentTimeMillis());

        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
            .body("""
                {
                    "code": "testapp%s",
                    "name": "Test Application %s",
                    "description": "A test application",
                    "defaultBaseUrl": "https://testapp.example.com"
                }
                """.formatted(uniqueId, uniqueId))
        .when()
            .post("/bff/admin/applications")
        .then()
            .statusCode(201)
            .body("id", notNullValue())
            .body("code", equalTo("testapp" + uniqueId))
            .body("name", equalTo("Test Application " + uniqueId))
            .body("active", equalTo(true));
    }

    @Test
    @DisplayName("Create application should return 400 when code exists")
    void createApplication_shouldReturn400_whenCodeExists() {
        String uniqueId = String.valueOf(System.currentTimeMillis());

        // Create first application
        createTestApplication("duplicate" + uniqueId, "Duplicate App");

        // Try to create with same code
        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
            .body("""
                {
                    "code": "duplicate%s",
                    "name": "Another App"
                }
                """.formatted(uniqueId))
        .when()
            .post("/bff/admin/applications")
        .then()
            .statusCode(400)
            .body("error", containsString("already exists"));
    }

    @Test
    @DisplayName("Create application should return 400 when invalid code format")
    void createApplication_shouldReturn400_whenInvalidCodeFormat() {
        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
            .body("""
                {
                    "code": "Invalid-Code!",
                    "name": "Test App"
                }
                """)
        .when()
            .post("/bff/admin/applications")
        .then()
            .statusCode(400)
            .body("error", containsString("Invalid"));
    }

    // ==================== Get Application ====================

    @Test
    @DisplayName("Get application should return application when exists")
    void getApplication_shouldReturnApplication_whenExists() {
        Application app = createTestApplication("gettest" + System.currentTimeMillis(), "Get Test App");

        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
        .when()
            .get("/bff/admin/applications/" + app.id)
        .then()
            .statusCode(200)
            .body("id", equalTo(app.id))
            .body("code", equalTo(app.code))
            .body("name", equalTo(app.name));
    }

    @Test
    @DisplayName("Get application should return 404 when not found")
    void getApplication_shouldReturn404_whenNotFound() {
        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
        .when()
            .get("/bff/admin/applications/999999999")
        .then()
            .statusCode(404);
    }

    @Test
    @DisplayName("Get application by code should return application when exists")
    void getApplicationByCode_shouldReturnApplication_whenExists() {
        Application app = createTestApplication("codetest" + System.currentTimeMillis(), "Code Test App");

        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
        .when()
            .get("/bff/admin/applications/by-code/" + app.code)
        .then()
            .statusCode(200)
            .body("code", equalTo(app.code));
    }

    // ==================== Update Application ====================

    @Test
    @DisplayName("Update application should update fields")
    void updateApplication_shouldUpdateFields_whenValid() {
        Application app = createTestApplication("updatetest" + System.currentTimeMillis(), "Update Test App");

        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
            .body("""
                {
                    "name": "Updated App Name",
                    "description": "Updated description"
                }
                """)
        .when()
            .put("/bff/admin/applications/" + app.id)
        .then()
            .statusCode(200)
            .body("name", equalTo("Updated App Name"))
            .body("description", equalTo("Updated description"));
    }

    // ==================== Activate/Deactivate ====================

    @Test
    @DisplayName("Deactivate application should set active to false")
    void deactivateApplication_shouldSetActiveFalse() {
        Application app = createTestApplication("deactivatetest" + System.currentTimeMillis(), "Deactivate Test App");

        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
        .when()
            .post("/bff/admin/applications/" + app.id + "/deactivate")
        .then()
            .statusCode(200)
            .body("message", equalTo("Application deactivated"));

        // Verify deactivated
        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
        .when()
            .get("/bff/admin/applications/" + app.id)
        .then()
            .statusCode(200)
            .body("active", equalTo(false));
    }

    @Test
    @DisplayName("Activate application should set active to true")
    void activateApplication_shouldSetActiveTrue() {
        Application app = createDeactivatedTestApplication();

        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
        .when()
            .post("/bff/admin/applications/" + app.id + "/activate")
        .then()
            .statusCode(200)
            .body("message", equalTo("Application activated"));
    }

    // ==================== Client Configuration ====================

    @Test
    @DisplayName("Configure client should create config")
    void configureClient_shouldCreateConfig() {
        Application app = createTestApplication("clientconfig" + System.currentTimeMillis(), "Client Config App");

        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
            .body("""
                {
                    "enabled": true,
                    "baseUrlOverride": "https://client.example.com"
                }
                """)
        .when()
            .put("/bff/admin/applications/" + app.id + "/clients/" + testClient.id)
        .then()
            .statusCode(200)
            .body("applicationId", equalTo(app.id))
            .body("clientId", equalTo(testClient.id))
            .body("enabled", equalTo(true))
            .body("baseUrlOverride", equalTo("https://client.example.com"));
    }

    @Test
    @DisplayName("Get client configs should return configs for application")
    void getClientConfigs_shouldReturnConfigs() {
        Application app = createTestApplication("listconfigs" + System.currentTimeMillis(), "List Configs App");

        // Create a config first using the admin API endpoint
        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
            .body("""
                {
                    "enabled": true,
                    "baseUrlOverride": "https://client.test.com"
                }
                """)
        .when()
            .put("/bff/admin/applications/" + app.id + "/clients/" + testClient.id)
        .then()
            .statusCode(200);

        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
        .when()
            .get("/bff/admin/applications/" + app.id + "/clients")
        .then()
            .statusCode(200)
            .body("clientConfigs", notNullValue())
            .body("total", greaterThanOrEqualTo(1));
    }

    @Test
    @DisplayName("Enable for client should enable application")
    void enableForClient_shouldEnable() {
        Application app = createTestApplication("enabletest" + System.currentTimeMillis(), "Enable Test App");

        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
        .when()
            .post("/bff/admin/applications/" + app.id + "/clients/" + testClient.id + "/enable")
        .then()
            .statusCode(200)
            .body("message", equalTo("Application enabled for client"));
    }

    @Test
    @DisplayName("Disable for client should disable application")
    void disableForClient_shouldDisable() {
        Application app = createTestApplication("disabletest" + System.currentTimeMillis(), "Disable Test App");

        // Enable first using the admin API endpoint
        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
        .when()
            .post("/bff/admin/applications/" + app.id + "/clients/" + testClient.id + "/enable")
        .then()
            .statusCode(200);

        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
        .when()
            .post("/bff/admin/applications/" + app.id + "/clients/" + testClient.id + "/disable")
        .then()
            .statusCode(200)
            .body("message", equalTo("Application disabled for client"));
    }

    // ==================== Helpers ====================

    private Application createTestApplication(String code, String name) {
        return QuarkusTransaction.requiringNew().call(() -> {
            Application app = new Application();
            app.id = TsidGenerator.generate();
            app.code = code;
            app.name = name;
            app.description = "Test description";
            app.defaultBaseUrl = "https://test.example.com";
            app.active = true;
            applicationRepo.persist(app);
            return app;
        });
    }

    private Application createDeactivatedTestApplication() {
        return QuarkusTransaction.requiringNew().call(() -> {
            Application app = new Application();
            app.id = TsidGenerator.generate();
            app.code = "deactivated" + System.currentTimeMillis();
            app.name = "Deactivated Test App";
            app.active = false;
            applicationRepo.persist(app);
            return app;
        });
    }
}
