//! Email Domain Mapping Operations
//!
//! Use cases for managing email domain mappings.

pub mod create;
pub mod delete;
pub mod events;
pub mod update;

pub use create::{CreateEmailDomainMappingCommand, CreateEmailDomainMappingUseCase};
pub use delete::{DeleteEmailDomainMappingCommand, DeleteEmailDomainMappingUseCase};
pub use events::*;
pub use update::{UpdateEmailDomainMappingCommand, UpdateEmailDomainMappingUseCase};
