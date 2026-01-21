package tech.flowcatalyst.platform.principal;

import tech.flowcatalyst.platform.authentication.IdpType;
import java.time.Instant;

/**
 * User identity information (embedded in Principal for USER type).
 */
public class UserIdentity {

    public String email;

    public String emailDomain;

    public IdpType idpType;

    public String externalIdpId; // Subject from OIDC token

    public String passwordHash; // For INTERNAL auth only (Argon2id)

    public Instant lastLoginAt;

    public UserIdentity() {
    }
}
