//go:build integration

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// anchorHolding builds an anchor principal whose roles granted exactly perms.
func anchorHolding(principalType string, perms ...string) context.Context {
	return auth.WithContext(context.Background(), &auth.AuthContext{
		PrincipalID:   "prn_permfromroles",
		Scope:         auth.ScopeAnchor,
		PrincipalType: principalType,
		Permissions:   perms,
	})
}

// A provisioned application's service account is anchor-scoped and holds only
// its application-service role. While anchor scope implied authority, its
// client credentials could list every principal on the platform; now reach and
// authority are separate and the role is what answers.
func TestListPrincipals_ProvisionedServiceAccountIsRefused(t *testing.T) {
	s := &State{Repo: principal.NewRepository(testpg.Pool(t))}

	serviceAccount := anchorHolding("SERVICE",
		"platform:application-service:event-type:create",
		"platform:application-service:subscription:create")

	_, err := s.list(serviceAccount, &listInput{})
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")
}

// The same route, for an anchor whose roles do grant the read.
func TestListPrincipals_AnchorWithTheViewPermissionPasses(t *testing.T) {
	s := &State{Repo: principal.NewRepository(testpg.Pool(t))}

	_, err := s.list(anchorHolding("USER", "platform:iam:user:view"), &listInput{})
	require.NoError(t, err)

	// And the super-admin wildcard, which is what the bootstrap admin holds.
	_, err = s.list(anchorHolding("USER", "platform:*:*:*"), &listInput{})
	require.NoError(t, err)
}

// An anchor carrying a read-only role may not write: the permission gate is
// the whole authority check now, so a viewer's permissions refuse a create.
func TestCreatePrincipal_ReadOnlyAnchorIsRefused(t *testing.T) {
	s := &State{Repo: principal.NewRepository(testpg.Pool(t))}

	readOnly := anchorHolding("USER", "platform:iam:user:view")
	_, err := s.create(readOnly, &apicommon.In[CreatePrincipalRequest]{
		Body: CreatePrincipalRequest{
			Email: "permfromroles@example.test", Scope: "ANCHOR",
		},
	})
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")
}
