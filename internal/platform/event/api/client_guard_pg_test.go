//go:build integration

package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/event"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

const platformScopedRefused = "clientId is required: a client-scoped caller cannot write platform-scoped rows"

// requireForbidden asserts the 403 FORBIDDEN envelope with the exact message.
func requireForbidden(t *testing.T, err error, msg string) {
	t.Helper()
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "FORBIDDEN")
	assert.Equal(t, msg, usecase.AsError(err).Message)
}

// F14 (owner ruling 2026-10-12, Java requireWritableClient / Rust
// caller_reach::require_writable_client): POST /api/events/batch and POST
// /api/events write an event only under a client the caller reaches. A
// non-anchor never writes a platform-scoped event: with no client given, a
// caller confined to exactly one client writes under it, any other is refused.
// A clientCode naming another client or none is refused for a non-anchor; an
// anchor's unknown code leaves the event unscoped (the event is a fact). Any
// refusal rejects the whole batch.
func TestEventIngest_ClientGuard(t *testing.T) {
	pool := testpg.Pool(t)
	s := &State{Repo: event.NewRepository(pool), Clients: client.NewRepository(pool)}
	sfx := strings.ToLower(tsid.GenerateUntyped())
	aCode, bCode, cCode := "acme-"+sfx, "beta-"+sfx, "coda-"+sfx
	aID, bID, cID := tsid.Generate(tsid.Client), tsid.Generate(tsid.Client), tsid.Generate(tsid.Client)
	for _, c := range [][2]string{{aID, aCode}, {bID, bCode}, {cID, cCode}} {
		_, err := pool.Exec(context.Background(), `INSERT INTO tnt_clients (id, name, identifier) VALUES ($1, $2, $2)`, c[0], c[1])
		require.NoError(t, err)
	}
	perms := []string{"platform:messaging:batch:events-write"}
	anchor := appCtx(&auth.AuthContext{PrincipalID: "p_eg_anchor", Scope: auth.ScopeAnchor, Permissions: perms, AllApplications: true})
	single := appCtx(&auth.AuthContext{PrincipalID: "p_eg_client", Scope: auth.ScopeClient, Clients: []string{aID}, Permissions: perms, AllApplications: true})
	partner := appCtx(&auth.AuthContext{PrincipalID: "p_eg_partner", Scope: auth.ScopePartner, Clients: []string{aID, cID}, Permissions: perms, AllApplications: true})
	none := appCtx(&auth.AuthContext{PrincipalID: "p_eg_none", Scope: auth.ScopePartner, Permissions: perms, AllApplications: true})

	str := func(v string) *string { return &v }
	item := func(clientID, clientCode *string) BatchEventItem {
		return BatchEventItem{
			ID: tsid.GenerateUntyped(), Type: "eg:sales:order:created", Source: "eg",
			Data: json.RawMessage(`{"n":1}`), ClientID: clientID, ClientCode: clientCode,
		}
	}
	ingest := func(ctx context.Context, items ...BatchEventItem) ([]BatchResultItem, error) {
		out, err := s.batchIngest(ctx, &apicommon.In[BatchRequest]{Body: BatchRequest{Items: items}})
		if err != nil {
			return nil, err
		}
		return out.Body.Results, nil
	}
	// stored answers whether the event was written, and its client.
	stored := func(id string) (bool, *string) {
		t.Helper()
		var n int
		require.NoError(t, pool.QueryRow(context.Background(), `SELECT count(*) FROM msg_events WHERE id = $1`, id).Scan(&n))
		if n == 0 {
			return false, nil
		}
		var cid *string
		require.NoError(t, pool.QueryRow(context.Background(), `SELECT client_id FROM msg_events WHERE id = $1`, id).Scan(&cid))
		return true, cid
	}
	requireStoredUnder := func(res []BatchResultItem, want *string) {
		t.Helper()
		require.Len(t, res, 1)
		require.Equal(t, "SUCCESS", res[0].Status, res[0].Error)
		ok, cid := stored(res[0].ID)
		require.True(t, ok)
		assert.Equal(t, want, cid)
	}

	// A client-scoped caller: another client's id or code, or an unknown code,
	// refuses the whole batch; the valid item before it is not written.
	for _, c := range []struct {
		id, code *string
		msg      string
	}{
		{id: &bID, msg: "No access to client: " + bID},
		{code: &bCode, msg: "No access to client: " + bCode},
		{code: str("nosuch-client"), msg: "No access to client: nosuch-client"},
	} {
		ok := item(&aID, nil)
		_, err := ingest(single, ok, item(c.id, c.code))
		requireForbidden(t, err, c.msg)
		written, _ := stored(ok.ID)
		assert.False(t, written, "a refused batch writes nothing")
	}

	// Its own client by id or code; with none given, its one client.
	res, err := ingest(single, item(nil, &aCode))
	require.NoError(t, err)
	requireStoredUnder(res, &aID)
	res, err = ingest(single, item(nil, nil))
	require.NoError(t, err)
	requireStoredUnder(res, &aID)
	// An explicit clientId wins over the code.
	res, err = ingest(single, item(&aID, &bCode))
	require.NoError(t, err)
	requireStoredUnder(res, &aID)

	// A caller reaching several clients (or none) must name one.
	_, err = ingest(partner, item(nil, nil))
	requireForbidden(t, err, platformScopedRefused)
	_, err = ingest(none, item(nil, nil))
	requireForbidden(t, err, platformScopedRefused)
	res, err = ingest(partner, item(nil, &cCode))
	require.NoError(t, err)
	requireStoredUnder(res, &cID)

	// Anchor: any client; an unknown code leaves the event unscoped (Java's
	// rule; the event is a fact, kept).
	res, err = ingest(anchor, item(nil, &bCode))
	require.NoError(t, err)
	requireStoredUnder(res, &bID)
	res, err = ingest(anchor, item(nil, str("nosuch-client")))
	require.NoError(t, err)
	requireStoredUnder(res, nil)
	res, err = ingest(anchor, item(nil, nil))
	require.NoError(t, err)
	requireStoredUnder(res, nil)

	// Item validation still answers per item before the client guard.
	bad := item(nil, nil)
	bad.Data = nil
	res, err = ingest(single, bad, item(nil, nil))
	require.NoError(t, err)
	require.Len(t, res, 2)
	assert.Equal(t, "BAD_REQUEST", res[0].Status)
	assert.Equal(t, "SUCCESS", res[1].Status)

	// Singular create: a caller with no client and none given is refused; a
	// caller with clients defaults to its first; the explicit guard holds.
	create := func(ctx context.Context, clientID *string) (*CreatedEvent, error) {
		out, err := s.create(ctx, &apicommon.In[CreateEventRequest]{Body: CreateEventRequest{
			EventType: "eg:sales:order:created", Source: "eg", Data: json.RawMessage(`{"n":1}`), ClientID: clientID,
		}})
		if err != nil {
			return nil, err
		}
		return &out.Body.Event, nil
	}
	_, err = create(none, nil)
	requireForbidden(t, err, platformScopedRefused)
	_, err = create(single, &bID)
	requireForbidden(t, err, "No access to client: "+bID)
	ev, err := create(partner, nil)
	require.NoError(t, err)
	assert.Equal(t, &aID, ev.ClientID)
	ev, err = create(anchor, nil)
	require.NoError(t, err)
	assert.Nil(t, ev.ClientID)
}
