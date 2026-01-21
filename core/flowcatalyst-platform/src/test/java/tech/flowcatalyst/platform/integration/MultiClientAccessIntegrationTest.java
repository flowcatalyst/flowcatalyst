package tech.flowcatalyst.platform.integration;

import io.quarkus.test.junit.QuarkusTest;
import jakarta.inject.Inject;
import org.junit.jupiter.api.DisplayName;
import org.junit.jupiter.api.Tag;
import org.junit.jupiter.api.Test;
import tech.flowcatalyst.platform.principal.AnchorDomain;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.platform.client.Client;
import tech.flowcatalyst.platform.principal.AnchorDomainRepository;
import tech.flowcatalyst.platform.client.ClientService;
import tech.flowcatalyst.platform.principal.UserService;
import tech.flowcatalyst.platform.principal.UserScope;
import tech.flowcatalyst.platform.shared.TsidGenerator;

import java.util.Set;

import java.util.UUID;

import static org.assertj.core.api.Assertions.*;

/**
 * Integration tests for multi-client access control.
 * Tests end-to-end client access calculation with real database.
 */
@Tag("integration")
@QuarkusTest
class MultiClientAccessIntegrationTest {

    @Inject
    ClientService clientService;

    @Inject
    UserService userService;

    @Inject
    AnchorDomainRepository anchorDomainRepo;

    // ========================================
    // HELPER METHODS
    // ========================================

    private String uniqueEmail(String prefix) {
        return prefix + "-" + UUID.randomUUID().toString().substring(0, 8) + "@test.com";
    }

    private String uniqueCode(String prefix) {
        return prefix + "-" + UUID.randomUUID().toString().substring(0, 8);
    }

    private String uniqueDomain(String prefix) {
        return prefix + "-" + UUID.randomUUID().toString().substring(0, 8) + ".com";
    }

    // ========================================
    // ANCHOR DOMAIN USER TESTS
    // ========================================

    @Test
    @DisplayName("Anchor domain user should access all active clients")
    void anchorDomainUser_shouldAccessAllClients_whenDomainIsAnchor() {
        // Arrange: Register anchor domain (e.g., internal company domain)
        String domain = uniqueDomain("mycompany");
        AnchorDomain anchor = new AnchorDomain();
        anchor.id = TsidGenerator.generate();
        anchor.domain = domain;
        anchorDomainRepo.persist(anchor);

        // Create several clients (customer accounts)
        Client client1 = clientService.createClient("Customer A", uniqueCode("customer-a"));
        Client client2 = clientService.createClient("Customer B", uniqueCode("customer-b"));
        Client client3 = clientService.createClient("Customer C", uniqueCode("customer-c"));

        // Create anchor domain user (internal employee)
        Principal admin = userService.createInternalUser(
            uniqueEmail("admin"),
            "SecurePass123!",
            "Platform Admin",
            null,
            UserScope.ANCHOR
        );

        // Act: Get accessible clients
        Set<String> accessible = clientService.getAccessibleClients(admin.id);

        // Assert: Can access all active clients (created in this test)
        assertThat(accessible).contains(client1.id, client2.id, client3.id);
    }

    @Test
    @DisplayName("Anchor domain user should not see inactive clients")
    void anchorDomainUser_shouldNotSeeInactive_whenClientsDeactivated() {
        // Arrange: Register anchor domain
        String domain = uniqueDomain("mycompany");
        AnchorDomain anchor = new AnchorDomain();
        anchor.id = TsidGenerator.generate();
        anchor.domain = domain;
        anchorDomainRepo.persist(anchor);

        // Create 3 clients
        Client active1 = clientService.createClient("Active 1", uniqueCode("active-1"));
        Client active2 = clientService.createClient("Active 2", uniqueCode("active-2"));
        Client inactive = clientService.createClient("Inactive", uniqueCode("inactive"));

        // Deactivate one client
        clientService.deactivateClient(inactive.id, "Test deactivation", "system");

        // Create anchor user
        Principal admin = userService.createInternalUser(
            uniqueEmail("admin"),
            "SecurePass123!",
            "Admin",
            null, UserScope.ANCHOR
        );

        // Act
        Set<String> accessible = clientService.getAccessibleClients(admin.id);

        // Assert: Only active clients visible
        assertThat(accessible).contains(active1.id, active2.id);
        assertThat(accessible).doesNotContain(inactive.id);
    }

