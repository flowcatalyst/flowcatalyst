package tech.flowcatalyst.platform.authentication.panache;

import io.quarkus.hibernate.orm.panache.PanacheRepositoryBase;
import jakarta.enterprise.context.ApplicationScoped;
import tech.flowcatalyst.platform.authentication.IdpRoleMapping;
import tech.flowcatalyst.platform.authentication.IdpRoleMappingRepository;
import tech.flowcatalyst.platform.authentication.entity.IdpRoleMappingEntity;
import tech.flowcatalyst.platform.authentication.mapper.IdpRoleMappingMapper;

import java.util.Optional;

/**
 * Panache-based implementation of IdpRoleMappingRepository.
 */
@ApplicationScoped
public class PanacheIdpRoleMappingRepository
    implements IdpRoleMappingRepository, PanacheRepositoryBase<IdpRoleMappingEntity, String> {

    @Override
    public Optional<IdpRoleMapping> findByIdpRoleName(String idpRoleName) {
        return find("idpRoleName", idpRoleName)
            .firstResultOptional()
            .map(IdpRoleMappingMapper::toDomain);
    }

    @Override
    public void persist(IdpRoleMapping mapping) {
        IdpRoleMappingEntity entity = IdpRoleMappingMapper.toEntity(mapping);
        persist(entity);
    }

    @Override
    public void delete(IdpRoleMapping mapping) {
        deleteById(mapping.id);
    }
}
