//! Multi-category sync orchestrator.
//!
//! Build a [`DefinitionSet`] for one application using the per-category
//! [`*Definition`](definitions) builders, then push it through
//! [`DefinitionSynchronizer::sync`]:
//!
//! ```ignore
//! use fc_sdk::client::FlowCatalystClient;
//! use fc_sdk::sync::{
//!     DefinitionSet, DefinitionSynchronizer, EventTypeDefinition,
//!     RoleDefinition, SyncOptions,
//! };
//!
//! # async fn run() {
//! let client = FlowCatalystClient::new("https://platform.example.com")
//!     .with_token("…");
//! let sync = DefinitionSynchronizer::new(client);
//!
//! let set = DefinitionSet::for_application("orders")
//!     .add_role(RoleDefinition::make("admin").with_display_name("Administrator"))
//!     .add_event_type(EventTypeDefinition::make(
//!         "orders:fulfilment:shipment:shipped",
//!         "Shipment Shipped",
//!     ));
//!
//! let result = sync.sync(&set, &SyncOptions::with_remove_unlisted()).await;
//! if result.has_errors() {
//!     for (category, msg) in result.errors() {
//!         eprintln!("{category}: {msg}");
//!     }
//! }
//! # }
//! ```
//!
//! Failures are captured per-category on the returned [`SyncResult`] rather
//! than propagated — one round-trip, full visibility.
//!
//! For one-off per-category sync without bundling, use the lower-level
//! [`crate::client::sync`] wrappers directly.

mod definition_set;
mod definitions;
mod options;
mod result;
mod synchronizer;

pub use definition_set::DefinitionSet;
pub use definitions::{
    DispatchPoolDefinition, EventTypeDefinition, PrincipalDefinition, ProcessDefinition,
    RoleDefinition, SubscriptionBinding, SubscriptionDefinition,
};
pub use options::SyncOptions;
pub use result::{CategorySyncResult, SyncResult};
pub use synchronizer::DefinitionSynchronizer;
