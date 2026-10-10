//go:build integration

package api

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// Event types have no client (msg_event_types has no client column). A
// clientId in the create body (the SDKs expose it) is accepted and ignored:
// the type is platform-wide, its created event names no client, and creating
// it needs anchor reach whatever clientId says. It used to pass the scope
// check for the caller's own client and then be dropped, so a client user
// created a platform-wide event type.
func TestCreateEventType_ClientIDIsIgnored(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := eventtype.NewRepository(pool)
	s := &State{Repo: repo, UoW: testpg.NewUoW(t)}
	body := func(code string) *apicommon.In[CreateEventTypeRequest] {
		var req CreateEventTypeRequest
		require.NoError(t, json.Unmarshal([]byte(`{"code":"`+code+`","name":"N","clientId":"cli_etignore01"}`), &req))
		return &apicommon.In[CreateEventTypeRequest]{Body: req}
	}

	// An anchor's create succeeds; nothing names a client.
	out, err := s.create(testpg.AnchorCtx(), body("etignore:orders:order:created"))
	require.NoError(t, err)
	got, err := repo.FindByID(ctx, out.Body.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Nil(t, got.ClientID)
	var data map[string]any
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT data FROM msg_events WHERE type = $1 AND data->>'eventTypeId' = $2`,
		operations.EventTypeCreatedType, out.Body.ID).Scan(&data))
	assert.NotContains(t, data, "clientId", "the created event must not name a client the type does not have")

	// A client user naming its own client is refused: the type would be
	// platform-wide.
	clientUser := testpg.WithAuth(ctx, &auth.AuthContext{
		PrincipalID: "prn_etignore0001", Scope: auth.ScopeClient,
		Clients:     []string{"cli_etignore01"},
		Permissions: []string{"platform:messaging:event-type:create"},
	})
	_, err = s.create(clientUser, body("etignore:orders:order:shipped"))
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "SCOPE_FORBIDDEN")
	none, err := repo.FindByCode(ctx, "etignore:orders:order:shipped")
	require.NoError(t, err)
	assert.Nil(t, none)
}
