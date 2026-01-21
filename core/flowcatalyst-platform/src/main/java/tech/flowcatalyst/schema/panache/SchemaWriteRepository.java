package tech.flowcatalyst.schema.panache;

import io.quarkus.hibernate.orm.panache.PanacheRepositoryBase;
import jakarta.enterprise.context.ApplicationScoped;
import tech.flowcatalyst.schema.Schema;
import tech.flowcatalyst.schema.entity.SchemaEntity;
import tech.flowcatalyst.schema.mapper.SchemaMapper;

import java.time.Instant;

/**
 * Write-side repository for Schema entities.
 * Extends PanacheRepositoryBase for efficient entity persistence.
 */
@ApplicationScoped
public class SchemaWriteRepository implements PanacheRepositoryBase<SchemaEntity, String> {

    /**
     * Persist a new schema.
     */
    public void persistSchema(Schema schema) {
        Schema toSave = schema;
        if (toSave.createdAt() == null) {
            toSave = toSave.toBuilder().createdAt(Instant.now()).build();
        }
        toSave = toSave.toBuilder().updatedAt(Instant.now()).build();
        SchemaEntity entity = SchemaMapper.toEntity(toSave);
        persist(entity);
    }

    /**
     * Update an existing schema.
     */
    public void updateSchema(Schema schema) {
        Schema toUpdate = schema.toBuilder().updatedAt(Instant.now()).build();
        SchemaEntity entity = findById(schema.id());
        if (entity != null) {
            SchemaMapper.updateEntity(entity, toUpdate);
        }
    }

    /**
     * Delete a schema by ID.
     */
    public boolean deleteSchemaById(String id) {
        return deleteById(id);
    }
}
