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
import tech.flowcatalyst.platform.principal.PrincipalType;
import tech.flowcatalyst.platform.principal.UserService;
import tech.flowcatalyst.platform.principal.UserScope;
import tech.flowcatalyst.platform.client.Client;
import tech.flowcatalyst.platform.client.ClientService;
import tech.flowcatalyst.platform.client.ClientStatus;

import java.util.List;
import java.util.Set;

import static io.restassured.RestAssured.given;
import static org.hamcrest.Matchers.*;

/**
 * Integration tests for ClientAdminResource.
 */
@Tag("integration")
@QuarkusTest
class ClientAdminResourceTest {

    @Inject
    ClientService clientService;

    @Inject
    UserService userService;

    @Inject
    JwtKeyService jwtKeyService;

    @Inject
    RoleService roleService;

    private String adminToken;
    private Principal adminUser;

    @BeforeEach
    void setUp() {
        // Create an admin user and get a token
        try {
            adminUser = userService.createInternalUser(
                "admin-" + System.currentTimeMillis() + "@test.com",
                "Password123!",
                "Admin User",
                null,
                UserScope.ANCHOR
            );
            // Assign the platform:platform-admin role to the user in the database
            roleService.assignRole(adminUser.id, "platform:platform-admin", "TEST");
        } catch (Exception e) {
            // User might already exist in another test
            adminUser = userService.findByEmail("admin-client-test@test.com").orElse(null);
            if (adminUser == null) {
                throw new RuntimeException("Failed to create admin user", e);
            }
        }

        adminToken = jwtKeyService.issueSessionToken(adminUser.id, adminUser.userIdentity.email, Set.of("platform:platform-admin"), List.of("*"));
    }

    // ==================== List Clients ====================

    @Test
    @DisplayName("List clients should return empty list initially")
    void listClients_shouldReturnList_whenAuthenticated() {
        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
        .when()
            .get("/bff/admin/clients")
        .then()
            .statusCode(200)
            .body("clients", notNullValue())
            .body("total", greaterThanOrEqualTo(0));
    }

    @Test
    @DisplayName("List clients should return 401 when not authenticated")
    void listClients_shouldReturn401_whenNotAuthenticated() {
        given()
            .contentType(ContentType.JSON)
        .when()
            .get("/bff/admin/clients")
        .then()
            .statusCode(401);
    }

    // ==================== Create Client ====================

    @Test
    @DisplayName("Create client should return 201 when valid request")
    void createClient_shouldReturn201_whenValidRequest() {
        String uniqueId = String.valueOf(System.currentTimeMillis());

        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
            .body("""
                {
                    "name": "Test Client %s",
                    "identifier": "test-client-%s"
                }
                """.formatted(uniqueId, uniqueId))
        .when()
            .post("/bff/admin/clients")
        .then()
            .statusCode(201)
            .body("id", notNullValue())
            .body("name", equalTo("Test Client " + uniqueId))
            .body("identifier", equalTo("test-client-" + uniqueId))
            .body("status", equalTo("ACTIVE"));
    }

    @Test
    @DisplayName("Create client should return 400 when identifier exists")
    void createClient_shouldReturn400_whenIdentifierExists() {
        String uniqueId = String.valueOf(System.currentTimeMillis());

        // Create first client
        clientService.createClient("First Client", "duplicate-" + uniqueId);

        // Try to create with same identifier
        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
            .body("""
                {
                    "name": "Second Client",
                    "identifier": "duplicate-%s"
                }
                """.formatted(uniqueId))
        .when()
            .post("/bff/admin/clients")
        .then()
            .statusCode(400)
            .body("error", containsString("already exists"));
    }

    @Test
    @DisplayName("Create client should return 400 when name is blank")
    void createClient_shouldReturn400_whenNameBlank() {
        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
            .body("""
                {
                    "name": "",
                    "identifier": "valid-identifier"
                }
                """)
        .when()
            .post("/bff/admin/clients")
        .then()
            .statusCode(400);
    }

    // ==================== Get Client ====================

