package tech.flowcatalyst.platform.dev;

import io.quarkus.arc.Arc;
import io.quarkus.runtime.LaunchMode;
import io.quarkus.runtime.StartupEvent;
import jakarta.enterprise.context.ApplicationScoped;
import jakarta.enterprise.context.control.ActivateRequestContext;
import jakarta.enterprise.event.Observes;
import jakarta.inject.Inject;
import org.eclipse.microprofile.config.inject.ConfigProperty;
import org.jboss.logging.Logger;
import tech.flowcatalyst.platform.authentication.AuthProvider;
import tech.flowcatalyst.platform.authentication.IdpType;
import tech.flowcatalyst.platform.principal.*;
import tech.flowcatalyst.platform.client.*;
import tech.flowcatalyst.platform.application.Application;
import tech.flowcatalyst.platform.application.ApplicationOperations;
import tech.flowcatalyst.platform.application.ApplicationRepository;
import tech.flowcatalyst.platform.application.operations.createapplication.CreateApplicationCommand;
import tech.flowcatalyst.platform.shared.TsidGenerator;
import tech.flowcatalyst.platform.audit.AuditContext;
import tech.flowcatalyst.eventtype.EventTypeOperations;
import tech.flowcatalyst.eventtype.EventTypeRepository;
import tech.flowcatalyst.eventtype.operations.createeventtype.CreateEventTypeCommand;
import tech.flowcatalyst.platform.common.ExecutionContext;

import java.util.List;
import java.util.Optional;

/**
 * Seeds development data on application startup.
 *
 * Runs when any of:
 * - quarkus.launch-mode=DEV or TEST (and flowcatalyst.dev.seed-data=true)
 * - flowcatalyst.dev.force-seed=true (for dev-build native executables)
 *
 * Default credentials:
 *   Platform Admin: admin@flowcatalyst.local / DevPassword123!
 *   Client Admin:   alice@acme.com / DevPassword123!
 *   Regular User:   bob@acme.com / DevPassword123!
 */
@ApplicationScoped
public class DevDataSeeder {

    private static final Logger LOG = Logger.getLogger(DevDataSeeder.class);
    private static final String DEV_PASSWORD = "DevPassword123!";

    @Inject
    LaunchMode launchMode;

    @ConfigProperty(name = "flowcatalyst.dev.seed-data", defaultValue = "true")
    boolean seedDataEnabled;

    @ConfigProperty(name = "flowcatalyst.dev.force-seed", defaultValue = "false")
    boolean forceSeed;

    @Inject
    PasswordService passwordService;

    @Inject
    ClientRepository clientRepo;

    @Inject
    PrincipalRepository principalRepo;

    @Inject
    AnchorDomainRepository anchorDomainRepo;

    @Inject
    ClientAuthConfigRepository authConfigRepo;

    @Inject
    ClientAccessGrantRepository grantRepo;

    @Inject
    ApplicationRepository applicationRepo;

    @Inject
    EventTypeRepository eventTypeRepo;

    // These are looked up lazily to avoid early bean resolution that can cause startup issues
    private AuditContext getAuditContext() {
        return Arc.container().instance(AuditContext.class).get();
    }

    private ApplicationOperations getApplicationOperations() {
        return Arc.container().instance(ApplicationOperations.class).get();
    }

    private EventTypeOperations getEventTypeOperations() {
        return Arc.container().instance(EventTypeOperations.class).get();
    }

    @ActivateRequestContext
    void onStart(@Observes StartupEvent event) {
        if (!shouldSeed()) {
            return;
        }

        LOG.info("=== DEV DATA SEEDER ===");
        LOG.info("Seeding development data...");

        try {
            seedData();
            LOG.info("Development data seeded successfully!");
            LOG.info("");
            LOG.info("Default logins:");
            LOG.infof("  Platform Admin: admin@flowcatalyst.local / %s", DEV_PASSWORD);
            LOG.infof("  Client Admin:   alice@acme.com / %s", DEV_PASSWORD);
            LOG.infof("  Regular User:   bob@acme.com / %s", DEV_PASSWORD);
            LOG.info("=======================");
        } catch (Exception e) {
            LOG.warn("Dev data seeding skipped (data may already exist): " + e.getMessage());
        }
    }

