package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// PlatformEventTypeDef mirrors the event-type sync input shape.
type PlatformEventTypeDef struct {
	Code   string
	Name   string
	Schema json.RawMessage // nil-OK; empty means "no schema attached yet"
}

// PlatformEventTypes returns the full catalog. Codes are
// {application}:{subdomain}:{aggregate}:{event}. It lists exactly what the
// platform's use cases emit (emitted_catalogue_test.go fails on drift either
// way); migration 071 removed the 16 codes older seeds wrote that nothing
// emits.
func PlatformEventTypes() []PlatformEventTypeDef {
	schemas := platformEventSchemas()
	out := make([]PlatformEventTypeDef, 0, 160)

	// groupAs names each event "<label> <Event>"; group derives the label
	// from the aggregate segment.
	groupAs := func(prefix, label string, events ...string) {
		for _, ev := range events {
			code := prefix + ":" + ev
			out = append(out, PlatformEventTypeDef{
				Code:   code,
				Name:   label + " " + titleCase(ev),
				Schema: schemas[code],
			})
		}
	}
	group := func(prefix string, events ...string) {
		groupAs(prefix, titleCase(lastSegment(prefix, ':')), events...)
	}
	push := func(code, name string) {
		out = append(out, PlatformEventTypeDef{
			Code:   code,
			Name:   name,
			Schema: schemas[code],
		})
	}

	// ── platform:iam ────────────────────────────────────────────────────
	group("platform:iam:user",
		"created", "updated", "activated", "deactivated", "deleted",
		"roles-assigned", "application-access-assigned",
		"client-access-granted", "client-access-revoked",
		"logged-in", "password-reset-completed",
		"developer-credential-set", "developer-credential-revoked")
	push("platform:iam:principals:synced", "Principals Synced")

	group("platform:iam:serviceaccount",
		"created", "updated", "deactivated", "deleted", "roles-assigned",
		"token-regenerated", "secret-regenerated")

	// Client lifecycle events are emitted under platform:admin (the client
	// use cases, internal/platform/client/operations); enabling applications
	// for a client is emitted under platform:iam by the application use case.
	// Migration 070 renamed the platform:iam:client:* rows an older seed wrote.
	group("platform:iam:client", "applications-updated")

	group("platform:iam:application",
		"created", "updated", "activated", "deactivated", "deleted",
		"service-account-provisioned", "enabled-for-client", "disabled-for-client")

	// ── platform:admin ──────────────────────────────────────────────────
	group("platform:admin:client",
		"created", "updated", "activated", "suspended", "deactivated", "deleted", "note-added")

	group("platform:admin:cors", "origin-added", "origin-deleted")

	group("platform:admin:eventtype",
		"created", "updated", "archived", "deleted",
		"schema-added", "schema-finalised", "schema-deprecated")
	push("platform:admin:eventtypes:synced", "Event Types Synced")

	group("platform:admin:connection", "created", "updated", "deleted", "synced")

	group("platform:admin:dispatch-pool",
		"created", "updated", "suspended", "activated", "archived", "deleted")
	push("platform:admin:dispatch-pools:synced", "Dispatch Pools Synced")

	group("platform:admin:subscription",
		"created", "updated", "paused", "resumed", "deleted", "synced")

	// Identity and access administration (the auth, identity-provider,
	// email-domain-mapping, role and webauthn use cases emit these under
	// platform:admin).
	group("platform:admin:anchor-domain", "created", "updated", "deleted")
	group("platform:admin:auth-config", "created", "updated", "deleted")
	groupAs("platform:admin:oauth-client", "OAuth Client",
		"created", "updated", "activated", "deactivated", "deleted",
		"secret-rotated", "previous-secret-revoked")
	group("platform:admin:identity-provider", "created", "updated", "deleted")
	group("platform:admin:email-domain-mapping",
		"created", "updated", "provider-changed", "deleted")
	groupAs("platform:admin:idp-role-mapping", "IdP Role Mapping", "created", "deleted")
	group("platform:admin:role",
		"created", "updated", "deleted", "permission-granted", "permission-revoked")
	push("platform:admin:roles:synced", "Roles Synced")
	group("platform:admin:passkey", "registered", "authenticated", "revoked")

	group("platform:admin:platform-config", "property-set", "property-deleted", "access-granted", "access-revoked")

	group("platform:admin:process", "created", "updated", "archived", "deleted")
	push("platform:admin:processes:synced", "Processes Synced")

	group("platform:admin:scheduled-job",
		"created", "updated", "paused", "resumed", "archived", "deleted", "fired-manually")
	push("platform:admin:scheduledjobs:synced", "Scheduled Jobs Synced")

	// Function events: the platform:function:* family Java and Rust emit,
	// named as theirs (owner rulings 2026-10-12, 2026-10-13; migrations 073
	// and 074 renamed the older platform:admin:function* rows). The Go-only
	// events, which Java and Rust have no counterpart for, keep their
	// platform:admin:function* codes (docs/platform-bug-fixes.md F11).
	groupAs("platform:function:function", "Function", "created", "updated", "deleted")
	groupAs("platform:function:version", "Function Version", "published", "retired", "pruned")
	groupAs("platform:function:alias", "Function Alias", "changed", "removed")
	groupAs("platform:function:config", "Function Config", "updated")
	groupAs("platform:function:secret", "Function Secret", "set", "deleted")
	groupAs("platform:function:domain", "Function Domain", "claimed", "released")
	groupAs("platform:function:trace", "Function Trace", "enabled", "disabled")
	group("platform:admin:function", "promoted")
	group("platform:admin:function-setting", "set", "deleted")
	group("platform:admin:function-route", "set", "deleted")
	group("platform:admin:function-schedule", "wired")

	// ── platform:ingestion ──────────────────────────────────────────────
	groupAs("platform:ingestion:ingestion", "Ingestion",
		"created", "updated", "deleted", "paused", "resumed", "version-saved", "version-live", "managed",
		"failure-dismissed")
	groupAs("platform:ingestion:set", "Ingestion Set", "applied", "released")

	// ── platform:messaging ──────────────────────────────────────────────
	group("platform:messaging:dispatch-job", "cancelled", "completed", "payload-edited")
	push("platform:messaging:dispatch-jobs:resent", "Dispatch Jobs Resent")

	// ── platform:developer ──────────────────────────────────────────────
	push("platform:developer:application-openapi:synced", "Application OpenAPI Synced")

	// ── platform:portal ─────────────────────────────────────────────────
	groupAs("platform:portal:app", "Portal App", "created", "updated", "deleted")
	groupAs("platform:portal:identity", "Portal Identity",
		"ensured", "status-set", "deleted", "app-granted", "app-revoked")

	return out
}

