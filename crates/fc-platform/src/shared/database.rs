//! PostgreSQL Database Connection (SQLx)
//!
//! Provides:
//! - `PgPool` creation with shared env-driven pool config.
//! - `SecretProvider` abstraction (env / AWS Secrets Manager) and a background
//!   refresh task that polls the provider on an interval and updates the pool's
//!   connection options when the DB password rotates. Existing repositories do
//!   not need to change — `PgPool::set_connect_options` mutates the pool in
//!   place, so any future connection (including reconnects after `max_lifetime`)
//!   uses the new credentials.
//!
//! This mirrors the TS `flowcatalyst` approach (timer-based polling + graceful
//! refresh) but takes advantage of sqlx's in-place options update so we don't
//! need to swap pool handles or refactor every repository.

use sqlx::postgres::{PgConnectOptions, PgPool, PgPoolOptions};
use std::str::FromStr;
use std::sync::Arc;
use std::time::Duration;
use tracing::{error, info, warn};

// ── Pool config ──────────────────────────────────────────────────────────────

#[derive(Clone, Debug)]
struct PoolConfig {
    max_connections: u32,
    min_connections: u32,
    connect_timeout: u64,
    idle_timeout: u64,
    max_lifetime: u64,
}

impl PoolConfig {
    fn from_env() -> Self {
        Self {
            max_connections: env_parse("FC_DB_MAX_CONNECTIONS", 10),
            min_connections: env_parse("FC_DB_MIN_CONNECTIONS", 2),
            connect_timeout: env_parse("FC_DB_CONNECT_TIMEOUT_SECS", 10),
            idle_timeout: env_parse("FC_DB_IDLE_TIMEOUT_SECS", 300),
            max_lifetime: env_parse("FC_DB_MAX_LIFETIME_SECS", 1800),
        }
    }
}

fn env_parse<T: FromStr>(key: &str, default: T) -> T {
    std::env::var(key).ok().and_then(|v| v.parse().ok()).unwrap_or(default)
}

/// Create a new SQLx PgPool with connection pooling.
///
/// Environment-configurable pool settings:
/// * `FC_DB_MAX_CONNECTIONS` (default: 10)
/// * `FC_DB_MIN_CONNECTIONS` (default: 2)
/// * `FC_DB_CONNECT_TIMEOUT_SECS` (default: 10)
/// * `FC_DB_IDLE_TIMEOUT_SECS` (default: 300)
/// * `FC_DB_MAX_LIFETIME_SECS` (default: 1800)
pub async fn create_pool(database_url: &str) -> Result<PgPool, sqlx::Error> {
    let cfg = PoolConfig::from_env();
    info!(
        max_connections = cfg.max_connections,
        min_connections = cfg.min_connections,
        "Creating SQLx PgPool"
    );

    let pool = PgPoolOptions::new()
        .max_connections(cfg.max_connections)
        .min_connections(cfg.min_connections)
        .acquire_timeout(Duration::from_secs(cfg.connect_timeout))
        .idle_timeout(Duration::from_secs(cfg.idle_timeout))
        .max_lifetime(Duration::from_secs(cfg.max_lifetime))
        .connect(database_url)
        .await?;

    info!("SQLx PgPool established");
    Ok(pool)
}

// ── Secret provider ──────────────────────────────────────────────────────────

/// A source for the database connection URL. Implementations are async because
/// cloud providers (Secrets Manager, GCP Secret Manager) require network calls.
#[async_trait::async_trait]
pub trait SecretProvider: Send + Sync {
    fn name(&self) -> &'static str;
    async fn get_db_url(&self) -> Result<String, anyhow::Error>;
}

/// AWS Secrets Manager provider. Reads `{"username":..., "password":..., "port":...}`
/// JSON from a secret and constructs a `postgresql://` URL using the supplied
/// host and database name.
pub struct AwsSecretProvider {
    secret_arn: String,
    host: String,
    db_name: String,
    fallback_port: String,
}

impl AwsSecretProvider {
    pub fn new(secret_arn: String, host: String, db_name: String, fallback_port: String) -> Self {
        Self { secret_arn, host, db_name, fallback_port }
    }
}

