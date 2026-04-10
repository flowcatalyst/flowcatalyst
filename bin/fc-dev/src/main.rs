//! FlowCatalyst Development Monolith
//!
//! All-in-one binary for local development containing:
//! - Message Router (with embedded SQLite queue)
//! - API Server (for publishing messages)
//! - Outbox Processor (configurable database backend)
//! - Platform APIs (events, subscriptions, auth, etc.)
//! - Metrics endpoint

use std::sync::Arc;
use std::time::Duration;
use clap::Parser;
use tokio::signal;
use tokio::sync::broadcast;
use tokio::net::TcpListener;
use anyhow::Result;
use tracing::{info, warn, error};
use axum::{
    routing::get,
    response::Json,
    Router,
};
use tower_http::cors::{CorsLayer, Any};
use tower_http::set_header::SetResponseHeaderLayer;
use tower_http::trace::TraceLayer;
use tower_http::services::{ServeDir, ServeFile};
use axum::http::header::CACHE_CONTROL;
use axum::http::HeaderValue;

use rust_embed::Embed;

use fc_common::{RouterConfig, PoolConfig, QueueConfig};

/// Embedded frontend static files (compiled into the binary from frontend/dist/).
/// In dev, set FC_STATIC_DIR to override with a live directory.
#[derive(Embed)]
#[folder = "../../frontend/dist/"]
#[prefix = ""]
struct FrontendAssets;
use fc_router::{
    QueueManager, HttpMediator, LifecycleManager, LifecycleConfig,
    WarningService, WarningServiceConfig, HealthService, HealthServiceConfig,
    CircuitBreakerRegistry as RouterCircuitBreakerRegistry,
    api::create_router as create_api_router,
};
use fc_queue::sqlite::SqliteQueue;
use fc_queue::EmbeddedQueue;
use fc_outbox::enhanced_processor::{EnhancedOutboxProcessor, EnhancedProcessorConfig};
use fc_outbox::http_dispatcher::HttpDispatcherConfig;
use fc_outbox::postgres::PostgresOutboxRepository;

// Platform imports
use fc_platform::service::{AuthService, AuthConfig, AuthorizationService, AuditService, PasswordService, OidcService, OidcSyncService};
use fc_platform::api::middleware::{AppState, AuthLayer};
use fc_platform::api::{
    EventsState,
    DispatchJobsState,
    FilterOptionsState, filter_options_router, event_type_filters_router, events_router, dispatch_jobs_router,
    EventTypesState,
    ClientsState,
    PrincipalsState,
    RolesState,
    SubscriptionsState,
    OAuthClientsState,
    AuthConfigState,
    AuditLogsState,
    ApplicationsState,
    DispatchPoolsState,
    MonitoringState, LeaderState, CircuitBreakerRegistry, InFlightTracker,
    DebugState,
    ServiceAccountsState,
    // New domain APIs
    ConnectionsState,
    CorsState,
    IdentityProvidersState,
    EmailDomainMappingsState,
    PlatformConfigState,
    ConfigAccessState,
    LoginAttemptsState,
    MeState,
    SdkEventsState,
    SdkClientsState,
    SdkPrincipalsState,
    SdkRolesState,
    WellKnownState,
    ClientSelectionState,
    ApplicationRolesSdkState,
    PublicApiState,
    PasswordResetApiState,
    SdkSyncState,
    SdkAuditBatchState,
    SdkDispatchJobsState,
    BffRolesState,
    BffEventTypesState,
    AuthState,
    OAuthState,
    OidcLoginApiState,
};
use fc_platform::router::PlatformRoutes;
use fc_platform::repository::{
    EventRepository, EventTypeRepository, DispatchJobRepository, DispatchPoolRepository,
    SubscriptionRepository, ServiceAccountRepository, PrincipalRepository, ClientRepository,
    ApplicationRepository, RoleRepository, OAuthClientRepository,
    AnchorDomainRepository, ClientAuthConfigRepository, ClientAccessGrantRepository, IdpRoleMappingRepository,
    AuditLogRepository, ApplicationClientConfigRepository,
    // New repos
    ConnectionRepository, CorsOriginRepository, IdentityProviderRepository,
    EmailDomainMappingRepository, PlatformConfigRepository, PlatformConfigAccessRepository,
    LoginAttemptRepository,
    PasswordResetTokenRepository,
    OidcLoginStateRepository,
    RefreshTokenRepository,
    AuthorizationCodeRepository,
};
use fc_platform::usecase::PgUnitOfWork;
use fc_platform::shared::encryption_service::EncryptionService;
use fc_platform::operations::{
    // Application use cases
    CreateApplicationUseCase, UpdateApplicationUseCase,
    ActivateApplicationUseCase, DeactivateApplicationUseCase,
    // Service Account use cases
    CreateServiceAccountUseCase, UpdateServiceAccountUseCase, DeleteServiceAccountUseCase,
    AssignRolesUseCase, RegenerateAuthTokenUseCase, RegenerateSigningSecretUseCase,
    // Dispatch Pool use cases
    CreateDispatchPoolUseCase, UpdateDispatchPoolUseCase,
    ArchiveDispatchPoolUseCase, DeleteDispatchPoolUseCase,
};

