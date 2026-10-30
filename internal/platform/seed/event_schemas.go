package seed

import (
	"encoding/json"
)

// platformEventSchemas returns the JSON-Schema map. Keys match
// event-type codes exactly. Each schema describes the data its use case
// emits (ToDataJSON): a field always sent is required, an omitempty field
// optional, a nil slice null. The DSL helpers (obj/reqStr/optStr/etc.)
// build the schemas consumed cross-language. A schema change reaches an
// existing database only through a migration (the seeder never replaces an
// attached 1.0); see 072.
func platformEventSchemas() map[string]json.RawMessage {
	m := map[string]json.RawMessage{}

	// ── platform:iam:user ───────────────────────────────────────────────
	m["platform:iam:user:created"] = obj(reqStr("principalId"), reqStr("email"))
	m["platform:iam:user:updated"] = obj(reqStr("principalId"), reqStr("name"))
	m["platform:iam:user:activated"] = obj(reqStr("principalId"))
	m["platform:iam:user:deactivated"] = obj(reqStr("principalId"))
	m["platform:iam:user:deleted"] = obj(reqStr("principalId"), reqStr("email"))
	m["platform:iam:user:roles-assigned"] = obj(
		reqStr("principalId"),
		reqStrArray("roles"),
		reqStrArray("added"),
		reqStrArray("removed"),
	)
	m["platform:iam:user:application-access-assigned"] = obj(
		reqStr("userId"),
		reqStrArray("applicationIds"),
		reqStrArray("added"),
		reqStrArray("removed"),
	)
	m["platform:iam:user:client-access-granted"] = obj(
		reqStr("principalId"), reqStr("clientId"),
	)
	m["platform:iam:user:client-access-revoked"] = obj(
		reqStr("principalId"), reqStr("clientId"),
	)
	// logged-in carries a richer payload — federatedClaims oneOf, etc.
	m["platform:iam:user:logged-in"] = mustRaw(map[string]any{
		"$schema": "http://json-schema.org/draft-07/schema#",
		"type":    "object",
		"properties": map[string]any{
			"userId":               map[string]any{"type": "string"},
			"email":                map[string]any{"type": "string"},
			"loginMethod":          map[string]any{"type": "string", "enum": []string{"INTERNAL", "OIDC"}},
			"identityProviderCode": map[string]any{"type": []string{"string", "null"}},
			"flowcatalystClaims": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"email":        map[string]any{"type": "string"},
					"type":         map[string]any{"type": "string"},
					"roles":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"clients":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"applications": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				},
				"required": []string{"email", "type", "roles", "clients", "applications"},
			},
			"federatedClaims": map[string]any{
				"oneOf": []any{
					map[string]any{
						"type": "object",
						"properties": map[string]any{
							"accessToken": map[string]any{"type": "object"},
							"idToken":     map[string]any{"type": "object"},
						},
						"required": []string{"accessToken", "idToken"},
					},
					map[string]any{"type": "null"},
				},
			},
		},
		"required":             []string{"userId", "email", "loginMethod", "flowcatalystClaims"},
		"additionalProperties": true,
	})
	m["platform:iam:user:password-reset-completed"] = obj(reqStr("principalId"))

	// ── platform:iam:principals (sync — plural aggregate) ───────────────
	m["platform:iam:principals:synced"] = obj(
		reqStr("applicationCode"),
		reqU32("created"),
		reqU32("updated"),
		reqU32("deactivated"),
		reqStrArray("syncedEmails"),
	)

	// ── platform:iam:serviceaccount ─────────────────────────────────────
	m["platform:iam:serviceaccount:created"] = obj(
		reqStr("serviceAccountId"), reqStr("code"), reqStr("name"),
	)
	m["platform:iam:serviceaccount:updated"] = obj(reqStr("serviceAccountId"), reqStr("name"))
	m["platform:iam:serviceaccount:deleted"] = obj(reqStr("serviceAccountId"), reqStr("code"))
	m["platform:iam:serviceaccount:roles-assigned"] = obj(
		reqStr("serviceAccountId"),
		reqStrArray("rolesAdded"),
		reqStrArray("rolesRemoved"),
	)
	m["platform:iam:serviceaccount:token-regenerated"] = obj(
		reqStr("serviceAccountId"), reqStr("code"),
	)
	m["platform:iam:serviceaccount:secret-regenerated"] = obj(
		reqStr("serviceAccountId"), reqStr("code"),
	)

	// ── platform:admin:client ───────────────────────────────────────────
	// The data each client use case emits (client/operations ToDataJSON).
	m["platform:admin:client:created"] = obj(reqStr("clientId"), reqStr("name"), reqStr("identifier"))
	m["platform:admin:client:updated"] = obj(reqStr("clientId"), reqStr("name"))
	m["platform:admin:client:activated"] = obj(reqStr("clientId"))
	m["platform:admin:client:suspended"] = obj(reqStr("clientId"), reqStr("reason"))
	m["platform:admin:client:deactivated"] = obj(reqStr("clientId"), optStr("reason"))
	m["platform:admin:client:deleted"] = obj(reqStr("clientId"), reqStr("identifier"))
	m["platform:admin:client:note-added"] = obj(reqStr("clientId"), reqStr("category"), reqStr("text"))

	// ── platform:iam:client ─────────────────────────────────────────────
	// Each list is null when empty.
	m["platform:iam:client:applications-updated"] = obj(
		reqStr("clientId"), reqNullableStrArray("enabledApplicationIds"),
		reqNullableStrArray("enabledAdded"), reqNullableStrArray("disabledRemoved"),
	)

	// ── platform:iam:application ────────────────────────────────────────
	m["platform:iam:application:created"] = obj(
		reqStr("applicationId"), reqStr("code"), reqStr("name"),
	)
	m["platform:iam:application:updated"] = obj(reqStr("applicationId"), reqStr("name"))
	m["platform:iam:application:activated"] = obj(reqStr("applicationId"))
	m["platform:iam:application:deactivated"] = obj(reqStr("applicationId"))
	m["platform:iam:application:deleted"] = obj(reqStr("applicationId"), reqStr("code"),
		reqU32("platformConfigPropertiesDeleted"), reqU32("platformConfigGrantsDeleted"))
	m["platform:iam:application:service-account-provisioned"] = obj(
		reqStr("applicationId"), reqStr("applicationCode"),
		reqStr("serviceAccountId"), reqStr("serviceAccountCode"),
	)
	m["platform:iam:application:enabled-for-client"] = obj(
		reqStr("applicationId"), reqStr("clientId"), reqStr("configId"),
	)
	m["platform:iam:application:disabled-for-client"] = obj(
		reqStr("applicationId"), reqStr("clientId"), reqStr("configId"),
	)

	// ── platform:admin:cors ─────────────────────────────────────────────
	m["platform:admin:cors:origin-added"] = obj(reqStr("originId"), reqStr("origin"))
	m["platform:admin:cors:origin-deleted"] = obj(reqStr("originId"), reqStr("origin"))

	// ── platform:admin:eventtype ────────────────────────────────────────
	m["platform:admin:eventtype:created"] = obj(
		reqStr("eventTypeId"), reqStr("code"), reqStr("name"), optStr("description"),
		reqStr("application"), reqStr("subdomain"), reqStr("aggregate"),
		reqStr("eventName"), optStr("clientId"),
	)
	m["platform:admin:eventtype:updated"] = obj(
		reqStr("eventTypeId"), reqStr("name"), optStr("description"),
	)
	m["platform:admin:eventtype:archived"] = obj(reqStr("eventTypeId"), reqStr("code"))
	m["platform:admin:eventtype:deleted"] = obj(reqStr("eventTypeId"), reqStr("code"))
	m["platform:admin:eventtype:schema-added"] = obj(reqStr("eventTypeId"), reqStr("specVersion"))
	m["platform:admin:eventtype:schema-finalised"] = obj(
		reqStr("eventTypeId"), reqStr("specVersion"), optStr("deprecatedVersion"),
	)
	m["platform:admin:eventtype:schema-deprecated"] = obj(reqStr("eventTypeId"), reqStr("specVersion"))
	m["platform:admin:eventtypes:synced"] = obj(
		reqStr("applicationCode"),
		reqU32("created"), reqU32("updated"), reqU32("deleted"),
		reqStrArray("syncedCodes"),
	)

	// ── platform:admin:connection ───────────────────────────────────────
	m["platform:admin:connection:created"] = obj(
		reqStr("connectionId"), reqStr("code"), reqStr("name"),
	)
	m["platform:admin:connection:updated"] = obj(reqStr("connectionId"), reqStr("name"))
	m["platform:admin:connection:deleted"] = obj(reqStr("connectionId"), reqStr("code"))
	m["platform:admin:connection:synced"] = obj(
		reqStr("applicationCode"), optStr("clientId"),
		reqU32("created"), reqU32("updated"), reqU32("deleted"),
		reqStrArray("syncedCodes"),
	)

	// ── platform:admin:dispatch-pool ────────────────────────────────────
	m["platform:admin:dispatch-pool:created"] = obj(reqStr("poolId"), reqStr("code"), reqStr("name"))
	m["platform:admin:dispatch-pool:updated"] = obj(reqStr("poolId"), reqStr("name"))
	m["platform:admin:dispatch-pool:archived"] = obj(reqStr("poolId"), reqStr("code"))
	m["platform:admin:dispatch-pool:deleted"] = obj(reqStr("poolId"), reqStr("code"))
	m["platform:admin:dispatch-pools:synced"] = obj(
		reqStr("applicationCode"),
		reqU32("created"), reqU32("updated"), reqU32("deleted"),
		reqStrArray("syncedCodes"),
	)

	// ── platform:admin:subscription ─────────────────────────────────────
	m["platform:admin:subscription:created"] = obj(
		reqStr("subscriptionId"), reqStr("code"), reqStr("name"),
	)
	m["platform:admin:subscription:updated"] = obj(reqStr("subscriptionId"), reqStr("name"))
	m["platform:admin:subscription:paused"] = obj(reqStr("subscriptionId"))
	m["platform:admin:subscription:resumed"] = obj(reqStr("subscriptionId"))
	m["platform:admin:subscription:deleted"] = obj(reqStr("subscriptionId"), reqStr("code"))
	m["platform:admin:subscription:synced"] = obj(
		reqStr("applicationCode"), optStr("clientId"),
		reqU32("created"), reqU32("updated"), reqU32("deleted"),
		reqStrArray("syncedCodes"),
	)

	// ── platform:iam:user (developer credentials) ───────────────────────
	m["platform:iam:user:developer-credential-set"] = obj(reqStr("userId"))
	m["platform:iam:user:developer-credential-revoked"] = obj(reqStr("userId"))

	// ── platform:iam:serviceaccount ─────────────────────────────────────
	m["platform:iam:serviceaccount:deactivated"] = obj(reqStr("serviceAccountId"))

	// ── platform:admin:dispatch-pool ────────────────────────────────────
	m["platform:admin:dispatch-pool:suspended"] = obj(reqStr("poolId"), reqStr("code"))
	m["platform:admin:dispatch-pool:activated"] = obj(reqStr("poolId"), reqStr("code"))

	// ── platform:admin:anchor-domain ────────────────────────────────────
	m["platform:admin:anchor-domain:created"] = obj(reqStr("anchorDomainId"), reqStr("domain"))
	m["platform:admin:anchor-domain:updated"] = obj(reqStr("anchorDomainId"), reqStr("domain"))
	m["platform:admin:anchor-domain:deleted"] = obj(reqStr("anchorDomainId"), reqStr("domain"))

	// ── platform:admin:auth-config ──────────────────────────────────────
	m["platform:admin:auth-config:created"] = obj(reqStr("authConfigId"), reqStr("emailDomain"))
	m["platform:admin:auth-config:updated"] = obj(reqStr("authConfigId"), reqStr("emailDomain"))
	m["platform:admin:auth-config:deleted"] = obj(reqStr("authConfigId"), reqStr("emailDomain"))

	// ── platform:admin:oauth-client ─────────────────────────────────────
	m["platform:admin:oauth-client:created"] = obj(
		reqStr("oauthClientId"), reqStr("clientId"), reqStr("clientName"),
	)
	m["platform:admin:oauth-client:updated"] = obj(reqStr("oauthClientId"), reqStr("clientName"))
	m["platform:admin:oauth-client:activated"] = obj(reqStr("oauthClientId"))
	m["platform:admin:oauth-client:deactivated"] = obj(reqStr("oauthClientId"))
	m["platform:admin:oauth-client:deleted"] = obj(reqStr("oauthClientId"), reqStr("clientId"))
	m["platform:admin:oauth-client:secret-rotated"] = obj(
		reqStr("oauthClientId"), optDateTime("previousSecretExpiresAt"),
	)
	m["platform:admin:oauth-client:previous-secret-revoked"] = obj(reqStr("oauthClientId"))

	// ── platform:admin:identity-provider ────────────────────────────────
	m["platform:admin:identity-provider:created"] = obj(reqStr("identityProviderId"), reqStr("code"))
	m["platform:admin:identity-provider:updated"] = obj(reqStr("identityProviderId"), reqStr("code"))
	m["platform:admin:identity-provider:deleted"] = obj(reqStr("identityProviderId"), reqStr("code"))

	// ── platform:admin:email-domain-mapping ─────────────────────────────
	m["platform:admin:email-domain-mapping:created"] = obj(reqStr("mappingId"), reqStr("emailDomain"))
	m["platform:admin:email-domain-mapping:updated"] = obj(reqStr("mappingId"), reqStr("emailDomain"))
	m["platform:admin:email-domain-mapping:provider-changed"] = obj(
		reqStr("mappingId"), reqStr("emailDomain"),
		reqStr("fromIdentityProviderId"), reqStr("toIdentityProviderId"),
	)
	m["platform:admin:email-domain-mapping:deleted"] = obj(reqStr("mappingId"), reqStr("emailDomain"))

	// ── platform:admin:idp-role-mapping ─────────────────────────────────
	m["platform:admin:idp-role-mapping:created"] = obj(
		reqStr("mappingId"), reqStr("idpType"), reqStr("idpRoleName"), reqStr("platformRoleName"),
	)
	m["platform:admin:idp-role-mapping:deleted"] = obj(reqStr("mappingId"), reqStr("idpRoleName"))

	// ── platform:admin:role ─────────────────────────────────────────────
	m["platform:admin:role:created"] = obj(reqStr("roleId"), reqStr("name"))
	m["platform:admin:role:updated"] = obj(reqStr("roleId"), reqStr("name"))
	m["platform:admin:role:deleted"] = obj(reqStr("roleId"), reqStr("name"))
	m["platform:admin:role:permission-granted"] = obj(
		reqStr("roleId"), reqStr("roleName"), reqStr("permission"),
	)
	m["platform:admin:role:permission-revoked"] = obj(
		reqStr("roleId"), reqStr("roleName"), reqStr("permission"),
	)
	// The application-scoped sync sets applicationCode and syncedCodes; the
	// platform-catalogue sync leaves both out.
	m["platform:admin:roles:synced"] = obj(
		reqU32("created"), reqU32("updated"), reqU32("removed"), reqU32("total"),
		optStr("applicationCode"), optStrArray("syncedCodes"),
	)

	// ── platform:admin:passkey ──────────────────────────────────────────
	m["platform:admin:passkey:registered"] = obj(reqStr("credentialId"), reqStr("userId"), optStr("name"))
	m["platform:admin:passkey:authenticated"] = obj(reqStr("credentialId"), reqStr("userId"))
	m["platform:admin:passkey:revoked"] = obj(reqStr("credentialId"), reqStr("userId"))

	// ── platform:admin:platform-config ──────────────────────────────────
	m["platform:admin:platform-config:property-set"] = obj(
		reqStr("configId"), reqStr("applicationCode"), reqStr("section"), reqStr("property"),
	)
	m["platform:admin:platform-config:property-deleted"] = obj(
		reqStr("configId"), reqStr("applicationCode"), reqStr("section"), reqStr("property"), optStr("clientId"),
	)
	m["platform:admin:platform-config:access-granted"] = obj(
		reqStr("accessId"), reqStr("applicationCode"), reqStr("roleCode"), reqBool("canWrite"),
	)
	m["platform:admin:platform-config:access-revoked"] = obj(
		reqStr("accessId"), reqStr("applicationCode"), reqStr("roleCode"),
	)

	// ── platform:admin:process ──────────────────────────────────────────
	m["platform:admin:process:created"] = obj(reqStr("processId"), reqStr("code"), reqStr("name"))
	m["platform:admin:process:updated"] = obj(reqStr("processId"), reqStr("name"))
	m["platform:admin:process:archived"] = obj(reqStr("processId"), reqStr("code"))
	m["platform:admin:process:deleted"] = obj(reqStr("processId"), reqStr("code"))
	m["platform:admin:processes:synced"] = obj(
		reqStr("applicationCode"),
		reqU32("created"), reqU32("updated"), reqU32("deleted"),
		reqStrArray("syncedCodes"),
	)

	// ── platform:admin:scheduled-job ────────────────────────────────────
	for _, ev := range []string{"created", "updated", "paused", "resumed", "archived", "deleted"} {
		m["platform:admin:scheduled-job:"+ev] = obj(reqStr("scheduledJobId"), reqStr("code"))
	}
	m["platform:admin:scheduled-job:fired-manually"] = obj(
		reqStr("scheduledJobId"), reqStr("code"), reqStr("instanceId"),
	)
	// Each list holds scheduled-job ids; an empty one is null.
	m["platform:admin:scheduledjobs:synced"] = obj(
		reqStr("applicationCode"),
		reqNullableStrArray("created"), reqNullableStrArray("updated"), reqNullableStrArray("archived"),
	)

	// ── platform:ingestion:ingestion ────────────────────────────────────
	m["platform:ingestion:ingestion:created"] = obj(
		reqStr("ingestionId"), reqStr("applicationId"), reqStr("clientId"), reqStr("code"), reqU32("version"),
	)
	m["platform:ingestion:ingestion:updated"] = obj(reqStr("ingestionId"), reqStr("code"), reqU32("version"))
	m["platform:ingestion:ingestion:deleted"] = obj(reqStr("ingestionId"), reqStr("code"))
	m["platform:ingestion:ingestion:paused"] = obj(reqStr("ingestionId"), reqStr("code"))
	m["platform:ingestion:ingestion:resumed"] = obj(reqStr("ingestionId"), reqStr("code"))
	m["platform:ingestion:ingestion:version-saved"] = obj(
		reqStr("ingestionId"), reqStr("code"), reqU32("version"), reqBool("live"),
	)
	m["platform:ingestion:ingestion:version-live"] = obj(reqStr("ingestionId"), reqStr("code"), reqU32("version"))
	m["platform:ingestion:ingestion:managed"] = obj(reqStr("ingestionId"), reqStr("code"), optStr("managedBy"))
	m["platform:ingestion:ingestion:failure-dismissed"] = obj(
		reqStr("ingestionId"), reqStr("code"), reqStr("failureId"), reqStr("category"),
	)
	// The entry's value is not in the event (it is in no audit row either): the
	// names of its settings and the ingestions it was checked against are.
	m["platform:ingestion:ingestion:config-set"] = obj(
		reqStr("ingestionId"), reqStr("code"), reqStr("key"), reqStrArray("settings"), reqStrArray("ingestions"),
	)
	m["platform:ingestion:ingestion:config-deleted"] = obj(
		reqStr("ingestionId"), reqStr("code"), reqStr("key"), reqStrArray("ingestions"),
	)
	m["platform:ingestion:set:applied"] = obj(
		reqStr("setId"), reqStr("name"), reqStr("applyId"), optStr("goingBackTo"),
		reqU32("created"), reqU32("changed"), reqU32("adopted"), reqU32("removed"),
	)
	m["platform:ingestion:set:released"] = obj(reqStr("setId"), reqStr("name"), reqU32("released"))
	// Ingestion traces (docs/ingestion-trace-design.md decision 9).
	m["platform:ingestion:trace:enabled"] = obj(
		reqStr("ingestionId"), reqStr("code"), reqStr("windowId"), reqU32("windowSecs"), reqU32("sampleEverySecs"), optDateTime("hardStop"),
	)
	m["platform:ingestion:trace:disabled"] = obj(reqStr("ingestionId"), reqStr("code"), reqStr("windowId"))

	// ── platform:function (shared with Java and Rust), platform:admin:function* (Go-only)
	// The shared codes carry Java's and Rust's schemas, except that
	// function:created has no runtime (a Go function has none; Go's runtime
	// is per version). The signer pair is never sent (Go does not sign).
	m["platform:function:function:created"] = obj(
		reqStr("functionId"), reqStr("address"), reqStr("applicationId"), optStr("clientId"),
	)
	m["platform:function:function:updated"] = obj(
		reqStr("functionId"), reqStr("address"), optStr("description"), reqStr("status"),
	)
	m["platform:function:function:deleted"] = obj(reqStr("functionId"), reqStr("address"))
	m["platform:function:version:published"] = obj(
		reqStr("functionId"), reqStr("address"), reqStr("versionId"), reqU32("version"),
		reqStr("digest"), reqStr("pool"), optStr("signerIssuer"), optStr("signerSubject"),
	)
	m["platform:function:version:retired"] = obj(
		reqStr("functionId"), reqStr("address"), reqStr("versionId"), reqU32("version"),
	)
	m["platform:function:version:pruned"] = obj(
		reqStr("functionId"), reqStr("address"), reqStr("versionId"), reqU32("version"), reqStr("digest"),
	)
	// previousVersionId is absent when the alias is new.
	m["platform:function:alias:changed"] = obj(
		reqStr("functionId"), reqStr("address"), reqStr("alias"), reqStr("versionId"),
		reqU32("version"), optStr("previousVersionId"),
	)
	m["platform:function:alias:removed"] = obj(
		reqStr("functionId"), reqStr("address"), reqStr("alias"), reqStr("versionId"), reqU32("version"),
	)
	// Keys only, never values.
	m["platform:function:config:updated"] = obj(reqStr("functionId"), reqStr("address"), reqStrArray("keys"))
	m["platform:function:secret:set"] = obj(reqStr("functionId"), reqStr("address"), reqStr("key"))
	m["platform:function:secret:deleted"] = obj(reqStr("functionId"), reqStr("address"), reqStr("key"))
	m["platform:function:trace:enabled"] = obj(
		reqStr("functionId"), reqStr("address"), reqStr("context"), optDateTime("enabledUntil"), reqBool("indefinite"), reqU32("sampleRate"),
	)
	m["platform:function:trace:disabled"] = obj(reqStr("functionId"), reqStr("address"), reqStr("context"))
	// owner is "platform" or the owning client's id.
	m["platform:function:domain:claimed"] = obj(reqStr("domainId"), reqStr("hostname"), reqStr("owner"))
	m["platform:function:domain:released"] = obj(reqStr("domainId"), reqStr("hostname"), reqStr("owner"))
	// number is the version now live, 0 when live was removed.
	m["platform:admin:function:promoted"] = obj(
		reqStr("functionId"), reqStr("address"), reqU32("number"), reqStr("dispatchPoolCode"),
		reqU32("subscriptionsCreated"), reqU32("subscriptionsUpdated"), reqU32("subscriptionsDeleted"),
		reqU32("schedulesCreated"), reqU32("schedulesUpdated"), reqU32("schedulesDeleted"),
	)
	// DB settings only (kind is "DB"); CONFIG and SECRET emit the shared codes.
	m["platform:admin:function-setting:set"] = obj(reqStr("functionId"), reqStr("kind"), reqStr("key"))
	m["platform:admin:function-setting:deleted"] = obj(reqStr("functionId"), reqStr("kind"), reqStr("key"))
	m["platform:admin:function-route:set"] = obj(
		reqStr("functionId"), reqStr("routeId"), reqStr("hostname"), reqStr("pathPrefix"),
	)
	m["platform:admin:function-route:deleted"] = obj(
		reqStr("functionId"), reqStr("routeId"), reqStr("hostname"), reqStr("pathPrefix"),
	)
	m["platform:admin:function-schedule:wired"] = obj(
		reqStr("functionId"), reqStr("scheduledJobId"), reqStr("code"),
		reqEnum("action", "created", "updated", "deleted"),
	)

	// ── platform:messaging:dispatch-job ─────────────────────────────────
	m["platform:messaging:dispatch-job:cancelled"] = obj(reqStr("dispatchJobId"))
	m["platform:messaging:dispatch-job:completed"] = obj(reqStr("dispatchJobId"))
	m["platform:messaging:dispatch-job:payload-edited"] = obj(
		reqStr("dispatchJobId"), reqStr("status"),
		reqU32("previousSize"), reqStr("previousSha256"),
		reqU32("size"), reqStr("sha256"),
		reqStr("contentType"), reqStr("previousContentType"),
	)
	m["platform:messaging:dispatch-jobs:resent"] = obj(reqStrArray("ids"))

	// ── platform:developer:application-openapi ──────────────────────────
	m["platform:developer:application-openapi:synced"] = obj(
		reqStr("applicationId"), reqStr("applicationCode"), reqStr("specId"),
		reqStr("version"), reqStr("specHash"), optStr("archivedPriorVersion"),
		reqBool("hasBreaking"), reqBool("unchanged"),
	)

	// ── platform:portal ─────────────────────────────────────────────────
	for _, ev := range []string{"created", "updated", "deleted"} {
		m["platform:portal:app:"+ev] = obj(
			reqStr("portalAppId"), reqStr("clientId"), reqStr("code"), reqStr("name"),
		)
	}
	m["platform:portal:identity:ensured"] = obj(
		reqStr("identityId"), reqStr("clientId"), reqStr("email"), reqBool("created"),
		reqStr("source"), optStr("portalAppId"), optStr("portalAppCode"),
	)
	m["platform:portal:identity:status-set"] = obj(reqStr("identityId"), reqStr("clientId"), reqStr("status"))
	m["platform:portal:identity:deleted"] = obj(reqStr("identityId"), reqStr("clientId"), reqStr("email"))
	m["platform:portal:identity:app-granted"] = obj(
		reqStr("identityId"), reqStr("clientId"), reqStr("portalAppId"), reqStr("portalAppCode"),
		reqStr("source"),
	)
	m["platform:portal:identity:app-revoked"] = obj(
		reqStr("identityId"), reqStr("clientId"), reqStr("portalAppId"), reqStr("portalAppCode"),
	)

	return m
}