#[async_trait::async_trait]
impl SecretProvider for AwsSecretProvider {
    fn name(&self) -> &'static str { "aws-secrets-manager" }

    async fn get_db_url(&self) -> Result<String, anyhow::Error> {
        let config = aws_config::load_defaults(aws_config::BehaviorVersion::latest()).await;
        let sm = aws_sdk_secretsmanager::Client::new(&config);

        let secret = sm.get_secret_value()
            .secret_id(&self.secret_arn)
            .send()
            .await
            .map_err(|e| anyhow::anyhow!("Failed to get DB secret from Secrets Manager: {}", e))?;

        let secret_string = secret.secret_string()
            .ok_or_else(|| anyhow::anyhow!("DB secret has no string value"))?;

        let creds: serde_json::Value = serde_json::from_str(secret_string)
            .map_err(|e| anyhow::anyhow!("Failed to parse DB secret JSON: {}", e))?;

        let username = creds["username"].as_str()
            .ok_or_else(|| anyhow::anyhow!("DB secret missing 'username' field"))?;
        let password = creds["password"].as_str()
            .ok_or_else(|| anyhow::anyhow!("DB secret missing 'password' field"))?;
        let port = creds["port"].as_u64()
            .map(|p| p.to_string())
            .unwrap_or_else(|| self.fallback_port.clone());

        let password_encoded = urlencoding::encode(password);
        let url = if self.host.contains(':') {
            format!("postgresql://{}:{}@{}/{}", username, password_encoded, self.host, self.db_name)
        } else {
            format!("postgresql://{}:{}@{}:{}/{}", username, password_encoded, self.host, port, self.db_name)
        };
        Ok(url)
    }
}

// ── Background refresh task ──────────────────────────────────────────────────

/// Spawn a background task that polls `provider` on `interval` and, when the
/// resolved DB URL changes, updates the connection options on the pool.
///
/// Mirrors the TypeScript flowcatalyst approach (timer-based polling + graceful
/// refresh). Takes advantage of AWS RDS's dual-password rotation window: both
/// old and new passwords are valid for a period after rotation, so a periodic
/// poll catches the change before the old password is invalidated.
///
/// Disable by passing `Duration::ZERO` for `interval`.
pub fn start_secret_refresh(
    provider: Arc<dyn SecretProvider>,
    pg_pool: PgPool,
    initial_url: String,
    interval: Duration,
) {
    if interval.is_zero() {
        info!("DB secret refresh disabled (interval=0)");
        return;
    }
    info!(
        provider = provider.name(),
        interval_secs = interval.as_secs(),
        "Starting DB secret refresh task"
    );
    tokio::spawn(async move {
        let mut current_url = initial_url;
        loop {
            tokio::time::sleep(interval).await;
            match provider.get_db_url().await {
                Ok(new_url) => {
                    if new_url == current_url {
                        continue;
                    }
                    info!(
                        provider = provider.name(),
                        "DB credentials changed — updating pool connect options"
                    );
                    match PgConnectOptions::from_str(&new_url) {
                        Ok(opts) => {
                            // New connections (and reconnects after `max_lifetime`)
                            // will use the new credentials. The dual-password
                            // window on RDS keeps existing connections valid
                            // until they cycle out naturally.
                            pg_pool.set_connect_options(opts);
                            current_url = new_url;
                            info!("Pool connect options updated successfully");
                        }
                        Err(e) => {
                            error!(error = %e, "Failed to parse refreshed DB URL");
                        }
                    }
                }
                Err(e) => {
                    warn!(
                        provider = provider.name(),
                        error = %e,
                        "Failed to poll secret provider for credential changes"
                    );
                }
            }
        }
    });
}

// ── Migrations ───────────────────────────────────────────────────────────────

/// Migration profile. Selects which optional migrations apply.
///
/// `Embedded` is for local dev (`fc-dev`) using `postgresql_embedded`. It skips
/// production-only migrations like declarative partitioning, which add
/// operational machinery (partition manager, retention sweeps) that aren't
/// useful when the data dir is throwaway.
///
/// `Production` is for `fc-server` and any RDS-backed deployment.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum MigrationProfile {
    Embedded,
    Production,
}