    // ========================================
    // HOME CLIENT USER TESTS
    // ========================================

    @Test
    @DisplayName("Regular user should only access home client")
    void regularUser_shouldAccessOnlyHomeClient_whenNoGrants() {
        // Arrange: Create 2 customer clients
        Client client1 = clientService.createClient("Customer A", uniqueCode("customer-a"));
        Client client2 = clientService.createClient("Customer B", uniqueCode("customer-b"));

        // Create user belonging to client 1
        Principal user = userService.createInternalUser(
            uniqueEmail("user"),
            "SecurePass123!",
            "Customer A User",
            client1.id, UserScope.CLIENT
        );

        // Act
        Set<String> accessible = clientService.getAccessibleClients(user.id);

        // Assert: Only has access to home client
        assertThat(accessible).containsExactly(client1.id);
        assertThat(accessible).doesNotContain(client2.id);
    }

    @Test
    @DisplayName("User with no home client and no grants should have no access")
    void user_shouldHaveNoAccess_whenNoHomeClientAndNoGrants() {
        // Arrange: Create client
        Client client = clientService.createClient("Customer A", uniqueCode("customer-a"));

        // Create user with NO home client (e.g., partner user before grants)
        Principal partner = userService.createInternalUser(
            uniqueEmail("partner"),
            "SecurePass123!",
            "Partner User",
            null,
            UserScope.PARTNER
        );

        // Act
        Set<String> accessible = clientService.getAccessibleClients(partner.id);

        // Assert: No access to any client
        assertThat(accessible).isEmpty();
    }

    // ========================================
    // CLIENT ACCESS GRANT TESTS
    // ========================================

    @Test
    @DisplayName("Partner user should access granted clients")
    void partnerUser_shouldAccessGrantedClients_whenGrantsExist() {
        // Arrange: Create 3 customer clients
        Client customerA = clientService.createClient("Customer A", uniqueCode("customer-a"));
        Client customerB = clientService.createClient("Customer B", uniqueCode("customer-b"));
        Client customerC = clientService.createClient("Customer C", uniqueCode("customer-c"));

        // Create partner user (no home client)
        Principal partner = userService.createInternalUser(
            uniqueEmail("partner"),
            "SecurePass123!",
            "Logistics Partner",
            null, UserScope.PARTNER
        );

        // Grant access to customers A and B (not C)
        clientService.grantClientAccess(partner.id, customerA.id);
        clientService.grantClientAccess(partner.id, customerB.id);

        // Act
        Set<String> accessible = clientService.getAccessibleClients(partner.id);

        // Assert: Can access A and B, but not C
        assertThat(accessible).containsExactlyInAnyOrder(customerA.id, customerB.id);
        assertThat(accessible).doesNotContain(customerC.id);
    }

    @Test
    @DisplayName("User with home client and grants should access both")
    void user_shouldAccessBoth_whenHasHomeClientAndGrants() {
        // Arrange: Create 3 clients
        Client homeClient = clientService.createClient("Home Client", uniqueCode("home"));
        Client grantedA = clientService.createClient("Granted A", uniqueCode("granted-a"));
        Client grantedB = clientService.createClient("Granted B", uniqueCode("granted-b"));

        // Create user with home client
        Principal user = userService.createInternalUser(
            uniqueEmail("user"),
            "SecurePass123!",
            "User",
            homeClient.id, UserScope.CLIENT
        );

        // Grant access to additional clients
        clientService.grantClientAccess(user.id, grantedA.id);
        clientService.grantClientAccess(user.id, grantedB.id);

        // Act
        Set<String> accessible = clientService.getAccessibleClients(user.id);

        // Assert: Access to home + 2 granted = 3 total
        assertThat(accessible).containsExactlyInAnyOrder(
            homeClient.id,
            grantedA.id,
            grantedB.id
        );
    }

