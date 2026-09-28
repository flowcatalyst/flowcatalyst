package api

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// Owner decision #19: service-account create, update and delete need anchor
// scope on top of the permission. A service account's SERVICE principal is
// anchor-tier, so a non-anchor administrator holding only the permission
// could otherwise mint an anchor-tier credential — an escalation.
func TestServiceAccountWritesRequireAnchor(t *testing.T) {
	nonAnchor := auth.WithContext(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_clientadmin1",
		Scope:       auth.ScopeClient,
		Permissions: []string{
			"platform:iam:service-account:create",
			"platform:iam:service-account:update",
			"platform:iam:service-account:delete",
		},
	})
	s := &State{} // the gate refuses before any repository is touched

	forbidden := func(t *testing.T, err error) {
		t.Helper()
		require.Error(t, err)
		var ue *usecase.Error
		require.True(t, errors.As(err, &ue), "a usecase error, got %T", err)
		assert.Equal(t, usecase.KindAuthorization, ue.Kind)
	}
	_, err := s.create(nonAnchor, &apicommon.In[CreateServiceAccountRequest]{Body: CreateServiceAccountRequest{Code: "x", Name: "X"}})
	forbidden(t, err)
	_, err = s.update(nonAnchor, &updateInput{ID: "sac_x"})
	forbidden(t, err)
	_, err = s.delete(nonAnchor, &apicommon.IDInput{ID: "sac_x"})
	forbidden(t, err)

	assert.NoError(t, requireAnchorWriter(&auth.AuthContext{
		Scope: auth.ScopeAnchor, Permissions: []string{"platform:iam:service-account:delete"},
	}, auth.CanDeleteServiceAccounts))
}
