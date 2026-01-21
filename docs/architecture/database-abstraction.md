# Database Abstraction Architecture (Planned)

> **Status:** Planned / Not Implemented
> **Last Updated:** January 2025

This document describes a planned architecture for supporting multiple database backends (MongoDB, PostgreSQL, MySQL) for the FlowCatalyst control plane. Implementation is deferred due to the maintenance burden of full normalization.

---

## Motivation

FlowCatalyst currently requires MongoDB for all deployments. Some use cases would benefit from SQL database support:

- **Simpler deployments** - Organizations with existing PostgreSQL/MySQL infrastructure
- **Reduced operational burden** - No need to learn MongoDB for control-plane-only deployments
- **Familiar tooling** - pgAdmin, MySQL Workbench vs MongoDB Compass
- **Managed database options** - RDS, Cloud SQL, Azure Database without MongoDB Atlas

---

## Scope

### In Scope: Control Plane

Repositories managing identity, access, and configuration:

| Repository | Entity | Complexity |
|------------|--------|------------|
| PrincipalRepository | Users & Service Accounts | Medium |
| RoleRepository | Authorization roles | Low |
| ClientRepository | Tenant organizations | Low |
| ApplicationRepository | Applications | Low |
| ServiceAccountRepository | Machine credentials | Medium |
| OAuthClientRepository | OAuth clients | Low |
| RefreshTokenRepository | Token management | Low |
| AuthorizationCodeRepository | OAuth codes | Low |
| OidcLoginStateRepository | OIDC state | Low |
| AnchorDomainRepository | Platform domains | Low |
| ClientAuthConfigRepository | Client auth config | Low |
| ClientAccessGrantRepository | Access grants | Low |
| IdpRoleMappingRepository | IDP mappings | Low |
| AuditLogRepository | Audit trail | Low |

### Out of Scope: Messaging Layer

These repositories depend on MongoDB-specific features and remain MongoDB-only:

| Repository | Reason |
|------------|--------|
| EventRepository | High-volume writes, change streams, TTL indexes |
| EventReadRepository | Aggregation pipelines, distinct cascades |
| EventTypeRepository | String parsing aggregations |
| SubscriptionRepository | Wildcard matching, hybrid filtering |
| DispatchJobRepository | Array operations, scheduler queries |
| DispatchJobReadRepository | Read projections, aggregations |
| DispatchPoolRepository | Coupled to dispatch system |
| SchemaRepository | JSON schema storage |

**When messaging is disabled**, these repositories are not instantiated.

---

## Design Constraints

### No JSON/JSONB Columns

All data must be fully normalized into relational tables. This increases schema complexity but ensures:
- Full queryability without JSON operators
- Consistent behavior across PostgreSQL and MySQL
- No database-specific JSON syntax

### Strings for Enums

Database enum types are not used. All enum values stored as VARCHAR strings:
- Avoids PostgreSQL `CREATE TYPE` migrations
- Consistent across all databases
- Application layer validates enum values

### Full Normalization

All embedded arrays become junction tables:
- `Principal.roles[]` → `principal_roles` table
- `Client.notes[]` → `client_notes` table
- `Subscription.event_types[]` → `subscription_event_types` table

---

## Architecture

### Repository Interface Pattern

```
┌─────────────────────────────────────────────────────────────┐
│                      Use Cases                               │
│  (CreateClientUseCase, CreatePrincipalUseCase, etc.)        │
└─────────────────────────────┬───────────────────────────────┘
                              │
┌─────────────────────────────▼───────────────────────────────┐
│              Repository Traits (Unchanged)                   │
│  trait ClientRepository: Send + Sync { ... }                │
│  trait PrincipalRepository: Send + Sync { ... }             │
└─────────────────────────────┬───────────────────────────────┘
                              │
         ┌────────────────────┼────────────────────┐
         ▼                    ▼                    ▼
┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐
│ MongoRepository │  │ PostgresRepository│  │ MySqlRepository │
│ (mongodb crate) │  │ (sqlx)           │  │ (sqlx)          │
└─────────────────┘  └─────────────────┘  └─────────────────┘
```

