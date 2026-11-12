//go:build integration

package api

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/go-sdk/usecase"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// A permission says what a caller may read; the tier and client list say for
// which tenants. A client-tier caller holding the audit view permission reads
// only the rows of its own clients: never another tenant's, never a platform
// record (a row with no client).
func TestAuditReads_ConfinedToTheCallersClients(t *testing.T) {
	pool := testpg.Pool(t)
	repo := audit.NewRepository(pool)
	s := &State{Repo: repo}
	sfx := strings.ToLower(tsid.GenerateUntyped())[5:]
	clientA, clientB := "clt_a"+sfx[:8], "clt_b"+sfx[:8]
	entity := "SECRD" + sfx // unique entity type: facet values are ours alone

	seed := func(op string, client *string, at time.Duration) string {
		id := tsid.Generate(tsid.AuditLog)
		require.NoError(t, repo.Insert(context.Background(), &audit.Log{
			ID: id, EntityType: entity, EntityID: "ent-" + op, Operation: op,
			ClientID: client, PerformedAt: time.Now().UTC().Add(at),
		}))
		return id
	}
	a, b := clientA, clientB
	rowA := seed("OPA"+sfx, &a, -3*time.Second)
	rowB := seed("OPB"+sfx, &b, -2*time.Second)
	rowPlatform := seed("OPP"+sfx, nil, -1*time.Second)

	viewer := func(clients ...string) context.Context {
		return testpg.WithAuth(context.Background(), &auth.AuthContext{
			PrincipalID: "prn_auditviewer", Scope: auth.ScopeClient, Clients: clients,
			Permissions: []string{viewPerm}, AllApplications: true,
		})
	}
	anchor := testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_auditanchor", Scope: auth.ScopeAnchor,
		Permissions: []string{viewPerm}, AllApplications: true,
	})
	ids := func(rows []AuditLogResponse) []string {
		var out []string
		for _, r := range rows {
			out = append(out, r.ID)
		}
		return out
	}
	list := func(ctx context.Context, in listInput) []string {
		t.Helper()
		in.EntityType = entity
		in.PageSize = 200
		out, err := s.list(ctx, &in)
		require.NoError(t, err)
		return ids(out.Body.AuditLogs)
	}

	t.Run("list", func(t *testing.T) {
		got := list(viewer(clientA), listInput{})
		assert.Equal(t, []string{rowA}, got, "own row only: no other tenant, no platform record")
		assert.ElementsMatch(t, []string{rowA, rowB, rowPlatform}, list(anchor, listInput{}), "an anchor sees all three")
	})

	t.Run("a clientIds filter cannot widen it", func(t *testing.T) {
		assert.Empty(t, list(viewer(clientA), listInput{ClientIDs: clientB}))
		assert.Equal(t, []string{rowA}, list(viewer(clientA), listInput{ClientIDs: clientA + "," + clientB}))
	})

	t.Run("by id", func(t *testing.T) {
		got, err := s.getByID(viewer(clientA), &apicommon.IDInput{ID: rowA})
		require.NoError(t, err)
		assert.Equal(t, rowA, got.Body.ID)
		for _, other := range []string{rowB, rowPlatform} {
			_, err = s.getByID(viewer(clientA), &apicommon.IDInput{ID: other})
			testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "AuditLog_NOT_FOUND")
		}
		for _, id := range []string{rowA, rowB, rowPlatform} {
			_, err = s.getByID(anchor, &apicommon.IDInput{ID: id})
			require.NoError(t, err, "an anchor reads every row")
		}
	})

	t.Run("by entity and by principal", func(t *testing.T) {
		in := func(op string) *byEntityInput { return &byEntityInput{EntityType: entity, EntityID: "ent-" + op} }
		out, err := s.byEntity(viewer(clientA), in("OPA"+sfx))
		require.NoError(t, err)
		assert.Equal(t, []string{rowA}, ids(out.Body.AuditLogs))
		for _, op := range []string{"OPB" + sfx, "OPP" + sfx} {
			out, err = s.byEntity(viewer(clientA), in(op))
			require.NoError(t, err)
			assert.Empty(t, out.Body.AuditLogs)
			out, err = s.byEntity(anchor, in(op))
			require.NoError(t, err)
			assert.Len(t, out.Body.AuditLogs, 1)
		}
	})

	t.Run("facets", func(t *testing.T) {
		ops, err := s.operations(viewer(clientA), &apicommon.Empty{})
		require.NoError(t, err)
		assert.Contains(t, ops.Body.Operations, "OPA"+sfx)
		assert.NotContains(t, ops.Body.Operations, "OPB"+sfx)
		assert.NotContains(t, ops.Body.Operations, "OPP"+sfx)
		ets, err := s.entityTypes(viewer(clientA), &apicommon.Empty{})
		require.NoError(t, err)
		assert.Contains(t, ets.Body.EntityTypes, entity)
		cids, err := s.clientIDs(viewer(clientA), &apicommon.Empty{})
		require.NoError(t, err)
		assert.Contains(t, cids.Body.ClientIDs, clientA)
		assert.NotContains(t, cids.Body.ClientIDs, clientB)
		// A viewer of B only sees B's values, and none of A's.
		ops, err = s.operations(viewer(clientB), &apicommon.Empty{})
		require.NoError(t, err)
		assert.Contains(t, ops.Body.Operations, "OPB"+sfx)
		assert.NotContains(t, ops.Body.Operations, "OPA"+sfx)
		// An anchor sees every value.
		ops, err = s.operations(anchor, &apicommon.Empty{})
		require.NoError(t, err)
		assert.Subset(t, ops.Body.Operations, []string{"OPA" + sfx, "OPB" + sfx, "OPP" + sfx})
		cids, err = s.clientIDs(anchor, &apicommon.Empty{})
		require.NoError(t, err)
		assert.Subset(t, cids.Body.ClientIDs, []string{clientA, clientB})
	})

	t.Run("a caller with no clients sees nothing", func(t *testing.T) {
		assert.Empty(t, list(viewer(), listInput{}))
	})
}
