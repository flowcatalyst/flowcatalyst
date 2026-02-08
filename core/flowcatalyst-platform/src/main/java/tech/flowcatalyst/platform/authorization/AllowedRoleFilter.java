package tech.flowcatalyst.platform.authorization;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import tech.flowcatalyst.platform.authentication.domain.EmailDomainMapping;
import tech.flowcatalyst.platform.authentication.domain.EmailDomainMappingRepository;
import tech.flowcatalyst.platform.authentication.domain.ScopeType;

import java.util.Optional;
import java.util.Set;
import java.util.stream.Collectors;

/**
 * Bridges between role IDs (stored in EmailDomainMapping.allowedRoleIds)
 * and role names (used in role assignments).
 *
 * <p>This service resolves role IDs to names at the enforcement point,
 * keeping IDs as the source of truth in the database.
 */
@ApplicationScoped
public class AllowedRoleFilter {

    @Inject
    EmailDomainMappingRepository emailDomainMappingRepo;

    @Inject
    AuthRoleRepository authRoleRepo;

    /**
     * Get the set of allowed role names for an email domain.
     *
     * @param emailDomain the email domain to check
     * @return Optional.empty() if no mapping found or no restrictions apply (ANCHOR scope or empty allowedRoleIds);
     *         Optional.of(nameSet) if restrictions apply
     */
    public Optional<Set<String>> getAllowedRoleNames(String emailDomain) {
        if (emailDomain == null || emailDomain.isBlank()) {
            return Optional.empty();
        }

        var mappingOpt = emailDomainMappingRepo.findByEmailDomain(emailDomain.toLowerCase());
        if (mappingOpt.isEmpty()) {
            return Optional.empty();
        }

        var mapping = mappingOpt.get();

        // ANCHOR scope has no role restrictions
        if (mapping.scopeType == ScopeType.ANCHOR) {
            return Optional.empty();
        }

        // No restrictions if allowedRoleIds is empty
        if (!mapping.hasRoleRestrictions()) {
            return Optional.empty();
        }

        // Resolve role IDs to names
        var roles = authRoleRepo.findByIds(mapping.allowedRoleIds);
        var allowedNames = roles.stream()
            .map(r -> r.name)
            .collect(Collectors.toSet());

        return Optional.of(allowedNames);
    }

    /**
     * Filter a set of role names to only include those allowed for the given email domain.
     *
     * @param requestedRoleNames the role names to filter
     * @param emailDomain the email domain to check restrictions for
     * @return the filtered set of allowed role names (returns all if no restrictions apply)
     */
    public Set<String> filterAllowedRoles(Set<String> requestedRoleNames, String emailDomain) {
        var allowedOpt = getAllowedRoleNames(emailDomain);
        if (allowedOpt.isEmpty()) {
            return requestedRoleNames;
        }
        var allowed = allowedOpt.get();
        return requestedRoleNames.stream()
            .filter(allowed::contains)
            .collect(Collectors.toSet());
    }
}
