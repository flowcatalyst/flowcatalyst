//! Platform Event Type Definitions
//!
//! Canonical list of platform domain events. Codes match the EVENT_TYPE
//! constants on each domain event struct exactly. Used by the BFF
//! sync-platform endpoint and the dev seeder.

use std::collections::HashMap;
use serde_json::Value;
use crate::event_type::operations::SyncEventTypeInput;
use super::platform_event_schemas;

/// Returns all platform domain event type definitions with JSON schemas.
///
/// Two subdomains:
///
///   platform:iam:*   — Identity & access management (users, service accounts,
///                      clients, roles, applications, anchor domains, auth configs)
///   platform:admin:* — Platform administration (CORS, identity providers,
///                      email domain mappings, event types, connections,
///                      dispatch pools, subscriptions)
pub fn definitions() -> Vec<SyncEventTypeInput> {
    let mut defs = Vec::new();
    let schemas = platform_event_schemas::schemas();

    // ─── platform:iam ─────────────────────────────────────────────────

    // User
    group(&mut defs, &schemas, "platform:iam:user", &[
        "created", "updated", "activated", "deactivated", "deleted",
        "roles-assigned", "application-access-assigned",
        "client-access-granted", "client-access-revoked",
        "logged-in", "password-reset-requested", "password-reset-completed",
    ]);
    // Principals (sync — aggregate is plural)
    push(&mut defs, &schemas, "platform:iam:principals:synced", "Principals Synced");

    // Service Account (no hyphen in aggregate)
    group(&mut defs, &schemas, "platform:iam:serviceaccount", &[
        "created", "updated", "deleted",
        "roles-assigned", "token-regenerated", "secret-regenerated",
    ]);

    // Client
    group(&mut defs, &schemas, "platform:iam:client", &[
        "created", "updated", "activated", "suspended", "deleted", "note-added",
    ]);

    // Role
    group(&mut defs, &schemas, "platform:iam:role", &[
        "created", "updated", "deleted",
    ]);
    push(&mut defs, &schemas, "platform:iam:roles:synced", "Roles Synced");

    // Application
    group(&mut defs, &schemas, "platform:iam:application", &[
        "created", "updated", "activated", "deactivated", "deleted",
        "service-account-provisioned",
        "enabled-for-client", "disabled-for-client",
    ]);

    // Anchor Domain
    group(&mut defs, &schemas, "platform:iam:anchor-domain", &[
        "created", "deleted",
    ]);

    // Auth Config
    group(&mut defs, &schemas, "platform:iam:auth-config", &[
        "created", "updated", "deleted",
    ]);

    // ─── platform:admin ───────────────────────────────────────────────

    // CORS
    group(&mut defs, &schemas, "platform:admin:cors", &[
        "origin-added", "origin-deleted",
    ]);

    // Identity Provider
    group(&mut defs, &schemas, "platform:admin:idp", &[
        "created", "updated", "deleted",
    ]);

    // Email Domain Mapping
    group(&mut defs, &schemas, "platform:admin:edm", &[
        "created", "updated", "deleted",
    ]);

    // Event Type (no hyphen in aggregate)
    group(&mut defs, &schemas, "platform:admin:eventtype", &[
        "created", "updated", "archived", "deleted",
        "schema-added", "schema-finalised", "schema-deprecated",
    ]);
    push(&mut defs, &schemas, "platform:admin:eventtypes:synced", "Event Types Synced");

    // Connection
    group(&mut defs, &schemas, "platform:admin:connection", &[
        "created", "updated", "deleted",
    ]);

    // Dispatch Pool
    group(&mut defs, &schemas, "platform:admin:dispatch-pool", &[
        "created", "updated", "archived", "deleted",
    ]);
    push(&mut defs, &schemas, "platform:admin:dispatch-pools:synced", "Dispatch Pools Synced");

    // Subscription
    group(&mut defs, &schemas, "platform:admin:subscription", &[
        "created", "updated", "paused", "resumed", "deleted", "synced",
    ]);

    defs
}

/// Add a group of events under the same prefix.
fn group(
    defs: &mut Vec<SyncEventTypeInput>,
    schemas: &HashMap<&str, Value>,
    prefix: &str,
    events: &[&str],
) {
    for event in events {
        let code = format!("{}:{}", prefix, event);
        let aggregate = prefix.rsplit(':').next().unwrap_or(prefix);
        let name = format!("{} {}", title_case(aggregate), title_case(event));
        let schema = schemas.get(code.as_str()).cloned();
        defs.push(SyncEventTypeInput {
            code,
            name,
            description: None,
            schema,
        });
    }
}

/// Add a single event type.
fn push(
    defs: &mut Vec<SyncEventTypeInput>,
    schemas: &HashMap<&str, Value>,
    code: &str,
    name: &str,
) {
    defs.push(SyncEventTypeInput {
        code: code.to_string(),
        name: name.to_string(),
        description: None,
        schema: schemas.get(code).cloned(),
    });
}

fn title_case(s: &str) -> String {
    s.split('-')
        .map(|w| {
            let mut c = w.chars();
            match c.next() {
                None => String::new(),
                Some(f) => f.to_uppercase().collect::<String>() + c.as_str(),
            }
        })
        .collect::<Vec<_>>()
        .join(" ")
}
