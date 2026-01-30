package tech.flowcatalyst.platform.common;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.application.Application;
import tech.flowcatalyst.platform.application.ApplicationRepository;
import tech.flowcatalyst.platform.authorization.PermissionRegistry;
import tech.flowcatalyst.platform.client.ClientAccessService;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.platform.principal.PrincipalRepository;
import tech.flowcatalyst.platform.principal.UserScope;

import java.util.Collections;
import java.util.HashSet;
import java.util.List;
import java.util.Set;
import java.util.stream.Collectors;

/**
 * Factory for building AuthorizationContext from principal information.
 *
 * <p>This service loads and resolves all authorization information needed
 * for use case execution, including:
 * <ul>
 *   <li>Principal roles and permissions</li>
 *   <li>Application access (explicitly granted applications)</li>
 *   <li>Client access scope</li>
 * </ul>
 */
@ApplicationScoped
public class AuthorizationContextFactory {

    @Inject
    PrincipalRepository principalRepository;

    @Inject
    ApplicationRepository applicationRepository;

    @Inject
    ClientAccessService clientAccessService;

    @Inject
    PermissionRegistry permissionRegistry;

    /**
     * Build an AuthorizationContext for a principal.
     *
     * @param principalId the principal ID to build context for
     * @return the authorization context
     * @throws IllegalArgumentException if principal not found
     */
    public AuthorizationContext build(String principalId) {
        Principal principal = principalRepository.findById(principalId);
        if (principal == null) {
            throw new IllegalArgumentException("Principal not found: " + principalId);
        }
        return build(principal);
    }

    /**
     * Build an AuthorizationContext from a Principal object.
     *
     * <p>Use this overload when you already have the principal loaded
     * to avoid an extra database lookup.
     *
     * @param principal the principal to build context for
     * @return the authorization context
     */
    public AuthorizationContext build(Principal principal) {
        // Get roles
        Set<String> roles = principal.getRoleNames();

        // Resolve permissions from roles
        Set<String> permissions = resolvePermissions(roles);

        // Get accessible application IDs and codes
        Set<String> accessibleAppIds = new HashSet<>(
            principal.accessibleApplicationIds != null ? principal.accessibleApplicationIds : Collections.emptyList()
        );
        Set<String> accessibleAppCodes = resolveApplicationCodes(accessibleAppIds);

        // Get client access
        Set<String> accessibleClientIds = clientAccessService.getAccessibleClients(principal);
        boolean canAccessAllClients = principal.scope == UserScope.ANCHOR ||
            clientAccessService.isAnchorDomainUser(principal);

        return new AuthorizationContext(
            principal.id,
            principal.type,
            roles,
            permissions,
            accessibleAppIds,
            accessibleAppCodes,
            accessibleClientIds,
            canAccessAllClients
        );
    }

    /**
     * Resolve permissions from a set of roles.
     *
     * @param roles the role names
     * @return set of permission names derived from the roles
     */
    private Set<String> resolvePermissions(Set<String> roles) {
        if (roles == null || roles.isEmpty()) {
            return Collections.emptySet();
        }

        Set<String> permissions = new HashSet<>();
        for (String roleName : roles) {
            permissionRegistry.getRole(roleName)
                .ifPresent(role -> permissions.addAll(role.permissionStrings()));
        }
        return permissions;
    }

    /**
     * Resolve application codes from a set of application IDs.
     *
     * @param applicationIds the application IDs
     * @return set of application codes
     */
    private Set<String> resolveApplicationCodes(Set<String> applicationIds) {
        if (applicationIds == null || applicationIds.isEmpty()) {
            return Collections.emptySet();
        }

        List<Application> applications = applicationRepository.findByIds(applicationIds);
        return applications.stream()
            .map(app -> app.code)
            .collect(Collectors.toSet());
    }
}