/// Run all SQL migrations from the migrations/ directory.
pub async fn run_migrations(pool: &PgPool, profile: MigrationProfile) -> Result<(), sqlx::Error> {
    info!(?profile, "Running database migrations...");

    // Migrations applied to every profile.
    let core_migrations = [
        include_str!("../../../../migrations/001_tenant_tables.sql"),
        include_str!("../../../../migrations/002_iam_tables.sql"),
        include_str!("../../../../migrations/003_application_tables.sql"),
        include_str!("../../../../migrations/004_messaging_tables.sql"),
        include_str!("../../../../migrations/005_outbox_tables.sql"),
        include_str!("../../../../migrations/006_audit_tables.sql"),
        include_str!("../../../../migrations/007_oauth_tables.sql"),
        include_str!("../../../../migrations/008_auth_tracking_tables.sql"),
        include_str!("../../../../migrations/009_p0_alignment.sql"),
        include_str!("../../../../migrations/010_auth_state_tables.sql"),
        include_str!("../../../../migrations/011_dispatch_job_tables.sql"),
        include_str!("../../../../migrations/012_projection_columns.sql"),
        include_str!("../../../../migrations/013_drop_connection_endpoint.sql"),
        include_str!("../../../../migrations/014_widen_attempt_type.sql"),
        include_str!("../../../../migrations/015_dispatch_jobs_write_indexes.sql"),
    ];

    for (i, sql) in core_migrations.iter().enumerate() {
        apply_migration(pool, sql).await?;
        info!("Migration {} applied successfully", i + 1);
    }

    // Production-only migrations.
    if profile == MigrationProfile::Production {
        let production_migrations = [
            ("018_partition_messaging_tables", include_str!("../../../../migrations/018_partition_messaging_tables.sql")),
        ];
        for (name, sql) in production_migrations.iter() {
            apply_migration(pool, sql).await?;
            info!(migration = %name, "Production migration applied");
        }
    }

    info!("All database migrations completed");
    Ok(())
}

async fn apply_migration(pool: &PgPool, sql: &str) -> Result<(), sqlx::Error> {
    for statement in split_sql_statements(sql) {
        let cleaned: String = statement
            .lines()
            .filter(|line| !line.trim_start().starts_with("--"))
            .collect::<Vec<_>>()
            .join("\n");
        let trimmed = cleaned.trim();
        if trimmed.is_empty() {
            continue;
        }
        sqlx::query(trimmed).execute(pool).await?;
    }
    Ok(())
}

/// Split a SQL script into top-level statements on `;`, respecting:
/// - dollar-quoted bodies (`$$ ... $$` or `$tag$ ... $tag$`) used by `DO`/`CREATE FUNCTION`
/// - single-quoted strings (`'foo''bar'`)
/// - line comments (`-- ...`) and block comments (`/* ... */`)
fn split_sql_statements(sql: &str) -> Vec<String> {
    let bytes = sql.as_bytes();
    let mut out = Vec::new();
    let mut buf = String::new();
    let mut i = 0;

    enum State {
        Normal,
        SingleQuote,
        LineComment,
        BlockComment,
        DollarQuote(String), // tag including the dollars, e.g. "$$" or "$plpgsql$"
    }

    let mut state = State::Normal;

    while i < bytes.len() {
        match &state {
            State::Normal => {
                let b = bytes[i];
                // Try to recognize a dollar-quote tag opener: $...$
                if b == b'$' {
                    if let Some(tag_end) = bytes[i + 1..].iter().position(|&c| c == b'$') {
                        let tag_body = &bytes[i + 1..i + 1 + tag_end];
                        let valid_tag = tag_body
                            .iter()
                            .all(|&c| c.is_ascii_alphanumeric() || c == b'_');
                        if valid_tag {
                            let full_tag =
                                String::from_utf8_lossy(&bytes[i..=i + 1 + tag_end]).into_owned();
                            buf.push_str(&full_tag);
                            i += full_tag.len();
                            state = State::DollarQuote(full_tag);
                            continue;
                        }
                    }
                    buf.push(b as char);
                    i += 1;
                } else if b == b'\'' {
                    buf.push('\'');
                    i += 1;
                    state = State::SingleQuote;
                } else if b == b'-' && i + 1 < bytes.len() && bytes[i + 1] == b'-' {
                    buf.push_str("--");
                    i += 2;
                    state = State::LineComment;
                } else if b == b'/' && i + 1 < bytes.len() && bytes[i + 1] == b'*' {
                    buf.push_str("/*");
                    i += 2;
                    state = State::BlockComment;
                } else if b == b';' {
                    out.push(std::mem::take(&mut buf));
                    i += 1;
                } else {
                    buf.push(b as char);
                    i += 1;
                }
            }
            State::SingleQuote => {
                let b = bytes[i];
                buf.push(b as char);
                i += 1;
                if b == b'\'' {
                    if i < bytes.len() && bytes[i] == b'\'' {
                        buf.push('\'');
                        i += 1;
                    } else {
                        state = State::Normal;
                    }
                }
            }
            State::LineComment => {
                let b = bytes[i];
                buf.push(b as char);
                i += 1;
                if b == b'\n' {
                    state = State::Normal;
                }
            }
            State::BlockComment => {
                let b = bytes[i];
                buf.push(b as char);
                i += 1;
                if b == b'*' && i < bytes.len() && bytes[i] == b'/' {
                    buf.push('/');
                    i += 1;
                    state = State::Normal;
                }
            }
            State::DollarQuote(tag) => {
                if bytes[i..].starts_with(tag.as_bytes()) {
                    buf.push_str(tag);
                    i += tag.len();
                    state = State::Normal;
                } else {
                    buf.push(bytes[i] as char);
                    i += 1;
                }
            }
        }
    }

    if !buf.trim().is_empty() {
        out.push(buf);
    }
    out
}