### Runtime Selection

```rust
// Config-driven repository instantiation
pub fn create_repositories(config: &DatabaseConfig) -> Arc<dyn Repositories> {
    match config.database_type {
        DatabaseType::MongoDB => Arc::new(MongoRepositories::new(&config.mongodb)),
        DatabaseType::PostgreSQL => Arc::new(PostgresRepositories::new(&config.postgresql)),
        DatabaseType::MySQL => Arc::new(MySqlRepositories::new(&config.mysql)),
    }
}
```

### Unit of Work Adaptation

When messaging is disabled, UnitOfWork commits only:
1. Aggregate state change
2. Audit log entry

Events are not persisted (no event streaming without MongoDB).

```rust
#[async_trait]
impl UnitOfWork for SqlUnitOfWork {
    async fn commit<E, T, C>(&self, aggregate: &T, event: E, command: &C) -> UseCaseResult<E>
    where
        E: DomainEvent + Serialize + Send + 'static,
        T: Serialize + HasId + Send + Sync,
        C: Serialize + Send + Sync,
    {
        let mut tx = self.pool.begin().await?;

        // 1. Upsert aggregate
        self.upsert_aggregate(&mut tx, aggregate).await?;

        // 2. Insert audit log (events skipped - messaging disabled)
        let audit_log = AuditLog::from_event(&event, command);
        sqlx::query!(
            "INSERT INTO audit_logs (id, entity_type, entity_id, operation, ...) VALUES ($1, $2, ...)",
            audit_log.id, audit_log.entity_type, ...
        ).execute(&mut *tx).await?;

        tx.commit().await?;
        UseCaseResult::success(event)
    }
}
```

---

## Schema Design

### ID Strategy

All IDs remain TSID Crockford Base32 strings (13 characters):

```sql
id VARCHAR(13) PRIMARY KEY  -- e.g., "0HZXEQ5Y8JY5Z"
```

### Core Tables

#### Principals (Users & Service Accounts)

```sql
CREATE TABLE principals (
    id VARCHAR(13) PRIMARY KEY,
    principal_type VARCHAR(20) NOT NULL,      -- USER, SERVICE
    scope VARCHAR(20) NOT NULL,               -- ANCHOR, PARTNER, CLIENT
    client_id VARCHAR(13),
    name VARCHAR(255) NOT NULL,
    active BOOLEAN NOT NULL DEFAULT true,

    -- Flattened UserIdentity (nullable for SERVICE type)
    email VARCHAR(255),
    email_verified BOOLEAN,
    password_hash VARCHAR(255),

    -- Flattened ServiceAccount (nullable for USER type)
    sa_code VARCHAR(100),
    sa_signing_secret VARCHAR(255),

    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,

    CONSTRAINT uq_principal_email UNIQUE (email),
    CONSTRAINT uq_principal_sa_code UNIQUE (sa_code),
    CONSTRAINT fk_principal_client FOREIGN KEY (client_id) REFERENCES clients(id)
);

CREATE INDEX idx_principals_scope ON principals(scope);
CREATE INDEX idx_principals_client ON principals(client_id);
CREATE INDEX idx_principals_type ON principals(principal_type);
```

#### Principal Roles (Junction)

```sql
CREATE TABLE principal_roles (
    id VARCHAR(13) PRIMARY KEY,
    principal_id VARCHAR(13) NOT NULL,
    role_code VARCHAR(100) NOT NULL,
    assignment_source VARCHAR(20) NOT NULL,   -- CODE, ADMIN, IDP
    assigned_at TIMESTAMP NOT NULL,

    CONSTRAINT uq_principal_role UNIQUE (principal_id, role_code),
    CONSTRAINT fk_principal_role_principal FOREIGN KEY (principal_id)
        REFERENCES principals(id) ON DELETE CASCADE
);

CREATE INDEX idx_principal_roles_principal ON principal_roles(principal_id);
CREATE INDEX idx_principal_roles_code ON principal_roles(role_code);
```

