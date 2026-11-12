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
	"github.com/flowcatalyst/go-sdk/usecase"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// The administrative routes require reach and the family's permission.
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