#[cfg(test)]
mod sql_split_tests {
    use super::split_sql_statements;

    #[test]
    fn splits_simple_statements() {
        let sql = "SELECT 1; SELECT 2;";
        let parts = split_sql_statements(sql);
        assert_eq!(parts.len(), 2);
    }

    #[test]
    fn preserves_dollar_quoted_block() {
        let sql = "DO $$ BEGIN SELECT 1; SELECT 2; END $$; SELECT 3;";
        let parts = split_sql_statements(sql);
        assert_eq!(parts.len(), 2);
        assert!(parts[0].contains("BEGIN"));
        assert!(parts[0].contains("END"));
    }

    #[test]
    fn handles_tagged_dollar_quote() {
        let sql = "CREATE FUNCTION f() RETURNS void AS $body$ BEGIN END; $body$ LANGUAGE plpgsql; SELECT 1;";
        let parts = split_sql_statements(sql);
        assert_eq!(parts.len(), 2);
    }

    #[test]
    fn ignores_semicolons_in_strings() {
        let sql = "INSERT INTO t VALUES ('a;b'); SELECT 1;";
        let parts = split_sql_statements(sql);
        assert_eq!(parts.len(), 2);
    }
}

// ── Built-in role seeding ────────────────────────────────────────────────────

/// Ensure the platform's built-in roles (defined in `role::entity::roles::all()`)
/// exist in `iam_roles`. Called on every startup.
///
/// **Upsert-only, no reconciliation:** inserts missing rows, leaves existing
/// rows alone. If an admin renames or deletes a built-in role at runtime, this
/// won't resurrect it — that's intentional. Built-in role definitions in code
/// are the platform's **initial state**, not an authoritative mirror.
///
/// Permissions for newly-inserted roles are also seeded from code.
pub async fn seed_builtin_roles(pool: &PgPool) -> Result<(), sqlx::Error> {
    use crate::role::repository::RoleRepository;
    use crate::role::entity::roles;

    let repo = RoleRepository::new(pool);
    let mut inserted = 0;

    for role in roles::all() {
        if repo.find_by_name(&role.name).await
            .map_err(|e| sqlx::Error::Protocol(format!("find_by_name({}): {}", role.name, e)))?
            .is_some()
        {
            continue;
        }
        repo.insert(&role).await
            .map_err(|e| sqlx::Error::Protocol(format!("insert({}): {}", role.name, e)))?;
        info!(role = %role.name, "Seeded built-in role");
        inserted += 1;
    }

    if inserted > 0 {
        info!(count = inserted, "Built-in role seeding complete");
    }
    Ok(())
}