#### Principal Client Access (Junction)

```sql
CREATE TABLE principal_client_access (
    principal_id VARCHAR(13) NOT NULL,
    client_id VARCHAR(13) NOT NULL,

    PRIMARY KEY (principal_id, client_id),
    CONSTRAINT fk_pca_principal FOREIGN KEY (principal_id)
        REFERENCES principals(id) ON DELETE CASCADE,
    CONSTRAINT fk_pca_client FOREIGN KEY (client_id)
        REFERENCES clients(id) ON DELETE CASCADE
);
```

#### Clients

```sql
CREATE TABLE clients (
    id VARCHAR(13) PRIMARY KEY,
    identifier VARCHAR(100) NOT NULL UNIQUE,
    name VARCHAR(255) NOT NULL,
    status VARCHAR(20) NOT NULL,              -- ACTIVE, SUSPENDED, ARCHIVED
    status_reason TEXT,
    status_changed_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE INDEX idx_clients_status ON clients(status);
CREATE INDEX idx_clients_identifier ON clients(identifier);
```

#### Client Notes (Junction)

```sql
CREATE TABLE client_notes (
    id VARCHAR(13) PRIMARY KEY,
    client_id VARCHAR(13) NOT NULL,
    category VARCHAR(50),
    text TEXT NOT NULL,
    added_by VARCHAR(13),
    created_at TIMESTAMP NOT NULL,

    CONSTRAINT fk_client_note_client FOREIGN KEY (client_id)
        REFERENCES clients(id) ON DELETE CASCADE,
    CONSTRAINT fk_client_note_author FOREIGN KEY (added_by)
        REFERENCES principals(id)
);

CREATE INDEX idx_client_notes_client ON client_notes(client_id);
```

#### Roles

```sql
CREATE TABLE auth_roles (
    id VARCHAR(13) PRIMARY KEY,
    code VARCHAR(100) NOT NULL UNIQUE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    source VARCHAR(20) NOT NULL,              -- CODE, DATABASE, SDK
    application_code VARCHAR(100),
    client_managed BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE INDEX idx_roles_source ON auth_roles(source);
CREATE INDEX idx_roles_application ON auth_roles(application_code);
```

#### Role Permissions (Junction)

```sql
CREATE TABLE role_permissions (
    role_id VARCHAR(13) NOT NULL,
    permission VARCHAR(100) NOT NULL,

    PRIMARY KEY (role_id, permission),
    CONSTRAINT fk_role_permission_role FOREIGN KEY (role_id)
        REFERENCES auth_roles(id) ON DELETE CASCADE
);
```

#### Applications

```sql
CREATE TABLE applications (
    id VARCHAR(13) PRIMARY KEY,
    code VARCHAR(100) NOT NULL UNIQUE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    application_type VARCHAR(20) NOT NULL,    -- APPLICATION, INTEGRATION
    active BOOLEAN NOT NULL DEFAULT true,
    service_account_id VARCHAR(13),
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);

CREATE INDEX idx_applications_type ON applications(application_type);
CREATE INDEX idx_applications_active ON applications(active);
```

#### Service Accounts

```sql
CREATE TABLE service_accounts (
    id VARCHAR(13) PRIMARY KEY,
    code VARCHAR(100) NOT NULL UNIQUE,
    name VARCHAR(255) NOT NULL,
    application_id VARCHAR(13),
    signing_secret VARCHAR(255) NOT NULL,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,

    CONSTRAINT fk_sa_application FOREIGN KEY (application_id)
        REFERENCES applications(id)
);

CREATE INDEX idx_service_accounts_application ON service_accounts(application_id);
```

#### Service Account Clients (Junction)

