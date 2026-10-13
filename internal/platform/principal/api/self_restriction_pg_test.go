//go:build integration

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

func seedAnchorAdmin(t *testing.T, id string) {
	t.Helper()
	_, err := testpg.Pool(t).Exec(context.Background(),
		`INSERT INTO iam_principals (id, type, scope, name, active, email, all_applications)
		 VALUES ($1, 'USER', 'ANCHOR', 'Self Restr Admin', TRUE, $2, TRUE)`,
		id, id+"@selfrestr.example")
	require.NoError(t, err)
}

func adminCaller(id string) context.Context {
	return auth.WithContext(context.Background(), &auth.AuthContext{
		PrincipalID:     id,
		Scope:           auth.ScopeAnchor,
		AllApplications: true,
		Permissions:     []string{"platform:*:*:*"},
	})
}

func selfRestrictState(t *testing.T) *State {
	pool := testpg.Pool(t)
	return &State{
		Repo:         principal.NewRepository(pool),
		Applications: application.NewRepository(pool),
		UoW:          testpg.NewUoW(t),
	}
}

func allApps(t *testing.T, s *State, id string) bool {
	t.Helper()
	p, err := s.Repo.FindByID(context.Background(), ids.PrincipalID(id))
	require.NoError(t, err)
	require.NotNil(t, p)
	return p.AllApplications
}

func TestAssignAppAccess_SelfRestriction_RequiresConfirmation(t *testing.T) {
	s := selfRestrictState(t)
	const me, other = "prn_selfrst0001", "prn_selfrst0002"
	seedAnchorAdmin(t, me)
	seedAnchorAdmin(t, other)
	ctx := adminCaller(me)
	f := false
	tr := true

	for name, confirm := range map[string]*bool{"absent": nil, "false": &f} {
		_, err := s.assignApplicationAccess(ctx, &assignAppAccessInput{ID: ids.PrincipalID(me),
			Body: AssignApplicationAccessRequest{ApplicationIDs: []string{}, AllApplications: &f, ConfirmSelfRestriction: confirm}})
		testpg.RequireUsecaseError(t, err, usecase.KindConflict, "SELF_RESTRICTION_REQUIRES_CONFIRMATION")
		assert.Equal(t, 409, usecase.AsError(err).HTTPStatus(), name)
		assert.Contains(t, usecase.AsError(err).Message, "no longer be able to grant all-applications access")
		assert.True(t, allApps(t, s, me), "unconfirmed request must change nothing (%s)", name)
	}

	// Confirmed, and another administrator exists (the shared fixture DB always
	// has `other`): succeeds.
	out, err := s.assignApplicationAccess(ctx, &assignAppAccessInput{ID: ids.PrincipalID(me),
		Body: AssignApplicationAccessRequest{ApplicationIDs: []string{}, AllApplications: &f, ConfirmSelfRestriction: &tr}})
	require.NoError(t, err)
	assert.False(t, out.Body.AllApplications)
	assert.False(t, allApps(t, s, me))
}

func TestAssignAppAccess_SelfRestriction_NotRequiredOtherwise(t *testing.T) {
	s := selfRestrictState(t)
	const me, other = "prn_selfrst0003", "prn_selfrst0004"
	seedAnchorAdmin(t, me)
	seedAnchorAdmin(t, other)
	ctx := adminCaller(me)
	f := false
	tr := true

	// Restricting SOMEONE ELSE needs no confirmation (ordinary case).
	_, err := s.assignApplicationAccess(ctx, &assignAppAccessInput{ID: ids.PrincipalID(other),
		Body: AssignApplicationAccessRequest{ApplicationIDs: []string{}, AllApplications: &f}})
	require.NoError(t, err)
	assert.False(t, allApps(t, s, other))

	// Self edit that leaves the flag alone (nil) or keeps it true needs none.
	_, err = s.assignApplicationAccess(ctx, &assignAppAccessInput{ID: ids.PrincipalID(me),
		Body: AssignApplicationAccessRequest{ApplicationIDs: []string{}}})
	require.NoError(t, err)
	_, err = s.assignApplicationAccess(ctx, &assignAppAccessInput{ID: ids.PrincipalID(me),
		Body: AssignApplicationAccessRequest{ApplicationIDs: []string{}, AllApplications: &tr}})
	require.NoError(t, err)
	assert.True(t, allApps(t, s, me))
}

func TestAssignAppAccess_PermissionChecksUnchanged(t *testing.T) {
	s := selfRestrictState(t)
	const me = "prn_selfrst0005"
	seedAnchorAdmin(t, me)
	f := false
	tr := true

	// No write permission: refused before any load, confirmation or not.
	noPerm := auth.WithContext(context.Background(), &auth.AuthContext{
		PrincipalID: me, Scope: auth.ScopeAnchor, AllApplications: true})
	_, err := s.assignApplicationAccess(noPerm, &assignAppAccessInput{ID: ids.PrincipalID(me),
		Body: AssignApplicationAccessRequest{ApplicationIDs: []string{}, AllApplications: &f, ConfirmSelfRestriction: &tr}})
	require.Error(t, err)
	assert.True(t, allApps(t, s, me))

	// A caller without all-applications access still cannot GRANT it.
	restricted := auth.WithContext(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_selfrstother", Scope: auth.ScopeAnchor, Permissions: []string{"platform:*:*:*"}})
	_, err = s.assignApplicationAccess(restricted, &assignAppAccessInput{ID: ids.PrincipalID(me),
		Body: AssignApplicationAccessRequest{ApplicationIDs: []string{}, AllApplications: &tr}})
	require.Error(t, err)
}
