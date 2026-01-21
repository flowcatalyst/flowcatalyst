package tech.flowcatalyst.platform.application.panache;

import io.quarkus.hibernate.orm.panache.PanacheRepositoryBase;
import jakarta.enterprise.context.ApplicationScoped;
import tech.flowcatalyst.platform.application.Application;
import tech.flowcatalyst.platform.application.entity.ApplicationEntity;
import tech.flowcatalyst.platform.application.mapper.ApplicationMapper;

import java.time.Instant;

/**
 * Write-side repository for Application entities.
 * Extends PanacheRepositoryBase for efficient entity persistence.
 */
@ApplicationScoped
public class ApplicationWriteRepository implements PanacheRepositoryBase<ApplicationEntity, String> {

    /**
     * Persist a new application.
     */
    public void persistApplication(Application application) {
        if (application.createdAt == null) {
            application.createdAt = Instant.now();
        }
        application.updatedAt = Instant.now();
        ApplicationEntity entity = ApplicationMapper.toEntity(application);
        persist(entity);
    }

    /**
     * Update an existing application.
     */
    public void updateApplication(Application application) {
        application.updatedAt = Instant.now();
        ApplicationEntity entity = findById(application.id);
        if (entity != null) {
            ApplicationMapper.updateEntity(entity, application);
        }
    }

    /**
     * Delete an application by ID.
     */
    public boolean deleteApplicationById(String id) {
        return deleteById(id);
    }
}
