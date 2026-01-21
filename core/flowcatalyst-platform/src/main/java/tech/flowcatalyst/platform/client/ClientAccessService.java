package tech.flowcatalyst.platform.client;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.platform.principal.PrincipalType;
import tech.flowcatalyst.platform.principal.UserScope;
import tech.flowcatalyst.platform.principal.AnchorDomainRepository;

import java.time.Instant;
import java.util.HashSet;
import java.util.List;
import java.util.Optional;
import java.util.Set;
import java.util.stream.Collectors;

/**
 * Service for calculating which clients a principal can access.
 *
 * Access rules based on UserScope:
 * 1. ANCHOR scope -> ALL clients (global access)
 * 2. CLIENT scope -> Home client + IDP additional clients + explicit grants
 * 3. PARTNER scope -> IDP granted clients + explicit grants
 *
 * Legacy fallback: Check anchor domain table for backwards compatibility.
 */
@ApplicationScoped
public class ClientAccessService {

    @Inject
    ClientRepository clientRepo;

    @Inject
    AnchorDomainRepository anchorDomainRepo;

    @Inject
    ClientAccessGrantRepository grantRepo;

    @Inject
    ClientAuthConfigService authConfigService;

    /**
     * Calculate which clients a principal can access.
     *
     * @param principal The principal (user or service account)
     * @return Set of accessible client IDs
     */
    public Set<String> getAccessibleClients(Principal principal) {
        Set<String> clientIds = new HashSet<>();

        // 1. Check explicit scope first (new model)
        if (principal.scope != null) {
            switch (principal.scope) {
                case ANCHOR -> {
                    // ANCHOR scope: ALL active clients
                    return clientRepo.findAllActive().stream()
                        .map(c -> c.id)
                        .collect(Collectors.toSet());
                }
                case CLIENT -> {
                    // CLIENT scope: home client + IDP additional clients + explicit grants
                    if (principal.clientId != null) {
                        clientIds.add(principal.clientId);
                    }
                    // Add additional clients from IDP config
                    addIdpAccessibleClients(principal, clientIds);
                }
                case PARTNER -> {
                    // PARTNER scope: IDP granted clients + explicit grants
                    // No home client for partners
                    addIdpAccessibleClients(principal, clientIds);
                }
            }
        } else {
            // Legacy fallback: Check if anchor domain user (global access)
            if (principal.type == PrincipalType.USER && principal.userIdentity != null) {
                String domain = principal.userIdentity.emailDomain;
                if (domain != null && anchorDomainRepo.existsByDomain(domain)) {
                    // Return ALL active client IDs
                    return clientRepo.findAllActive().stream()
                        .map(c -> c.id)
                        .collect(Collectors.toSet());
                }
            }

            // Add home client if exists (legacy behavior)
            if (principal.clientId != null) {
                clientIds.add(principal.clientId);
            }
        }

        // Add explicitly granted clients (filter expired grants)
        // This applies to all scopes for per-user exceptions
        List<ClientAccessGrant> grants = grantRepo.findByPrincipalId(principal.id);
        Instant now = Instant.now();

        grants.stream()
            .filter(g -> g.expiresAt == null || g.expiresAt.isAfter(now))
            .map(g -> g.clientId)
            .forEach(clientIds::add);

        // Filter out inactive/suspended clients
        if (!clientIds.isEmpty()) {
            List<Client> clients = clientRepo.findByIds(clientIds);
            return clients.stream()
                .filter(c -> c.status == ClientStatus.ACTIVE)
                .map(c -> c.id)
                .collect(Collectors.toSet());
        }

        return clientIds;
    }

    /**
     * Add accessible clients from the user's IDP configuration.
     * For CLIENT type: adds additional clients
     * For PARTNER type: adds granted clients
     */
    private void addIdpAccessibleClients(Principal principal, Set<String> clientIds) {
        if (principal.type != PrincipalType.USER || principal.userIdentity == null) {
            return;
        }

        String domain = principal.userIdentity.emailDomain;
        if (domain == null) {
            return;
        }

        Optional<ClientAuthConfig> configOpt = authConfigService.findByEmailDomain(domain);
        if (configOpt.isEmpty()) {
            return;
        }

        ClientAuthConfig config = configOpt.get();
        // getAllAccessibleClientIds returns the appropriate list based on config type
        clientIds.addAll(config.getAllAccessibleClientIds());
    }

    /**
     * Check if principal can access a specific client.
     *
     * @param principal The principal
     * @param clientId The client ID to check
     * @return true if principal can access the client
     */
    public boolean canAccessClient(Principal principal, String clientId) {
        return getAccessibleClients(principal).contains(clientId);
    }

    /**
     * Check if principal has global access (ANCHOR scope or legacy anchor domain).
     *
     * @param principal The principal
     * @return true if anchor scope/domain user
     */
    public boolean isAnchorDomainUser(Principal principal) {
        // Check new scope first
        if (principal.scope == UserScope.ANCHOR) {
            return true;
        }

        // Legacy fallback: check anchor domain table
        if (principal.type != PrincipalType.USER || principal.userIdentity == null) {
            return false;
        }

        String domain = principal.userIdentity.emailDomain;
        return domain != null && anchorDomainRepo.existsByDomain(domain);
    }
}