// seedPlatformEventTypes upserts the full event-type catalog:
// insert if not present
// (by code), update name on existing rows, and attach a SpecVersion if
// the catalog supplies one and no version with that name exists yet.
func (s *Seeder) seedPlatformEventTypes(ctx context.Context) error {
	defs := PlatformEventTypes()
	var inserted int
	for _, d := range defs {
		et, err := eventtype.New(d.Code, d.Name)
		if err != nil {
			return fmt.Errorf("compose %s: %w", d.Code, err)
		}
		et.Source = eventtype.SourceUI
		now := time.Now().UTC()

		// Upsert by code. If a row already exists, just refresh name +
		// updated_at; never blow away source/status/client_scoped.
		var id string
		if err := s.pool.QueryRow(ctx,
			`SELECT id FROM msg_event_types WHERE code = $1`, d.Code).Scan(&id); err != nil {
			id = tsid.Generate(tsid.EventType)
			if _, err := s.pool.Exec(ctx,
				`INSERT INTO msg_event_types
				     (id, code, name, description, status, source, client_scoped,
				      application, subdomain, aggregate, created_at, updated_at)
				 VALUES ($1, $2, $3, $4, 'CURRENT', 'UI', false, $5, $6, $7, $8, $8)
				 ON CONFLICT (code) DO NOTHING`,
				id, et.Code, et.Name, et.Description,
				et.Application, et.Subdomain, et.Aggregate, now); err != nil {
				return fmt.Errorf("insert event type %s: %w", d.Code, err)
			}
			// race-safe re-fetch
			if err := s.pool.QueryRow(ctx,
				`SELECT id FROM msg_event_types WHERE code = $1`, d.Code).Scan(&id); err != nil {
				return fmt.Errorf("lookup id %s: %w", d.Code, err)
			}
			inserted++
		} else {
			if _, err := s.pool.Exec(ctx,
				`UPDATE msg_event_types SET name = $1, updated_at = $2 WHERE id = $3`,
				et.Name, now, id); err != nil {
				return fmt.Errorf("update event type %s: %w", d.Code, err)
			}
		}

		// Schema attach (idempotent). Version "1.0", the same initial version
		// the create operation and the SDKs use. A legacy 'v1' row (what this
		// seeder wrote before migration 055 renamed them) also counts as
		// present, so the same schema is never attached twice.
		if len(d.Schema) == 0 {
			continue
		}
		var existing int
		if err := s.pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM msg_event_type_spec_versions
			  WHERE event_type_id = $1 AND version IN ('1.0', 'v1')`, id).Scan(&existing); err != nil {
			return fmt.Errorf("count spec versions %s: %w", d.Code, err)
		}
		if existing > 0 {
			continue
		}
		if _, err := s.pool.Exec(ctx,
			`INSERT INTO msg_event_type_spec_versions
			     (id, event_type_id, version, mime_type, schema_content, schema_type,
			      status, created_at, updated_at)
			 VALUES ($1, $2, '1.0', 'application/schema+json', $3, 'JSON_SCHEMA', 'CURRENT', $4, $4)`,
			tsid.Generate(tsid.Schema), id, []byte(d.Schema), now); err != nil {
			return fmt.Errorf("insert spec version %s: %w", d.Code, err)
		}
	}
	if inserted > 0 {
		slog.Info("seeded platform event types", "inserted", inserted, "total", len(defs))
	}
	return nil
}

// seedPlatformEventSchemas is a no-op now that schemas are attached
// inside seedPlatformEventTypes — keep the function so seed.go's
// scaffolding doesn't drift, and we have a place to hang schema-only
// upgrades in the future (e.g. a "2.0" rollout).
func (s *Seeder) seedPlatformEventSchemas(_ context.Context) error { return nil }

// ── helpers ──────────────────────────────────────────────────────────────

func lastSegment(s string, sep rune) string {
	if i := strings.LastIndex(s, string(sep)); i >= 0 {
		return s[i+1:]
	}
	return s
}

func titleCase(s string) string {
	parts := strings.Split(s, "-")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}
