package tech.flowcatalyst.platform.principal.panache;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.persistence.EntityManager;
import tech.flowcatalyst.platform.principal.Principal;
import tech.flowcatalyst.platform.principal.PrincipalRepository;
import tech.flowcatalyst.platform.principal.PrincipalType;
import tech.flowcatalyst.platform.principal.entity.PrincipalEntity;
import tech.flowcatalyst.platform.principal.entity.PrincipalManagedApplicationEntity;
import tech.flowcatalyst.platform.principal.entity.PrincipalRoleEntity;
import tech.flowcatalyst.platform.principal.mapper.PrincipalMapper;

import java.util.ArrayList;
import java.util.Collection;
import java.util.List;
import java.util.Optional;

/**
 * Read-side repository for Principal entities.
 * Uses EntityManager directly to return domain objects without conflicts.
 */
@ApplicationScoped
public class PrincipalReadRepository implements PrincipalRepository {

    @Inject
    EntityManager em;

    @Inject
    PrincipalWriteRepository writeRepo;

    @Override
    public Principal findById(String id) {
        return findByIdOptional(id).orElse(null);
    }

    @Override
    public Optional<Principal> findByIdOptional(String id) {
        PrincipalEntity entity = em.find(PrincipalEntity.class, id);
        if (entity == null) {
            return Optional.empty();
        }
        Principal p = PrincipalMapper.toDomain(entity);
        p.roles = loadRoles(id);
        p.managedApplicationIds = loadManagedApplicationIds(id);
        return Optional.of(p);
    }

    @Override
    public Optional<Principal> findByEmail(String email) {
        var results = em.createQuery(
                "FROM PrincipalEntity WHERE email = :email", PrincipalEntity.class)
            .setParameter("email", email)
            .getResultList();

        if (results.isEmpty()) {
            return Optional.empty();
        }
        Principal p = PrincipalMapper.toDomain(results.get(0));
        p.roles = loadRoles(p.id);
        p.managedApplicationIds = loadManagedApplicationIds(p.id);
        return Optional.of(p);
    }

    @Override
    public Optional<Principal> findByServiceAccountCode(String code) {
        @SuppressWarnings("unchecked")
        List<PrincipalEntity> results = em.createNativeQuery(
                "SELECT * FROM principals WHERE service_account->>'code' = :code",
                PrincipalEntity.class)
            .setParameter("code", code)
            .getResultList();

        if (results.isEmpty()) {
            return Optional.empty();
        }
        Principal p = PrincipalMapper.toDomain(results.get(0));
        p.roles = loadRoles(p.id);
        p.managedApplicationIds = loadManagedApplicationIds(p.id);
        return Optional.of(p);
    }

    @Override
    public Optional<Principal> findByServiceAccountId(String serviceAccountId) {
        var results = em.createQuery(
                "FROM PrincipalEntity WHERE serviceAccountId = :serviceAccountId",
                PrincipalEntity.class)
            .setParameter("serviceAccountId", serviceAccountId)
            .getResultList();

        if (results.isEmpty()) {
            return Optional.empty();
        }
        Principal p = PrincipalMapper.toDomain(results.get(0));
        p.roles = loadRoles(p.id);
        p.managedApplicationIds = loadManagedApplicationIds(p.id);
        return Optional.of(p);
    }

    @Override
    public List<Principal> findByType(PrincipalType type) {
        return em.createQuery("FROM PrincipalEntity WHERE type = :type", PrincipalEntity.class)
            .setParameter("type", type)
            .getResultList()
            .stream()
            .map(this::toDomainWithRoles)
            .toList();
    }

    @Override
    public List<Principal> findByClientId(String clientId) {
        return em.createQuery("FROM PrincipalEntity WHERE clientId = :clientId", PrincipalEntity.class)
            .setParameter("clientId", clientId)
            .getResultList()
            .stream()
            .map(this::toDomainWithRoles)
            .toList();
    }

    @Override
    public List<Principal> findByIds(Collection<String> ids) {
        if (ids == null || ids.isEmpty()) {
            return new ArrayList<>();
        }
        return em.createQuery("FROM PrincipalEntity WHERE id IN :ids", PrincipalEntity.class)
            .setParameter("ids", ids)
            .getResultList()
            .stream()
            .map(this::toDomainWithRoles)
            .toList();
    }

    @Override
    public List<Principal> findUsersByClientId(String clientId) {
        return em.createQuery(
                "FROM PrincipalEntity WHERE clientId = :clientId AND type = :type",
                PrincipalEntity.class)
            .setParameter("clientId", clientId)
            .setParameter("type", PrincipalType.USER)
            .getResultList()
            .stream()
            .map(this::toDomainWithRoles)
            .toList();
    }

