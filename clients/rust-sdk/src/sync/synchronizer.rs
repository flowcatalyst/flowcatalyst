//! `DefinitionSynchronizer` — the orchestrator that pushes a bundled
//! `DefinitionSet` to the platform's per-category sync endpoints.
//!
//! Categories are sync'd in a fixed order so referential FKs already exist
//! by the time later categories arrive:
//!
//! 1. roles
//! 2. event_types
//! 3. dispatch_pools (before subscriptions: the platform silently drops a
//!    subscription's unresolvable `dispatchPoolCode`)
//! 4. connections
//! 5. subscriptions (skipped, with an error, when connections failed)
//! 6. processes
//! 7. scheduled_jobs
//! 8. principals
//! 9. openapi
//! 10. docs
//!
//! Each category is validated locally first (duplicates, code shapes, URLs,
//! crons, ...); an invalid category fails without any request. Each category
//! is an independent HTTP call. A failure in one does **not** abort the
//! rest — the error is captured on the returned [`SyncResult`]'s
//! per-category field and the next category proceeds (the one exception is
//! subscriptions after a failed connections sync). The result is
//! `#[must_use]`: call [`SyncResult::into_result`] so failures are not
//! ignored.
//!
//! No filesystem reflection: this SDK requires programmatic builder use.

use super::definition_set::DefinitionSet;
use super::definitions::{
    ConnectionDefinition, DispatchPoolDefinition, DocDefinition, EventTypeDefinition,
    PrincipalDefinition, ProcessDefinition, RoleDefinition, ScheduledJobDefinition,
    SubscriptionDefinition,
};
use super::options::SyncOptions;
use super::result::{CategorySyncResult, SyncResult};
use super::validate;
use crate::client::docs::SyncDocsRequest;
use crate::client::scheduled_jobs::SyncScheduledJobsRequest;
use crate::client::{
    ClientError, FlowCatalystClient, SyncConnectionsRequest, SyncDispatchPoolsRequest,
    SyncEventTypesRequest, SyncPrincipalsRequest, SyncResult as WireSyncResult, SyncRolesRequest,
    SyncSubscriptionsRequest,
};

/// Orchestrates per-category sync calls against a `FlowCatalystClient`.
///
/// Create one via [`DefinitionSynchronizer::new`] and reuse — the
/// underlying HTTP client is `Clone`-cheap and stateless beyond its auth
/// token.
#[derive(Debug, Clone)]
pub struct DefinitionSynchronizer {
    client: FlowCatalystClient,
    subscription_target_base_url: Option<String>,
}

impl DefinitionSynchronizer {
    pub fn new(client: FlowCatalystClient) -> Self {
        Self {
            client,
            subscription_target_base_url: None,
        }
    }

    /// Base URL that path-style subscription targets (starting with `/`) are
    /// resolved against, e.g. `https://orders.internal` + `/hooks/ship`.
    /// Absolute `http(s)` targets are always used as given.
    pub fn with_subscription_target_base_url(mut self, base_url: impl Into<String>) -> Self {
        self.subscription_target_base_url = Some(base_url.into());
        self
    }

