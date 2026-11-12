package operations_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/go-sdk/usecase"
)

func account(active bool, appID string, clients ...string) *serviceaccount.ServiceAccount {
	sa := serviceaccount.New("wield-acct", "Wield")
	sa.Active = active
	sa.ClientIDs = clients
	if appID != "" {
		a := ids.ApplicationID(appID)
		sa.ApplicationID = &a
	}
	return sa
}

func requireError(t *testing.T, err error, kind usecase.Kind, code string) {
	t.Helper()
	ue := usecase.AsError(err)
	require.NotNil(t, ue, "expected a usecase error, got %v", err)
	assert.Equal(t, kind, ue.Kind)
	assert.Equal(t, code, ue.Code)
}

func TestMayWieldServiceAccount(t *testing.T) {
	t.Parallel()
	operator := &auth.AuthContext{Scope: auth.ScopeAnchor, AllApplications: true}
	appAnchor := &auth.AuthContext{Scope: auth.ScopeAnchor, Applications: []string{"app_a"}}
	client := &auth.AuthContext{Scope: auth.ScopeClient, AllApplications: true, Clients: []string{"c1", "c2"}}
	clientApp := &auth.AuthContext{Scope: auth.ScopeClient, Clients: []string{"c1"}, Applications: []string{"app_a"}}

	cases := []struct {
		name string
		ac   *auth.AuthContext
		sa   *serviceaccount.ServiceAccount
		code string // "" = allowed
		kind usecase.Kind
	}{
		{"operator, platform account", operator, account(true, ""), "", usecase.Kind("")},
		{"operator, application account", operator, account(true, "app_b"), "", usecase.Kind("")},
		{"operator, client account", operator, account(true, "", "c9"), "", usecase.Kind("")},
		{"app-scoped anchor, own application", appAnchor, account(true, "app_a"), "", usecase.Kind("")},
		{"app-scoped anchor, other application", appAnchor, account(true, "app_b"), "SCOPE_FORBIDDEN", usecase.KindAuthorization},
		{"app-scoped anchor, account with no application", appAnchor, account(true, ""), "SCOPE_FORBIDDEN", usecase.KindAuthorization},
		{"client, account inside its clients", client, account(true, "", "c1", "c2"), "", usecase.Kind("")},
		{"client, account partly outside", client, account(true, "", "c1", "c9"), "SCOPE_FORBIDDEN", usecase.KindAuthorization},
		{"client, account linked to no client", client, account(true, ""), "SCOPE_FORBIDDEN", usecase.KindAuthorization},
		{"client, application account (no client)", client, account(true, "app_a"), "SCOPE_FORBIDDEN", usecase.KindAuthorization},
		{"client, account inside clients and its application", clientApp, account(true, "app_a", "c1"), "", usecase.Kind("")},
		{"client, account inside clients, other application", clientApp, account(true, "app_b", "c1"), "SCOPE_FORBIDDEN", usecase.KindAuthorization},
		{"nil account", operator, nil, "SERVICE_ACCOUNT_NOT_FOUND", usecase.KindValidation},
		{"inactive account", operator, account(false, ""), "SERVICE_ACCOUNT_INACTIVE", usecase.KindValidation},
		{"no caller", nil, account(true, ""), "SCOPE_FORBIDDEN", usecase.KindAuthorization},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := operations.MayWieldServiceAccount(tc.ac, tc.sa)
			if tc.code == "" {
				require.NoError(t, err)
				return
			}
			requireError(t, err, tc.kind, tc.code)
		})
	}
}

func TestMayWieldApplicationIdentity(t *testing.T) {
	t.Parallel()
	operator := &auth.AuthContext{Scope: auth.ScopeAnchor, AllApplications: true}
	appAnchor := &auth.AuthContext{Scope: auth.ScopeAnchor, Applications: []string{"app_a"}}
	client := &auth.AuthContext{Scope: auth.ScopeClient, AllApplications: true, Clients: []string{"c1"}}

	require.NoError(t, operations.MayWieldApplicationIdentity(operator, "app_x"))
	require.NoError(t, operations.MayWieldApplicationIdentity(appAnchor, "app_a"))
	requireError(t, operations.MayWieldApplicationIdentity(appAnchor, "app_b"), usecase.KindAuthorization, "APPLICATION_ACCESS_REQUIRED")
	requireError(t, operations.MayWieldApplicationIdentity(client, "app_a"), usecase.KindAuthorization, "APPLICATION_ACCESS_REQUIRED")
	requireError(t, operations.MayWieldApplicationIdentity(nil, "app_a"), usecase.KindAuthorization, "APPLICATION_ACCESS_REQUIRED")
}
