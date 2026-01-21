package tech.flowcatalyst.platform.authentication.oauth.panache;

import io.quarkus.hibernate.orm.panache.PanacheRepositoryBase;
import jakarta.enterprise.context.ApplicationScoped;
import tech.flowcatalyst.platform.authentication.oauth.AuthorizationCode;
import tech.flowcatalyst.platform.authentication.oauth.AuthorizationCodeRepository;
import tech.flowcatalyst.platform.authentication.oauth.entity.AuthorizationCodeEntity;
import tech.flowcatalyst.platform.authentication.oauth.mapper.AuthorizationCodeMapper;
import tech.flowcatalyst.platform.shared.TsidGenerator;

import java.time.Instant;
import java.util.Optional;

/**
 * Panache-based implementation of AuthorizationCodeRepository.
 */
@ApplicationScoped
public class PanacheAuthorizationCodeRepository
    implements AuthorizationCodeRepository, PanacheRepositoryBase<AuthorizationCodeEntity, String> {

    @Override
    public Optional<AuthorizationCode> findValidCode(String code) {
        return find("code = ?1 and used = false and expiresAt > ?2", code, Instant.now())
            .firstResultOptional()
            .map(AuthorizationCodeMapper::toDomain);
    }

    @Override
    public void persist(AuthorizationCode authCode) {
        if (authCode.id == null) {
            authCode.id = TsidGenerator.generate();
        }
        if (authCode.createdAt == null) {
            authCode.createdAt = Instant.now();
        }
        AuthorizationCodeEntity entity = AuthorizationCodeMapper.toEntity(authCode);
        persist(entity);
    }

    @Override
    public void markAsUsed(String code) {
        update("used = true where code = ?1", code);
    }
}
