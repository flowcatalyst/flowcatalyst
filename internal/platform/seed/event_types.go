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

// PlatformEventTypeDef mirrors fc-platform/src/event_type/operations::SyncEventTypeInput.
type PlatformEventTypeDef struct {
	Code   string
	Name   string
	Schema json.RawMessage // nil-OK; empty means "no schema attached yet"
}

// PlatformEventTypes returns the full catalog in the same order as
// fc-platform/src/seed/platform_event_types.rs::definitions(). Codes are
// {application}:{subdomain}:{aggregate}:{event}.
func PlatformEventTypes() []PlatformEventTypeDef {
	schemas := platformEventSchemas()
	out := make([]PlatformEventTypeDef, 0, 64)

	group := func(prefix string, events ...string) {
		aggregate := lastSegment(prefix, ':')
		for _, ev := range events {
			code := prefix + ":" + ev
			out = append(out, PlatformEventTypeDef{
				Code:   code,
				Name:   titleCase(aggregate) + " " + titleCase(ev),
				Schema: schemas[code],
			})
		}
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
		"logged-in", "password-reset-requested", "password-reset-completed")
	push("platform:iam:principals:synced", "Principals Synced")

	group("platform:iam:serviceaccount",
		"created", "updated", "deleted", "roles-assigned",
		"token-regenerated", "secret-regenerated")

	group("platform:iam:client",
		"created", "updated", "activated", "suspended", "deleted", "note-added")

	group("platform:iam:role", "created", "updated", "deleted")
	push("platform:iam:roles:synced", "Roles Synced")

	group("platform:iam:application",
		"created", "updated", "activated", "deactivated", "deleted",
		"service-account-provisioned", "enabled-for-client", "disabled-for-client")

	group("platform:iam:anchor-domain", "created", "deleted")

	group("platform:iam:auth-config", "created", "updated", "deleted")

	// ── platform:admin ──────────────────────────────────────────────────
	group("platform:admin:cors", "origin-added", "origin-deleted")

	group("platform:admin:idp", "created", "updated", "deleted")

	group("platform:admin:edm", "created", "updated", "deleted")

	group("platform:admin:eventtype",
		"created", "updated", "archived", "deleted",
		"schema-added", "schema-finalised", "schema-deprecated")
	push("platform:admin:eventtypes:synced", "Event Types Synced")

	group("platform:admin:connection", "created", "updated", "deleted")

	group("platform:admin:dispatch-pool",
		"created", "updated", "archived", "deleted")
	push("platform:admin:dispatch-pools:synced", "Dispatch Pools Synced")

	group("platform:admin:subscription",
		"created", "updated", "paused", "resumed", "deleted", "synced")

	return out
}

// seedPlatformEventTypes upserts the full event-type catalog. Mirrors
// the Rust SyncEventTypesInputUseCase behaviour: insert if not present
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

		// Schema attach (idempotent). Version "v1" by convention.
		if len(d.Schema) == 0 {
			continue
		}
		var existing int
		if err := s.pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM msg_event_type_spec_versions
			  WHERE event_type_id = $1 AND version = 'v1'`, id).Scan(&existing); err != nil {
			return fmt.Errorf("count spec versions %s: %w", d.Code, err)
		}
		if existing > 0 {
			continue
		}
		if _, err := s.pool.Exec(ctx,
			`INSERT INTO msg_event_type_spec_versions
			     (id, event_type_id, version, mime_type, schema_content, schema_type,
			      status, created_at, updated_at)
			 VALUES ($1, $2, 'v1', 'application/schema+json', $3, 'JSON', 'CURRENT', $4, $4)`,
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
// upgrades in the future (e.g. a "v2" rollout).
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