    private boolean shouldSeed() {
        // Force seed bypasses launch mode check (for dev-build native executables)
        if (forceSeed) {
            LOG.info("Force seed enabled - seeding regardless of launch mode");
            return true;
        }

        if (launchMode != LaunchMode.DEVELOPMENT && launchMode != LaunchMode.TEST) {
            LOG.debug("Skipping dev seeder - not in dev/test mode (use flowcatalyst.dev.force-seed=true to override)");
            return false;
        }

        if (!seedDataEnabled) {
            LOG.debug("Skipping dev seeder - disabled via config");
            return false;
        }

        return true;
    }

    void seedData() {
        // Set up SYSTEM principal for audit context since we're outside HTTP request
        // Must be inside @Transactional because it may need to persist the SYSTEM principal
        getAuditContext().setSystemPrincipal();

        seedAnchorDomain();
        seedClients();
        seedAuthConfig();
        seedUsers();
        seedApplications();
        seedEventTypes();
    }

    private void seedAnchorDomain() {
        if (anchorDomainRepo.existsByDomain("flowcatalyst.local")) {
            return;
        }

        AnchorDomain anchor = new AnchorDomain();
        anchor.id = TsidGenerator.generate();
        anchor.domain = "flowcatalyst.local";
        anchorDomainRepo.persist(anchor);
        LOG.info("Created anchor domain: flowcatalyst.local");
    }

    private void seedClients() {
        createClientIfNotExists("Acme Corporation", "acme", ClientStatus.ACTIVE);
        createClientIfNotExists("Globex Industries", "globex", ClientStatus.ACTIVE);
        createClientIfNotExists("Initech Solutions", "initech", ClientStatus.ACTIVE);
        createClientIfNotExists("Umbrella Corp", "umbrella", ClientStatus.SUSPENDED);
    }

    private Client createClientIfNotExists(String name, String identifier, ClientStatus status) {
        Optional<Client> existing = clientRepo.findByIdentifier(identifier);
        if (existing.isPresent()) {
            return existing.get();
        }

        Client client = new Client();
        client.id = TsidGenerator.generate();
        client.name = name;
        client.identifier = identifier;
        client.status = status;
        clientRepo.persist(client);
        LOG.infof("Created client: %s (%s)", name, identifier);
        return client;
    }

    private void seedAuthConfig() {
        createAuthConfigIfNotExists("flowcatalyst.local", AuthProvider.INTERNAL);
        createAuthConfigIfNotExists("acme.com", AuthProvider.INTERNAL);
        createAuthConfigIfNotExists("globex.com", AuthProvider.INTERNAL);
        createAuthConfigIfNotExists("initech.com", AuthProvider.INTERNAL);
        createAuthConfigIfNotExists("partner.io", AuthProvider.INTERNAL);
    }

    private void createAuthConfigIfNotExists(String domain, AuthProvider provider) {
        if (authConfigRepo.findByEmailDomain(domain).isPresent()) {
            return;
        }

        ClientAuthConfig config = new ClientAuthConfig();
        config.id = TsidGenerator.generate();
        config.emailDomain = domain;
        config.authProvider = provider;
        authConfigRepo.persist(config);
    }

    private void seedUsers() {
        String passwordHash = passwordService.hashPassword(DEV_PASSWORD);

        // Platform admin (anchor domain - access to all clients)
        createUserIfNotExists(
            "admin@flowcatalyst.local",
            "Platform Administrator",
            null,  // No home client
            passwordHash,
            List.of("platform:super-admin")
        );

        // Get Acme client
        Client acme = clientRepo.findByIdentifier("acme").orElse(null);
        Client globex = clientRepo.findByIdentifier("globex").orElse(null);

        if (acme != null) {
            // Acme client admin
            createUserIfNotExists(
                "alice@acme.com",
                "Alice Johnson",
                acme.id,
                passwordHash,
                List.of("acme:client-admin", "dispatch:admin")
            );

            // Acme regular user
            createUserIfNotExists(
                "bob@acme.com",
                "Bob Smith",
                acme.id,
                passwordHash,
                List.of("dispatch:user")
            );
        }

        if (globex != null) {
            // Globex user
            createUserIfNotExists(
                "charlie@globex.com",
                "Charlie Brown",
                globex.id,
                passwordHash,
                List.of("dispatch:admin")
            );
        }

        // Partner user (cross-client access)
        Principal partner = createUserIfNotExists(
            "diana@partner.io",
            "Diana Partner",
            null,  // No home client
            passwordHash,
            List.of("dispatch:viewer")
        );

        // Grant partner access to Acme and Globex
        if (partner != null && acme != null) {
            createGrantIfNotExists(partner.id, acme.id);
        }
        if (partner != null && globex != null) {
            createGrantIfNotExists(partner.id, globex.id);
        }
    }

