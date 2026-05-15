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
    DispatchPoolDefinition, EventTypeDefinition, PrincipalDefinition, ProcessDefinition,
    RoleDefinition, SubscriptionDefinition,
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
    pub principals: Vec<PrincipalDefinition>,
    pub processes: Vec<ProcessDefinition>,
}

impl DefinitionSet {
    /// Start a new set for `application_code`.
    pub fn for_application(application_code: impl Into<String>) -> Self {
        Self {
            application_code: application_code.into(),
            ..Self::default()
        }
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

    pub fn has_principals(&self) -> bool {
        !self.principals.is_empty()
    }

    pub fn has_processes(&self) -> bool {
        !self.processes.is_empty()
    }

    pub fn is_empty(&self) -> bool {
        !self.has_roles()
            && !self.has_event_types()
            && !self.has_subscriptions()
            && !self.has_dispatch_pools()
            && !self.has_principals()
            && !self.has_processes()
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
