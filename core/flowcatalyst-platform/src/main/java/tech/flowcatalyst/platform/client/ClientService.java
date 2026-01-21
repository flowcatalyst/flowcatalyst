package tech.flowcatalyst.platform.client;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.ws.rs.BadRequestException;
import jakarta.ws.rs.NotFoundException;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.platform.principal.PrincipalRepository;
import tech.flowcatalyst.platform.shared.TsidGenerator;

import java.util.List;
import java.util.Optional;
import java.util.Set;

/**
 * Service for client CRUD operations and access management.
 */
@ApplicationScoped
public class ClientService {

    @Inject
    ClientRepository clientRepo;

    @Inject
    ClientAccessGrantRepository grantRepo;

    @Inject
    PrincipalRepository principalRepo;

    @Inject
    ClientAccessService clientAccessService;

    /**
     * Create a new client.
     *
     * @param name Client display name
     * @param identifier Unique client identifier/slug (e.g., "acme-corp"). Max 60 chars, lowercase, hyphens/underscores allowed.
     * @return Created client
     * @throws BadRequestException if identifier already exists or is invalid
     */
    public Client createClient(String name, String identifier) {
        // Validate identifier format
        if (identifier == null || identifier.isBlank()) {
            throw new BadRequestException("Client identifier is required");
        }
        if (identifier.length() > 60) {
            throw new BadRequestException("Client identifier must be 60 characters or less");
        }
        if (!identifier.matches("^[a-z][a-z0-9_-]*$")) {
            throw new BadRequestException("Client identifier must be lowercase, start with a letter, and contain only letters, numbers, hyphens, and underscores");
        }

        // Validate identifier uniqueness
        if (clientRepo.findByIdentifier(identifier).isPresent()) {
            throw new BadRequestException("Client identifier already exists: " + identifier);
        }

        // Create client
        Client client = new Client();
        client.id = TsidGenerator.generate();
        client.name = name;
        client.identifier = identifier;
        client.status = ClientStatus.ACTIVE;

        clientRepo.persist(client);
        return client;
    }

    /**
     * Update client name.
     *
     * @param clientId Client ID
     * @param name New client name
     * @return Updated client
     * @throws NotFoundException if client not found
     */
    public Client updateClient(String clientId, String name) {
        Client client = clientRepo.findByIdOptional(clientId)
            .orElseThrow(() -> new NotFoundException("Client not found"));

        client.name = name;
        clientRepo.update(client);
        return client;
    }

    /**
     * Change client status.
     *
     * @param clientId Client ID
     * @param status New status
     * @param reason Status reason (e.g., "ACCOUNT_NOT_PAID")
     * @param note Optional note to add to audit trail
     * @param changedBy Principal ID of who made the change
     * @throws NotFoundException if client not found
     */
    public void changeClientStatus(String clientId, ClientStatus status, String reason, String note, String changedBy) {
        Client client = clientRepo.findByIdOptional(clientId)
            .orElseThrow(() -> new NotFoundException("Client not found"));

        client.changeStatus(status, reason, note, changedBy);
        clientRepo.update(client);
    }

    /**
     * Deactivate a client (soft delete).
     * Deactivated clients cannot be accessed.
     *
     * @param clientId Client ID
     * @param reason Reason for deactivation
     * @param changedBy Who deactivated the client
     * @throws NotFoundException if client not found
     */
    public void deactivateClient(String clientId, String reason, String changedBy) {
        changeClientStatus(clientId, ClientStatus.INACTIVE, reason,
            "Client deactivated: " + reason, changedBy);
    }

    /**
     * Suspend a client.
     *
     * @param clientId Client ID
     * @param reason Reason for suspension
     * @param changedBy Who suspended the client
     * @throws NotFoundException if client not found
     */
    public void suspendClient(String clientId, String reason, String changedBy) {
        changeClientStatus(clientId, ClientStatus.SUSPENDED, reason,
            "Client suspended: " + reason, changedBy);
    }

    /**
     * Activate a client (un-suspend or un-deactivate).
     *
     * @param clientId Client ID
     * @param changedBy Who activated the client
     * @throws NotFoundException if client not found
     */
    public void activateClient(String clientId, String changedBy) {
        changeClientStatus(clientId, ClientStatus.ACTIVE, null,
            "Client activated", changedBy);
    }