use sqlx::sqlite::SqlitePoolOptions;

/// FlowCatalyst Development Server
#[derive(Parser, Debug)]
#[command(name = "fc-dev")]
#[command(about = "FlowCatalyst Development Monolith - All components in one binary")]
struct Args {
    /// API server port
    #[arg(long, env = "FC_API_PORT", default_value = "3000")]
    api_port: u16,

    /// Metrics server port
    #[arg(long, env = "FC_METRICS_PORT", default_value = "9090")]
    metrics_port: u16,

    /// Outbox database type: sqlite, postgres, mongo
    #[arg(long, env = "FC_OUTBOX_DB_TYPE", default_value = "sqlite")]
    outbox_db_type: String,

    /// Outbox database URL (for postgres/mongo)
    #[arg(long, env = "FC_OUTBOX_DB_URL")]
    outbox_db_url: Option<String>,

    /// MongoDB database name (when using mongo outbox)
    #[arg(long, env = "FC_OUTBOX_MONGO_DB", default_value = "flowcatalyst")]
    outbox_mongo_db: String,

    /// MongoDB collection name for outbox
    #[arg(long, env = "FC_OUTBOX_MONGO_COLLECTION", default_value = "outbox")]
    outbox_mongo_collection: String,

    /// Default pool concurrency
    #[arg(long, env = "FC_POOL_CONCURRENCY", default_value = "10")]
    pool_concurrency: u32,

    /// Enable outbox processor
    #[arg(long, env = "FC_OUTBOX_ENABLED", default_value = "false")]
    outbox_enabled: bool,

    /// Outbox poll interval in milliseconds
    #[arg(long, env = "FC_OUTBOX_POLL_INTERVAL_MS", default_value = "1000")]
    outbox_poll_interval_ms: u64,

    // Platform configuration

    /// PostgreSQL database URL
    #[arg(long, env = "FC_DATABASE_URL", default_value = "postgresql://localhost:5432/flowcatalyst")]
    database_url: String,
}

