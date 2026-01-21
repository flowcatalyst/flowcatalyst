package tech.flowcatalyst.platform.client.entity;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.EnumType;
import jakarta.persistence.Enumerated;
import jakarta.persistence.Id;
import jakarta.persistence.Table;
import org.hibernate.annotations.JdbcTypeCode;
import org.hibernate.type.SqlTypes;
import tech.flowcatalyst.platform.client.ClientStatus;
import java.time.Instant;

/**
 * JPA entity for clients table.
 */
@Entity
@Table(name = "clients")
public class ClientEntity {

    @Id
    @Column(name = "id", length = 17)
    public String id;

    @Column(name = "name", nullable = false)
    public String name;

    @Column(name = "identifier", nullable = false, unique = true, length = 100)
    public String identifier;

    @Column(name = "status", nullable = false, length = 50)
    @Enumerated(EnumType.STRING)
    public ClientStatus status;

    @Column(name = "status_reason")
    public String statusReason;

    @Column(name = "status_changed_at")
    public Instant statusChangedAt;

    @JdbcTypeCode(SqlTypes.JSON)
    @Column(name = "notes", columnDefinition = "jsonb")
    public String notes;

    @Column(name = "created_at", nullable = false)
    public Instant createdAt;

    @Column(name = "updated_at", nullable = false)
    public Instant updatedAt;

    public ClientEntity() {
    }
}