    /// Sync one application's definitions. Returns a per-category result;
    /// call [`SyncResult::into_result`] (or inspect
    /// [`SyncResult::has_errors`] / [`SyncResult::errors`]) to find what
    /// failed.
    pub async fn sync(&self, set: &DefinitionSet, options: &SyncOptions) -> SyncResult {
        let mut out = SyncResult {
            application_code: set.application_code.clone(),
            ..SyncResult::default()
        };
        let app = set.application_code.as_str();
        let remove = options.remove_unlisted;

        if options.sync_roles && set.has_roles() {
            out.roles = Some(self.run_roles(app, &set.roles, remove).await);
        }
        if options.sync_event_types && set.has_event_types() {
            out.event_types = Some(self.run_event_types(app, &set.event_types, remove).await);
        }
        if options.sync_dispatch_pools && set.has_dispatch_pools() {
            out.dispatch_pools = Some(
                self.run_dispatch_pools(app, &set.dispatch_pools, remove)
                    .await,
            );
        }
        if options.sync_connections && set.has_connections() {
            out.connections = Some(
                self.run_connections(app, set.client_id.as_deref(), &set.connections, remove)
                    .await,
            );
        }
        if options.sync_subscriptions && set.has_subscriptions() {
            out.subscriptions = Some(
                match out.connections.as_ref().and_then(|c| c.error.as_ref()) {
                    Some(conn_err) => CategorySyncResult::from_error(format!(
                        "Subscriptions not synced because the connections sync failed: {conn_err}"
                    )),
                    None => {
                        self.run_subscriptions(
                            app,
                            set.client_id.as_deref(),
                            &set.subscriptions,
                            remove,
                        )
                        .await
                    }
                },
            );
        }
        if options.sync_processes && set.has_processes() {
            out.processes = Some(self.run_processes(app, &set.processes, remove).await);
        }
        if options.sync_scheduled_jobs && set.has_scheduled_jobs() {
            out.scheduled_jobs = Some(
                self.run_scheduled_jobs(app, &set.scheduled_jobs, remove)
                    .await,
            );
        }
        if options.sync_principals && set.has_principals() {
            out.principals = Some(self.run_principals(app, &set.principals, remove).await);
        }
        if options.sync_openapi
            && let Some(spec) = set.openapi_spec.clone()
        {
            out.openapi = Some(self.run_openapi(app, spec).await);
        }
        if options.sync_docs && set.has_docs() {
            out.docs = Some(self.run_docs(app, &set.docs).await);
        }

        out
    }

    /// Sync multiple applications sequentially. Returns one result per
    /// input set, in the same order. Per-set errors do not abort the
    /// chain — each set is independent.
    pub async fn sync_all(&self, sets: &[DefinitionSet], options: &SyncOptions) -> Vec<SyncResult> {
        let mut results = Vec::with_capacity(sets.len());
        for set in sets {
            results.push(self.sync(set, options).await);
        }
        results
    }

    // ── per-category runners ───────────────────────────────────────────

    async fn run_roles(
        &self,
        app: &str,
        roles: &[RoleDefinition],
        remove_unlisted: bool,
    ) -> CategorySyncResult {
        if let Err(e) = validate::roles(app, roles) {
            return CategorySyncResult::from_error(e);
        }
        let req = SyncRolesRequest {
            roles: roles
                .iter()
                .cloned()
                .map(|mut r| {
                    r.permissions = r.permissions.iter().map(|p| p.to_lowercase()).collect();
                    r.into_wire()
                })
                .collect(),
        };
        match self.client.roles().sync(app, &req, remove_unlisted).await {
            Ok(r) => from_wire(r),
            Err(e) => CategorySyncResult::from_error(format_error(e)),
        }
    }

    async fn run_event_types(
        &self,
        app: &str,
        event_types: &[EventTypeDefinition],
        remove_unlisted: bool,
    ) -> CategorySyncResult {
        if let Err(e) = validate::event_types(app, event_types) {
            return CategorySyncResult::from_error(e);
        }
        let req = SyncEventTypesRequest {
            event_types: event_types
                .iter()
                .cloned()
                .map(EventTypeDefinition::into_wire)
                .collect(),
        };
        match self
            .client
            .event_types()
            .sync(app, &req, remove_unlisted)
            .await
        {
            Ok(r) => from_wire(r),
            Err(e) => CategorySyncResult::from_error(format_error(e)),
        }
    }

    async fn run_connections(
        &self,
        app: &str,
        client_id: Option<&str>,
        connections: &[ConnectionDefinition],
        remove_unlisted: bool,
    ) -> CategorySyncResult {
        if let Err(e) = validate::connections(app, connections) {
            return CategorySyncResult::from_error(e);
        }
        let req = SyncConnectionsRequest {
            client_id: client_id.map(str::to_string),
            connections: connections
                .iter()
                .cloned()
                .map(ConnectionDefinition::into_wire)
                .collect(),
        };
        match self
            .client
            .connections()
            .sync(app, &req, remove_unlisted)
            .await
        {
            Ok(r) => from_wire(r),
            Err(e) => CategorySyncResult::from_error(format_error(e)),
        }
    }