// ── DSL — JSON-Schema builder helpers ────────────────────────────────────

type prop struct {
	name     string
	schema   any
	required bool
}

func obj(props ...prop) json.RawMessage {
	properties := map[string]any{}
	required := []string{}
	for _, p := range props {
		properties[p.name] = p.schema
		if p.required {
			required = append(required, p.name)
		}
	}
	return mustRaw(map[string]any{
		"$schema":              "http://json-schema.org/draft-07/schema#",
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	})
}

func reqStr(name string) prop {
	return prop{name: name, schema: map[string]any{"type": "string"}, required: true}
}

func optStr(name string) prop {
	return prop{name: name, schema: map[string]any{"type": []string{"string", "null"}}}
}

func reqBool(name string) prop {
	return prop{name: name, schema: map[string]any{"type": "boolean"}, required: true}
}

func reqU32(name string) prop {
	return prop{name: name, schema: map[string]any{"type": "integer", "minimum": 0}, required: true}
}

func reqStrArray(name string) prop {
	return prop{
		name:     name,
		schema:   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		required: true,
	}
}

// optStrArray is a string array the producer leaves out when empty.
func optStrArray(name string) prop {
	return prop{name: name, schema: map[string]any{"type": "array", "items": map[string]any{"type": "string"}}}
}

// reqNullableStrArray is a string array always present, null when empty.
func reqNullableStrArray(name string) prop {
	return prop{
		name:     name,
		schema:   map[string]any{"type": []string{"array", "null"}, "items": map[string]any{"type": "string"}},
		required: true,
	}
}

// optDateTime is an RFC 3339 timestamp the producer leaves out when unset.
func optDateTime(name string) prop {
	return prop{name: name, schema: map[string]any{"type": "string", "format": "date-time"}}
}

func reqEnum(name string, values ...string) prop {
	return prop{name: name, schema: map[string]any{"type": "string", "enum": values}, required: true}
}

func mustRaw(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic("seed: marshal schema: " + err.Error())
	}
	return json.RawMessage(b)
}