    @Test
    @DisplayName("Revoking client grant should immediately remove access")
    void revokeClientGrant_shouldRemoveAccess_whenGrantRevoked() {
        // Arrange: Create client and user
        Client client = clientService.createClient("Customer", uniqueCode("customer"));
        Principal partner = userService.createInternalUser(
            uniqueEmail("partner"),
            "SecurePass123!",
            "Partner",
            null, UserScope.PARTNER
        );

        // Grant access
        clientService.grantClientAccess(partner.id, client.id);

        // Verify access granted
        Set<String> beforeRevoke = clientService.getAccessibleClients(partner.id);
        assertThat(beforeRevoke).contains(client.id);

        // Act: Revoke access
        clientService.revokeClientAccess(partner.id, client.id);

        // Assert: Access immediately removed
        Set<String> afterRevoke = clientService.getAccessibleClients(partner.id);
        assertThat(afterRevoke).doesNotContain(client.id);
        assertThat(afterRevoke).isEmpty();
    }

    // ========================================
    // GRANT MANAGEMENT TESTS
    // ========================================

    @Test
    @DisplayName("Cannot grant access if user already has home client")
    void grantClientAccess_shouldFail_whenUserAlreadyHasHomeClient() {
        // Arrange: Create client
        Client client = clientService.createClient("Customer", uniqueCode("customer"));

        // Create user with this client as home
        Principal user = userService.createInternalUser(
            uniqueEmail("user"),
            "SecurePass123!",
            "User",
            client.id, UserScope.CLIENT
        );

        // Act & Assert: Cannot grant same client (redundant)
        assertThatThrownBy(() ->
            clientService.grantClientAccess(user.id, client.id)
        )
        .hasMessageContaining("already belongs to this client");
    }

    @Test
    @DisplayName("Cannot grant same client access twice")
    void grantClientAccess_shouldFail_whenGrantAlreadyExists() {
        // Arrange
        Client client = clientService.createClient("Customer", uniqueCode("customer"));
        Principal partner = userService.createInternalUser(
            uniqueEmail("partner"),
            "SecurePass123!",
            "Partner",
            null, UserScope.PARTNER
        );

        // First grant
        clientService.grantClientAccess(partner.id, client.id);

        // Act & Assert: Second grant fails
        assertThatThrownBy(() ->
            clientService.grantClientAccess(partner.id, client.id)
        )
        .hasMessageContaining("already exists");
    }

    @Test
    @DisplayName("Revoking non-existent grant should fail")
    void revokeClientAccess_shouldFail_whenGrantDoesNotExist() {
        // Arrange
        Client client = clientService.createClient("Customer", uniqueCode("customer"));
        Principal partner = userService.createInternalUser(
            uniqueEmail("partner"),
            "SecurePass123!",
            "Partner",
            null, UserScope.PARTNER
        );

        // Act & Assert: Revoke without grant
        assertThatThrownBy(() ->
            clientService.revokeClientAccess(partner.id, client.id)
        )
        .hasMessageContaining("not found");
    }

    // ========================================
    // CLIENT ISOLATION TESTS
    // ========================================

    @Test
    @DisplayName("Users from different clients should not see each other")
    void users_shouldNotSeeEachOther_whenFromDifferentClients() {
        // Arrange: Create 2 separate clients
        Client clientA = clientService.createClient("Client A", uniqueCode("client-a"));
        Client clientB = clientService.createClient("Client B", uniqueCode("client-b"));

        // Create users in each client
        Principal userA = userService.createInternalUser(
            uniqueEmail("user-a"),
            "SecurePass123!",
            "User A",
            clientA.id, UserScope.CLIENT
        );

        Principal userB = userService.createInternalUser(
            uniqueEmail("user-b"),
            "SecurePass123!",
            "User B",
            clientB.id, UserScope.CLIENT
        );

        // Act
        Set<String> userAAccess = clientService.getAccessibleClients(userA.id);
        Set<String> userBAccess = clientService.getAccessibleClients(userB.id);

        // Assert: Complete isolation
        assertThat(userAAccess).containsExactly(clientA.id);
        assertThat(userBAccess).containsExactly(clientB.id);
        assertThat(userAAccess).doesNotContain(clientB.id);
        assertThat(userBAccess).doesNotContain(clientA.id);
    }

