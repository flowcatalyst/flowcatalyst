//go:build integration

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// lookup is consulted BEFORE login, to decide which identity provider an email
// domain signs in through. It never had an anchor gate and must not gain one:
// the caller has no principal yet.
func TestLookupNeedsNoPrincipal(t *testing.T) {
	s := &State{Repo: emaildomainmapping.NewRepository(testpg.Pool(t))}

	out, err := s.lookup(context.Background(), &lookupInput{Domain: "nobody-here.example.test"})
	require.NoError(t, err, "the pre-login lookup must not require authentication")
	require.NotNil(t, out)
}

// Its administrative siblings do require reach and the family's permission.
func TestMappingAdminRoutesRequireThePermission(t *testing.T) {
	s := &State{Repo: emaildomainmapping.NewRepository(testpg.Pool(t))}

	bareAnchor := auth.WithContext(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_edm_probe", Scope: auth.ScopeAnchor,
	})
	_, err := s.list(bareAnchor, &apicommon.Empty{})
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")

	withPerm := auth.WithContext(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_edm_probe",
		Scope:       auth.ScopeAnchor,
		Permissions: []string{"platform:iam:email-domain-mapping:view"},
	})
	_, err = s.list(withPerm, &apicommon.Empty{})
	require.NoError(t, err)
}
