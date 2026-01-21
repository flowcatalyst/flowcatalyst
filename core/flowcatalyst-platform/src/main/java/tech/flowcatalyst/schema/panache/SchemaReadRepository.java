package tech.flowcatalyst.schema.panache;

import jakarta.enterprise.context.ApplicationScoped;
import jakarta.inject.Inject;
import jakarta.persistence.EntityManager;
import tech.flowcatalyst.eventtype.SchemaType;
import tech.flowcatalyst.schema.Schema;
import tech.flowcatalyst.schema.SchemaRepository;
import tech.flowcatalyst.schema.entity.SchemaEntity;
import tech.flowcatalyst.schema.mapper.SchemaMapper;

import java.util.List;
import java.util.Optional;

/**
 * Read-side repository for Schema entities.
 * Uses EntityManager directly to return domain objects without conflicts.
 */
@ApplicationScoped
public class SchemaReadRepository implements SchemaRepository {

    @Inject
    EntityManager em;

    @Inject
    SchemaWriteRepository writeRepo;

    @Override
    public Schema findById(String id) {
        return findByIdOptional(id).orElse(null);
    }

    @Override
    public Optional<Schema> findByIdOptional(String id) {
        SchemaEntity entity = em.find(SchemaEntity.class, id);
        return Optional.ofNullable(entity).map(SchemaMapper::toDomain);
    }

    @Override
    public Optional<Schema> findByEventTypeAndVersion(String eventTypeId, String version) {
        var results = em.createQuery(
                "FROM SchemaEntity WHERE eventTypeId = :eventTypeId AND version = :version", SchemaEntity.class)
            .setParameter("eventTypeId", eventTypeId)
            .setParameter("version", version)
            .getResultList();
        return results.isEmpty() ? Optional.empty() : Optional.of(SchemaMapper.toDomain(results.get(0)));
    }

    @Override
    public List<Schema> findByEventType(String eventTypeId) {
        return em.createQuery("FROM SchemaEntity WHERE eventTypeId = :eventTypeId", SchemaEntity.class)
            .setParameter("eventTypeId", eventTypeId)
            .getResultList()
            .stream()
            .map(SchemaMapper::toDomain)
            .toList();
    }

    @Override
    public List<Schema> findStandalone() {
        return em.createQuery("FROM SchemaEntity WHERE eventTypeId IS NULL", SchemaEntity.class)
            .getResultList()
            .stream()
            .map(SchemaMapper::toDomain)
            .toList();
    }

    @Override
    public List<Schema> findBySchemaType(SchemaType schemaType) {
        return em.createQuery("FROM SchemaEntity WHERE schemaType = :schemaType", SchemaEntity.class)
            .setParameter("schemaType", schemaType)
            .getResultList()
            .stream()
            .map(SchemaMapper::toDomain)
            .toList();
    }

    @Override
    public List<Schema> listAll() {
        return em.createQuery("FROM SchemaEntity", SchemaEntity.class)
            .getResultList()
            .stream()
            .map(SchemaMapper::toDomain)
            .toList();
    }

    @Override
    public long count() {
        return em.createQuery("SELECT COUNT(e) FROM SchemaEntity e", Long.class)
            .getSingleResult();
    }

    @Override
    public boolean existsByEventTypeAndVersion(String eventTypeId, String version) {
        Long count = em.createQuery(
                "SELECT COUNT(e) FROM SchemaEntity e WHERE e.eventTypeId = :eventTypeId AND e.version = :version",
                Long.class)
            .setParameter("eventTypeId", eventTypeId)
            .setParameter("version", version)
            .getSingleResult();
        return count > 0;
    }

    // Write operations delegate to WriteRepository
    @Override
    public void persist(Schema schema) {
        writeRepo.persistSchema(schema);
    }

    @Override
    public void update(Schema schema) {
        writeRepo.updateSchema(schema);
    }

    @Override
    public void delete(Schema schema) {
        writeRepo.deleteSchemaById(schema.id());
    }

    @Override
    public boolean deleteById(String id) {
        return writeRepo.deleteSchemaById(id);
    }
}