#[tokio::main]
async fn main() -> Result<()> {
    // Load .env.development (or .env) if present
    let _ = dotenvy::from_filename(".env.development")
        .or_else(|_| dotenvy::dotenv());

    // Set dev defaults for env vars that aren't set
    // These make fc-dev zero-config (only DB URL needed).
    if std::env::var("FLOWCATALYST_APP_KEY").is_err() {
        std::env::set_var("FLOWCATALYST_APP_KEY", "MpU3dI07kjZmZGROrElYfDXQgab30e3wr0KTnxQbePg=");
    }
    if std::env::var("FC_DEV_MODE").is_err() {
        std::env::set_var("FC_DEV_MODE", "true");
    }

    // Initialize logging (JSON if LOG_FORMAT=json, text otherwise)
    fc_common::logging::init_logging("fc-dev");

    let args = Args::parse();

    info!("Starting FlowCatalyst Dev Monolith (Rust)");
    info!("API port: {}, Metrics port: {}", args.api_port, args.metrics_port);

    // Setup shutdown signal
    let (shutdown_tx, _) = broadcast::channel::<()>(1);

    // 1. Setup SQLite for embedded queue
    let queue_pool = SqlitePoolOptions::new()
        .max_connections(5)
        .connect("sqlite::memory:")
        .await?;

    // 2. Initialize embedded queue (SQLite-based, mimics SQS FIFO)
    let queue = Arc::new(SqliteQueue::new(
        queue_pool.clone(),
        "dev-queue".to_string(),
        30, // visibility timeout
    ));
    queue.init_schema().await?;
    info!("Embedded SQLite queue initialized");

    // 3. Initialize HTTP Mediator (dev mode: HTTP/1.1, shorter timeout)
    let mediator = Arc::new(HttpMediator::dev());

    // 4. Create QueueManager (central orchestrator)
    let queue_manager = Arc::new(QueueManager::new(mediator.clone()));
    queue_manager.add_consumer(queue.clone()).await;

    // 4b. Create Warning and Health services
    let warning_service = Arc::new(WarningService::new(WarningServiceConfig::default()));
    let health_service = Arc::new(HealthService::new(
        HealthServiceConfig::default(),
        warning_service.clone(),
    ));

    // 5. Apply router configuration
    let router_config = RouterConfig {
        processing_pools: vec![
            PoolConfig {
                code: "DEFAULT".to_string(),
                concurrency: args.pool_concurrency,
                rate_limit_per_minute: None,
            },
        ],
        queues: vec![
            QueueConfig {
                name: "dev-queue".to_string(),
                uri: "sqlite::memory:".to_string(),
                connections: 1,
                visibility_timeout: 30,
            },
        ],
    };
    queue_manager.apply_config(router_config).await?;

    // 6. Start lifecycle manager (visibility extension, health checks)
    let lifecycle = LifecycleManager::start(
        queue_manager.clone(),
        warning_service.clone(),
        health_service.clone(),
        LifecycleConfig::default(),
    );

    // 7. Outbox processor — deferred until after AuthService is ready (needs a service token).
    //    We store the config now and start it after step 8c.
    let outbox_pool: Option<sqlx::PgPool> = if args.outbox_enabled && args.outbox_db_type == "postgres" {
        let outbox_db_url = args.outbox_db_url.as_deref()
            .unwrap_or(&args.database_url);
        info!(
            db_type = %args.outbox_db_type,
            db_url = %outbox_db_url,
            poll_interval_ms = args.outbox_poll_interval_ms,
            "Connecting to outbox database"
        );
        Some(sqlx::postgres::PgPoolOptions::new()
            .max_connections(5)
            .connect(outbox_db_url)
            .await
            .map_err(|e| anyhow::anyhow!("Outbox PostgreSQL connection failed: {}", e))?)
    } else {
        None
    };

    // 8. Setup platform services and APIs
    info!("Initializing platform services...");

    // 8a. Connect to MongoDB
    // 8a. Connect to PostgreSQL
    info!("Connecting to PostgreSQL...");
    let pg_db = fc_platform::shared::database::create_connection(&args.database_url).await
        .map_err(|e| anyhow::anyhow!("PostgreSQL connection failed: {}", e))?;

    // Run PostgreSQL migrations
    fc_platform::shared::database::run_migrations(&pg_db).await
        .map_err(|e| anyhow::anyhow!("PostgreSQL migrations failed: {}", e))?;

    // Seed development data
    let seeder = fc_platform::seed::DevDataSeeder::new(pg_db.clone());
    if let Err(e) = seeder.seed().await {
        tracing::warn!("Dev data seeding skipped (data may already exist): {}", e);
    }

    // 8b. Create SQLx pool (for migrated repositories)
    let pg_pool = fc_platform::shared::database::create_pool(&args.database_url).await
        .map_err(|e| anyhow::anyhow!("SQLx pool creation failed: {}", e))?;

    // 8c. Initialize all repositories
    let event_repo = Arc::new(EventRepository::new(&pg_pool));
    let event_type_repo = Arc::new(EventTypeRepository::new(&pg_db));
    let dispatch_job_repo = Arc::new(DispatchJobRepository::new(&pg_db));
    let dispatch_pool_repo = Arc::new(DispatchPoolRepository::new(&pg_db));
    let subscription_repo = Arc::new(SubscriptionRepository::new(&pg_db));
    let service_account_repo = Arc::new(ServiceAccountRepository::new(&pg_db));
    let principal_repo = Arc::new(PrincipalRepository::new(&pg_db));
    let client_repo = Arc::new(ClientRepository::new(&pg_db));
    let application_repo = Arc::new(ApplicationRepository::new(&pg_db));
    let role_repo = Arc::new(RoleRepository::new(&pg_db));
    let oauth_client_repo = Arc::new(OAuthClientRepository::new(&pg_db));
    let anchor_domain_repo = Arc::new(AnchorDomainRepository::new(&pg_db));
    let client_auth_config_repo = Arc::new(ClientAuthConfigRepository::new(&pg_db));
    let client_access_grant_repo = Arc::new(ClientAccessGrantRepository::new(&pg_db));
    let idp_role_mapping_repo = Arc::new(IdpRoleMappingRepository::new(&pg_db));
    let audit_log_repo = Arc::new(AuditLogRepository::new(&pg_db));
    let application_client_config_repo = Arc::new(ApplicationClientConfigRepository::new(&pg_db));
    // New domain repositories
    let connection_repo = Arc::new(ConnectionRepository::new(&pg_db));
    let cors_repo = Arc::new(CorsOriginRepository::new(&pg_db));
    let idp_repo = Arc::new(IdentityProviderRepository::new(&pg_db));
    let edm_repo = Arc::new(EmailDomainMappingRepository::new(&pg_db));
    let platform_config_repo = Arc::new(PlatformConfigRepository::new(&pg_db));
    let platform_config_access_repo = Arc::new(PlatformConfigAccessRepository::new(&pg_db));
    let login_attempt_repo = Arc::new(LoginAttemptRepository::new(&pg_db));
    let password_reset_repo = Arc::new(PasswordResetTokenRepository::new(&pg_db));
    let oidc_login_state_repo = Arc::new(OidcLoginStateRepository::new(&pg_db));
    let refresh_token_repo = Arc::new(RefreshTokenRepository::new(&pg_db));
    let auth_code_repo = Arc::new(AuthorizationCodeRepository::new(&pg_db));
    let pending_auth_repo = Arc::new(fc_platform::PendingAuthRepository::new(&pg_db));
    info!("Platform repositories initialized");

    // 8b2. Create UnitOfWork for atomic commits
    let unit_of_work = Arc::new(PgUnitOfWork::new(pg_db.clone()));

    // 8b3. Create use cases
    let create_application_use_case = Arc::new(CreateApplicationUseCase::new(application_repo.clone(), unit_of_work.clone()));
    let update_application_use_case = Arc::new(UpdateApplicationUseCase::new(application_repo.clone(), unit_of_work.clone()));
    let activate_application_use_case = Arc::new(ActivateApplicationUseCase::new(application_repo.clone(), unit_of_work.clone()));
    let deactivate_application_use_case = Arc::new(DeactivateApplicationUseCase::new(application_repo.clone(), unit_of_work.clone()));

    let create_service_account_use_case = Arc::new(CreateServiceAccountUseCase::new(service_account_repo.clone(), unit_of_work.clone()));
    let update_service_account_use_case = Arc::new(UpdateServiceAccountUseCase::new(service_account_repo.clone(), unit_of_work.clone()));
    let delete_service_account_use_case = Arc::new(DeleteServiceAccountUseCase::new(service_account_repo.clone(), unit_of_work.clone()));
    let assign_roles_use_case = Arc::new(AssignRolesUseCase::new(service_account_repo.clone(), unit_of_work.clone()));
    let regenerate_token_use_case = Arc::new(RegenerateAuthTokenUseCase::new(service_account_repo.clone(), unit_of_work.clone()));
    let regenerate_secret_use_case = Arc::new(RegenerateSigningSecretUseCase::new(service_account_repo.clone(), unit_of_work.clone()));

    let create_dispatch_pool_use_case = Arc::new(CreateDispatchPoolUseCase::new(dispatch_pool_repo.clone(), unit_of_work.clone()));
    let update_dispatch_pool_use_case = Arc::new(UpdateDispatchPoolUseCase::new(dispatch_pool_repo.clone(), unit_of_work.clone()));
    let archive_dispatch_pool_use_case = Arc::new(ArchiveDispatchPoolUseCase::new(dispatch_pool_repo.clone(), unit_of_work.clone()));
    let delete_dispatch_pool_use_case = Arc::new(DeleteDispatchPoolUseCase::new(dispatch_pool_repo.clone(), unit_of_work.clone()));
    info!("Use cases initialized");

    // Sync code-defined roles to database
    {
        let role_sync = fc_platform::service::RoleSyncService::new(
            RoleRepository::new(&pg_db)
        );
        if let Err(e) = role_sync.sync_code_defined_roles().await {
            tracing::warn!("Role sync failed: {}", e);
        }
    }

    // 8c. Initialize auth services (auto-generate RSA keys for dev, like Java)
    let (private_key, public_key) = AuthConfig::load_or_generate_rsa_keys(None, None)
        .expect("Failed to initialize JWT keys");

    let auth_config = AuthConfig {
        rsa_private_key: Some(private_key),
        rsa_public_key: Some(public_key),
        rsa_public_key_previous: None, // Dev mode: no key rotation
        secret_key: String::new(),
        issuer: std::env::var("FC_JWT_ISSUER")
            .or_else(|_| std::env::var("FC_EXTERNAL_BASE_URL"))
            .or_else(|_| std::env::var("EXTERNAL_BASE_URL"))
            .unwrap_or_else(|_| "http://localhost:8080".to_string()),
        audience: std::env::var("FC_JWT_ISSUER")
            .or_else(|_| std::env::var("FC_EXTERNAL_BASE_URL"))
            .or_else(|_| std::env::var("EXTERNAL_BASE_URL"))
            .unwrap_or_else(|_| "http://localhost:8080".to_string()),
        access_token_expiry_secs: std::env::var("FC_ACCESS_TOKEN_EXPIRY_SECS")
            .ok().and_then(|v| v.parse().ok()).unwrap_or(3600),
        session_token_expiry_secs: std::env::var("FC_SESSION_TOKEN_EXPIRY_SECS")
            .ok().and_then(|v| v.parse().ok()).unwrap_or(28800),
        refresh_token_expiry_secs: std::env::var("FC_REFRESH_TOKEN_EXPIRY_SECS")
            .ok().and_then(|v| v.parse().ok()).unwrap_or(86400 * 30),
    };
    let auth_service = Arc::new(AuthService::new(auth_config));
    let authz_service = Arc::new(AuthorizationService::new(role_repo.clone()));
    let password_service = Arc::new(PasswordService::default());
    info!("Auth services initialized");

    // 7b. Start outbox processor now that AuthService is ready — generate a
    //     long-lived internal service token so the outbox HTTP dispatcher can
    //     authenticate against the SDK batch endpoints.
    let outbox_handle: Option<tokio::task::JoinHandle<()>> = if let Some(pool) = outbox_pool {
        use fc_platform::principal::entity::Principal;

        let internal_principal = Principal::new_service("outbox-processor", "Outbox Processor (internal)");
        let token = auth_service.generate_access_token(&internal_principal)
            .map_err(|e| anyhow::anyhow!("Failed to generate outbox service token: {}", e))?;
        info!("Generated internal service token for outbox processor");

        let repository = Arc::new(PostgresOutboxRepository::new(pool));
        let api_base_url = format!("http://localhost:{}", args.api_port);

        let config = EnhancedProcessorConfig {
            poll_interval: Duration::from_millis(args.outbox_poll_interval_ms),
            http_config: HttpDispatcherConfig {
                api_base_url,
                api_token: Some(token),
                ..Default::default()
            },
            ..Default::default()
        };

        let processor = Arc::new(
            EnhancedOutboxProcessor::new(config, repository)
                .map_err(|e| anyhow::anyhow!("Failed to create outbox processor: {}", e))?
        );

        let proc_clone = processor.clone();
        let mut shutdown_rx = shutdown_tx.subscribe();
        let handle = tokio::spawn(async move {
            tokio::select! {
                _ = processor.start() => {}
                _ = shutdown_rx.recv() => {
                    info!("Outbox processor received shutdown signal");
                    proc_clone.stop();
                }
            }
        });

        info!("Outbox processor started");
        Some(handle)
    } else {
        None
    };

    // 8d. Create AppState for authentication middleware
    let app_state = AppState {
        auth_service: auth_service.clone(),
        authz_service: authz_service.clone(),
    };

    // 8e. Build API states
    let events_state = EventsState { event_repo: event_repo.clone() };
    let sync_event_types_use_case = Arc::new(fc_platform::event_type::operations::SyncEventTypesUseCase::new(event_type_repo.clone()));
    let dispatch_jobs_state = DispatchJobsState { dispatch_job_repo: dispatch_job_repo.clone() };
    let filter_options_state = FilterOptionsState {
        client_repo: client_repo.clone(),
        event_type_repo: event_type_repo.clone(),
        subscription_repo: subscription_repo.clone(),
        dispatch_pool_repo: dispatch_pool_repo.clone(),
        application_repo: application_repo.clone(),
    };
    let event_types_state = EventTypesState {
        event_type_repo: event_type_repo.clone(),
        sync_use_case: sync_event_types_use_case.clone(),
    };
    let audit_service = Arc::new(AuditService::new(audit_log_repo.clone()));
    let clients_state = ClientsState {
        client_repo: client_repo.clone(),
        application_repo: Some(application_repo.clone()),
        application_client_config_repo: Some(application_client_config_repo.clone()),
        audit_service: Some(audit_service.clone()),
    };
    let principals_state = PrincipalsState {
        principal_repo: principal_repo.clone(),
        audit_service: Some(audit_service),
        password_service: None,
        anchor_domain_repo: Some(anchor_domain_repo.clone()),
        client_auth_config_repo: Some(client_auth_config_repo.clone()),
        application_repo: Some(application_repo.clone()),
        app_client_config_repo: Some(application_client_config_repo.clone()),
    };
    let roles_state = RolesState { role_repo: role_repo.clone(), application_repo: Some(application_repo.clone()) };
    let sync_subscriptions_use_case = Arc::new(fc_platform::subscription::operations::SyncSubscriptionsUseCase::new(
        subscription_repo.clone(), connection_repo.clone(), dispatch_pool_repo.clone(),
    ));
    let subscriptions_state = SubscriptionsState { subscription_repo: subscription_repo.clone(), sync_use_case: sync_subscriptions_use_case.clone() };
    let oauth_clients_state = OAuthClientsState { oauth_client_repo: oauth_client_repo.clone() };
    let auth_config_state = AuthConfigState {
        anchor_domain_repo: anchor_domain_repo.clone(),
        client_auth_config_repo: client_auth_config_repo.clone(),
        idp_role_mapping_repo: idp_role_mapping_repo.clone(),
        principal_repo: Some(principal_repo.clone()),
    };
    let audit_logs_state = AuditLogsState { audit_log_repo: audit_log_repo.clone(), principal_repo: principal_repo.clone() };
    // New domain API states
    let connections_state = ConnectionsState { connection_repo };
    let cors_state = CorsState { cors_repo };
    let idp_state = IdentityProvidersState { idp_repo: idp_repo.clone() };
    let edm_state = EmailDomainMappingsState { edm_repo: edm_repo.clone(), idp_repo: idp_repo.clone() };
    let public_api_state = PublicApiState { config_repo: platform_config_repo.clone() };
    let platform_config_state = PlatformConfigState { config_repo: platform_config_repo };
    let config_access_state = ConfigAccessState { access_repo: platform_config_access_repo };
    let login_attempts_state = LoginAttemptsState { login_attempt_repo: login_attempt_repo.clone() };
    let me_state = MeState {
        client_repo: client_repo.clone(),
        application_repo: application_repo.clone(),
        app_client_config_repo: application_client_config_repo.clone(),
    };
    let sdk_events_state = SdkEventsState { event_repo: event_repo.clone() };
    let well_known_state = WellKnownState {
        auth_service: auth_service.clone(),
        external_base_url: format!("http://localhost:{}", args.api_port),
    };
    let client_selection_state = ClientSelectionState {
        principal_repo: principal_repo.clone(),
        client_repo: client_repo.clone(),
        role_repo: role_repo.clone(),
        grant_repo: client_access_grant_repo,
        auth_service: auth_service.clone(),
    };
    let application_roles_sdk_state = ApplicationRolesSdkState {
        application_repo: application_repo.clone(),
        role_repo: role_repo.clone(),
    };
    let email_service: Arc<dyn fc_platform::shared::email_service::EmailService> =
        Arc::from(fc_platform::shared::email_service::create_email_service());
    let password_reset_state = PasswordResetApiState {
        password_reset_repo,
        principal_repo: principal_repo.clone(),
        password_service: password_service.clone(),
        unit_of_work: unit_of_work.clone(),
        email_service: email_service.clone(),
        external_base_url: format!("http://localhost:{}", args.api_port),
    };

    // Auth/OAuth/OIDC states
    let oidc_sync_service = Arc::new(OidcSyncService::new(
        principal_repo.clone(),
        idp_role_mapping_repo.clone(),
    ));
    let oidc_service = Arc::new(OidcService::new());
    let encryption_service = EncryptionService::from_env().map(Arc::new);
    let oidc_login_state = OidcLoginApiState::new(
        anchor_domain_repo.clone(),
        idp_repo.clone(),
        edm_repo.clone(),
        oidc_login_state_repo,
        oidc_sync_service,
        auth_service.clone(),
        unit_of_work.clone(),
    ).with_session_cookie_settings("fc_session", false, "Lax", 86400)
     .with_external_base_url(
         std::env::var("FC_EXTERNAL_BASE_URL")
             .unwrap_or_else(|_| "http://localhost:4200".to_string())
     );
    let oidc_login_state = if let Some(enc_svc) = encryption_service {
        oidc_login_state.with_encryption_service(enc_svc)
    } else {
        warn!("FLOWCATALYST_APP_KEY not set — OIDC client secrets cannot be decrypted");
        oidc_login_state
    };
    let embedded_auth_state = AuthState::new(
        auth_service.clone(),
        principal_repo.clone(),
        password_service.clone(),
        refresh_token_repo.clone(),
        edm_repo.clone(),
        idp_repo.clone(),
        login_attempt_repo.clone(),
    );
    let oauth_state = OAuthState::new(
        oauth_client_repo.clone(),
        principal_repo.clone(),
        auth_service.clone(),
        oidc_service,
        auth_code_repo,
        refresh_token_repo,
        pending_auth_repo,
        password_service,
        login_attempt_repo.clone(),
    );
    let sdk_clients_state = SdkClientsState { client_repo: client_repo.clone(), unit_of_work: unit_of_work.clone() };
    let sdk_principals_state = SdkPrincipalsState { principal_repo: principal_repo.clone(), role_repo: role_repo.clone(), unit_of_work: unit_of_work.clone() };
    let sdk_roles_state = SdkRolesState {
        role_repo: role_repo.clone(),
        application_repo: application_repo.clone(),
    };

    let applications_state = ApplicationsState {
        application_repo: application_repo.clone(),
        service_account_repo: service_account_repo.clone(),
        role_repo: role_repo.clone(),
        client_config_repo: application_client_config_repo.clone(),
        client_repo: client_repo.clone(),
        create_use_case: create_application_use_case,
        update_use_case: update_application_use_case,
        activate_use_case: activate_application_use_case,
        deactivate_use_case: deactivate_application_use_case,
    };
    let sync_dispatch_pools_use_case = Arc::new(fc_platform::dispatch_pool::operations::SyncDispatchPoolsUseCase::new(dispatch_pool_repo.clone()));
    let dispatch_pools_state = DispatchPoolsState {
        dispatch_pool_repo: dispatch_pool_repo.clone(),
        create_use_case: create_dispatch_pool_use_case,
        update_use_case: update_dispatch_pool_use_case,
        archive_use_case: archive_dispatch_pool_use_case,
        delete_use_case: delete_dispatch_pool_use_case,
        sync_use_case: sync_dispatch_pools_use_case.clone(),
    };
    let service_accounts_state = ServiceAccountsState {
        repo: service_account_repo.clone(),
        oauth_client_repo: oauth_client_repo.clone(),
        create_use_case: create_service_account_use_case,
        update_use_case: update_service_account_use_case,
        delete_use_case: delete_service_account_use_case,
        assign_roles_use_case,
        regenerate_token_use_case,
        regenerate_secret_use_case,
    };
    let debug_state = DebugState {
        event_repo: event_repo.clone(),
        dispatch_job_repo: dispatch_job_repo.clone(),
    };

    // SDK sync state
    let sync_roles_use_case = Arc::new(fc_platform::role::operations::SyncRolesUseCase::new(role_repo.clone(), application_repo.clone()));
    let sync_principals_use_case = Arc::new(fc_platform::principal::operations::SyncPrincipalsUseCase::new(principal_repo.clone(), application_repo.clone()));
    let sdk_sync_state = SdkSyncState {
        sync_roles_use_case,
        sync_event_types_use_case: sync_event_types_use_case.clone(),
        sync_subscriptions_use_case: sync_subscriptions_use_case.clone(),
        sync_dispatch_pools_use_case: sync_dispatch_pools_use_case.clone(),
        sync_principals_use_case,
    };
    let sdk_audit_batch_state = SdkAuditBatchState {
        audit_log_repo: audit_log_repo.clone(),
        application_repo: application_repo.clone(),
        client_repo: client_repo.clone(),
    };
    let sdk_dispatch_jobs_state = SdkDispatchJobsState {
        dispatch_job_repo: dispatch_job_repo.clone(),
    };
    let bff_roles_state = BffRolesState {
        role_repo: role_repo.clone(),
        application_repo: Some(application_repo.clone()),
        unit_of_work: unit_of_work.clone(),
    };
    let bff_event_types_state = BffEventTypesState {
        event_type_repo: event_type_repo.clone(),
        application_repo: Some(application_repo.clone()),
        sync_use_case: sync_event_types_use_case.clone(),
        unit_of_work: unit_of_work.clone(),
    };

    // Monitoring state with leader election and circuit breakers
    let monitoring_state = MonitoringState {
        leader_state: LeaderState::new(uuid::Uuid::new_v4().to_string()),
        circuit_breakers: CircuitBreakerRegistry::new(),
        in_flight: InFlightTracker::new(),
        dispatch_job_repo: dispatch_job_repo.clone(),
        start_time: std::time::Instant::now(),
    };

    // 8f. Build platform API router via centralized PlatformRoutes builder
    let routes = PlatformRoutes {
        events: events_state.clone(),
        event_types: event_types_state,
        dispatch_jobs: dispatch_jobs_state.clone(),
        filter_options: filter_options_state.clone(),
        clients: clients_state,
        principals: principals_state,
        roles: roles_state,
        subscriptions: subscriptions_state,
        oauth_clients: oauth_clients_state,
        audit_logs: audit_logs_state,
        monitoring: monitoring_state,
        auth: embedded_auth_state,
        bff_roles: bff_roles_state,
        bff_event_types: bff_event_types_state,
        debug: debug_state,
        auth_config: auth_config_state,
        applications: applications_state,
        dispatch_pools: dispatch_pools_state,
        service_accounts: service_accounts_state,
        connections: connections_state,
        cors: cors_state,
        identity_providers: idp_state,
        email_domain_mappings: edm_state,
        platform_config: platform_config_state,
        config_access: config_access_state,
        login_attempts: login_attempts_state,
        me: me_state,
        sdk_events: sdk_events_state,
        sdk_clients: sdk_clients_state,
        sdk_principals: sdk_principals_state,
        sdk_roles: sdk_roles_state,
        sdk_dispatch_jobs: sdk_dispatch_jobs_state,
        oidc_login: oidc_login_state,
        oauth: oauth_state,
        well_known: well_known_state,
        client_selection: client_selection_state,
        application_roles_sdk: application_roles_sdk_state,
        sdk_sync: sdk_sync_state,
        sdk_audit_batch: sdk_audit_batch_state,
        public: public_api_state,
        password_reset: password_reset_state,
        static_dir: None, // fc-dev handles SPA serving itself (embedded or FC_STATIC_DIR)
    };
    let (platform_app, _openapi) = routes.build();

    // Dev-specific extra routes: admin API mirrors of BFF routes
    // (frontend generated client uses /api/admin/*)
    let platform_router = platform_app
        .nest("/api/admin/events", events_router(events_state).into())
        .nest("/api/admin/events/filter-options", filter_options_router(filter_options_state.clone()).into())
        .nest("/api/admin/dispatch-jobs", dispatch_jobs_router(dispatch_jobs_state).into())
        .nest("/api/admin/event-types/filters", event_type_filters_router(filter_options_state).into())
        // Add auth middleware
        .layer(AuthLayer::new(app_state));

    info!("Platform APIs configured");

    // 9. Start API server (merge router API with platform APIs)
    let router_circuit_breaker = Arc::new(RouterCircuitBreakerRegistry::default());
    let router_api = create_api_router(
        queue.clone(),
        queue_manager.clone(),
        warning_service.clone(),
        health_service.clone(),
        router_circuit_breaker,
    );

    let api_app = Router::new()
        .nest("/q/router", router_api)
        .merge(platform_router)
        .layer(TraceLayer::new_for_http())
        .layer(CorsLayer::new().allow_origin(Any).allow_methods(Any).allow_headers(Any));

    // Static frontend serving — uses FC_STATIC_DIR if set (for live reload),
    // otherwise serves from the embedded frontend assets compiled into the binary.
    let api_app = if let Ok(static_dir) = std::env::var("FC_STATIC_DIR") {
        let index_path = std::path::PathBuf::from(&static_dir).join("index.html");
        if index_path.exists() {
            info!(dir = %static_dir, "Serving frontend from filesystem (live reload)");
            let assets_dir = std::path::PathBuf::from(&static_dir).join("assets");
            let assets_service = tower::ServiceBuilder::new()
                .layer(SetResponseHeaderLayer::overriding(
                    CACHE_CONTROL,
                    HeaderValue::from_static("public, max-age=31536000, immutable"),
                ))
                .service(ServeDir::new(&assets_dir));

            api_app
                .route("/auth/login", axum::routing::get(embedded_spa_handler))
                .route("/auth/forgot-password", axum::routing::get(embedded_spa_handler))
                .route("/auth/reset-password", axum::routing::get(embedded_spa_handler))
                .nest_service("/assets", assets_service)
                .fallback_service(
                    ServeDir::new(&static_dir)
                        .fallback(ServeFile::new(index_path))
                )
        } else {
            warn!(dir = %static_dir, "FC_STATIC_DIR set but index.html not found — using embedded assets");
            api_app.fallback(axum::routing::get(embedded_asset_handler))
        }
    } else {
        info!("Serving embedded frontend (compiled into binary)");
        api_app
            .route("/auth/login", axum::routing::get(embedded_spa_handler))
            .route("/auth/forgot-password", axum::routing::get(embedded_spa_handler))
            .route("/auth/reset-password", axum::routing::get(embedded_spa_handler))
            .fallback(axum::routing::get(embedded_asset_handler))
    };

    let api_addr = format!("0.0.0.0:{}", args.api_port);
    info!("API server listening on http://{}", api_addr);

    let api_listener = TcpListener::bind(&api_addr).await?;
    let api_handle = {
        let mut shutdown_rx = shutdown_tx.subscribe();
        tokio::spawn(async move {
            let server = axum::serve(api_listener, api_app);
            tokio::select! {
                result = server => {
                    if let Err(e) = result {
                        error!("API server error: {}", e);
                    }
                }
                _ = shutdown_rx.recv() => {
                    info!("API server shutting down");
                }
            }
        })
    };

    // 10. Start metrics server
    let metrics_addr = format!("0.0.0.0:{}", args.metrics_port);
    info!("Metrics server listening on http://{}/metrics", metrics_addr);

    let metrics_app = Router::new()
        .route("/metrics", get(metrics_handler))
        .route("/health", get(health_handler));

    let metrics_listener = TcpListener::bind(&metrics_addr).await?;
    let metrics_handle = {
        let mut shutdown_rx = shutdown_tx.subscribe();
        tokio::spawn(async move {
            let server = axum::serve(metrics_listener, metrics_app);
            tokio::select! {
                result = server => {
                    if let Err(e) = result {
                        error!("Metrics server error: {}", e);
                    }
                }
                _ = shutdown_rx.recv() => {
                    info!("Metrics server shutting down");
                }
            }
        })
    };

    // 11. Start QueueManager (blocking - runs consumer loops)
    let manager_handle = {
        let manager = queue_manager.clone();
        let mut shutdown_rx = shutdown_tx.subscribe();
        tokio::spawn(async move {
            tokio::select! {
                result = manager.clone().start() => {
                    if let Err(e) = result {
                        error!("QueueManager error: {}", e);
                    }
                }
                _ = shutdown_rx.recv() => {
                    info!("QueueManager received shutdown signal");
                    manager.shutdown().await;
                }
            }
        })
    };

    info!("FlowCatalyst Dev Monolith started successfully");
    info!("Press Ctrl+C to shutdown");

    // Wait for shutdown signal
    shutdown_signal().await;
    info!("Shutdown signal received, initiating graceful shutdown...");

    // Broadcast shutdown to all components
    let _ = shutdown_tx.send(());

    // Stop lifecycle manager
    lifecycle.shutdown().await;

    // Wait for all handles with timeout
    let shutdown_timeout = Duration::from_secs(30);
    let _ = tokio::time::timeout(shutdown_timeout, async {
        let _ = api_handle.await;
        let _ = metrics_handle.await;
        let _ = manager_handle.await;
        if let Some(h) = outbox_handle {
            let _ = h.await;
        }
    }).await;

    info!("FlowCatalyst Dev Monolith shutdown complete");
    Ok(())
}

