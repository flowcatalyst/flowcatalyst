//go:build integration

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// The by-id read and its sub-routes apply the list's visibility rule
// (PrincipalVisible) to principals with no home client too. Before, they only
// checked access when client_id was set, so a client administrator could read
// an anchor user (name, email, roles) or a partner of other clients by id.
// Out of scope answers the not-found a missing id would (PR-3/PR-4).
func TestPlatformPrincipalsByIDFollowTheListRule(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	s := &State{Repo: principal.NewRepository(pool), Applications: application.NewRepository(pool)}

	const (
		callerClient  = "cli_platvis0001"
		otherClient   = "cli_platvis0002"
		anchorPID     = "prn_platvisanch1"
		partnerMine   = "prn_platvispartm"
		partnerOther  = "prn_platvisparto"
		callerPID     = "prn_platviscall1"
		partnerSelfID = partnerMine
	)
	exec := func(sql string, args ...any) {
		t.Helper()
		_, err := pool.Exec(ctx, sql, args...)
		require.NoError(t, err)
	}
	exec(`INSERT INTO iam_principals (id, type, scope, client_id, name, active, email)
	      VALUES ($1, 'USER', 'ANCHOR', NULL, 'Anchor Admin', TRUE, 'platvis-anchor@example.com')`, anchorPID)
	exec(`INSERT INTO iam_principals (id, type, scope, client_id, name, active, email)
	      VALUES ($1, 'USER', 'PARTNER', NULL, 'Partner Mine', TRUE, 'platvis-pm@example.com')`, partnerMine)
	exec(`INSERT INTO iam_principals (id, type, scope, client_id, name, active, email)
	      VALUES ($1, 'USER', 'PARTNER', NULL, 'Partner Other', TRUE, 'platvis-po@example.com')`, partnerOther)
	exec(`INSERT INTO iam_client_access_grants (id, principal_id, client_id, granted_by) VALUES ('cag_platvis0001', $1, $2, $3)`, partnerMine, callerClient, anchorPID)
	exec(`INSERT INTO iam_client_access_grants (id, principal_id, client_id, granted_by) VALUES ('cag_platvis0002', $1, $2, $3)`, partnerOther, otherClient, anchorPID)

	clientAdmin := auth.WithContext(ctx, clientScopedCaller(callerPID, callerClient, testPermUserView))
	anchor := auth.WithContext(ctx, &auth.AuthContext{PrincipalID: "prn_platvisanch2", Scope: auth.ScopeAnchor, Permissions: []string{testPermUserView}})
	partnerSelf := auth.WithContext(ctx, &auth.AuthContext{PrincipalID: partnerSelfID, Scope: auth.ScopePartner, Clients: []string{callerClient}})

	type read func(context.Context, *apicommon.IDInput) error
	routes := map[string]read{
		"getByID":    func(c context.Context, in *apicommon.IDInput) error { _, err := s.getByID(c, in); return err },
		"getVersion": func(c context.Context, in *apicommon.IDInput) error { _, err := s.getVersion(c, in); return err },
		"listRoles":  func(c context.Context, in *apicommon.IDInput) error { _, err := s.listRoles(c, in); return err },
		"listApplicationAccess": func(c context.Context, in *apicommon.IDInput) error {
			_, err := s.listApplicationAccess(c, in)
			return err
		},
		"listAvailableApplications": func(c context.Context, in *apicommon.IDInput) error {
			_, err := s.listAvailableApplications(c, in)
			return err
		},
	}
	notFound := func(t *testing.T, id string, err error) {
		t.Helper()
		require.Error(t, err)
		status, got := envelopeOf(err)
		require.Equal(t, 404, status, "want 404, got %v", err)
		assertCanonicalNotFound(t, "Principal", id, got, "out-of-scope platform principal")
	}

	for name, r := range routes {
		t.Run(name, func(t *testing.T) {
			// A client administrator: the anchor user and the other client's
			// partner are not found; the partner granted their client is read.
			notFound(t, anchorPID, r(clientAdmin, &apicommon.IDInput{ID: anchorPID}))
			notFound(t, partnerOther, r(clientAdmin, &apicommon.IDInput{ID: partnerOther}))
			require.NoError(t, r(clientAdmin, &apicommon.IDInput{ID: partnerMine}))
			// An anchor reads all of them.
			for _, id := range []string{anchorPID, partnerMine, partnerOther} {
				require.NoError(t, r(anchor, &apicommon.IDInput{ID: id}), id)
			}
		})
	}
	// A principal with no home client still reads itself (the routes that
	// exempt self do so before the permission check).
	_, err := s.getByID(partnerSelf, &apicommon.IDInput{ID: partnerSelfID})
	require.NoError(t, err)
	_, err = s.getVersion(partnerSelf, &apicommon.IDInput{ID: partnerSelfID})
	require.NoError(t, err)
}