    async fn run_subscriptions(
        &self,
        app: &str,
        client_id: Option<&str>,
        subscriptions: &[SubscriptionDefinition],
        remove_unlisted: bool,
    ) -> CategorySyncResult {
        let resolved = match validate::subscriptions(
            app,
            subscriptions,
            self.subscription_target_base_url.as_deref(),
        ) {
            Ok(r) => r,
            Err(e) => return CategorySyncResult::from_error(e),
        };
        let req = SyncSubscriptionsRequest {
            client_id: client_id.map(str::to_string),
            subscriptions: resolved
                .into_iter()
                .map(SubscriptionDefinition::into_wire)
                .collect(),
        };
        match self
            .client
            .subscriptions()
            .sync(app, &req, remove_unlisted)
            .await
        {
            Ok(r) => from_wire(r),
            Err(e) => CategorySyncResult::from_error(format_error(e)),
        }
    }

    async fn run_dispatch_pools(
        &self,
        app: &str,
        pools: &[DispatchPoolDefinition],
        remove_unlisted: bool,
    ) -> CategorySyncResult {
        if let Err(e) = validate::dispatch_pools(app, pools) {
            return CategorySyncResult::from_error(e);
        }
        let req = SyncDispatchPoolsRequest {
            pools: pools
                .iter()
                .cloned()
                .map(DispatchPoolDefinition::into_wire)
                .collect(),
        };
        match self
            .client
            .dispatch_pools()
            .sync(app, &req, remove_unlisted)
            .await
        {
            Ok(r) => from_wire(r),
            Err(e) => CategorySyncResult::from_error(format_error(e)),
        }
    }

    async fn run_principals(
        &self,
        app: &str,
        principals: &[PrincipalDefinition],
        remove_unlisted: bool,
    ) -> CategorySyncResult {
        if let Err(e) = validate::principals(app, principals) {
            return CategorySyncResult::from_error(e);
        }
        let req = SyncPrincipalsRequest {
            principals: principals
                .iter()
                .cloned()
                .map(|p| p.into_wire_for_app(app))
                .collect(),
        };
        match self
            .client
            .principals()
            .sync(app, &req, remove_unlisted)
            .await
        {
            Ok(r) => from_wire(r),
            Err(e) => CategorySyncResult::from_error(format_error(e)),
        }
    }

    async fn run_processes(
        &self,
        app: &str,
        processes: &[ProcessDefinition],
        remove_unlisted: bool,
    ) -> CategorySyncResult {
        if let Err(e) = validate::processes(app, processes) {
            return CategorySyncResult::from_error(e);
        }
        let inputs = processes
            .iter()
            .cloned()
            .map(ProcessDefinition::into_wire)
            .collect();
        match self
            .client
            .processes()
            .sync(app, inputs, remove_unlisted)
            .await
        {
            Ok(r) => from_wire(r),
            Err(e) => CategorySyncResult::from_error(format_error(e)),
        }
    }

    async fn run_docs(&self, app: &str, docs: &[DocDefinition]) -> CategorySyncResult {
        if let Err(e) = validate::docs(app, docs) {
            return CategorySyncResult::from_error(e);
        }
        // Declarative full replace: the whole set is sent, no removeUnlisted.
        let req = SyncDocsRequest {
            docs: docs.iter().cloned().map(DocDefinition::into_wire).collect(),
        };
        match self.client.docs().sync(app, &req).await {
            Ok(r) => from_wire(r),
            Err(e) => CategorySyncResult::from_error(format_error(e)),
        }
    }

    /// Jobs are grouped by `client_id` (first-seen order) and one request is
    /// sent per group. Every group is validated before any request is sent.
    async fn run_scheduled_jobs(
        &self,
        app: &str,
        jobs: &[ScheduledJobDefinition],
        archive_unlisted: bool,
    ) -> CategorySyncResult {
        let mut groups: Vec<(Option<&str>, Vec<&ScheduledJobDefinition>)> = Vec::new();
        for job in jobs {
            let key = job.client_id.as_deref();
            match groups.iter_mut().find(|(k, _)| *k == key) {
                Some((_, g)) => g.push(job),
                None => groups.push((key, vec![job])),
            }
        }
        for (client, group) in &groups {
            let scope = client.map(|c| format!(" (client {c})")).unwrap_or_default();
            if let Err(e) = validate::scheduled_jobs(app, &scope, group) {
                return CategorySyncResult::from_error(e);
            }
        }

        let mut merged = CategorySyncResult::default();
        let mut errors: Vec<String> = Vec::new();
        for (client, group) in groups {
            let req = SyncScheduledJobsRequest {
                client_id: client.map(str::to_string),
                jobs: group
                    .into_iter()
                    .cloned()
                    .map(ScheduledJobDefinition::into_wire)
                    .collect(),
                archive_unlisted,
            };
            match self.client.scheduled_jobs().sync(app, &req).await {
                Ok(r) => {
                    // The endpoint returns per-code vectors rather than
                    // counts; synced_codes is created + updated only.
                    merged.created += r.created.len() as u32;
                    merged.updated += r.updated.len() as u32;
                    merged.deleted += r.archived.len() as u32;
                    merged.synced_codes.extend(r.created);
                    merged.synced_codes.extend(r.updated);
                }
                Err(e) => errors.push(match client {
                    Some(c) => format!("client {c}: {}", format_error(e)),
                    None => format_error(e),
                }),
            }
        }
        if !errors.is_empty() {
            merged.error = Some(errors.join("; "));
        }
        merged
    }