    @Override
    public List<Principal> findActiveUsersByClientId(String clientId) {
        return em.createQuery(
                "FROM PrincipalEntity WHERE clientId = :clientId AND type = :type AND active = true",
                PrincipalEntity.class)
            .setParameter("clientId", clientId)
            .setParameter("type", PrincipalType.USER)
            .getResultList()
            .stream()
            .map(this::toDomainWithRoles)
            .toList();
    }

    @Override
    public List<Principal> findByClientIdAndTypeAndActive(String clientId, PrincipalType type, Boolean active) {
        return em.createQuery(
                "FROM PrincipalEntity WHERE clientId = :clientId AND type = :type AND active = :active",
                PrincipalEntity.class)
            .setParameter("clientId", clientId)
            .setParameter("type", type)
            .setParameter("active", active)
            .getResultList()
            .stream()
            .map(this::toDomainWithRoles)
            .toList();
    }

    @Override
    public List<Principal> findByClientIdAndType(String clientId, PrincipalType type) {
        return em.createQuery(
                "FROM PrincipalEntity WHERE clientId = :clientId AND type = :type",
                PrincipalEntity.class)
            .setParameter("clientId", clientId)
            .setParameter("type", type)
            .getResultList()
            .stream()
            .map(this::toDomainWithRoles)
            .toList();
    }

    @Override
    public List<Principal> findByClientIdAndActive(String clientId, Boolean active) {
        return em.createQuery(
                "FROM PrincipalEntity WHERE clientId = :clientId AND active = :active",
                PrincipalEntity.class)
            .setParameter("clientId", clientId)
            .setParameter("active", active)
            .getResultList()
            .stream()
            .map(this::toDomainWithRoles)
            .toList();
    }

    @Override
    public List<Principal> findByActive(Boolean active) {
        return em.createQuery("FROM PrincipalEntity WHERE active = :active", PrincipalEntity.class)
            .setParameter("active", active)
            .getResultList()
            .stream()
            .map(this::toDomainWithRoles)
            .toList();
    }

    @Override
    public List<Principal> listAll() {
        return em.createQuery("FROM PrincipalEntity", PrincipalEntity.class)
            .getResultList()
            .stream()
            .map(this::toDomainWithRoles)
            .toList();
    }

    @Override
    public Optional<Principal> findByServiceAccountClientId(String clientId) {
        @SuppressWarnings("unchecked")
        List<PrincipalEntity> results = em.createNativeQuery(
                "SELECT * FROM principals WHERE type = 'SERVICE' AND service_account->>'clientId' = :clientId",
                PrincipalEntity.class)
            .setParameter("clientId", clientId)
            .getResultList();

        if (results.isEmpty()) {
            return Optional.empty();
        }
        Principal p = PrincipalMapper.toDomain(results.get(0));
        p.roles = loadRoles(p.id);
        p.managedApplicationIds = loadManagedApplicationIds(p.id);
        return Optional.of(p);
    }

    @Override
    public long countByEmailDomain(String domain) {
        return em.createQuery("SELECT COUNT(e) FROM PrincipalEntity e WHERE e.emailDomain = :domain", Long.class)
            .setParameter("domain", domain)
            .getSingleResult();
    }

    // Write operations delegate to WriteRepository
    @Override
    public void persist(Principal principal) {
        writeRepo.persistPrincipal(principal);
    }

    @Override
    public void update(Principal principal) {
        writeRepo.updatePrincipal(principal);
    }

    @Override
    public void updateOnly(Principal principal) {
        writeRepo.updatePrincipalOnly(principal);
    }

    @Override
    public boolean deleteById(String id) {
        return writeRepo.deletePrincipal(id);
    }

    // ========================================================================
    // Helper Methods
    // ========================================================================

    private Principal toDomainWithRoles(PrincipalEntity entity) {
        Principal p = PrincipalMapper.toDomain(entity);
        p.roles = loadRoles(entity.id);
        p.managedApplicationIds = loadManagedApplicationIds(entity.id);
        return p;
    }

    private List<Principal.RoleAssignment> loadRoles(String principalId) {
        List<PrincipalRoleEntity> roleEntities = em.createQuery(
                "SELECT r FROM PrincipalRoleEntity r WHERE r.principalId = :id",
                PrincipalRoleEntity.class)
            .setParameter("id", principalId)
            .getResultList();

        return PrincipalMapper.toRoleAssignments(roleEntities);
    }

    private List<String> loadManagedApplicationIds(String principalId) {
        List<PrincipalManagedApplicationEntity> entities = em.createQuery(
                "SELECT m FROM PrincipalManagedApplicationEntity m WHERE m.principalId = :id",
                PrincipalManagedApplicationEntity.class)
            .setParameter("id", principalId)
            .getResultList();

        return PrincipalMapper.toManagedApplicationIds(entities);
    }
}
