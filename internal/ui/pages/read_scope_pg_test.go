//go:build integration

package pages

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/event"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// The /ui pages that read the same data as the API routes apply the same tenant
// rule: a client-tier viewer sees its own clients' rows (and, where the API
// does, the client-less rows of ordinary applications) and a 404 for the rest;
// an anchor sees everything.
func TestUIReads_ConfinedToTheCallersTenants(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	sfx := strings.ToLower(tsid.GenerateUntyped())[5:]
	clientA, clientB := "clt_a"+sfx[:8], "clt_b"+sfx[:8]
	a, b := clientA, clientB
	at := time.Now().UTC().Add(time.Hour)

	perms := []string{
		"platform:admin:audit-log:view", "platform:messaging:event:view", "platform:messaging:event:view-raw",
		"platform:messaging:dispatch-job:view-raw", "platform:iam:service-account:view",
	}
	viewer := &auth.AuthContext{
		PrincipalID: "prn_uiviewer", Scope: auth.ScopeClient, Clients: []string{clientA}, Roles: []string{"r"},
		Permissions: perms, AllApplications: true, PrincipalType: auth.PrincipalTypeUser,
	}
	anchor := &auth.AuthContext{
		PrincipalID: "prn_uianchor", Scope: auth.ScopeAnchor, Roles: []string{"r"},
		Permissions: perms, AllApplications: true, PrincipalType: auth.PrincipalTypeUser,
	}
	get := func(ac *auth.AuthContext, target string, p mounter) (int, string) {
		t.Helper()
		rec := serveMounted(t, ac, req{target: target}, p)
		return rec.Code, rec.Body.String()
	}

	// Events: the read page and the raw page.
	seedEvent := func(n, typ string, client *string) string {
		id := "ue" + sfx + n
		_, err := pool.Exec(ctx, `INSERT INTO msg_events (id, type, source, time, deduplication_id, client_id, created_at)
			VALUES ($1, $2, 'test://scope', NOW(), $1, $3, $4)`, id, typ, client, at)
		require.NoError(t, err)
		_, err = pool.Exec(ctx, `INSERT INTO msg_events_read (id, type, source, time, client_id, application, subdomain, aggregate, created_at)
			VALUES ($1, $2::text, 'test://scope', NOW(), $3, split_part($2::text, ':', 1), 'sub', 'agg', $4)`, id, typ, client, at)
		require.NoError(t, err)
		return id
	}
	evA := seedEvent("a", "uirsa"+sfx+":sub:agg:happened", &a)
	evB := seedEvent("b", "uirsb"+sfx+":sub:agg:happened", &b)
	evN := seedEvent("n", "uirsn"+sfx+":sub:agg:happened", nil)
	evP := seedEvent("p", "platform:iam:user:created", nil)

	events := &Events{Events: event.NewRepository(pool), Clients: client.NewRepository(pool), Shell: shellForTest()}
	rawEvents := &DebugEvents{Events: event.NewRepository(pool), Shell: shellForTest()}
	t.Run("events page detail", func(t *testing.T) {
		for _, id := range []string{evA, evN} {
			code, _ := get(viewer, "/ui/events/"+id, events)
			assert.Equal(t, http.StatusOK, code, id)
		}
		code, _ := get(viewer, "/ui/events/"+evB, events)
		assert.Equal(t, http.StatusForbidden, code, "another tenant's event keeps the 403 the API answers")
		code, _ = get(viewer, "/ui/events/"+evP, events)
		assert.Equal(t, http.StatusNotFound, code, "the platform's client-less control-plane event")
		for _, id := range []string{evA, evB, evN, evP} {
			code, _ = get(anchor, "/ui/events/"+id, events)
			assert.Equal(t, http.StatusOK, code, "an anchor reads %s", id)
		}
	})
	t.Run("raw events page", func(t *testing.T) {
		_, body := get(viewer, "/ui/debug/events?size=1000", rawEvents)
		assert.Contains(t, body, "/ui/debug/events/"+evA)
		assert.Contains(t, body, "/ui/debug/events/"+evN)
		assert.NotContains(t, body, "/ui/debug/events/"+evB)
		assert.NotContains(t, body, "/ui/debug/events/"+evP)
		_, body = get(anchor, "/ui/debug/events?size=1000", rawEvents)
		for _, id := range []string{evA, evB, evN, evP} {
			assert.Contains(t, body, "/ui/debug/events/"+id, "an anchor sees %s", id)
		}
		for _, id := range []string{evB, evP} {
			code, _ := get(viewer, "/ui/debug/events/"+id, rawEvents)
			assert.Equal(t, http.StatusNotFound, code, id)
		}
		code, _ := get(viewer, "/ui/debug/events/"+evA, rawEvents)
		assert.Equal(t, http.StatusOK, code)
	})

	// Raw dispatch jobs.
	seedJob := func(n string, client *string) string {
		id := "uj" + sfx + n
		_, err := pool.Exec(ctx, `INSERT INTO msg_dispatch_jobs (id, code, target_url, status, client_id, created_at, updated_at)
			VALUES ($1, $2, 'http://example.invalid/hook', 'PENDING', $3, $4, $4)`, id, "uirs"+sfx+":sub:agg:"+n, client, at)
		require.NoError(t, err)
		return id
	}
	jobA, jobB := seedJob("a", &a), seedJob("b", &b)
	rawJobs := &DebugDispatchJobs{Jobs: dispatchjob.NewRepository(pool), Shell: shellForTest()}
	t.Run("raw dispatch jobs page", func(t *testing.T) {
		_, body := get(viewer, "/ui/debug/dispatch-jobs?size=1000", rawJobs)
		assert.Contains(t, body, "/ui/debug/dispatch-jobs/"+jobA)
		assert.NotContains(t, body, "/ui/debug/dispatch-jobs/"+jobB)
		_, body = get(anchor, "/ui/debug/dispatch-jobs?size=1000", rawJobs)
		assert.Contains(t, body, "/ui/debug/dispatch-jobs/"+jobA)
		assert.Contains(t, body, "/ui/debug/dispatch-jobs/"+jobB)
	})

	// Audit log.
	auditRepo := audit.NewRepository(pool)
	seedAudit := func(n string, client *string) string {
		id := "ua" + sfx + n
		require.NoError(t, auditRepo.Insert(ctx, &audit.Log{
			ID: id, EntityType: "UiRs" + sfx, EntityID: "e-" + n, Operation: "UiRsOp" + strings.ToUpper(n) + sfx,
			ClientID: client, PerformedAt: time.Now().UTC(),
		}))
		return id
	}
	audA, audB, audP := seedAudit("a", &a), seedAudit("b", &b), seedAudit("p", nil)
	auditPage := &AuditLogPage{Store: auditRepo, Apps: newAppHarness().store, Clients: client.NewRepository(pool), Shell: shellForTest()}
	t.Run("audit log page", func(t *testing.T) {
		_, body := get(viewer, "/ui/audit-log?entityType=UiRs"+sfx, auditPage)
		assert.Contains(t, body, audA)
		assert.NotContains(t, body, audB)
		assert.NotContains(t, body, audP)
		assert.NotContains(t, body, "UiRsOpB"+sfx, "facet values of another tenant's rows are absent")
		_, body = get(anchor, "/ui/audit-log?entityType=UiRs"+sfx, auditPage)
		for _, id := range []string{audA, audB, audP} {
			assert.Contains(t, body, id, "an anchor sees %s", id)
		}
		for _, id := range []string{audB, audP} {
			code, _ := get(viewer, "/ui/audit-log/"+id, auditPage)
			assert.Equal(t, http.StatusNotFound, code, id)
		}
		code, _ := get(viewer, "/ui/audit-log/"+audA, auditPage)
		assert.Equal(t, http.StatusOK, code)
	})

	// Service accounts.
	seedSA := func(n string, clients []string) string {
		id := "sac_ui" + sfx + n
		_, err := pool.Exec(ctx, `INSERT INTO iam_service_accounts (id, code, name, client_ids, active, created_at, updated_at)
			VALUES ($1, $1, $2, $3, true, NOW(), NOW())`, id, "UiRsSa"+strings.ToUpper(n)+sfx, clients)
		require.NoError(t, err)
		return id
	}
	saA, saB := seedSA("a", []string{clientA}), seedSA("b", []string{clientB})
	saPlatform := seedSA("p", []string{})
	saPage := &ServiceAccounts{
		Accounts: serviceaccount.NewRepository(pool), Principals: principal.NewRepository(pool),
		OAuthClients: platformauth.NewRepository(pool).OAuthClients, Roles: role.NewRepository(pool),
		Clients: client.NewRepository(pool), Apps: application.NewRepository(pool),
		Connections: connection.NewRepository(pool), Shell: shellForTest(),
	}
	t.Run("service accounts page", func(t *testing.T) {
		_, body := get(viewer, "/ui/service-accounts", saPage)
		assert.Contains(t, body, "UiRsSaA"+sfx)
		assert.NotContains(t, body, "UiRsSaB"+sfx)
		assert.NotContains(t, body, "UiRsSaP"+sfx)
		_, body = get(anchor, "/ui/service-accounts", saPage)
		for _, n := range []string{"A", "B", "P"} {
			assert.Contains(t, body, "UiRsSa"+n+sfx, "an anchor sees account %s", n)
		}
		for _, id := range []string{saB, saPlatform} {
			code, _ := get(viewer, "/ui/service-accounts/"+id, saPage)
			assert.Equal(t, http.StatusNotFound, code, id)
		}
		code, _ := get(viewer, "/ui/service-accounts/"+saA, saPage)
		assert.Equal(t, http.StatusOK, code)
		code, _ = get(anchor, "/ui/service-accounts/"+saB, saPage)
		assert.Equal(t, http.StatusOK, code)
	})
}
