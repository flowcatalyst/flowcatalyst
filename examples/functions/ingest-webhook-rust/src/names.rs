//! Everything you rename for a new function lives here. Replace every "changeme".

/// Code of the FlowCatalyst application that owns BOTH functions of the pair.
/// The target function must be in this application AND the same client.
pub const APP_CODE: &str = "changeme";

/// Name (not address) of the webhook function the jobs are delivered to; its
/// address is `<APP_CODE>.<name>`. It is any function with a webhook endpoint
/// at `DELIVER_PATH`, for example payload-logger from the dispatch-demo examples.
pub const TARGET_FUNCTION_NAME: &str = "changeme-processor";
pub const DELIVER_PATH: &str = "/deliver";

/// Dispatch pool and connection every job created by `ctx.dispatch` goes
/// through (declared with `dispatches_with`; a call cannot choose). Codes, never
/// ids. The connection is required and needs a service account that can sign;
/// the pool is optional (drop `.pool(..)` from the declaration to use none).
/// Both must exist on the platform before the first dispatch. Pausing the
/// connection holds the jobs.
pub const DISPATCH_POOL: &str = "changeme-pool";
pub const DISPATCH_CONNECTION: &str = "changeme-connection";

/// This function's public endpoint: `POST <runner or domain>/fn/<address>/ingest`.
pub const INGEST_PATH: &str = "POST /ingest";

/// The shared secret the sender signs with, a declared secret of this function
/// (set it with `fcdev fn set <address> --secret INGEST_SIGNING_SECRET=...`).
pub const SIGNING_SECRET_KEY: &str = "INGEST_SIGNING_SECRET";
/// The header carrying `sha256=` + hex(HMAC-SHA256(secret, raw body)).
pub const SIGNATURE_HEADER: &str = "X-Signature";

/// Shown on the dispatch job. A job's message group also orders delivery: jobs
/// with the same group are delivered one at a time, in order. The sender may
/// pick one with `X-Message-Group`; this is the default.
pub const DEFAULT_MESSAGE_GROUP: &str = "changeme-ingest";
pub const DESCRIPTOR: &str = "changeme webhook payload";

pub fn target_address() -> String {
    format!("{APP_CODE}.{TARGET_FUNCTION_NAME}")
}
