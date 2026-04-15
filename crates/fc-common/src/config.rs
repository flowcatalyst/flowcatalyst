//! Shared environment variable helper functions.
//!
//! All binaries should use these instead of defining their own.

use std::str::FromStr;

/// Read an env var or return the default.
pub fn env_or(key: &str, default: &str) -> String {
    std::env::var(key).unwrap_or_else(|_| default.to_string())
}

/// Read an env var, trying the primary key first, then an alias.
/// This allows both TS-style (`PORT`) and Rust-style (`FC_API_PORT`) env vars.
pub fn env_or_alias(primary: &str, alias: &str, default: &str) -> String {
    std::env::var(primary)
        .or_else(|_| std::env::var(alias))
        .unwrap_or_else(|_| default.to_string())
}

/// Read an env var and parse it, or return the default.
pub fn env_or_parse<T: FromStr>(key: &str, default: T) -> T {
    std::env::var(key)
        .ok()
        .and_then(|v| v.parse().ok())
        .unwrap_or(default)
}

/// Read an env var (with alias) and parse it, or return the default.
pub fn env_or_alias_parse<T: FromStr>(primary: &str, alias: &str, default: T) -> T {
    std::env::var(primary)
        .or_else(|_| std::env::var(alias))
        .ok()
        .and_then(|v| v.parse().ok())
        .unwrap_or(default)
}

/// Read an env var as a boolean (`"true"` or `"1"` → true), or return the default.
pub fn env_bool(key: &str, default: bool) -> bool {
    std::env::var(key)
        .ok()
        .map(|v| v == "true" || v == "1")
        .unwrap_or(default)
}

/// Read an env var as a boolean with an alias fallback.
pub fn env_bool_alias(primary: &str, alias: &str, default: bool) -> bool {
    std::env::var(primary)
        .or_else(|_| std::env::var(alias))
        .ok()
        .map(|v| v == "true" || v == "1")
        .unwrap_or(default)
}

/// Read a required env var, returning an error if missing.
pub fn env_required(key: &str) -> anyhow::Result<String> {
    std::env::var(key).map_err(|_| anyhow::anyhow!("{} environment variable is required", key))
}
