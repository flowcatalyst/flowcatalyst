package tech.flowcatalyst.platform.client.panache;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.persistence.EntityManager;
import tech.flowcatalyst.platform.client.Client;
import tech.flowcatalyst.platform.client.ClientRepository;
import tech.flowcatalyst.platform.client.ClientStatus;
import tech.flowcatalyst.platform.client.entity.ClientEntity;
import tech.flowcatalyst.platform.client.mapper.ClientMapper;

import java.util.List;
import java.util.Optional;
import java.util.Set;

/**
 * Read-side repository for Client entities.
 * Uses EntityManager directly to return domain objects without conflicts.
 */
@ApplicationScoped
public class ClientReadRepository implements ClientRepository {

    @Inject
    EntityManager em;

    @Inject
    ClientWriteRepository writeRepo;

    @Override
    public Client findById(String id) {
        return findByIdOptional(id).orElse(null);
    }

    @Override
    public Optional<Client> findByIdOptional(String id) {
        ClientEntity entity = em.find(ClientEntity.class, id);
        return Optional.ofNullable(entity).map(ClientMapper::toDomain);
    }

    @Override
    public Optional<Client> findByIdentifier(String identifier) {
        var results = em.createQuery(
                "FROM ClientEntity WHERE identifier = :identifier", ClientEntity.class)
            .setParameter("identifier", identifier)
            .getResultList();
        return results.isEmpty() ? Optional.empty() : Optional.of(ClientMapper.toDomain(results.get(0)));
    }

    @Override
    public List<Client> findAllActive() {
        return em.createQuery("FROM ClientEntity WHERE status = :status", ClientEntity.class)
            .setParameter("status", ClientStatus.ACTIVE)
            .getResultList()
            .stream()
            .map(ClientMapper::toDomain)
            .toList();
    }

    @Override
    public List<Client> findByIds(Set<String> ids) {
        if (ids == null || ids.isEmpty()) {
            return List.of();
        }
        return em.createQuery("FROM ClientEntity WHERE id IN :ids", ClientEntity.class)
            .setParameter("ids", ids)
            .getResultList()
            .stream()
            .map(ClientMapper::toDomain)
            .toList();
    }

    @Override
    public List<Client> listAll() {
        return em.createQuery("FROM ClientEntity", ClientEntity.class)
            .getResultList()
            .stream()
            .map(ClientMapper::toDomain)
            .toList();
    }

    // Write operations delegate to WriteRepository
    @Override
    public void persist(Client client) {
        writeRepo.persistClient(client);
    }

    @Override
    public void update(Client client) {
        writeRepo.updateClient(client);
    }

    @Override
    public void delete(Client client) {
        writeRepo.deleteClient(client.id);
    }
}
