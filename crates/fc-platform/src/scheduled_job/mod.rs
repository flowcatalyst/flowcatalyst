//! Scheduled Job Aggregate
//!
//! Cron-triggered webhook jobs with optional completion callbacks. The
//! ScheduledJob *definition* is a full DDD aggregate with use cases, domain
//! events and audit logs. Each firing produces a *ScheduledJobInstance* —
//! these are platform-infrastructure plumbing rows (like dispatch-job
//! lifecycle), so they bypass UoW. Same for instance log entries.

pub mod entity;
pub mod repository;
pub mod instance_repository;
pub mod operations;
pub mod api;
pub mod scheduler;

pub use entity::{
    CompletionStatus, InstanceStatus, LogLevel, ScheduledJob, ScheduledJobInstance,
    ScheduledJobInstanceLog, ScheduledJobStatus, TriggerKind,
};
pub use repository::ScheduledJobRepository;
pub use instance_repository::{InstanceListFilters, ScheduledJobInstanceRepository};
