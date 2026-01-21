package tech.flowcatalyst.platform.admin;

import io.quarkus.narayana.jta.QuarkusTransaction;
import io.quarkus.test.junit.QuarkusTest;
import io.restassured.http.ContentType;
import io.restassured.response.Response;
import jakarta.inject.Inject;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import tech.flowcatalyst.platform.authentication.JwtKeyService;
import tech.flowcatalyst.platform.authentication.oauth.OAuthClient;
import tech.flowcatalyst.platform.authentication.oauth.OAuthClient.ClientType;
import tech.flowcatalyst.platform.authentication.oauth.OAuthClientRepository;
import tech.flowcatalyst.platform.authorization.RoleService;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.platform.principal.UserService;
import tech.flowcatalyst.platform.principal.UserScope;
import tech.flowcatalyst.platform.shared.TsidGenerator;

import java.util.List;
import java.util.Set;

import static io.restassured.RestAssured.given;
import static org.hamcrest.Matchers.*;

/**
 * Integration tests for OAuthClientAdminResource.
 */
@Tag("integration")
@QuarkusTest
class OAuthClientAdminResourceTest {

    @Inject
    UserService userService;

    @Inject
    OAuthClientRepository clientRepo;

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
            "oauth-admin-" + uniqueId + "@test.com",
            "Password123!",
            "Admin User",
            null, UserScope.ANCHOR
        );

        // Assign the platform:platform-admin role to the user in the database
        roleService.assignRole(adminUser.id, "platform:platform-admin", "TEST");

        adminToken = jwtKeyService.issueSessionToken(adminUser.id, adminUser.userIdentity.email, Set.of("platform:platform-admin"), List.of("*"));
    }

    // ==================== List Clients ====================

    @Test
    @DisplayName("List clients should return list when authenticated")
    void listClients_shouldReturnList_whenAuthenticated() {
        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
        .when()
            .get("/bff/admin/oauth-clients")
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
            .get("/bff/admin/oauth-clients")
        .then()
            .statusCode(401);
    }

    // ==================== Create Public Client ====================

    @Test
    @DisplayName("Create public client should return 201 with no secret")
    void createPublicClient_shouldReturn201_withNoSecret() {
        String uniqueId = String.valueOf(System.currentTimeMillis());

        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
            .body("""
                {
                    "clientName": "Test SPA %s",
                    "clientType": "PUBLIC",
                    "redirectUris": ["http://localhost:3000/callback"],
                    "grantTypes": ["authorization_code", "refresh_token"],
                    "defaultScopes": ["openid", "profile"],
                    "pkceRequired": true
                }
                """.formatted(uniqueId))
        .when()
            .post("/bff/admin/oauth-clients")
        .then()
            .statusCode(201)
            .body("client.id", notNullValue())
            .body("client.clientId", startsWith("fc_"))
            .body("client.clientName", equalTo("Test SPA " + uniqueId))
            .body("client.clientType", equalTo("PUBLIC"))
            .body("client.pkceRequired", equalTo(true))
            .body("client.active", equalTo(true))
            .body("clientSecret", nullValue()); // No secret for public clients
    }

    // ==================== Create Confidential Client ====================

    @Test
    @DisplayName("Create confidential client should return 201 with secret")
    void createConfidentialClient_shouldReturn201_withSecret() {
        String uniqueId = String.valueOf(System.currentTimeMillis());

        Response response = given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
            .body("""
                {
                    "clientName": "Test Server %s",
                    "clientType": "CONFIDENTIAL",
                    "redirectUris": ["https://api.example.com/callback"],
                    "grantTypes": ["client_credentials", "authorization_code"],
                    "pkceRequired": false
                }
                """.formatted(uniqueId))
        .when()
            .post("/bff/admin/oauth-clients")
        .then()
            .statusCode(201)
            .body("client.id", notNullValue())
            .body("client.clientId", startsWith("fc_"))
            .body("client.clientType", equalTo("CONFIDENTIAL"))
            .body("clientSecret", notNullValue()) // Secret returned once
            .extract().response();

        // Verify secret is returned and is proper length
        String secret = response.path("clientSecret");
        assert secret != null && secret.length() > 20;
    }

    @Test
    @DisplayName("Create client should return 400 when missing required fields")
    void createClient_shouldReturn400_whenMissingRequiredFields() {
        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
            .body("""
                {
                    "clientName": "Missing Fields"
                }
                """)
        .when()
            .post("/bff/admin/oauth-clients")
        .then()
            .statusCode(400);
    }

    // ==================== Get Client ====================

    @Test
    @DisplayName("Get client should return client when exists")
    void getClient_shouldReturnClient_whenExists() {
        // Create a client first
        OAuthClient client = createTestClient();

        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
        .when()
            .get("/bff/admin/oauth-clients/" + client.id)
        .then()
            .statusCode(200)
            .body("id", equalTo(client.id))
            .body("clientId", equalTo(client.clientId))
            .body("clientName", equalTo(client.clientName));
    }

    @Test
    @DisplayName("Get client should return 404 when not found")
    void getClient_shouldReturn404_whenNotFound() {
        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
        .when()
            .get("/bff/admin/oauth-clients/999999999")
        .then()
            .statusCode(404);
    }

    @Test
    @DisplayName("Get client by client_id should return client when exists")
    void getClientByClientId_shouldReturnClient_whenExists() {
        OAuthClient client = createTestClient();

        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
        .when()
            .get("/bff/admin/oauth-clients/by-client-id/" + client.clientId)
        .then()
            .statusCode(200)
            .body("clientId", equalTo(client.clientId));
    }

    // ==================== Update Client ====================

    @Test
    @DisplayName("Update client should update fields")
    void updateClient_shouldUpdateFields_whenValid() {
        OAuthClient client = createTestClient();

        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
            .body("""
                {
                    "clientName": "Updated Client Name",
                    "redirectUris": ["http://localhost:4000/callback", "http://localhost:5000/callback"]
                }
                """)
        .when()
            .put("/bff/admin/oauth-clients/" + client.id)
        .then()
            .statusCode(200)
            .body("clientName", equalTo("Updated Client Name"))
            .body("redirectUris", hasSize(2));
    }

    // ==================== Rotate Secret ====================

    @Test
    @DisplayName("Rotate secret should return new secret for confidential client")
    void rotateSecret_shouldReturnNewSecret_forConfidentialClient() {
        OAuthClient client = createTestConfidentialClient();

        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
        .when()
            .post("/bff/admin/oauth-clients/" + client.id + "/rotate-secret")
        .then()
            .statusCode(200)
            .body("clientId", equalTo(client.clientId))
            .body("clientSecret", notNullValue());
    }

    @Test
    @DisplayName("Rotate secret should return 400 for public client")
    void rotateSecret_shouldReturn400_forPublicClient() {
        OAuthClient client = createTestClient();

        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
        .when()
            .post("/bff/admin/oauth-clients/" + client.id + "/rotate-secret")
        .then()
            .statusCode(400)
            .body("error", containsString("public"));
    }

    // ==================== Activate/Deactivate ====================

    @Test
    @DisplayName("Deactivate client should set active to false")
    void deactivateClient_shouldSetActiveFalse() {
        OAuthClient client = createTestClient();

        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
        .when()
            .post("/bff/admin/oauth-clients/" + client.id + "/deactivate")
        .then()
            .statusCode(200)
            .body("message", equalTo("Client deactivated"));

        // Verify deactivated
        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
        .when()
            .get("/bff/admin/oauth-clients/" + client.id)
        .then()
            .statusCode(200)
            .body("active", equalTo(false));
    }

    @Test
    @DisplayName("Activate client should set active to true")
    void activateClient_shouldSetActiveTrue() {
        OAuthClient client = createDeactivatedTestClient();

        given()
            .header("Authorization", "Bearer " + adminToken)
            .contentType(ContentType.JSON)
        .when()
            .post("/bff/admin/oauth-clients/" + client.id + "/activate")
        .then()
            .statusCode(200)
            .body("message", equalTo("Client activated"));
    }

    // ==================== Helpers ====================

    private OAuthClient createTestClient() {
        return QuarkusTransaction.requiringNew().call(() -> {
            OAuthClient client = new OAuthClient();
            client.id = TsidGenerator.generate();
            client.clientId = "fc_test_" + System.currentTimeMillis();
            client.clientName = "Test Client";
            client.clientType = ClientType.PUBLIC;
            client.redirectUris = List.of("http://localhost:3000/callback");
            client.grantTypes = List.of("authorization_code", "refresh_token");
            client.pkceRequired = true;
            client.active = true;
            clientRepo.persist(client);
            return client;
        });
    }

    private OAuthClient createTestConfidentialClient() {
        return QuarkusTransaction.requiringNew().call(() -> {
            OAuthClient client = new OAuthClient();
            client.id = TsidGenerator.generate();
            client.clientId = "fc_conf_" + System.currentTimeMillis();
            client.clientName = "Test Confidential Client";
            client.clientType = ClientType.CONFIDENTIAL;
            client.redirectUris = List.of("https://api.example.com/callback");
            client.grantTypes = List.of("client_credentials", "authorization_code");
            client.clientSecretRef = "encrypted:dummy_secret_ref"; // In real usage, this would be properly encrypted
            client.pkceRequired = false;
            client.active = true;
            clientRepo.persist(client);
            return client;
        });
    }

    private OAuthClient createDeactivatedTestClient() {
        return QuarkusTransaction.requiringNew().call(() -> {
            OAuthClient client = new OAuthClient();
            client.id = TsidGenerator.generate();
            client.clientId = "fc_deactivated_" + System.currentTimeMillis();
            client.clientName = "Deactivated Test Client";
            client.clientType = ClientType.PUBLIC;
            client.redirectUris = List.of("http://localhost:3000/callback");
            client.grantTypes = List.of("authorization_code", "refresh_token");
            client.pkceRequired = true;
            client.active = false;
            clientRepo.persist(client);
            return client;
        });
    }
}
