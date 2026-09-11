package portalidentity

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestIdentityState pins the admin-facing lifecycle: INVITED until the
// invitee sets a password or signs in (then ACTIVE), INVITE_EXPIRED once the
// set-password link lapses unused, SUSPENDED whenever the row is DISABLED —
// the "stuck on Invited" report was a Source column that could never move.
func TestIdentityState(t *testing.T) {
	now := time.Now().UTC()
	past, future := now.Add(-time.Hour), now.Add(time.Hour)
	hash := "argon2id$..."

	invited := New("clt_1", "a@x.test", "", SourceInvite)
	invited.InvitedAt, invited.InviteExpiresAt = &past, &future
	assert.Equal(t, StateInvited, invited.State(now), "live invite")

	expired := New("clt_1", "b@x.test", "", SourceInvite)
	expired.InvitedAt, expired.InviteExpiresAt = &past, &past
	assert.Equal(t, StateInviteExpired, expired.State(now), "lapsed invite")

	accepted := New("clt_1", "c@x.test", "", SourceInvite)
	accepted.InvitedAt, accepted.InviteExpiresAt = &past, &past
	accepted.PasswordHash = &hash
	assert.Equal(t, StateActive, accepted.State(now), "password set → ACTIVE even after the link's expiry")

	ssoInvite := New("clt_1", "d@org.test", "", SourceInvite)
	ssoInvite.InvitedAt = &past // SSO invites carry no expiry
	assert.Equal(t, StateInvited, ssoInvite.State(now), "SSO invite never expires")
	ssoInvite.LastLoginAt = &now
	assert.Equal(t, StateActive, ssoInvite.State(now), "first SSO sign-in → ACTIVE")

	jit := New("clt_1", "e@org.test", "", SourceJIT)
	assert.Equal(t, StateActive, jit.State(now), "JIT identities exist because they signed in")

	suspended := New("clt_1", "f@x.test", "", SourceInvite)
	suspended.PasswordHash = &hash
	suspended.Status = StatusDisabled
	assert.Equal(t, StateSuspended, suspended.State(now))
}

func TestIdentityGrantRevoke(t *testing.T) {
	i := New("clt_1", "a@x.test", "", SourceInvite)
	assert.True(t, i.Grant("pta_1", SourceInvite))
	assert.False(t, i.Grant("pta_1", SourceAdmin), "idempotent")
	assert.True(t, i.HasApp("pta_1"))
	assert.False(t, i.HasApp("pta_2"))
	assert.True(t, i.Revoke("pta_1"))
	assert.False(t, i.Revoke("pta_1"))
	assert.False(t, i.HasApp("pta_1"))
}

func TestAppCode(t *testing.T) {
	assert.Equal(t, "customer-portal", NormalizeAppCode("  Customer-Portal "))
	for _, ok := range []string{"a", "customer-portal", "supplier_portal2", "9lives"} {
		assert.True(t, ValidAppCode(ok), ok)
	}
	for _, bad := range []string{"", "-lead", "has space", "UPPER", "dot.ted"} {
		assert.False(t, ValidAppCode(bad), bad)
	}
}