```sql
CREATE TABLE service_account_clients (
    service_account_id VARCHAR(13) NOT NULL,
    client_id VARCHAR(13) NOT NULL,

    PRIMARY KEY (service_account_id, client_id),
    CONSTRAINT fk_sac_service_account FOREIGN KEY (service_account_id)
        REFERENCES service_accounts(id) ON DELETE CASCADE,
    CONSTRAINT fk_sac_client FOREIGN KEY (client_id)
        REFERENCES clients(id) ON DELETE CASCADE
);
```

#### Service Account Roles (Junction)

```sql
CREATE TABLE service_account_roles (
    id VARCHAR(13) PRIMARY KEY,
    service_account_id VARCHAR(13) NOT NULL,
    role_code VARCHAR(100) NOT NULL,
    assignment_source VARCHAR(20) NOT NULL,
    assigned_at TIMESTAMP NOT NULL,

    CONSTRAINT uq_sa_role UNIQUE (service_account_id, role_code),
    CONSTRAINT fk_sa_role_sa FOREIGN KEY (service_account_id)
        REFERENCES service_accounts(id) ON DELETE CASCADE
);
```

#### OAuth Clients

```sql
CREATE TABLE oauth_clients (
    id VARCHAR(13) PRIMARY KEY,
    client_id VARCHAR(255) NOT NULL UNIQUE,
    client_secret_hash VARCHAR(255),
    owner_client_id VARCHAR(13),
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,

    CONSTRAINT fk_oauth_owner FOREIGN KEY (owner_client_id)
        REFERENCES clients(id)
);
```

#### OAuth Client Applications (Junction)

```sql
CREATE TABLE oauth_client_applications (
    oauth_client_id VARCHAR(13) NOT NULL,
    application_id VARCHAR(13) NOT NULL,

    PRIMARY KEY (oauth_client_id, application_id),
    CONSTRAINT fk_oca_oauth FOREIGN KEY (oauth_client_id)
        REFERENCES oauth_clients(id) ON DELETE CASCADE,
    CONSTRAINT fk_oca_application FOREIGN KEY (application_id)
        REFERENCES applications(id) ON DELETE CASCADE
);
```

#### Refresh Tokens

```sql
CREATE TABLE refresh_tokens (
    id VARCHAR(13) PRIMARY KEY,
    token_hash VARCHAR(255) NOT NULL UNIQUE,
    principal_id VARCHAR(13) NOT NULL,
    family_id VARCHAR(13) NOT NULL,
    revoked BOOLEAN NOT NULL DEFAULT false,
    revoked_at TIMESTAMP,
    replaced_by VARCHAR(255),
    expires_at TIMESTAMP NOT NULL,
    last_used_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL,

    CONSTRAINT fk_refresh_principal FOREIGN KEY (principal_id)
        REFERENCES principals(id) ON DELETE CASCADE
);

CREATE INDEX idx_refresh_tokens_principal ON refresh_tokens(principal_id);
CREATE INDEX idx_refresh_tokens_family ON refresh_tokens(family_id);
CREATE INDEX idx_refresh_tokens_expires ON refresh_tokens(expires_at)
    WHERE NOT revoked;
```

#### Authorization Codes

```sql
CREATE TABLE authorization_codes (
    id VARCHAR(13) PRIMARY KEY,
    code VARCHAR(255) NOT NULL UNIQUE,
    principal_id VARCHAR(13) NOT NULL,
    oauth_client_id VARCHAR(13) NOT NULL,
    redirect_uri TEXT NOT NULL,
    scope TEXT,
    code_challenge VARCHAR(255),
    code_challenge_method VARCHAR(10),
    used BOOLEAN NOT NULL DEFAULT false,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL,

    CONSTRAINT fk_authcode_principal FOREIGN KEY (principal_id)
        REFERENCES principals(id),
    CONSTRAINT fk_authcode_oauth FOREIGN KEY (oauth_client_id)
        REFERENCES oauth_clients(id)
);
```

#### OIDC Login States