    private Principal createUserIfNotExists(String email, String name, String clientId,
                                            String passwordHash, List<String> roles) {
        Optional<Principal> existing = principalRepo.findByEmail(email);
        if (existing.isPresent()) {
            return existing.get();
        }

        Principal user = new Principal();
        user.id = TsidGenerator.generate();
        user.type = PrincipalType.USER;
        user.clientId = clientId;
        user.name = name;
        user.active = true;

        user.userIdentity = new UserIdentity();
        user.userIdentity.email = email;
        user.userIdentity.emailDomain = email.substring(email.indexOf('@') + 1);
        user.userIdentity.idpType = IdpType.INTERNAL;
        user.userIdentity.passwordHash = passwordHash;

        // Add roles (embedded in principal)
        for (String roleName : roles) {
            user.roles.add(new Principal.RoleAssignment(roleName, "DEV_SEEDER"));
        }

        principalRepo.persist(user);

        LOG.infof("Created user: %s (%s)", name, email);
        return user;
    }

    private void createGrantIfNotExists(String principalId, String clientId) {
        if (grantRepo.existsByPrincipalIdAndClientId(principalId, clientId)) {
            return;
        }

        ClientAccessGrant grant = new ClientAccessGrant();
        grant.id = TsidGenerator.generate();
        grant.principalId = principalId;
        grant.clientId = clientId;
        grantRepo.persist(grant);
    }

    // ========================================================================
    // Applications
    // ========================================================================

    private void seedApplications() {
        createApplicationIfNotExists("tms", "Transport Management System",
            "End-to-end transportation planning, execution, and optimization");
        createApplicationIfNotExists("wms", "Warehouse Management System",
            "Inventory control, picking, packing, and warehouse operations");
        createApplicationIfNotExists("oms", "Order Management System",
            "Order processing, fulfillment orchestration, and customer service");
        createApplicationIfNotExists("track", "Shipment Tracking",
            "Real-time visibility and tracking for shipments and assets");
        createApplicationIfNotExists("yard", "Yard Management System",
            "Dock scheduling, trailer tracking, and yard operations");
        createApplicationIfNotExists("platform", "Platform Services",
            "Core platform infrastructure and shared services");
    }

    private void createApplicationIfNotExists(String code, String name, String description) {
        if (applicationRepo.findByCode(code).isPresent()) {
            return;
        }

        ExecutionContext context = ExecutionContext.create(getAuditContext().requirePrincipalId());
        CreateApplicationCommand command = new CreateApplicationCommand(code, name, description, null, null);
        getApplicationOperations().createApplication(command, context);
        LOG.infof("Created application: %s (%s)", name, code);
    }

    // ========================================================================
    // Event Types
    // ========================================================================

