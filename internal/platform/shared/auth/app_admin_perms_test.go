package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

func holding(perms ...string) *AuthContext {
	return &AuthContext{PrincipalID: "prn_t", Scope: ScopeAnchor, Permissions: perms}
}

// F20: each guard admits exactly the permissions it names.
func TestApplicationAdminGuards(t *testing.T) {
	const (
		evAppSvc  = "platform:application-service:event:view"
		jobAppSvc = "platform:application-service:dispatch-job:view"
		uView     = "platform:application-service:user:view"
		uCreate   = "platform:application-service:user:create"
		iView     = "platform:application-service:invitation:view"
		iManage   = "platform:application-service:invitation:manage"
	)
	cases := []struct {
		name  string
		guard func(*AuthContext) error
		ok    []string
		deny  []string
	}{
		{
			"CanReadEvents", CanReadEvents,
			[]string{evAppSvc, "platform:messaging:event:view", "platform:*:*:*", "platform:application-service:*:*"},
			[]string{jobAppSvc, uView, "platform:messaging:event:view-raw", "platform:messaging:dispatch-job:view"},
		},
		{
			"CanReadDispatchJobs", CanReadDispatchJobs,
			[]string{jobAppSvc, "platform:messaging:dispatch-job:view", "platform:*:*:*"},
			[]string{evAppSvc, "platform:messaging:dispatch-job:view-raw", "platform:messaging:event:view"},
		},
		{
			"CanReadApplicationUsers", CanReadApplicationUsers,
			[]string{uView, "platform:iam:user:view", "platform:*:*:*"},
			[]string{uCreate, iView, iManage, evAppSvc},
		},
		{
			"CanInviteApplicationUsers", CanInviteApplicationUsers,
			[]string{uCreate, "platform:iam:user:create", "platform:*:*:*"},
			[]string{uView, iManage, iView},
		},
		{
			"CanReadApplicationInvitations", CanReadApplicationInvitations,
			[]string{iView, iManage, "platform:iam:user:view", "platform:*:*:*"},
			[]string{uView, uCreate},
		},
		{
			"CanManageApplicationInvitations", CanManageApplicationInvitations,
			[]string{iManage, "platform:iam:user:create", "platform:*:*:*"},
			[]string{iView, uView},
		},
	}
	for _, c := range cases {
		for _, p := range c.ok {
			assert.NoErrorf(t, c.guard(holding(p)), "%s admits %s", c.name, p)
		}
		for _, p := range c.deny {
			err := c.guard(holding(p))
			require.Errorf(t, err, "%s refuses %s", c.name, p)
			assert.Equal(t, "PERMISSION_REQUIRED", usecase.AsError(err).Code)
		}
		err := c.guard(nil)
		require.Error(t, err)
		assert.Equal(t, "UNAUTHENTICATED", usecase.AsError(err).Code)
		require.Errorf(t, c.guard(holding()), "%s refuses no permission", c.name)
	}
}

// CanReadAllEvents / CanReadAllDispatchJobs are what lifts the confinement:
// only the console's messaging view does, never the application-service one.
func TestReadAllLiftsConfinementOnlyForTheConsolePermission(t *testing.T) {
	assert.True(t, CanReadAllEvents(holding("platform:messaging:event:view")))
	assert.True(t, CanReadAllEvents(holding("platform:*:*:*")))
	assert.False(t, CanReadAllEvents(holding("platform:application-service:event:view")))
	assert.False(t, CanReadAllEvents(nil))
	assert.True(t, CanReadAllDispatchJobs(holding("platform:messaging:dispatch-job:view")))
	assert.False(t, CanReadAllDispatchJobs(holding("platform:application-service:dispatch-job:view")))
	assert.False(t, CanReadAllDispatchJobs(nil))
}