```sql
CREATE TABLE oidc_login_states (
    id VARCHAR(13) PRIMARY KEY,
    state VARCHAR(255) NOT NULL UNIQUE,
    nonce VARCHAR(255),
    redirect_uri TEXT,
    idp_type VARCHAR(50) NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL
);
```

#### Audit Logs

```sql
CREATE TABLE audit_logs (
    id VARCHAR(13) PRIMARY KEY,
    entity_type VARCHAR(100) NOT NULL,
    entity_id VARCHAR(13) NOT NULL,
    operation VARCHAR(50) NOT NULL,
    principal_id VARCHAR(13),
    principal_name VARCHAR(255),
    before_state TEXT,                        -- Serialized JSON as text
    after_state TEXT,                         -- Serialized JSON as text
    performed_at TIMESTAMP NOT NULL,
    correlation_id VARCHAR(255),
    client_id VARCHAR(13)
);

CREATE INDEX idx_audit_entity ON audit_logs(entity_type, entity_id);
CREATE INDEX idx_audit_principal ON audit_logs(principal_id);
CREATE INDEX idx_audit_time ON audit_logs(performed_at DESC);
CREATE INDEX idx_audit_client ON audit_logs(client_id);
```

#### Configuration Tables

```sql
CREATE TABLE anchor_domains (
    id VARCHAR(13) PRIMARY KEY,
    domain VARCHAR(255) NOT NULL UNIQUE,
    created_at TIMESTAMP NOT NULL
);

CREATE TABLE client_auth_configs (
    id VARCHAR(13) PRIMARY KEY,
    email_domain VARCHAR(255) NOT NULL UNIQUE,
    client_id VARCHAR(13) NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL,

    CONSTRAINT fk_cac_client FOREIGN KEY (client_id)
        REFERENCES clients(id)
);

CREATE TABLE client_access_grants (
    id VARCHAR(13) PRIMARY KEY,
    principal_id VARCHAR(13) NOT NULL,
    client_id VARCHAR(13) NOT NULL,
    granted_by VARCHAR(13),
    expires_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL,

    CONSTRAINT uq_access_grant UNIQUE (principal_id, client_id),
    CONSTRAINT fk_cag_principal FOREIGN KEY (principal_id)
        REFERENCES principals(id) ON DELETE CASCADE,
    CONSTRAINT fk_cag_client FOREIGN KEY (client_id)
        REFERENCES clients(id) ON DELETE CASCADE,
    CONSTRAINT fk_cag_grantor FOREIGN KEY (granted_by)
        REFERENCES principals(id)
);

CREATE TABLE idp_role_mappings (
    id VARCHAR(13) PRIMARY KEY,
    idp_type VARCHAR(50) NOT NULL,
    idp_role_name VARCHAR(255) NOT NULL,
    platform_role_code VARCHAR(100) NOT NULL,
    created_at TIMESTAMP NOT NULL,

    CONSTRAINT uq_idp_mapping UNIQUE (idp_type, idp_role_name)
);
```

---

## Configuration

### Application Config

```toml
[database]
# Options: mongodb, postgresql, mysql
type = "mongodb"

[database.mongodb]
uri = "mongodb://localhost:27017"
database = "flowcatalyst"

[database.postgresql]
url = "postgres://user:pass@localhost:5432/flowcatalyst"
max_connections = 10

[database.mysql]
url = "mysql://user:pass@localhost:3306/flowcatalyst"
max_connections = 10

[features]
# When false, messaging repositories are not loaded
# SQL databases only support messaging_enabled = false
messaging_enabled = true
```

### Feature Matrix

| Feature | MongoDB | PostgreSQL | MySQL |
|---------|---------|------------|-------|
| Control Plane (IAM, Clients) | Yes | Planned | Planned |
| Event Streaming | Yes | No | No |
| Dispatch Jobs | Yes | No | No |
| Subscriptions | Yes | No | No |
| Change Streams | Yes | No | No |
| TTL Auto-Expiration | Yes | Manual cleanup | Manual cleanup |

---

## Migration Strategy