    private void seedEventTypes() {
        // TMS - Transport Management Events
        createEventTypeIfNotExists("tms:planning:load:created", "Load Created",
            "A new load has been created in the system");
        createEventTypeIfNotExists("tms:planning:load:updated", "Load Updated",
            "Load details have been modified");
        createEventTypeIfNotExists("tms:planning:load:tendered", "Load Tendered",
            "Load has been tendered to a carrier");
        createEventTypeIfNotExists("tms:planning:load:accepted", "Load Accepted",
            "Carrier has accepted the load tender");
        createEventTypeIfNotExists("tms:planning:load:rejected", "Load Rejected",
            "Carrier has rejected the load tender");
        createEventTypeIfNotExists("tms:planning:route:optimized", "Route Optimized",
            "Route optimization completed for shipments");

        createEventTypeIfNotExists("tms:execution:shipment:dispatched", "Shipment Dispatched",
            "Shipment has been dispatched for delivery");
        createEventTypeIfNotExists("tms:execution:shipment:in-transit", "Shipment In Transit",
            "Shipment is currently in transit");
        createEventTypeIfNotExists("tms:execution:shipment:delivered", "Shipment Delivered",
            "Shipment has been successfully delivered");
        createEventTypeIfNotExists("tms:execution:shipment:exception", "Shipment Exception",
            "An exception occurred during shipment execution");
        createEventTypeIfNotExists("tms:execution:driver:assigned", "Driver Assigned",
            "Driver has been assigned to a shipment");
        createEventTypeIfNotExists("tms:execution:driver:checked-in", "Driver Checked In",
            "Driver has checked in at a facility");

        createEventTypeIfNotExists("tms:billing:invoice:generated", "Invoice Generated",
            "Freight invoice has been generated");
        createEventTypeIfNotExists("tms:billing:invoice:approved", "Invoice Approved",
            "Freight invoice has been approved for payment");
        createEventTypeIfNotExists("tms:billing:payment:processed", "Payment Processed",
            "Payment has been processed for carrier");

        // WMS - Warehouse Management Events
        createEventTypeIfNotExists("wms:inventory:receipt:completed", "Receipt Completed",
            "Inbound receipt has been completed");
        createEventTypeIfNotExists("wms:inventory:putaway:completed", "Putaway Completed",
            "Inventory putaway has been completed");
        createEventTypeIfNotExists("wms:inventory:adjustment:made", "Inventory Adjusted",
            "Inventory adjustment has been recorded");
        createEventTypeIfNotExists("wms:inventory:cycle-count:completed", "Cycle Count Completed",
            "Inventory cycle count has been completed");
        createEventTypeIfNotExists("wms:inventory:transfer:completed", "Transfer Completed",
            "Inventory transfer between locations completed");

        createEventTypeIfNotExists("wms:outbound:wave:released", "Wave Released",
            "Outbound wave has been released for picking");
        createEventTypeIfNotExists("wms:outbound:pick:completed", "Pick Completed",
            "Order picking has been completed");
        createEventTypeIfNotExists("wms:outbound:pack:completed", "Pack Completed",
            "Order packing has been completed");
        createEventTypeIfNotExists("wms:outbound:ship:confirmed", "Ship Confirmed",
            "Shipment has been confirmed and loaded");

        createEventTypeIfNotExists("wms:labor:task:assigned", "Task Assigned",
            "Work task has been assigned to associate");
        createEventTypeIfNotExists("wms:labor:task:completed", "Task Completed",
            "Work task has been completed by associate");

        // OMS - Order Management Events
        createEventTypeIfNotExists("oms:order:order:created", "Order Created",
            "New customer order has been created");
        createEventTypeIfNotExists("oms:order:order:confirmed", "Order Confirmed",
            "Order has been confirmed and validated");
        createEventTypeIfNotExists("oms:order:order:cancelled", "Order Cancelled",
            "Order has been cancelled");
        createEventTypeIfNotExists("oms:order:order:modified", "Order Modified",
            "Order has been modified after creation");

        createEventTypeIfNotExists("oms:fulfillment:allocation:completed", "Allocation Completed",
            "Inventory allocation for order completed");
        createEventTypeIfNotExists("oms:fulfillment:backorder:created", "Backorder Created",
            "Backorder created for unavailable items");
        createEventTypeIfNotExists("oms:fulfillment:split:occurred", "Order Split",
            "Order has been split into multiple shipments");

        createEventTypeIfNotExists("oms:returns:return:initiated", "Return Initiated",
            "Customer return has been initiated");
        createEventTypeIfNotExists("oms:returns:return:received", "Return Received",
            "Returned items have been received");
        createEventTypeIfNotExists("oms:returns:refund:processed", "Refund Processed",
            "Refund has been processed for return");

        // Track - Shipment Tracking Events
        createEventTypeIfNotExists("track:visibility:checkpoint:recorded", "Checkpoint Recorded",
            "Shipment checkpoint has been recorded");
        createEventTypeIfNotExists("track:visibility:eta:updated", "ETA Updated",
            "Estimated time of arrival has been updated");
        createEventTypeIfNotExists("track:visibility:delay:detected", "Delay Detected",
            "Shipment delay has been detected");
        createEventTypeIfNotExists("track:visibility:geofence:entered", "Geofence Entered",
            "Asset has entered a geofence area");
        createEventTypeIfNotExists("track:visibility:geofence:exited", "Geofence Exited",
            "Asset has exited a geofence area");

        createEventTypeIfNotExists("track:alerts:exception:raised", "Exception Raised",
            "Tracking exception has been raised");
        createEventTypeIfNotExists("track:alerts:temperature:breach", "Temperature Breach",
            "Temperature threshold has been breached");
        createEventTypeIfNotExists("track:alerts:tamper:detected", "Tamper Detected",
            "Potential tampering has been detected");

        // Yard - Yard Management Events
        createEventTypeIfNotExists("yard:gate:check-in:completed", "Gate Check-In",
            "Vehicle has completed gate check-in");
        createEventTypeIfNotExists("yard:gate:check-out:completed", "Gate Check-Out",
            "Vehicle has completed gate check-out");

        createEventTypeIfNotExists("yard:dock:appointment:scheduled", "Appointment Scheduled",
            "Dock appointment has been scheduled");
        createEventTypeIfNotExists("yard:dock:appointment:arrived", "Appointment Arrived",
            "Vehicle has arrived for dock appointment");
        createEventTypeIfNotExists("yard:dock:door:assigned", "Door Assigned",
            "Dock door has been assigned to trailer");
        createEventTypeIfNotExists("yard:dock:loading:started", "Loading Started",
            "Loading/unloading has started at dock");
        createEventTypeIfNotExists("yard:dock:loading:completed", "Loading Completed",
            "Loading/unloading has been completed");

        createEventTypeIfNotExists("yard:yard:trailer:spotted", "Trailer Spotted",
            "Trailer has been spotted at location");
        createEventTypeIfNotExists("yard:yard:trailer:moved", "Trailer Moved",
            "Trailer has been moved within yard");
        createEventTypeIfNotExists("yard:yard:trailer:sealed", "Trailer Sealed",
            "Trailer has been sealed");

        // Platform - Infrastructure Events
        createEventTypeIfNotExists("platform:integration:webhook:delivered", "Webhook Delivered",
            "Outbound webhook has been successfully delivered");
        createEventTypeIfNotExists("platform:integration:webhook:failed", "Webhook Failed",
            "Outbound webhook delivery has failed");
        createEventTypeIfNotExists("platform:integration:sync:completed", "Sync Completed",
            "Data synchronization has been completed");

        createEventTypeIfNotExists("platform:audit:login:success", "Login Success",
            "User has successfully logged in");
        createEventTypeIfNotExists("platform:audit:login:failed", "Login Failed",
            "User login attempt has failed");
        createEventTypeIfNotExists("platform:audit:permission:changed", "Permission Changed",
            "User permissions have been modified");

        // Platform - Control Plane Events (Dog-fooding)
        // EventType aggregate
        createEventTypeIfNotExists("platform:control-plane:event-type:created", "Event Type Created",
            "A new event type has been registered in the platform");
        createEventTypeIfNotExists("platform:control-plane:event-type:updated", "Event Type Updated",
            "Event type metadata has been updated");
        createEventTypeIfNotExists("platform:control-plane:event-type:archived", "Event Type Archived",
            "Event type has been archived");
        createEventTypeIfNotExists("platform:control-plane:event-type:deleted", "Event Type Deleted",
            "Event type has been deleted from the platform");
        createEventTypeIfNotExists("platform:control-plane:event-type:schema-added", "Event Type Schema Added",
            "A new schema version has been added to an event type");
        createEventTypeIfNotExists("platform:control-plane:event-type:schema-deprecated", "Event Type Schema Deprecated",
            "A schema version has been marked as deprecated");
        createEventTypeIfNotExists("platform:control-plane:event-type:schema-activated", "Event Type Schema Activated",
            "A schema version has been activated as current");

        // Application aggregate
        createEventTypeIfNotExists("platform:control-plane:application:created", "Application Created",
            "A new application has been registered in the platform");
        createEventTypeIfNotExists("platform:control-plane:application:updated", "Application Updated",
            "Application details have been updated");
        createEventTypeIfNotExists("platform:control-plane:application:activated", "Application Activated",
            "Application has been activated");
        createEventTypeIfNotExists("platform:control-plane:application:deactivated", "Application Deactivated",
            "Application has been deactivated");
        createEventTypeIfNotExists("platform:control-plane:application:deleted", "Application Deleted",
            "Application has been deleted from the platform");

        // Role aggregate
        createEventTypeIfNotExists("platform:control-plane:role:created", "Role Created",
            "A new role has been created");
        createEventTypeIfNotExists("platform:control-plane:role:updated", "Role Updated",
            "Role details or permissions have been updated");
        createEventTypeIfNotExists("platform:control-plane:role:deleted", "Role Deleted",
            "Role has been deleted");
        createEventTypeIfNotExists("platform:control-plane:role:synced", "Roles Synced",
            "Roles have been bulk synced from an external application");

        LOG.info("Event types seeded successfully");
    }

    private void createEventTypeIfNotExists(String code, String name, String description) {
        if (eventTypeRepo.findByCode(code).isPresent()) {
            return;
        }

        ExecutionContext context = ExecutionContext.create(getAuditContext().requirePrincipalId());
        CreateEventTypeCommand command = new CreateEventTypeCommand(code, name, description);
        getEventTypeOperations().createEventType(command, context);
    }
}
