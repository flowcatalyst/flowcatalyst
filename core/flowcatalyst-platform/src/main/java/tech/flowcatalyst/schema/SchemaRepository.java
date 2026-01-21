package tech.flowcatalyst.schema;

import tech.flowcatalyst.eventtype.SchemaType;

import java.util.List;
import java.util.Optional;

/**
 * Repository interface for Schema entities.
 * Exposes only approved data access methods - Panache internals are hidden.
 * Schemas can be standalone or linked to EventTypes.
 */
public interface SchemaRepository {

    // Read operations
    Schema findById(String id);
    Optional<Schema> findByIdOptional(String id);
    Optional<Schema> findByEventTypeAndVersion(String eventTypeId, String version);
    List<Schema> findByEventType(String eventTypeId);
    List<Schema> findStandalone();
    List<Schema> findBySchemaType(SchemaType schemaType);
    List<Schema> listAll();
    long count();
    boolean existsByEventTypeAndVersion(String eventTypeId, String version);

    // Write operations
    void persist(Schema schema);
    void update(Schema schema);
    void delete(Schema schema);
    boolean deleteById(String id);
}
