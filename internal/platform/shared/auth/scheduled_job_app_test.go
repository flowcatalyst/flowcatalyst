package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestScheduledJobAppServiceRules(t *testing.T) {
	const own, other = "app_own", "app_other"
	ownApp, otherApp := own, other
	ownP, otherP := &ownApp, &otherApp
	appSA := func(perm string) *AuthContext {
		return &AuthContext{Scope: ScopeAnchor, Applications: []string{own}, Permissions: []string{perm}}
	}
	const fire = "platform:application-service:scheduled-job:fire"
	const sync = "platform:application-service:scheduled-job:sync"
	messagingFire := &AuthContext{Scope: ScopeAnchor, Permissions: []string{"platform:messaging:scheduled-job:fire"}}
	messagingView := &AuthContext{Scope: ScopeAnchor, Permissions: []string{"platform:messaging:scheduled-job:view"}}

	t.Run("fire", func(t *testing.T) {
		assert.NoError(t, CanFireScheduledJob(messagingFire, otherP))
		assert.NoError(t, CanFireScheduledJob(messagingFire, nil))
		assert.NoError(t, CanFireScheduledJob(appSA(fire), ownP))
		assert.Error(t, CanFireScheduledJob(appSA(fire), otherP))
		assert.Error(t, CanFireScheduledJob(appSA(fire), nil))
		all := &AuthContext{Scope: ScopeAnchor, AllApplications: true, Permissions: []string{fire}}
		assert.NoError(t, CanFireScheduledJob(all, otherP))
		assert.Error(t, CanFireScheduledJob(all, nil))
		assert.Error(t, CanFireScheduledJob(appSA(sync), ownP), "sync does not fire")
		assert.Error(t, CanFireScheduledJob(messagingView, ownP))
	})
	t.Run("lookup", func(t *testing.T) {
		assert.NoError(t, CanLookUpScheduledJobs(messagingView))
		assert.NoError(t, CanLookUpScheduledJobs(appSA(fire)))
		assert.NoError(t, CanLookUpScheduledJobs(appSA(sync)))
		assert.Error(t, CanLookUpScheduledJobs(appSA("platform:application-service:scheduled-job-instance:write")))
		assert.True(t, CanLookUpScheduledJob(messagingView, otherP))
		assert.True(t, CanLookUpScheduledJob(messagingView, nil))
		for _, p := range []string{fire, sync} {
			assert.True(t, CanLookUpScheduledJob(appSA(p), ownP))
			assert.False(t, CanLookUpScheduledJob(appSA(p), otherP))
			assert.False(t, CanLookUpScheduledJob(appSA(p), nil))
		}
		all := &AuthContext{Scope: ScopeAnchor, AllApplications: true, Permissions: []string{fire}}
		assert.True(t, CanLookUpScheduledJob(all, otherP))
		assert.False(t, CanLookUpScheduledJob(all, nil))
	})
}
