//go:build integration

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/event"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// GET /api/events/filter-options lists only the values of events the caller
// can see (the bug 18 flaw, found here too; docs/platform-bug-fixes.md).
func TestEventFilterOptions_ScopedToTheCallersClients(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	mine, other := "clt_efomine0001", "clt_efoother001"
	for i, row := range []struct {
		typ    string
		client *string
	}{{"efomine:sales:order:created", &mine}, {"efoother:sales:order:created", &other}, {"efoplat:sales:order:created", nil}} {
		_, err := pool.Exec(ctx, `INSERT INTO msg_events_read (id, type, source, subject, time, data, application, subdomain, aggregate, client_id, created_at)
			VALUES ($1, $2::text, 'svc', 'order/1', NOW(), '{}', split_part($2::text, ':', 1), split_part($2::text, ':', 2), split_part($2::text, ':', 3), $3, NOW())`,
			"efo"+string(rune('a'+i)), row.typ, row.client)
		require.NoError(t, err)
	}
	s := &State{Repo: event.NewRepository(pool)}
	caller := auth.WithContext(ctx, &auth.AuthContext{PrincipalID: "prn_efo", Scope: auth.ScopeClient, Clients: []string{mine},
		Permissions: []string{"platform:messaging:event:view"}})

	out, err := s.filterOptions(caller, &apicommon.Empty{})
	require.NoError(t, err)
	var types, apps []string
	for _, o := range out.Body.EventTypes {
		types = append(types, o.Value)
	}
	for _, o := range out.Body.Applications {
		apps = append(apps, o.Value)
	}
	assert.Contains(t, types, "efomine:sales:order:created")
	assert.Contains(t, types, "efoplat:sales:order:created")
	assert.NotContains(t, types, "efoother:sales:order:created")
	assert.NotContains(t, apps, "efoother")
}
