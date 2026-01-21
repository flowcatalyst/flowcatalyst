package tech.flowcatalyst.platform.authentication.oidc.panache;

import io.quarkus.hibernate.orm.panache.PanacheRepositoryBase;
import jakarta.enterprise.context.ApplicationScoped;
import tech.flowcatalyst.platform.authentication.oidc.OidcLoginState;
import tech.flowcatalyst.platform.authentication.oidc.OidcLoginStateRepository;
import tech.flowcatalyst.platform.authentication.oidc.entity.OidcLoginStateEntity;
import tech.flowcatalyst.platform.authentication.oidc.mapper.OidcLoginStateMapper;

import java.time.Instant;
import java.util.Optional;

/**
 * Panache-based implementation of OidcLoginStateRepository.
 */
@ApplicationScoped
public class PanacheOidcLoginStateRepository
    implements OidcLoginStateRepository, PanacheRepositoryBase<OidcLoginStateEntity, String> {

    @Override
    public Optional<OidcLoginState> findValidState(String state) {
        return find("state = ?1 and expiresAt > ?2", state, Instant.now())
            .firstResultOptional()
            .map(OidcLoginStateMapper::toDomain);
    }

    @Override
    public void persist(OidcLoginState loginState) {
        if (loginState.createdAt == null) {
            loginState.createdAt = Instant.now();
        }
        if (loginState.expiresAt == null) {
            loginState.expiresAt = Instant.now().plusSeconds(600);
        }
        OidcLoginStateEntity entity = OidcLoginStateMapper.toEntity(loginState);
        persist(entity);
    }

    @Override
    public void deleteByState(String state) {
        delete("state", state);
    }
}
