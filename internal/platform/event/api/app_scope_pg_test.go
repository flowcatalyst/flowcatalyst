//go:build integration

package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/event"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

func appCtx(ac *auth.AuthContext) context.Context {
	return auth.WithContext(context.Background(), ac)
}

// TestEventIngest_ApplicationConfinement pins the application-scoped caller
// rules for POST /api/events and /api/events/batch: the application of an
// event is the first segment of its type.
func TestEventIngest_ApplicationConfinement(t *testing.T) {
	pool := testpg.Pool(t)
	s := &State{Repo: event.NewRepository(pool), Apps: application.NewRepository(pool)}
	sfx := strings.ToLower(tsid.GenerateUntyped())[5:]
	own, other := "own"+sfx, "oth"+sfx
	ownID, otherID := tsid.Generate(tsid.Application), tsid.Generate(tsid.Application)
	for _, a := range [][2]string{{ownID, own}, {otherID, other}} {
		_, err := pool.Exec(context.Background(), `INSERT INTO app_applications (id, code, name) VALUES ($1, $2, $2)`, a[0], a[1])
		require.NoError(t, err)
	}
	perms := []string{"platform:messaging:batch:events-write"}
	scoped := appCtx(&auth.AuthContext{PrincipalID: "p_sc", Scope: auth.ScopeAnchor, Permissions: perms, Applications: []string{ownID}})
	admin := appCtx(&auth.AuthContext{PrincipalID: "p_ad", Scope: auth.ScopeAnchor, Permissions: perms, AllApplications: true})

	item := func(typ string) BatchEventItem {
		return BatchEventItem{Type: typ, Source: "test://scope", Data: json.RawMessage(`{"n":1}`)}
	}
	batch := func(ctx context.Context, types ...string) []BatchResultItem {
		var items []BatchEventItem
		for _, ty := range types {
			items = append(items, item(ty))
		}
		out, err := s.batchIngest(ctx, &apicommon.In[BatchRequest]{Body: BatchRequest{Items: items}})
		require.NoError(t, err)
		require.Len(t, out.Body.Results, len(types))
		return out.Body.Results
	}

	// Mixed batch: own persisted, other / unknown refused per item, positional.
	res := batch(scoped, own+":a:b:c", other+":a:b:c", "nosuchapp:a:b:c", own+":a:b:d")
	assert.Equal(t, "SUCCESS", res[0].Status)
	assert.Equal(t, "BAD_REQUEST", res[1].Status)
	assert.Equal(t, eventTypeAppRefused, res[1].Error)
	assert.Equal(t, "BAD_REQUEST", res[2].Status)
	assert.Equal(t, eventTypeAppRefused, res[2].Error)
	assert.Equal(t, "SUCCESS", res[3].Status)
	var n int
	require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM msg_events WHERE type LIKE $1`, other+":%").Scan(&n))
	assert.Zero(t, n)

	// Non-scoped caller unaffected, including an unknown application code.
	res = batch(admin, other+":a:b:c", "nosuchapp:a:b:c")
	assert.Equal(t, "SUCCESS", res[0].Status)
	assert.Equal(t, "SUCCESS", res[1].Status)

	// Singular create: own ok, other / unknown 403, admin ok.
	create := func(ctx context.Context, typ string) error {
		_, err := s.create(ctx, &apicommon.In[CreateEventRequest]{Body: CreateEventRequest{
			EventType: typ, Source: "test://scope", Data: json.RawMessage(`{"n":1}`)}})
		return err
	}
	require.NoError(t, create(scoped, own+":a:b:c"))
	testpg.RequireUsecaseError(t, create(scoped, other+":a:b:c"), usecase.KindAuthorization, "FORBIDDEN")
	testpg.RequireUsecaseError(t, create(scoped, "nosuchapp:a:b:c"), usecase.KindAuthorization, "FORBIDDEN")
	require.NoError(t, create(admin, other+":a:b:c"))
}
