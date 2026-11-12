//go:build integration

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	appops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	clientops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

const (
	permEnable  = "platform:admin:application:enable-client"
	permDisable = "platform:admin:application:disable-client"
)

func anchorCaller(apps []string, perms ...string) context.Context {
	return auth.WithContext(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_anchorgate002", Scope: auth.ScopeAnchor,
		AllApplications: apps == nil, Applications: apps, Permissions: perms,
	})
}

func wantRefused(t *testing.T, err error, code string) {
	t.Helper()
	ue := usecase.AsError(err)
	require.NotNil(t, ue, "want %s, got %v", code, err)
	assert.Equal(t, usecase.KindAuthorization, ue.Kind)
	assert.Equal(t, code, ue.Code)
}

// The client-side enablement routes were anchor-only; they now need
// enable-client / disable-client (the bulk replace both), and an
// application-scoped caller is confined to its own application (and refused
// outright on the bulk replace, which rewrites every application).
func TestClientApplicationEnablementGates(t *testing.T) {
	pool := testpg.Pool(t)
	repo := client.NewRepository(pool)
	apps := application.NewRepository(pool)
	configs := application.NewClientConfigRepo(pool)
	uow := testpg.NewUoW(t)
	s := &State{Repo: repo, Applications: apps, ClientConfigs: configs, UoW: uow}

	mkApp := func(code string) string {
		t.Helper()
		ev, err := usecaseop.Run(testpg.AnchorCtx(), uow, appops.CreateApplication(apps),
			appops.CreateCommand{Code: code, Name: code}, testpg.TestEC())
		require.NoError(t, err)
		return ev.ApplicationID
	}
	mkClient := func(identifier string) string {
		t.Helper()
		ev, err := usecaseop.Run(testpg.AnchorCtx(), uow, clientops.CreateClient(repo),
			clientops.CreateCommand{Name: identifier, Identifier: identifier}, testpg.TestEC())
		require.NoError(t, err)
		return ev.ClientID
	}
	enabled := func(appID, clientID string) bool {
		t.Helper()
		c, err := configs.FindByApplicationAndClient(context.Background(), appID, clientID)
		require.NoError(t, err)
		return c != nil && c.Enabled
	}

	appA, appB := mkApp("cag-app-a"), mkApp("cag-app-b")
	cli := mkClient("cag-client-1")

	t.Run("enable", func(t *testing.T) {
		in := &appLinkInput{ID: ids.ClientID(cli), ApplicationID: ids.ApplicationID(appA)}
		_, err := s.enableApplication(anchorCaller(nil, permDisable), in)
		wantRefused(t, err, "PERMISSION_REQUIRED")
		assert.False(t, enabled(appA, cli))

		_, err = s.enableApplication(anchorCaller([]string{appB}, permEnable), in)
		wantRefused(t, err, "APPLICATION_ACCESS_REQUIRED")
		assert.False(t, enabled(appA, cli))

		_, err = s.enableApplication(anchorCaller([]string{appA}, permEnable), in)
		require.NoError(t, err)
		assert.True(t, enabled(appA, cli))
	})

	t.Run("disable", func(t *testing.T) {
		in := &appLinkInput{ID: ids.ClientID(cli), ApplicationID: ids.ApplicationID(appA)}
		require.True(t, enabled(appA, cli))
		_, err := s.disableApplication(anchorCaller(nil, permEnable), in)
		wantRefused(t, err, "PERMISSION_REQUIRED")
		assert.True(t, enabled(appA, cli))

		_, err = s.disableApplication(anchorCaller([]string{appB}, permDisable), in)
		wantRefused(t, err, "APPLICATION_ACCESS_REQUIRED")
		assert.True(t, enabled(appA, cli))

		_, err = s.disableApplication(anchorCaller(nil, permDisable), in)
		require.NoError(t, err)
		assert.False(t, enabled(appA, cli))
	})

	t.Run("bulk replace", func(t *testing.T) {
		in := &updateApplicationsInput{
			ID:   ids.ClientID(cli),
			Body: UpdateClientApplicationsRequest{EnabledApplicationIDs: []string{appA, appB}},
		}

		for _, perms := range [][]string{nil, {permEnable}, {permDisable}} {
			_, err := s.updateApplications(anchorCaller(nil, perms...), in)
			wantRefused(t, err, "PERMISSION_REQUIRED")
			assert.False(t, enabled(appA, cli), "state unchanged with %v", perms)
			assert.False(t, enabled(appB, cli), "state unchanged with %v", perms)
		}

		// Application-scoped, even holding both: refused outright.
		_, err := s.updateApplications(anchorCaller([]string{appA, appB}, permEnable, permDisable), in)
		wantRefused(t, err, "APPLICATION_ACCESS_REQUIRED")
		assert.False(t, enabled(appA, cli))

		_, err = s.updateApplications(anchorCaller(nil, permEnable, permDisable), in)
		require.NoError(t, err)
		assert.True(t, enabled(appA, cli))
		assert.True(t, enabled(appB, cli))
	})
}
