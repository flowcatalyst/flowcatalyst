//! `DefinitionSet` — one application's bundled definitions.
//!
//! Build a set programmatically with `DefinitionSet::for_application(code)`
//! plus chained `with_*` / `add_*` methods, then hand it to
//! [`crate::sync::DefinitionSynchronizer::sync`] for orchestrated push.
//!
//! Mirrors the Laravel SDK's `SyncDefinitionSet` and the TypeScript SDK's
//! `DefinitionSetBuilder` so application authors describe definitions the
//! same way across all three SDKs.

use super::definitions::{
    ConnectionDefinition, DispatchPoolDefinition, DocDefinition, EventTypeDefinition,
    PrincipalDefinition, ProcessDefinition, RoleDefinition, ScheduledJobDefinition,
    SubscriptionDefinition,
};

/// Container for all definitions belonging to one application.
///
/// `DefinitionSet` is both the data structure and the builder — append more
/// definitions with `with_*` / `add_*`; the methods consume and return
/// `self` for fluent chaining.
#[derive(Debug, Clone, Default)]
pub struct DefinitionSet {
    pub application_code: String,
    pub roles: Vec<RoleDefinition>,
    pub event_types: Vec<EventTypeDefinition>,
    pub subscriptions: Vec<SubscriptionDefinition>,
    pub dispatch_pools: Vec<DispatchPoolDefinition>,
    pub connections: Vec<ConnectionDefinition>,
    pub docs: Vec<DocDefinition>,
    /// Scope for the connections and subscriptions requests (`clientId` in
    /// the body). `None` syncs the application's global rows. Scheduled jobs
    /// are scoped per job (`ScheduledJobDefinition::client_id`) instead.
    pub client_id: Option<String>,
    pub principals: Vec<PrincipalDefinition>,
    pub processes: Vec<ProcessDefinition>,
    pub scheduled_jobs: Vec<ScheduledJobDefinition>,
    /// OpenAPI document for this application, as parsed JSON. Optional —
    /// only included if the consumer wants to publish their REST surface
    /// to the platform's catalogue. Per-application: a fresh sync
    /// replaces the previous version.
    pub openapi_spec: Option<serde_json::Value>,
}

impl DefinitionSet {
    /// Start a new set for `application_code`.
    pub fn for_application(application_code: impl Into<String>) -> Self {
        Self {
            application_code: application_code.into(),
            ..Self::default()
        }
    }

    /// Like [`for_application`](Self::for_application), scoping the
    /// connections and subscriptions requests to `client_id`.
    pub fn for_client(application_code: impl Into<String>, client_id: impl Into<String>) -> Self {
        Self {
            client_id: Some(client_id.into()),
            ..Self::for_application(application_code)
        }
    }

    pub fn with_client_id(mut self, client_id: impl Into<String>) -> Self {
        self.client_id = Some(client_id.into());
        self
    }

    pub fn with_connections(mut self, connections: Vec<ConnectionDefinition>) -> Self {
        self.connections = connections;
        self
    }

    pub fn add_connection(mut self, connection: ConnectionDefinition) -> Self {
        self.connections.push(connection);
        self
    }

    pub fn with_docs(mut self, docs: Vec<DocDefinition>) -> Self {
        self.docs = docs;
        self
    }

    pub fn add_doc(mut self, doc: DocDefinition) -> Self {
        self.docs.push(doc);
        self
    }

    pub fn with_roles(mut self, roles: Vec<RoleDefinition>) -> Self {
        self.roles = roles;
        self
    }

    pub fn add_role(mut self, role: RoleDefinition) -> Self {
        self.roles.push(role);
        self
    }

    pub fn with_event_types(mut self, event_types: Vec<EventTypeDefinition>) -> Self {
        self.event_types = event_types;
        self
    }

    pub fn add_event_type(mut self, event_type: EventTypeDefinition) -> Self {
        self.event_types.push(event_type);
        self
    }

    pub fn with_subscriptions(mut self, subscriptions: Vec<SubscriptionDefinition>) -> Self {
        self.subscriptions = subscriptions;
        self
    }

    pub fn add_subscription(mut self, subscription: SubscriptionDefinition) -> Self {
        self.subscriptions.push(subscription);
        self
    }

