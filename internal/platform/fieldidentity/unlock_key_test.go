package fieldidentity

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity/attestation"
)

func TestSignedInWithPIN(t *testing.T) {
	t.Parallel()
	yes := [][]string{{"pin"}, {"badge", "pin"}}
	no := [][]string{nil, {}, {"badge"}, {"hwk", "uv"}, {"pin", "imp"}, {"code", "imp"}, {"pin", "badge"}, {"pin", "pin"}, {"badge", "pin", "imp"}}
	for _, amr := range yes {
		assert.True(t, SignedInWithPIN(amr), "%v", amr)
	}
	for _, amr := range no {
		assert.False(t, SignedInWithPIN(amr), "%v", amr)
	}
}

func TestUnlockKeyDeadlineAndRevoke(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)
	k := NewUnlockKey(&Identity{ID: "fwu_1", ClientID: "clt_1"}, &Device{ID: "fwd_1"}, "jkt", "{}", "",
		attestation.None(), false, at, at, "fwk_0")
	assert.Equal(t, "fwk", k.ID[:3])
	assert.True(t, k.Live())
	p := DefaultPolicy("clt_1")
	assert.Equal(t, 0, p.PINReauthDays, "the default forces no periodic PIN")
	_, ok := k.PINDeadline(p)
	assert.False(t, ok)
	assert.False(t, k.PINDue(p, at.Add(10*365*24*time.Hour)), "never due by time")

	p.PINReauthDays = 7
	d, ok := k.PINDeadline(p)
	require.True(t, ok)
	assert.Equal(t, at.Add(7*24*time.Hour), d)
	assert.False(t, k.PINDue(p, d.Add(-time.Nanosecond)), "just before the boundary")
	assert.True(t, k.PINDue(p, d), "at the boundary")
	p.PINReauthDays = 1
	d, _ = k.PINDeadline(p)
	assert.Equal(t, at.Add(24*time.Hour), d, "the policy is read at the time of the check")

	assert.True(t, k.Revoke(UnlockRevokedAdmin, "prn_1", at))
	assert.False(t, k.Live())
	assert.False(t, k.Revoke(UnlockRevokedDevice, "fwu_1", at.Add(time.Hour)), "a revoked key stays as it was revoked")
	assert.Equal(t, UnlockRevokedAdmin, k.RevokedReason)
	assert.Equal(t, "prn_1", k.RevokedBy)
}

func TestPolicyUnlockKeyFieldsValidate(t *testing.T) {
	t.Parallel()
	p, err := PutPolicyCommand{ClientID: "clt_1"}.Build()
	require.NoError(t, err)
	assert.False(t, p.UnlockKeysEnabled)
	assert.Equal(t, UnlockAttestationNone, p.UnlockKeyAttestation, "omitted is NONE")
	assert.Equal(t, UnlockAttestationNone, DefaultPolicy("clt_1").UnlockKeyAttestation)

	p, err = PutPolicyCommand{ClientID: "clt_1", UnlockKeysEnabled: true, UnlockKeyAttestation: UnlockAttestationRequired}.Build()
	require.NoError(t, err)
	assert.True(t, p.UnlockKeysEnabled)
	assert.Equal(t, UnlockAttestationRequired, p.UnlockKeyAttestation)

	_, err = PutPolicyCommand{ClientID: "clt_1", UnlockKeyAttestation: "SOMETIMES"}.Build()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unlockKeyAttestation")

	for _, days := range []int{-1, 91} {
		_, err = PutPolicyCommand{ClientID: "clt_1", PINReauthDays: &days}.Build()
		require.Error(t, err, "pinReauthDays %d", days)
	}
	for _, days := range []int{0, 1, 90} {
		p, err = PutPolicyCommand{ClientID: "clt_1", PINReauthDays: &days}.Build()
		require.NoError(t, err, "pinReauthDays %d", days)
		assert.Equal(t, days, p.PINReauthDays)
	}
	p, err = PutPolicyCommand{ClientID: "clt_1"}.Build()
	require.NoError(t, err)
	assert.Equal(t, 0, p.PINReauthDays, "omitted is never")
}

func TestUnlockAttestationRuleValid(t *testing.T) {
	t.Parallel()
	assert.True(t, UnlockAttestationNone.Valid())
	assert.True(t, UnlockAttestationRequired.Valid())
	assert.False(t, UnlockKeyAttestationRule("").Valid())
	assert.False(t, UnlockKeyAttestationRule("required").Valid())
}

// Which revocations end the key's sessions (docs/field-unlock-keys-spec.md
// section 7), over every reason; a reason this version does not know (a row
// written by a newer one) ends them rather than keeps them.
func TestUnlockKeyRevokeReasonEndsSessions(t *testing.T) {
	t.Parallel()
	for reason, ends := range map[UnlockKeyRevokeReason]bool{
		UnlockRevokedAdmin: true, UnlockRevokedPolicyDisabled: true, UnlockRevokedPolicyChanged: true,
		UnlockRevokedAttestationRevoked: true,
		UnlockRevokedReplaced:           false, UnlockRevokedDevice: false, UnlockRevokedWorkerLocked: false,
		UnlockRevokedPINChanged: false, UnlockRevokedPINReset: false, UnlockRevokedWorkerSuspended: false,
		UnlockRevokedDeviceRevoked: false,
		"FROM_A_NEWER_VERSION":     true,
	} {
		assert.Equal(t, ends, reason.EndsSessions(), reason)
	}
}
