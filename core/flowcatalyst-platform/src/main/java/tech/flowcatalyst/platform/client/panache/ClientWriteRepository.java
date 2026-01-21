package tech.flowcatalyst.platform.client.panache;

import io.quarkus.hibernate.orm.panache.PanacheRepositoryBase;
import jakarta.enterprise.context.ApplicationScoped;
import tech.flowcatalyst.platform.client.Client;
import tech.flowcatalyst.platform.client.entity.ClientEntity;
import tech.flowcatalyst.platform.client.mapper.ClientMapper;

import java.time.Instant;

/**
 * Write-side repository for Client entities.
 * Extends PanacheRepositoryBase for efficient entity persistence.
 */
@ApplicationScoped
public class ClientWriteRepository implements PanacheRepositoryBase<ClientEntity, String> {

    /**
     * Persist a new client.
     */
    public void persistClient(Client client) {
        if (client.createdAt == null) {
            client.createdAt = Instant.now();
        }
        client.updatedAt = Instant.now();
        ClientEntity entity = ClientMapper.toEntity(client);
        persist(entity);
    }

    /**
     * Update an existing client.
     */
    public void updateClient(Client client) {
        client.updatedAt = Instant.now();
        ClientEntity entity = findById(client.id);
        if (entity != null) {
            ClientMapper.updateEntity(entity, client);
        }
    }

    /**
     * Delete a client by ID.
     */
    public boolean deleteClient(String id) {
        return deleteById(id);
    }
}