    @Test
    @DisplayName("Get client should return client when exists")
    void getClient_shouldReturnClient_whenExists() {
        String uniqueId = String.valueOf(System.currentTimeMillis());
        Client client = clientService.createClient("Get Test Client", "get-test-" + uniqueId);

        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
        .when()
            .get("/bff/admin/clients/" + client.id)
        .then()
            .statusCode(200)
            .body("id", equalTo(client.id))
            .body("name", equalTo("Get Test Client"))
            .body("identifier", equalTo("get-test-" + uniqueId));
    }

    @Test
    @DisplayName("Get client should return 404 when not found")
    void getClient_shouldReturn404_whenNotFound() {
        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
        .when()
            .get("/bff/admin/clients/999999999")
        .then()
            .statusCode(404);
    }

    // ==================== Update Client ====================

    @Test
    @DisplayName("Update client should update name")
    void updateClient_shouldUpdateName_whenValid() {
        String uniqueId = String.valueOf(System.currentTimeMillis());
        Client client = clientService.createClient("Original Name", "update-test-" + uniqueId);

        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
            .body("""
                {
                    "name": "Updated Name"
                }
                """)
        .when()
            .put("/bff/admin/clients/" + client.id)
        .then()
            .statusCode(200)
            .body("name", equalTo("Updated Name"));
    }

    // ==================== Status Management ====================

    @Test
    @DisplayName("Suspend client should change status to SUSPENDED")
    void suspendClient_shouldChangeStatus_whenValid() {
        String uniqueId = String.valueOf(System.currentTimeMillis());
        Client client = clientService.createClient("Suspend Test", "suspend-test-" + uniqueId);

        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
            .body("""
                {
                    "reason": "Non-payment"
                }
                """)
        .when()
            .post("/bff/admin/clients/" + client.id + "/suspend")
        .then()
            .statusCode(200)
            .body("message", equalTo("Client suspended"));

        // Verify status changed
        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
        .when()
            .get("/bff/admin/clients/" + client.id)
        .then()
            .statusCode(200)
            .body("status", equalTo("SUSPENDED"));
    }

    @Test
    @DisplayName("Activate client should change status to ACTIVE")
    void activateClient_shouldChangeStatus_whenSuspended() {
        String uniqueId = String.valueOf(System.currentTimeMillis());
        Client client = clientService.createClient("Activate Test", "activate-test-" + uniqueId);
        clientService.suspendClient(client.id, "Testing", "system");

        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
        .when()
            .post("/bff/admin/clients/" + client.id + "/activate")
        .then()
            .statusCode(200)
            .body("message", equalTo("Client activated"));
    }

    @Test
    @DisplayName("Deactivate client should change status to INACTIVE")
    void deactivateClient_shouldChangeStatus_whenValid() {
        String uniqueId = String.valueOf(System.currentTimeMillis());
        Client client = clientService.createClient("Deactivate Test", "deactivate-test-" + uniqueId);

        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
            .body("""
                {
                    "reason": "Contract ended"
                }
                """)
        .when()
            .post("/bff/admin/clients/" + client.id + "/deactivate")
        .then()
            .statusCode(200)
            .body("message", equalTo("Client deactivated"));
    }

    // ==================== Notes ====================

    @Test
    @DisplayName("Add note should return 201 when valid")
    void addNote_shouldReturn201_whenValid() {
        String uniqueId = String.valueOf(System.currentTimeMillis());
        Client client = clientService.createClient("Note Test", "note-test-" + uniqueId);

        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
            .body("""
                {
                    "category": "SUPPORT",
                    "text": "Customer reported issue"
                }
                """)
        .when()
            .post("/bff/admin/clients/" + client.id + "/notes")
        .then()
            .statusCode(201)
            .body("message", equalTo("Note added"));
    }

    // ==================== Get by Identifier ====================

    @Test
    @DisplayName("Get client by identifier should return client when exists")
    void getClientByIdentifier_shouldReturnClient_whenExists() {
        String uniqueId = String.valueOf(System.currentTimeMillis());
        Client client = clientService.createClient("Identifier Test", "identifier-test-" + uniqueId);

        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
        .when()
            .get("/bff/admin/clients/by-identifier/identifier-test-" + uniqueId)
        .then()
            .statusCode(200)
            .body("id", equalTo(client.id))
            .body("identifier", equalTo("identifier-test-" + uniqueId));
    }
}
