//! Login Attempt Aggregate
//!
//! Tracks authentication attempts for auditing.

pub mod api;
pub mod entity;
pub mod repository;

pub use api::{login_attempts_router, LoginAttemptsState};
pub use entity::{AttemptType, LoginAttempt, LoginOutcome};
pub use repository::LoginAttemptRepository;