    /**
     * Grant a principal access to a client.
     * Used for partners who need access to multiple customer clients.
     *
     * @param principalId Principal ID
     * @param clientId Client ID
     * @return Created grant
     * @throws NotFoundException if principal or client not found
     * @throws BadRequestException if grant already exists or if principal already belongs to client
     */
    public ClientAccessGrant grantClientAccess(String principalId, String clientId) {
        // Validate principal exists
        Principal principal = principalRepo.findByIdOptional(principalId)
            .orElseThrow(() -> new NotFoundException("Principal not found"));

        // Validate client exists
        Client client = clientRepo.findByIdOptional(clientId)
            .orElseThrow(() -> new NotFoundException("Client not found"));

        // Check if principal already belongs to this client as home client
        if (principal.clientId != null && principal.clientId.equals(clientId)) {
            throw new BadRequestException("Principal already belongs to this client as home client");
        }

        // Check if grant already exists
        if (grantRepo.existsByPrincipalIdAndClientId(principalId, clientId)) {
            throw new BadRequestException("Client access grant already exists");
        }

        // Create grant
        ClientAccessGrant grant = new ClientAccessGrant();
        grant.id = TsidGenerator.generate();
        grant.principalId = principalId;
        grant.clientId = clientId;

        grantRepo.persist(grant);
        return grant;
    }

    /**
     * Revoke a principal's access to a client.
     *
     * @param principalId Principal ID
     * @param clientId Client ID
     * @throws NotFoundException if grant not found
     */
    public void revokeClientAccess(String principalId, String clientId) {
        long deleted = grantRepo.deleteByPrincipalIdAndClientId(principalId, clientId);
        if (deleted == 0) {
            throw new NotFoundException("Client access grant not found");
        }
    }

    /**
     * Get all clients accessible by a principal.
     * Uses ClientAccessService to calculate accessible clients.
     *
     * @param principalId Principal ID
     * @return Set of accessible client IDs
     * @throws NotFoundException if principal not found
     */
    public Set<String> getAccessibleClients(String principalId) {
        Principal principal = principalRepo.findByIdOptional(principalId)
            .orElseThrow(() -> new NotFoundException("Principal not found"));

        return clientAccessService.getAccessibleClients(principal);
    }

    /**
     * Get all principals who have access to a specific client.
     *
     * @param clientId Client ID
     * @return List of principals
     */
    public List<Principal> getPrincipalsWithAccess(String clientId) {
        // Get all grants for this client
        List<ClientAccessGrant> grants = grantRepo.findByClientId(clientId);

        // Get principals for those grants
        List<String> principalIds = grants.stream()
            .map(g -> g.principalId)
            .toList();

        // Also include users who have this as their home client
        List<Principal> principals = new java.util.ArrayList<>(principalRepo.findByClientId(clientId));

        // Add granted principals
        if (!principalIds.isEmpty()) {
            List<Principal> grantedPrincipals = principalRepo.findByIds(principalIds);
            principals.addAll(grantedPrincipals);
        }

        return principals;
    }

    /**
     * Find client by ID.
     *
     * @param clientId Client ID
     * @return Optional containing the client if found
     */
    public Optional<Client> findById(String clientId) {
        return clientRepo.findByIdOptional(clientId);
    }

    /**
     * Find client by identifier.
     *
     * @param identifier Client identifier/slug
     * @return Optional containing the client if found
     */
    public Optional<Client> findByIdentifier(String identifier) {
        return clientRepo.findByIdentifier(identifier);
    }

    /**
     * Find all active clients.
     *
     * @return List of active clients
     */
    public List<Client> findAllActive() {
        return clientRepo.findAllActive();
    }

    /**
     * Find all clients (regardless of status).
     *
     * @return List of all clients
     */
    public List<Client> findAll() {
        return clientRepo.listAll();
    }

    /**
     * Add a note to a client's audit trail.
     *
     * @param clientId Client ID
     * @param category Note category (e.g., "SUPPORT", "BILLING")
     * @param text Note text
     * @param addedBy Who added the note
     * @throws NotFoundException if client not found
     */
    public void addNote(String clientId, String category, String text, String addedBy) {
        Client client = clientRepo.findByIdOptional(clientId)
            .orElseThrow(() -> new NotFoundException("Client not found"));

        client.addNote(category, text, addedBy);
        clientRepo.update(client);
    }
}
