package tech.flowcatalyst.serviceaccount.jpaentity;

import jakarta.persistence.*;

/**
 * JPA Entity for service_account_client_ids table (normalized from clientIds array).
 */
@Entity
@Table(name = "service_account_client_ids")
public class ServiceAccountClientIdEntity {

    @Id
    @GeneratedValue(strategy = GenerationType.IDENTITY)
    @Column(name = "id")
    public Long id;

    @Column(name = "service_account_id", nullable = false, length = 17)
    public String serviceAccountId;

    @Column(name = "client_id", nullable = false, length = 17)
    public String clientId;

    public ServiceAccountClientIdEntity() {
    }

    public ServiceAccountClientIdEntity(String serviceAccountId, String clientId) {
        this.serviceAccountId = serviceAccountId;
        this.clientId = clientId;
    }
}