    // ========================================
    // COMPLEX SCENARIOS
    // ========================================

    @Test
    @DisplayName("Complex scenario: Anchor user, regular users, and partner users")
    void complexScenario_shouldWorkCorrectly_withMultipleUserTypes() {
        // Arrange: Register anchor domain
        String domain = uniqueDomain("platform");
        AnchorDomain anchor = new AnchorDomain();
        anchor.id = TsidGenerator.generate();
        anchor.domain = domain;
        anchorDomainRepo.persist(anchor);

        // Create 3 customer clients
        Client customer1 = clientService.createClient("Customer 1", uniqueCode("customer-1"));
        Client customer2 = clientService.createClient("Customer 2", uniqueCode("customer-2"));
        Client customer3 = clientService.createClient("Customer 3", uniqueCode("customer-3"));

        // Create platform admin (anchor domain)
        Principal platformAdmin = userService.createInternalUser(
            uniqueEmail("admin"),
            "SecurePass123!",
            "Platform Admin",
            null, UserScope.ANCHOR
        );

        // Create customer users (home clients)
        Principal customer1User = userService.createInternalUser(
            uniqueEmail("customer1-user"),
            "SecurePass123!",
            "Customer 1 User",
            customer1.id, UserScope.CLIENT
        );

        Principal customer2User = userService.createInternalUser(
            uniqueEmail("customer2-user"),
            "SecurePass123!",
            "Customer 2 User",
            customer2.id, UserScope.CLIENT
        );

        // Create partner with grants to customers 1 and 2
        Principal partner = userService.createInternalUser(
            uniqueEmail("partner"),
            "SecurePass123!",
            "Logistics Partner",
            null, UserScope.PARTNER
        );

        clientService.grantClientAccess(partner.id, customer1.id);
        clientService.grantClientAccess(partner.id, customer2.id);

        // Act: Get access for all users
        Set<String> adminAccess = clientService.getAccessibleClients(platformAdmin.id);
        Set<String> customer1Access = clientService.getAccessibleClients(customer1User.id);
        Set<String> customer2Access = clientService.getAccessibleClients(customer2User.id);
        Set<String> partnerAccess = clientService.getAccessibleClients(partner.id);

        // Assert: Platform admin sees all (at least the 3 we created)
        assertThat(adminAccess).contains(customer1.id, customer2.id, customer3.id);

        // Assert: Customer users see only their client
        assertThat(customer1Access).containsExactly(customer1.id);
        assertThat(customer2Access).containsExactly(customer2.id);

        // Assert: Partner sees only granted clients
        assertThat(partnerAccess).containsExactlyInAnyOrder(customer1.id, customer2.id);
        assertThat(partnerAccess).doesNotContain(customer3.id);
    }

    @Test
    @DisplayName("Deactivating client should not affect access calculation logic")
    void deactivatingClient_shouldBeFilteredOut_whenCalculatingAccess() {
        // Arrange: Create 2 clients
        Client active = clientService.createClient("Active", uniqueCode("active"));
        Client toBeDeactivated = clientService.createClient("Will Deactivate", uniqueCode("deactivate"));

        // Create partner with access to both
        Principal partner = userService.createInternalUser(
            uniqueEmail("partner"),
            "SecurePass123!",
            "Partner",
            null, UserScope.PARTNER
        );

        clientService.grantClientAccess(partner.id, active.id);
        clientService.grantClientAccess(partner.id, toBeDeactivated.id);

        // Verify initial access
        Set<String> beforeDeactivation = clientService.getAccessibleClients(partner.id);
        assertThat(beforeDeactivation).containsExactlyInAnyOrder(active.id, toBeDeactivated.id);

        // Act: Deactivate one client
        clientService.deactivateClient(toBeDeactivated.id, "Test", "system");

        // Assert: Deactivated client not in accessible list
        Set<String> afterDeactivation = clientService.getAccessibleClients(partner.id);
        assertThat(afterDeactivation).containsExactly(active.id);
        assertThat(afterDeactivation).doesNotContain(toBeDeactivated.id);
    }
}