    pub fn with_dispatch_pools(mut self, dispatch_pools: Vec<DispatchPoolDefinition>) -> Self {
        self.dispatch_pools = dispatch_pools;
        self
    }

    pub fn add_dispatch_pool(mut self, dispatch_pool: DispatchPoolDefinition) -> Self {
        self.dispatch_pools.push(dispatch_pool);
        self
    }

    pub fn with_principals(mut self, principals: Vec<PrincipalDefinition>) -> Self {
        self.principals = principals;
        self
    }

    pub fn add_principal(mut self, principal: PrincipalDefinition) -> Self {
        self.principals.push(principal);
        self
    }

    pub fn with_processes(mut self, processes: Vec<ProcessDefinition>) -> Self {
        self.processes = processes;
        self
    }

    pub fn add_process(mut self, process: ProcessDefinition) -> Self {
        self.processes.push(process);
        self
    }

    pub fn with_scheduled_jobs(mut self, scheduled_jobs: Vec<ScheduledJobDefinition>) -> Self {
        self.scheduled_jobs = scheduled_jobs;
        self
    }

    pub fn add_scheduled_job(mut self, scheduled_job: ScheduledJobDefinition) -> Self {
        self.scheduled_jobs.push(scheduled_job);
        self
    }

    /// Attach an OpenAPI document (parsed JSON) to be published alongside
    /// the rest of the application's definitions on next sync.
    pub fn with_openapi_spec(mut self, spec: serde_json::Value) -> Self {
        self.openapi_spec = Some(spec);
        self
    }

    pub fn has_roles(&self) -> bool {
        !self.roles.is_empty()
    }

    pub fn has_event_types(&self) -> bool {
        !self.event_types.is_empty()
    }

    pub fn has_subscriptions(&self) -> bool {
        !self.subscriptions.is_empty()
    }

    pub fn has_dispatch_pools(&self) -> bool {
        !self.dispatch_pools.is_empty()
    }

    pub fn has_connections(&self) -> bool {
        !self.connections.is_empty()
    }

    pub fn has_docs(&self) -> bool {
        !self.docs.is_empty()
    }

    pub fn has_principals(&self) -> bool {
        !self.principals.is_empty()
    }

    pub fn has_processes(&self) -> bool {
        !self.processes.is_empty()
    }

    pub fn has_scheduled_jobs(&self) -> bool {
        !self.scheduled_jobs.is_empty()
    }

    pub fn has_openapi_spec(&self) -> bool {
        self.openapi_spec.is_some()
    }

    pub fn is_empty(&self) -> bool {
        !self.has_roles()
            && !self.has_event_types()
            && !self.has_subscriptions()
            && !self.has_dispatch_pools()
            && !self.has_connections()
            && !self.has_docs()
            && !self.has_principals()
            && !self.has_processes()
            && !self.has_scheduled_jobs()
            && !self.has_openapi_spec()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn empty_set_reports_empty() {
        let set = DefinitionSet::for_application("orders");
        assert!(set.is_empty());
        assert_eq!(set.application_code, "orders");
    }

    #[test]
    fn fluent_chain_accumulates() {
        let set = DefinitionSet::for_application("orders")
            .add_role(RoleDefinition::make("admin"))
            .add_event_type(EventTypeDefinition::make(
                "orders:fulfilment:shipment:shipped",
                "Shipment Shipped",
            ))
            .add_process(ProcessDefinition::make(
                "orders:fulfilment:flow",
                "Fulfilment Flow",
            ));

        assert!(!set.is_empty());
        assert!(set.has_roles());
        assert!(set.has_event_types());
        assert!(set.has_processes());
        assert!(!set.has_principals());
        assert_eq!(set.roles.len(), 1);
        assert_eq!(set.event_types.len(), 1);
        assert_eq!(set.processes.len(), 1);
    }

    #[test]
    fn with_replaces_add_appends() {
        let set = DefinitionSet::for_application("app")
            .add_role(RoleDefinition::make("first"))
            .add_role(RoleDefinition::make("second"));
        assert_eq!(set.roles.len(), 2);

        let replaced = set.with_roles(vec![RoleDefinition::make("only")]);
        assert_eq!(replaced.roles.len(), 1);
        assert_eq!(replaced.roles[0].name, "only");
    }
}
