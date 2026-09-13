//go:build integration

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/cors"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// /allowed is consulted by browsers before any credential exists, so it is
// deliberately public. Adding permission gates to the rest of this API must
// not catch it — with no principal at all it still answers.
func TestPublicAllowedNeedsNoPrincipal(t *testing.T) {
	s := &State{Repo: cors.NewRepository(testpg.Pool(t))}

	_, err := s.publicAllowed(context.Background(), &apicommon.Empty{})
	require.NoError(t, err, "the public origins route must not require authentication")
}

// The administrative routes beside it do require both reach and the CORS
// family's own permission.
func TestAdminCorsRoutesRequireThePermission(t *testing.T) {
	s := &State{Repo: cors.NewRepository(testpg.Pool(t))}

	bareAnchor := auth.WithContext(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_cors_probe", Scope: auth.ScopeAnchor,
	})
	_, err := s.list(bareAnchor, &apicommon.Empty{})
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")

	withPerm := auth.WithContext(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_cors_probe",
		Scope:       auth.ScopeAnchor,
		Permissions: []string{"platform:admin:cors-origin:view"},
	})
	_, err = s.list(withPerm, &apicommon.Empty{})
	require.NoError(t, err)
}