async fn metrics_handler() -> &'static str {
    // In a real implementation, you'd use metrics-exporter-prometheus
    // For now, return basic Prometheus format
    "# HELP fc_up FlowCatalyst is up\n# TYPE fc_up gauge\nfc_up 1\n"
}

async fn health_handler() -> Json<serde_json::Value> {
    Json(serde_json::json!({
        "status": "UP",
        "version": env!("CARGO_PKG_VERSION"),
        "components": {
            "queue": "UP",
            "router": "UP"
        }
    }))
}

async fn shutdown_signal() {
    let ctrl_c = async {
        signal::ctrl_c()
            .await
            .expect("failed to install Ctrl+C handler");
    };

    #[cfg(unix)]
    let terminate = async {
        signal::unix::signal(signal::unix::SignalKind::terminate())
            .expect("failed to install signal handler")
            .recv()
            .await;
    };

    #[cfg(not(unix))]
    let terminate = std::future::pending::<()>();

    tokio::select! {
        _ = ctrl_c => {},
        _ = terminate => {},
    }
}

/// Serve embedded frontend assets. Handles all GET requests that don't match API routes.
/// For HTML requests or root, serves index.html (SPA fallback).
/// For asset requests, serves the matching embedded file with correct MIME type.
async fn embedded_asset_handler(uri: axum::http::Uri) -> impl axum::response::IntoResponse {
    let path = uri.path().trim_start_matches('/');

    // Try exact path first (for assets like /assets/index-BKjElYp6.js)
    if let Some(file) = FrontendAssets::get(path) {
        let mime = mime_guess::from_path(path).first_or_octet_stream();
        let mut headers = axum::http::HeaderMap::new();
        headers.insert(
            axum::http::header::CONTENT_TYPE,
            mime.as_ref().parse().unwrap(),
        );
        // Immutable cache for hashed assets
        if path.starts_with("assets/") {
            headers.insert(
                axum::http::header::CACHE_CONTROL,
                "public, max-age=31536000, immutable".parse().unwrap(),
            );
        }
        return (headers, file.data.to_vec()).into_response();
    }

    // SPA fallback: serve index.html for all other paths
    embedded_spa_handler().await.into_response()
}

/// Serve the embedded index.html (SPA entry point).
async fn embedded_spa_handler() -> impl axum::response::IntoResponse {
    match FrontendAssets::get("index.html") {
        Some(file) => {
            let mut headers = axum::http::HeaderMap::new();
            headers.insert(
                axum::http::header::CONTENT_TYPE,
                "text/html; charset=utf-8".parse().unwrap(),
            );
            (headers, file.data.to_vec()).into_response()
        }
        None => (
            axum::http::StatusCode::NOT_FOUND,
            "Frontend not embedded in this build",
        ).into_response(),
    }
}

use axum::response::IntoResponse;
