//! Platform Event Schemas
//!
//! JSON Schemas for all platform domain event payloads. These describe the
//! non-envelope fields of each event (i.e., everything except EventMetadata).
//!
//! Schema keys match the EVENT_TYPE constants on the domain event structs
//! exactly. No webhook delivery events are registered here — emitting
//! events about webhook delivery would create circular dispatch loops.
//!
//! Schemas follow JSON Schema draft-07.

use serde_json::{json, Value};
use std::collections::HashMap;

/// Returns a map of event type code → JSON Schema for the event payload.
pub fn schemas() -> HashMap<&'static str, Value> {
    let mut m = HashMap::new();

    // ─── platform:iam:user ─────────────────────────────────────────────
    m.insert(
        "platform:iam:user:created",
        obj(&[
            req_str("principalId"),
            req_str("email"),
            req_str("emailDomain"),
            req_str("name"),
            req_str("scope"),
            opt_str("clientId"),
            req_bool("isAnchorUser"),
        ]),
    );
    m.insert(
        "platform:iam:user:updated",
        obj(&[req_str("principalId"), opt_str("name"), opt_str("email")]),
    );
    m.insert(
        "platform:iam:user:activated",
        obj(&[req_str("principalId")]),
    );
    m.insert(
        "platform:iam:user:deactivated",
        obj(&[req_str("principalId"), opt_str("reason")]),
    );
    m.insert("platform:iam:user:deleted", obj(&[req_str("principalId")]));
    m.insert(
        "platform:iam:user:roles-assigned",
        obj(&[
            req_str("principalId"),
            req_str_array("roles"),
            req_str_array("added"),
            req_str_array("removed"),
        ]),
    );
    m.insert(
        "platform:iam:user:application-access-assigned",
        obj(&[
            req_str("userId"),
            req_str_array("applicationIds"),
            req_str_array("added"),
            req_str_array("removed"),
        ]),
    );
    m.insert(
        "platform:iam:user:client-access-granted",
        obj(&[req_str("principalId"), req_str("clientId")]),
    );
    m.insert(
        "platform:iam:user:client-access-revoked",
        obj(&[req_str("principalId"), req_str("clientId")]),
    );
    m.insert(
        "platform:iam:user:logged-in",
        json!({
            "$schema": "http://json-schema.org/draft-07/schema#",
            "type": "object",
            "properties": {
                "userId": {"type": "string"},
                "email": {"type": "string"},
                "loginMethod": {"type": "string", "enum": ["INTERNAL", "OIDC"]},
                "identityProviderCode": {"type": ["string", "null"]},
                "flowcatalystClaims": {
                    "type": "object",
                    "properties": {
                        "email": {"type": "string"},
                        "type": {"type": "string"},
                        "roles": {"type": "array", "items": {"type": "string"}},
                        "clients": {"type": "array", "items": {"type": "string"}},
                        "applications": {"type": "array", "items": {"type": "string"}}
                    },
                    "required": ["email", "type", "roles", "clients", "applications"]
                },
                "federatedClaims": {
                    "oneOf": [
                        {
                            "type": "object",
                            "properties": {
                                "accessToken": {"type": "object"},
                                "idToken": {"type": "object"}
                            },
                            "required": ["accessToken", "idToken"]
                        },
                        {"type": "null"}
                    ]
                }
            },
            "required": ["userId", "email", "loginMethod", "flowcatalystClaims"],
            "additionalProperties": true
        }),
    );
    m.insert(
        "platform:iam:user:password-reset-requested",
        obj(&[req_str("principalId"), req_str("email")]),
    );
    m.insert(
        "platform:iam:user:password-reset-completed",
        obj(&[req_str("principalId"), req_str("email")]),
    );

    // ─── platform:iam:principals (sync — plural aggregate) ─────────────
    m.insert(
        "platform:iam:principals:synced",
        obj(&[
            req_str("applicationCode"),
            req_u32("created"),
            req_u32("updated"),
            req_u32("deactivated"),
            req_str_array("syncedEmails"),
        ]),
    );

    // ─── platform:iam:serviceaccount (no hyphen) ───────────────────────
    m.insert(
        "platform:iam:serviceaccount:created",
        obj(&[
            req_str("serviceAccountId"),
            req_str("code"),
            req_str("name"),
            opt_str("applicationId"),
            req_str_array("clientIds"),
        ]),
    );
    m.insert(
        "platform:iam:serviceaccount:updated",
        obj(&[
            req_str("serviceAccountId"),
            opt_str("name"),
            opt_str("description"),
            req_str_array("clientIdsAdded"),
            req_str_array("clientIdsRemoved"),
        ]),
    );
    m.insert(
        "platform:iam:serviceaccount:deleted",
        obj(&[req_str("serviceAccountId"), req_str("code")]),
    );
    m.insert(
        "platform:iam:serviceaccount:roles-assigned",
        obj(&[
            req_str("serviceAccountId"),
            req_str_array("rolesAdded"),
            req_str_array("rolesRemoved"),
        ]),
    );
    m.insert(
        "platform:iam:serviceaccount:token-regenerated",
        obj(&[req_str("serviceAccountId"), req_str("code")]),
    );
    m.insert(
        "platform:iam:serviceaccount:secret-regenerated",
        obj(&[req_str("serviceAccountId"), req_str("code")]),
    );

    // ─── platform:iam:client ───────────────────────────────────────────
    m.insert(
        "platform:iam:client:created",
        obj(&[
            req_str("clientId"),
            req_str("name"),
            req_str("identifier"),
            opt_str("description"),
        ]),
    );
    m.insert(
        "platform:iam:client:updated",
        obj(&[req_str("clientId"), opt_str("name"), opt_str("description")]),
    );
    m.insert(
        "platform:iam:client:activated",
        obj(&[req_str("clientId"), req_str("previousStatus")]),
    );
    m.insert(
        "platform:iam:client:suspended",
        obj(&[req_str("clientId"), req_str("reason")]),
    );
    m.insert(
        "platform:iam:client:deleted",
        obj(&[req_str("clientId"), req_str("name"), req_str("identifier")]),
    );
    m.insert(
        "platform:iam:client:note-added",
        obj(&[
            req_str("clientId"),
            req_str("category"),
            req_str("text"),
            req_str("author"),
        ]),
    );

    // ─── platform:iam:role ─────────────────────────────────────────────
    m.insert(
        "platform:iam:role:created",
        obj(&[
            req_str("roleId"),
            req_str("code"),
            req_str("displayName"),
            req_str("applicationCode"),
            req_str_array("permissions"),
        ]),
    );
    m.insert(
        "platform:iam:role:updated",
        obj(&[
            req_str("roleId"),
            opt_str("displayName"),
            opt_str("description"),
            req_str_array("permissionsAdded"),
            req_str_array("permissionsRemoved"),
        ]),
    );
    m.insert(
        "platform:iam:role:deleted",
        obj(&[req_str("roleId"), req_str("code")]),
    );
    m.insert(
        "platform:iam:roles:synced",
        obj(&[
            req_str("applicationCode"),
            req_u32("created"),
            req_u32("updated"),
            req_u32("deleted"),
            req_str_array("syncedNames"),
        ]),
    );

    // ─── platform:iam:application ──────────────────────────────────────
    m.insert(
        "platform:iam:application:created",
        obj(&[
            req_str("applicationId"),
            req_str("code"),
            req_str("name"),
            req_str("applicationType"),
        ]),
    );
    m.insert(
        "platform:iam:application:updated",
        obj(&[
            req_str("applicationId"),
            opt_str("name"),
            opt_str("description"),
        ]),
    );
    m.insert(
        "platform:iam:application:activated",
        obj(&[req_str("applicationId"), req_str("code")]),
    );
    m.insert(
        "platform:iam:application:deactivated",
        obj(&[req_str("applicationId"), req_str("code")]),
    );
    m.insert(
        "platform:iam:application:deleted",
        obj(&[req_str("applicationId"), req_str("code"), req_str("name")]),
    );
    m.insert(
        "platform:iam:application:service-account-provisioned",
        obj(&[
            req_str("applicationId"),
            req_str("applicationCode"),
            req_str("serviceAccountId"),
            req_str("serviceAccountCode"),
        ]),
    );
    m.insert(
        "platform:iam:application:enabled-for-client",
        obj(&[
            req_str("applicationId"),
            req_str("clientId"),
            req_str("configId"),
        ]),
    );
    m.insert(
        "platform:iam:application:disabled-for-client",
        obj(&[
            req_str("applicationId"),
            req_str("clientId"),
            req_str("configId"),
        ]),
    );

    // ─── platform:iam:anchor-domain ────────────────────────────────────
    m.insert(
        "platform:iam:anchor-domain:created",
        obj(&[req_str("anchorDomainId"), req_str("domain")]),
    );
    m.insert(
        "platform:iam:anchor-domain:deleted",
        obj(&[req_str("anchorDomainId"), req_str("domain")]),
    );

    // ─── platform:iam:auth-config ──────────────────────────────────────
    m.insert(
        "platform:iam:auth-config:created",
        obj(&[
            req_str("authConfigId"),
            req_str("emailDomain"),
            req_str("configType"),
        ]),
    );
    m.insert(
        "platform:iam:auth-config:updated",
        obj(&[req_str("authConfigId"), req_str("emailDomain")]),
    );
    m.insert(
        "platform:iam:auth-config:deleted",
        obj(&[req_str("authConfigId"), req_str("emailDomain")]),
    );

    // ─── platform:admin:cors ───────────────────────────────────────────
    m.insert(
        "platform:admin:cors:origin-added",
        obj(&[req_str("originId"), req_str("origin")]),
    );
    m.insert(
        "platform:admin:cors:origin-deleted",
        obj(&[req_str("originId"), req_str("origin")]),
    );

    // ─── platform:admin:idp ────────────────────────────────────────────
    m.insert(
        "platform:admin:idp:created",
        obj(&[
            req_str("idpId"),
            req_str("code"),
            req_str("name"),
            req_str("idpType"),
        ]),
    );
    m.insert(
        "platform:admin:idp:updated",
        obj(&[req_str("idpId"), opt_str("name")]),
    );
    m.insert(
        "platform:admin:idp:deleted",
        obj(&[req_str("idpId"), req_str("code")]),
    );

    // ─── platform:admin:edm ────────────────────────────────────────────
    m.insert(
        "platform:admin:edm:created",
        obj(&[
            req_str("mappingId"),
            req_str("emailDomain"),
            req_str("identityProviderId"),
            req_str("scopeType"),
        ]),
    );
    m.insert(
        "platform:admin:edm:updated",
        obj(&[req_str("mappingId"), req_str("emailDomain")]),
    );
    m.insert(
        "platform:admin:edm:deleted",
        obj(&[req_str("mappingId"), req_str("emailDomain")]),
    );

    // ─── platform:admin:eventtype (no hyphen) ───────────────────────────
    m.insert(
        "platform:admin:eventtype:created",
        obj(&[
            req_str("eventTypeId"),
            req_str("code"),
            req_str("name"),
            opt_str("description"),
            req_str("application"),
            req_str("subdomain"),
            req_str("aggregate"),
            req_str("eventName"),
            opt_str("clientId"),
        ]),
    );
    m.insert(
        "platform:admin:eventtype:updated",
        obj(&[
            req_str("eventTypeId"),
            opt_str("name"),
            opt_str("description"),
        ]),
    );
    m.insert(
        "platform:admin:eventtype:archived",
        obj(&[req_str("eventTypeId"), req_str("code")]),
    );
    m.insert(
        "platform:admin:eventtype:deleted",
        obj(&[req_str("eventTypeId"), req_str("code")]),
    );
    m.insert(
        "platform:admin:eventtype:schema-added",
        obj(&[
            req_str("eventTypeId"),
            req_str("version"),
            req_str("mimeType"),
            req_str("schemaType"),
        ]),
    );
    m.insert(
        "platform:admin:eventtype:schema-finalised",
        obj(&[
            req_str("eventTypeId"),
            req_str("version"),
            opt_str("deprecatedVersion"),
        ]),
    );
    m.insert(
        "platform:admin:eventtype:schema-deprecated",
        obj(&[req_str("eventTypeId"), req_str("version")]),
    );
    m.insert(
        "platform:admin:eventtypes:synced",
        obj(&[
            req_str("applicationCode"),
            req_u32("created"),
            req_u32("updated"),
            req_u32("deleted"),
            req_str_array("syncedCodes"),
        ]),
    );

    // ─── platform:admin:connection ──────────────────────────────────────
    m.insert(
        "platform:admin:connection:created",
        obj(&[
            req_str("connectionId"),
            req_str("code"),
            req_str("name"),
            req_str("endpoint"),
            req_str("serviceAccountId"),
            opt_str("clientId"),
        ]),
    );
    m.insert(
        "platform:admin:connection:updated",
        obj(&[
            req_str("connectionId"),
            req_str("code"),
            opt_str("name"),
            opt_str("endpoint"),
            opt_str("status"),
        ]),
    );
    m.insert(
        "platform:admin:connection:deleted",
        obj(&[
            req_str("connectionId"),
            req_str("code"),
            opt_str("clientId"),
        ]),
    );

    // ─── platform:admin:dispatch-pool ───────────────────────────────────
    m.insert(
        "platform:admin:dispatch-pool:created",
        obj(&[
            req_str("dispatchPoolId"),
            req_str("code"),
            req_str("name"),
            opt_str("clientId"),
        ]),
    );
    m.insert(
        "platform:admin:dispatch-pool:updated",
        obj(&[
            req_str("dispatchPoolId"),
            opt_str("name"),
            opt_u32("rateLimit"),
            opt_u32("concurrency"),
        ]),
    );
    m.insert(
        "platform:admin:dispatch-pool:archived",
        obj(&[req_str("dispatchPoolId"), req_str("code")]),
    );
    m.insert(
        "platform:admin:dispatch-pool:deleted",
        obj(&[req_str("dispatchPoolId"), req_str("code")]),
    );
    m.insert(
        "platform:admin:dispatch-pools:synced",
        obj(&[
            req_str("applicationCode"),
            req_u32("created"),
            req_u32("updated"),
            req_u32("deleted"),
            req_str_array("syncedCodes"),
        ]),
    );

    // ─── platform:admin:subscription ────────────────────────────────────
    m.insert(
        "platform:admin:subscription:created",
        obj(&[
            req_str("subscriptionId"),
            req_str("code"),
            req_str("name"),
            req_str("connectionId"),
            req_str_array("eventTypes"),
            opt_str("clientId"),
        ]),
    );
    m.insert(
        "platform:admin:subscription:updated",
        obj(&[
            req_str("subscriptionId"),
            opt_str("name"),
            req_str_array("eventTypesAdded"),
            req_str_array("eventTypesRemoved"),
        ]),
    );
    m.insert(
        "platform:admin:subscription:paused",
        obj(&[req_str("subscriptionId"), req_str("code")]),
    );
    m.insert(
        "platform:admin:subscription:resumed",
        obj(&[req_str("subscriptionId"), req_str("code")]),
    );
    m.insert(
        "platform:admin:subscription:deleted",
        obj(&[req_str("subscriptionId"), req_str("code")]),
    );
    m.insert(
        "platform:admin:subscription:synced",
        obj(&[
            req_str("applicationCode"),
            req_u32("created"),
            req_u32("updated"),
            req_u32("deleted"),
            req_str_array("syncedCodes"),
        ]),
    );

    m
}

