//go:build integration

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// seedSyncTarget inserts a USER principal with the given scope and home client
// ("" for none), named "before".
func seedSyncTarget(t *testing.T, id, email, scope, clientID string) {
	t.Helper()
	var client *string
	if clientID != "" {
		client = &clientID
	}
	_, err := testpg.Pool(t).Exec(context.Background(),
		`INSERT INTO iam_principals (id, type, scope, client_id, name, active, email)
		 VALUES ($1, 'USER', $2, $3, 'before', TRUE, $4)`, id, scope, client, email)
	require.NoError(t, err)
}

func requireSyncTargetForbidden(t *testing.T, err error, label, email string) {
	t.Helper()
	ue := usecase.AsError(err)
	require.NotNil(t, ue, "%s: want a refusal, got %v", label, err)
	assert.Equal(t, usecase.KindAuthorization, ue.Kind, label)
	assert.Equal(t, "SYNC_TARGET_FORBIDDEN", ue.Code, label)
	assert.Equal(t, "Not authorised to manage principal '"+email+"'", ue.Message, label)
}

// Target reach (F12) on POST /api/principals/sync: an existing user the caller
// could not administer (a non-anchor reaches only CLIENT-scope users homed at
// a client it can access) refuses the whole sync, 403 SYNC_TARGET_FORBIDDEN,
// as in Java and Rust. Before the fix a client administrator holding a user
// permission could rename, deactivate or re-role any user by email, an anchor
// administrator's included.
func TestUserSync_TargetsWithinCallersReach(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := principal.NewRepository(pool)
	s := &State{Repo: repo, Roles: role.NewRepository(pool), UoW: testpg.NewUoW(t)}

	const mine, theirs = "cli_syncreach0001", "cli_syncreach0002"
	seedSyncTarget(t, "prn_syncreach_own", "own@syncreach.test", "CLIENT", mine)
	seedSyncTarget(t, "prn_syncreach_oth", "other@syncreach.test", "CLIENT", theirs)
	seedSyncTarget(t, "prn_syncreach_anc", "anchor@syncreach.test", "ANCHOR", "")
	seedSyncTarget(t, "prn_syncreach_ptn", "partner@syncreach.test", "PARTNER", mine)
	seedSyncTarget(t, "prn_syncreach_nul", "clientless@syncreach.test", "CLIENT", "")

	clientAdmin := auth.WithContext(ctx, clientScopedCaller("prn_syncreach_adm", mine,
		"platform:iam:user:create", "platform:iam:user:update"))
	sync := func(c context.Context, emails ...string) error {
		in := make([]SyncUserInput, 0, len(emails))
		for _, e := range emails {
			in = append(in, SyncUserInput{Email: e, Name: "after"})
		}
		_, err := s.syncUsers(c, &apicommon.In[SyncUsersRequest]{Body: SyncUsersRequest{Principals: in}})
		return err
	}
	nameOf := func(email string) string {
		t.Helper()
		p, err := repo.FindByEmail(ctx, email)
		require.NoError(t, err)
		require.NotNil(t, p, email)
		return p.Name
	}

	for _, email := range []string{
		"other@syncreach.test", "anchor@syncreach.test", "partner@syncreach.test", "clientless@syncreach.test",
	} {
		requireSyncTargetForbidden(t, sync(clientAdmin, email), email, email)
		assert.Equal(t, "before", nameOf(email), "%s untouched", email)
	}
	// Mixed case in the payload matches the stored email.
	requireSyncTargetForbidden(t, sync(clientAdmin, "Anchor@SyncReach.test"), "mixed case", "anchor@syncreach.test")

	// One out-of-reach entry refuses the whole sync.
	requireSyncTargetForbidden(t, sync(clientAdmin, "own@syncreach.test", "other@syncreach.test"),
		"mixed batch", "other@syncreach.test")
	assert.Equal(t, "before", nameOf("own@syncreach.test"), "a refused sync writes nothing")
	none, err := repo.FindByEmail(ctx, "new@syncreach.test")
	require.NoError(t, err)
	assert.Nil(t, none)

	// The legitimate flows: a user of the caller's client, a new user.
	require.NoError(t, sync(clientAdmin, "own@syncreach.test", "new@syncreach.test"))
	assert.Equal(t, "after", nameOf("own@syncreach.test"))
	assert.Equal(t, "after", nameOf("new@syncreach.test"))

	// An anchor reaches everyone.
	require.NoError(t, sync(anchorHolding("USER", "platform:iam:user:update"),
		"other@syncreach.test", "anchor@syncreach.test", "partner@syncreach.test", "clientless@syncreach.test"))
	assert.Equal(t, "after", nameOf("anchor@syncreach.test"))
}
