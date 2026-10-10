//go:build integration

package api

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/event"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// GET /api/events/{id} carries the event's context data (bug 22 in
// docs/ui/README.md; docs/platform-bug-fixes.md): it read msg_events_read,
// which has no context column, so contextData was always empty.
func TestGetEvent_CarriesContextData(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	at := time.Now().UTC()
	_, err := pool.Exec(ctx, `INSERT INTO msg_events (id, spec_version, type, source, subject, time, data, deduplication_id, context_data, created_at)
		VALUES ('evctx00000001', '1.0', 'ctxtest:sales:order:created', 'svc', 'order/1', $1, '{}', 'dedup-evctx-1', '[{"key":"tenant","value":"acme"}]', $1)`, at)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO msg_events_read (id, type, source, subject, time, data, created_at)
		VALUES ('evctx00000001', 'ctxtest:sales:order:created', 'svc', 'order/1', $1, '{}', $1)`, at)
	require.NoError(t, err)

	s := &State{Repo: event.NewRepository(pool)}
	viewer := auth.WithContext(ctx, &auth.AuthContext{PrincipalID: "prn_evctx", Scope: auth.ScopeAnchor, Permissions: []string{"platform:messaging:event:view"}})
	out, err := s.getByID(viewer, &apicommon.IDInput{ID: "evctx00000001"})
	require.NoError(t, err)
	require.Len(t, out.Body.Context, 1)
	assert.Equal(t, ContextEntryDTO{Key: "tenant", Value: "acme"}, out.Body.Context[0])
}
