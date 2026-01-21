package tech.flowcatalyst.platform.client.entity;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.Id;
import jakarta.persistence.Table;
import jakarta.persistence.UniqueConstraint;
import java.time.Instant;

/**
 * JPA entity for client_access_grants table.
 */
@Entity
@Table(name = "client_access_grants",
    uniqueConstraints = @UniqueConstraint(columnNames = {"principal_id", "client_id"}))
public class ClientAccessGrantEntity {

    @Id
    @Column(name = "id", length = 17)
    public String id;

    @Column(name = "principal_id", nullable = false, length = 17)
    public String principalId;

    @Column(name = "client_id", nullable = false, length = 17)
    public String clientId;

    @Column(name = "granted_at", nullable = false)
    public Instant grantedAt;

    @Column(name = "expires_at")
    public Instant expiresAt;

    public ClientAccessGrantEntity() {
    }
}
