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

/// Run all SQL migrations from the migrations/ directory.
pub async fn run_migrations(pool: &PgPool) -> Result<(), sqlx::Error> {
    info!("Running database migrations...");

    let migration_files = [
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

    for (i, sql) in migration_files.iter().enumerate() {
        for statement in sql.split(';') {
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
        info!("Migration {} applied successfully", i + 1);
    }

    info!("All database migrations completed");
    Ok(())
}