### Phase 1: Interface Extraction

Extract repository traits from current MongoDB implementations:

```rust
// Current: Concrete implementation
pub struct ClientRepository { collection: Collection<Client> }

// Target: Trait + implementation
#[async_trait]
pub trait ClientRepository: Send + Sync {
    async fn find_by_id(&self, id: &str) -> Result<Option<Client>>;
    async fn find_by_identifier(&self, identifier: &str) -> Result<Option<Client>>;
    // ...
}

pub struct MongoClientRepository { collection: Collection<Client> }
impl ClientRepository for MongoClientRepository { ... }
```

### Phase 2: SQL Implementations

Create PostgreSQL/MySQL implementations using sqlx:

```rust
pub struct PostgresClientRepository { pool: PgPool }

#[async_trait]
impl ClientRepository for PostgresClientRepository {
    async fn find_by_identifier(&self, identifier: &str) -> Result<Option<Client>> {
        sqlx::query_as!(
            Client,
            "SELECT * FROM clients WHERE identifier = $1",
            identifier
        )
        .fetch_optional(&self.pool)
        .await
        .map_err(Into::into)
    }
}
```

### Phase 3: Junction Table Handling

Implement eager loading for denormalized fields:

```rust
impl PostgresPrincipalRepository {
    async fn find_by_id(&self, id: &str) -> Result<Option<Principal>> {
        // 1. Fetch principal
        let row = sqlx::query!("SELECT * FROM principals WHERE id = $1", id)
            .fetch_optional(&self.pool).await?;

        let Some(row) = row else { return Ok(None) };

        // 2. Fetch roles
        let roles = sqlx::query_as!(
            RoleAssignment,
            "SELECT * FROM principal_roles WHERE principal_id = $1",
            id
        ).fetch_all(&self.pool).await?;

        // 3. Fetch client access
        let clients = sqlx::query_scalar!(
            "SELECT client_id FROM principal_client_access WHERE principal_id = $1",
            id
        ).fetch_all(&self.pool).await?;

        // 4. Assemble
        Ok(Some(Principal {
            id: row.id,
            roles,
            assigned_clients: clients,
            // ...
        }))
    }
}
```

### Phase 4: Schema Migrations

Use sqlx migrations or standalone SQL files:

```
migrations/
├── 001_create_clients.sql
├── 002_create_principals.sql
├── 003_create_roles.sql
├── 004_create_applications.sql
├── 005_create_service_accounts.sql
├── 006_create_oauth.sql
├── 007_create_audit.sql
└── 008_create_config.sql
```

---

## Effort Estimate

| Task | Effort |
|------|--------|
| Interface extraction | 1-2 days |
| PostgreSQL implementations (16 repos) | 5-7 days |
| MySQL implementations | 3-5 days (after PostgreSQL) |
| Junction table handling | 2-3 days |
| SqlUnitOfWork | 1-2 days |
| Schema migrations | 2-3 days |
| Configuration & wiring | 1-2 days |
| Testing | 3-5 days |
| **Total** | **18-29 days** |

---

## Trade-offs

### Why Defer Implementation

1. **Maintenance Burden**: Every new feature requires updates to 3 implementations (MongoDB, PostgreSQL, MySQL) plus junction table handling.

2. **Testing Complexity**: Integration tests must run against all supported databases.

3. **Schema Migrations**: SQL schema changes require migration scripts; MongoDB is schema-flexible.

4. **Feature Divergence Risk**: SQL implementations may lag behind MongoDB features.

### When to Reconsider

- Significant demand from users without MongoDB expertise
- Cloud provider pricing makes MongoDB uneconomical
- Regulatory requirements mandate specific database vendors

---

## References

- [SQLx Documentation](https://docs.rs/sqlx)
- [MongoDB Rust Driver](https://docs.rs/mongodb)
- [FlowCatalyst Architecture Overview](overview.md)
- [ARCHITECTURE_DECISION.md](../../ARCHITECTURE_DECISION.md)