/// Look up the schema for a given event type code.
pub fn schema_for(code: &str) -> Option<Value> {
    schemas().remove(code)
}

// ── Schema builder helpers ─────────────────────────────────────────────────

type Prop = (&'static str, Value, bool);

fn obj(props: &[Prop]) -> Value {
    let mut properties = serde_json::Map::new();
    let mut required = Vec::new();

    for (name, schema, is_required) in props {
        properties.insert((*name).to_string(), schema.clone());
        if *is_required {
            required.push(json!(name));
        }
    }

    json!({
        "$schema": "http://json-schema.org/draft-07/schema#",
        "type": "object",
        "properties": properties,
        "required": required,
        "additionalProperties": false
    })
}

fn req_str(name: &'static str) -> Prop {
    (name, json!({"type": "string"}), true)
}

fn opt_str(name: &'static str) -> Prop {
    (name, json!({"type": ["string", "null"]}), false)
}

fn req_bool(name: &'static str) -> Prop {
    (name, json!({"type": "boolean"}), true)
}

fn req_u32(name: &'static str) -> Prop {
    (name, json!({"type": "integer", "minimum": 0}), true)
}

fn opt_u32(name: &'static str) -> Prop {
    (
        name,
        json!({"type": ["integer", "null"], "minimum": 0}),
        false,
    )
}

fn req_str_array(name: &'static str) -> Prop {
    (
        name,
        json!({"type": "array", "items": {"type": "string"}}),
        true,
    )
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn all_schemas_are_valid_json_schema() {
        let schemas = schemas();
        assert!(
            schemas.len() >= 70,
            "Expected 70+ schemas, got {}",
            schemas.len()
        );
        for (code, schema) in &schemas {
            assert_eq!(
                schema["type"], "object",
                "Schema for {} must be object type",
                code
            );
            assert!(
                schema["properties"].is_object(),
                "Schema for {} must have properties",
                code
            );
            assert!(
                schema["required"].is_array(),
                "Schema for {} must have required array",
                code
            );
        }
    }

    #[test]
    fn user_created_schema_has_expected_fields() {
        let schema = schema_for("platform:iam:user:created").unwrap();
        let props = schema["properties"].as_object().unwrap();
        assert!(props.contains_key("principalId"));
        assert!(props.contains_key("email"));
        assert!(props.contains_key("isAnchorUser"));
    }

    #[test]
    fn no_webhook_delivery_events() {
        let schemas = schemas();
        for code in schemas.keys() {
            assert!(
                !code.contains("webhook"),
                "No webhook events allowed: {}",
                code
            );
            assert!(
                !code.contains("delivery"),
                "No delivery events allowed: {}",
                code
            );
        }
    }

    #[test]
    fn only_two_subdomains() {
        let schemas = schemas();
        for code in schemas.keys() {
            assert!(
                code.starts_with("platform:iam:") || code.starts_with("platform:admin:"),
                "Event type '{}' must use platform:iam or platform:admin prefix",
                code,
            );
        }
    }

    #[test]
    fn definitions_and_schemas_are_aligned() {
        let schemas = schemas();
        let defs = crate::seed::platform_event_types::definitions();
        for def in &defs {
            assert!(
                schemas.contains_key(def.code.as_str()),
                "Definition '{}' has no matching schema",
                def.code,
            );
        }
        // Every schema key should be in definitions
        let def_codes: std::collections::HashSet<&str> =
            defs.iter().map(|d| d.code.as_str()).collect();
        for code in schemas.keys() {
            assert!(
                def_codes.contains(code),
                "Schema '{}' has no matching definition",
                code,
            );
        }
    }
}