    async fn run_openapi(&self, app: &str, spec: serde_json::Value) -> CategorySyncResult {
        match self.client.openapi().sync(app, spec).await {
            Ok(r) => {
                // OpenAPI sync is one-shot: report 1 row created/updated
                // (depending on whether content changed) and pass back
                // the published version in `synced_codes`.
                let (created, updated) = if r.unchanged {
                    (0, 0)
                } else if r.archived_prior_version.is_some() {
                    (0, 1)
                } else {
                    (1, 0)
                };
                CategorySyncResult {
                    created,
                    updated,
                    deleted: 0,
                    synced_codes: vec![r.version],
                    password_hash_ignored: Vec::new(),
                    skipped_roles: Vec::new(),
                    error: None,
                }
            }
            Err(e) => CategorySyncResult::from_error(format_error(e)),
        }
    }
}

fn from_wire(r: WireSyncResult) -> CategorySyncResult {
    CategorySyncResult {
        created: r.created,
        updated: r.updated,
        deleted: r.deleted,
        synced_codes: r.synced_codes,
        password_hash_ignored: r.password_hash_ignored,
        skipped_roles: r.skipped_roles,
        error: None,
    }
}

/// Render a `ClientError` as a single-line message suitable for storing
/// on `CategorySyncResult::error`. The API-error variant carries an HTTP
/// status plus the response body, which is often the most informative
/// part of a sync failure.
fn format_error(err: ClientError) -> String {
    err.to_string()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::sync::definitions::ProcessDefinition;

    /// Construct a synchronizer pointed at an unreachable host so every
    /// HTTP call fails — we use this to verify the orchestrator's
    /// fail-soft semantics without spinning up a real server.
    fn dummy_synchronizer() -> DefinitionSynchronizer {
        let client = FlowCatalystClient::new("http://127.0.0.1:1") // unbound
            .with_token("test-token");
        DefinitionSynchronizer::new(client)
    }

    #[tokio::test]
    async fn empty_set_skips_every_category() {
        let sync = dummy_synchronizer();
        let set = DefinitionSet::for_application("orders");
        let result = sync.sync(&set, &SyncOptions::defaults()).await;
        assert_eq!(result.application_code, "orders");
        assert!(result.roles.is_none());
        assert!(result.event_types.is_none());
        assert!(result.subscriptions.is_none());
        assert!(result.dispatch_pools.is_none());
        assert!(result.principals.is_none());
        assert!(result.processes.is_none());
        assert!(result.scheduled_jobs.is_none());
        assert!(result.openapi.is_none());
        assert!(!result.has_changes());
        assert!(!result.has_errors());
    }

    #[tokio::test]
    async fn category_disabled_via_options_is_skipped_even_when_populated() {
        let sync = dummy_synchronizer();
        let set = DefinitionSet::for_application("orders")
            .add_process(ProcessDefinition::make("orders:f:flow", "Flow"));

        let options = SyncOptions {
            sync_processes: false,
            ..SyncOptions::defaults()
        };
        let result = sync.sync(&set, &options).await;
        assert!(result.processes.is_none());
    }

    #[tokio::test]
    async fn transport_failure_is_captured_per_category_not_propagated() {
        let sync = dummy_synchronizer();
        // Single category populated. The unbound port guarantees ClientError.
        let set = DefinitionSet::for_application("orders")
            .add_process(ProcessDefinition::make("orders:f:flow", "Flow"));
        let result = sync.sync(&set, &SyncOptions::defaults()).await;

        // The processes call ran and recorded an error.
        let processes = result.processes.as_ref().expect("processes ran");
        assert!(processes.is_error());
        assert_eq!(processes.created, 0);

        // Aggregate helpers report the error without panicking.
        assert!(result.has_errors());
        let errors = result.errors();
        assert_eq!(errors.len(), 1);
        assert_eq!(errors[0].0, "processes");
    }

    #[tokio::test]
    async fn sync_all_preserves_input_order() {
        let sync = dummy_synchronizer();
        let sets = vec![
            DefinitionSet::for_application("alpha"),
            DefinitionSet::for_application("beta"),
            DefinitionSet::for_application("gamma"),
        ];
        let results = sync.sync_all(&sets, &SyncOptions::defaults()).await;
        assert_eq!(results.len(), 3);
        assert_eq!(results[0].application_code, "alpha");
        assert_eq!(results[1].application_code, "beta");
        assert_eq!(results[2].application_code, "gamma");
    }

    // ── stub-backed tests ──────────────────────────────────────────────

    use crate::client::test_support::MockPlatform;

    const OK: &str =
        r#"{"applicationCode":"orders","created":1,"updated":0,"deleted":0,"syncedCodes":["x"]}"#;

    fn route(path: &str) -> (&'static str, String, u16, &'static str) {
        (
            "POST",
            format!("/api/applications/orders/{path}/sync"),
            200,
            OK,
        )
    }

    async fn stub_all() -> MockPlatform {
        let routes: Vec<_> = [
            "roles",
            "event-types",
            "dispatch-pools",
            "connections",
            "subscriptions",
            "processes",
            "principals",
            "docs",
        ]
        .iter()
        .map(|p| route(p))
        .collect();
        let refs: Vec<(&str, &str, u16, &str)> = routes
            .iter()
            .map(|(m, p, s, b)| (*m, p.as_str(), *s, *b))
            .collect();
        MockPlatform::start(&refs).await
    }

    fn full_set() -> DefinitionSet {
        DefinitionSet::for_application("orders")
            .add_role(RoleDefinition::make("admin").with_permissions(vec!["Orders:A:B:C".into()]))
            .add_event_type(EventTypeDefinition::make("orders:a:b:c", "E"))
            .add_dispatch_pool(DispatchPoolDefinition::make("p", "P"))
            .add_connection(ConnectionDefinition::make("conn", "Conn"))
            .add_subscription(
                SubscriptionDefinition::make("s", "S", "/hook").with_connection_code("conn"),
            )
            .add_process(ProcessDefinition::make("orders:a:flow", "F"))
            .add_principal(
                PrincipalDefinition::make("a@example.com", "A").with_roles(vec!["admin".into()]),
            )
            .add_doc(DocDefinition::make("intro", "# Hi"))
    }

    #[tokio::test]
    async fn categories_run_in_order_with_pools_before_subscriptions() {
        let stub = stub_all().await;
        let sync = DefinitionSynchronizer::new(stub.client())
            .with_subscription_target_base_url("https://orders.example");
        let result = sync.sync(&full_set(), &SyncOptions::defaults()).await;
        assert!(!result.has_errors(), "{:?}", result.errors());
        let paths: Vec<String> = stub.requests().into_iter().map(|r| r.path).collect();
        let tail: Vec<&str> = paths
            .iter()
            .map(|p| p.rsplit('/').nth(1).unwrap_or(""))
            .collect();
        assert_eq!(
            tail,
            [
                "roles",
                "event-types",
                "dispatch-pools",
                "connections",
                "subscriptions",
                "processes",
                "principals",
                "docs"
            ]
        );
        let reqs = stub.requests();
        // roles lowercased, subscription target resolved, principal role prefixed
        assert_eq!(reqs[0].json()["roles"][0]["permissions"][0], "orders:a:b:c");
        assert_eq!(
            reqs[4].json()["subscriptions"][0]["target"],
            "https://orders.example/hook"
        );
        assert_eq!(reqs[6].json()["principals"][0]["roles"][0], "orders:admin");
        assert!(result.into_result().is_ok());
    }

    #[tokio::test]
    async fn failed_connections_skip_subscriptions_and_name_the_failure() {
        let stub = MockPlatform::start(&[(
            "POST",
            "/api/applications/orders/connections/sync",
            400,
            r#"{"code":"BAD","message":"nope"}"#,
        )])
        .await;
        let set = DefinitionSet::for_application("orders")
            .add_connection(ConnectionDefinition::make("conn", "Conn"))
            .add_subscription(SubscriptionDefinition::make(
                "s",
                "S",
                "https://x.example/h",
            ));
        let result = DefinitionSynchronizer::new(stub.client())
            .sync(&set, &SyncOptions::defaults())
            .await;
        assert_eq!(stub.requests().len(), 1, "subscriptions must not be sent");
        let sub = result.subscriptions.as_ref().unwrap();
        assert!(
            sub.error
                .as_ref()
                .unwrap()
                .contains("connections sync failed")
        );
        assert!(result.connections.as_ref().unwrap().is_error());
        assert!(result.into_result().is_err());
    }

    #[tokio::test]
    async fn invalid_categories_fail_locally_without_requests() {
        let stub = MockPlatform::start(&[]).await;
        let set = DefinitionSet::for_application("orders")
            .add_role(RoleDefinition::make("a").with_permissions(vec!["billing:x:y:z".into()]))
            .add_event_type(EventTypeDefinition::make("orders:a:b", "E"))
            .add_connection(ConnectionDefinition::make("c", "C"))
            .add_connection(ConnectionDefinition::make("c", "C"))
            .add_subscription(SubscriptionDefinition::make("s", "S", "/relative"))
            .add_doc(DocDefinition::make("Bad_Slug", "x"))
            .add_scheduled_job(ScheduledJobDefinition::make("job", "J").add_cron("* * * * *"));
        let result = DefinitionSynchronizer::new(stub.client())
            .sync(&set, &SyncOptions::defaults())
            .await;
        assert!(stub.requests().is_empty());
        let failed: Vec<&str> = result.errors().iter().map(|(c, _)| *c).collect();
        assert_eq!(
            failed,
            [
                "roles",
                "eventTypes",
                "connections",
                "subscriptions",
                "scheduledJobs",
                "docs"
            ]
        );
        let jobs = result.scheduled_jobs.as_ref().unwrap();
        assert!(jobs.error.as_ref().unwrap().contains("job"));
    }

    #[tokio::test]
    async fn scheduled_jobs_group_by_client_and_merge_created_updated_only() {
        let stub = MockPlatform::start(&[(
            "POST",
            "/api/applications/orders/scheduled-jobs/sync",
            200,
            r#"{"applicationCode":"orders","created":["a"],"updated":[],"archived":["old"]}"#,
        )])
        .await;
        let cron = "0 0 * * * *";
        let set = DefinitionSet::for_application("orders")
            .add_scheduled_job(ScheduledJobDefinition::make("a", "A").add_cron(cron))
            .add_scheduled_job(
                ScheduledJobDefinition::make("a", "A")
                    .add_cron(cron)
                    .with_client_id("clt_1"),
            );
        let result = DefinitionSynchronizer::new(stub.client())
            .sync(&set, &SyncOptions::scheduled_jobs_only())
            .await;
        let reqs = stub.requests();
        assert_eq!(reqs.len(), 2);
        assert!(reqs[0].json().get("clientId").is_none());
        assert_eq!(reqs[1].json()["clientId"], "clt_1");
        assert!(reqs[1].json()["jobs"][0].get("clientId").is_none());
        let jobs = result.scheduled_jobs.unwrap();
        assert_eq!((jobs.created, jobs.deleted), (2, 2));
        assert_eq!(jobs.synced_codes, vec!["a".to_string(), "a".to_string()]);
    }

    #[tokio::test]
    async fn connections_send_client_id_and_remove_unlisted() {
        let stub = stub_all().await;
        let set = DefinitionSet::for_client("orders", "clt_9")
            .add_connection(ConnectionDefinition::make("c", "C"));
        let result = DefinitionSynchronizer::new(stub.client())
            .sync(&set, &SyncOptions::with_remove_unlisted())
            .await;
        assert!(!result.has_errors());
        let r = stub.single();
        assert_eq!(r.query, "removeUnlisted=true");
        assert_eq!(r.json()["clientId"], "clt_9");
    }
}
