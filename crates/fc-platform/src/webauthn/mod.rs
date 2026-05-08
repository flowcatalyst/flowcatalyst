//! WebAuthn / Passkeys
//!
//! Public-key credential support for internal-auth users (those whose email
//! domain has no row in `email_domain_mapping`). Federated users never have
//! credentials here — see `project_passkeys_scope.md` for rationale.

pub mod entity;
pub mod repository;
pub mod ceremony_repository;
pub mod webauthn_service;
pub mod gate;
pub mod operations;
pub mod api;

pub use webauthn_service::WebauthnService;
pub use ceremony_repository::WebauthnCeremonyRepository;
pub use api::{webauthn_router, WebauthnApiState};
