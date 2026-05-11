//! Role Operations
//!
//! Use cases for role management.

pub mod create;
pub mod delete;
pub mod events;
pub mod sync;
pub mod update;

pub use create::{CreateRoleCommand, CreateRoleUseCase};
pub use delete::{DeleteRoleCommand, DeleteRoleUseCase};
pub use events::*;
pub use sync::{SyncRoleInput, SyncRolesCommand, SyncRolesUseCase};
pub use update::{UpdateRoleCommand, UpdateRoleUseCase};
