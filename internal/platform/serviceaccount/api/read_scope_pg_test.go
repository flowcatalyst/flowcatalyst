//go:build integration

package api

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// A service account is visible to a non-anchor caller only when it is linked to
// at least one client and the caller can access every client it is linked to;
// an application-scoped caller additionally sees only accounts bound to an
// application it can access. The view permission says what may be read, not for
// which tenants.
func TestServiceAccountReads_ConfinedToTheCallersTenants(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	s := &State{
		Repo: serviceaccount.NewRepository(pool), Principals: principal.NewRepository(pool),
		OAuthClients: platformauth.NewRepository(pool).OAuthClients,
	}
	sfx := strings.ToLower(tsid.GenerateUntyped())[5:]
	clientA, clientB := "clt_a"+sfx[:8], "clt_b"+sfx[:8]
	ownApp := testpg.EnsureApplication(t, "sarsc"+sfx)

	seed := func(n string, clients []string, app *string) string {
		id := "sac_" + sfx + n
		_, err := pool.Exec(ctx,
			`INSERT INTO iam_service_accounts (id, code, name, application_id, client_ids, active, created_at, updated_at)
			 VALUES ($1, $1, $1, $2, $3, true, NOW(), NOW())`, id, app, clients)
		require.NoError(t, err)
		return id
	}
	saA := seed("a", []string{clientA}, nil)
	saB := seed("b", []string{clientB}, nil)
	saBoth := seed("c", []string{clientA, clientB}, nil)
	saPlatform := seed("p", []string{}, nil)
	saApp := seed("d", []string{}, &ownApp)
	all := []string{saA, saB, saBoth, saPlatform, saApp}

	perms := []string{"platform:iam:service-account:view"}
	viewer := testpg.WithAuth(ctx, &auth.AuthContext{
		PrincipalID: "prn_saviewer", Scope: auth.ScopeClient, Clients: []string{clientA},
		Permissions: perms, AllApplications: true,
	})
	anchor := testpg.WithAuth(ctx, &auth.AuthContext{
		PrincipalID: "prn_saanchor", Scope: auth.ScopeAnchor, Permissions: perms, AllApplications: true,
	})
	appScoped := testpg.WithAuth(ctx, &auth.AuthContext{
		PrincipalID: "prn_saappsa", Scope: auth.ScopeAnchor, Permissions: perms, Applications: []string{ownApp},
	})
	mine := func(c context.Context) []string {
		t.Helper()
		out, err := s.list(c, &apicommon.Empty{})
		require.NoError(t, err)
		var got []string
		for _, sa := range out.Body.ServiceAccounts {
			for _, id := range all {
				if sa.ID == id {
					got = append(got, id)
				}
			}
		}
		return got
	}

	t.Run("list", func(t *testing.T) {
		assert.ElementsMatch(t, []string{saA}, mine(viewer),
			"only an account linked to the caller's clients alone: not B's, not one reaching B too, not a platform-wide one")
		assert.ElementsMatch(t, all, mine(anchor), "an anchor with all-applications access sees every account")
		assert.ElementsMatch(t, []string{saApp}, mine(appScoped), "an application-scoped caller sees its application's accounts only")
	})

	t.Run("by id and by code", func(t *testing.T) {
		got, err := s.getByID(viewer, &apicommon.IDInput{ID: saA})
		require.NoError(t, err)
		assert.Equal(t, saA, got.Body.ID)
		for _, id := range []string{saB, saBoth, saPlatform, saApp} {
			_, err = s.getByID(viewer, &apicommon.IDInput{ID: id})
			testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "ServiceAccount_NOT_FOUND")
			_, err = s.getByCode(viewer, &getByCodeInput{Code: id})
			testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "ServiceAccount_NOT_FOUND")
		}
		for _, id := range all {
			_, err = s.getByID(anchor, &apicommon.IDInput{ID: id})
			require.NoError(t, err, "an anchor reads %s", id)
		}
		_, err = s.getByID(appScoped, &apicommon.IDInput{ID: saApp})
		require.NoError(t, err)
		_, err = s.getByID(appScoped, &apicommon.IDInput{ID: saPlatform})
		testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "ServiceAccount_NOT_FOUND")
	})
}

// allAppsAnchorCtx is testpg.AnchorCtx with all-applications access: the
// principal for a read that is not meant to be confined to applications
// (testpg.AnchorCtx alone is application-scoped).
func allAppsAnchorCtx() context.Context {
	return testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_optestrunner1", Scope: auth.ScopeAnchor,
		Permissions: []string{"platform:*:*:*"}, AllApplications: true,
	})
}
