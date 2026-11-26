//go:build integration

package api

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity/attestation"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// unlockKey stores a live unlock key for the worker on the device.
func (f *fixture) unlockKey(workerID string, d *fieldidentity.Device) *fieldidentity.UnlockKey {
	f.t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(f.t, err)
	ident, err := f.s.Ops.R.Identities.FindByID(context.Background(), workerID)
	require.NoError(f.t, err)
	now := time.Now().UTC().Truncate(time.Microsecond)
	key := fieldidentity.NewUnlockKey(ident, d, "jkt-"+tsid.GenerateUntyped(), `{"kty":"EC"}`, "",
		attestation.Evaluate(nil, &k.PublicKey, nil, attestation.Options{}), false, now, now, "")
	tx, err := testpg.Pool(f.t).Begin(context.Background())
	require.NoError(f.t, err)
	require.NoError(f.t, f.s.Ops.R.UnlockKeys.Persist(context.Background(), key, usecaseTx(tx)))
	require.NoError(f.t, tx.Commit(context.Background()))
	return key
}

func TestUnlockKeysListRevokeAndDeviceView(t *testing.T) {
	f := newFixture(t, hasher(t))
	w := f.ensure(f.actx, "W-UK", "Unlock", f.codeA).Body
	d := f.device(fieldidentity.ModePersonal, nil, f.appA)
	key := f.unlockKey(w.ID, d)

	list, err := f.s.listUnlockKeys(f.actx, &apicommon.IDInput{ID: w.ID})
	require.NoError(t, err)
	require.Len(t, list.Body.UnlockKeys, 1)
	got := list.Body.UnlockKeys[0]
	assert.Equal(t, key.ID, got.ID)
	assert.Equal(t, "ACTIVE", got.Status)
	assert.Equal(t, d.ID, got.DeviceID)
	assert.Equal(t, "NONE", got.Attestation.Verdict)
	assert.Nil(t, got.PinRequiredBy, "the default forces no periodic PIN")
	seven := 7
	_, err = f.s.putPolicy(f.actx, &putPolicyInput{ClientID: f.clientID, Body: PutFieldPolicyRequest{UnlockKeysEnabled: true, PinReauthDays: &seven}})
	require.NoError(t, err)
	list, err = f.s.listUnlockKeys(f.actx, &apicommon.IDInput{ID: w.ID})
	require.NoError(t, err)
	require.NotNil(t, list.Body.UnlockKeys[0].PinRequiredBy)
	assert.Equal(t, key.PINVerifiedAt.Add(7*24*time.Hour).Format(time.RFC3339),
		list.Body.UnlockKeys[0].PinRequiredBy.Underlying().UTC().Format(time.RFC3339))

	dev, err := f.s.getDevice(f.actx, &apicommon.IDInput{ID: d.ID})
	require.NoError(t, err)
	require.NotNil(t, dev.Body.UnlockKey)
	assert.Equal(t, key.ID, dev.Body.UnlockKey.ID)
	devs, err := f.s.listDevices(f.actx, &listDevicesInput{ClientID: f.clientID})
	require.NoError(t, err)
	require.Len(t, devs.Body.Devices, 1)
	assert.Equal(t, key.ID, devs.Body.Devices[0].UnlockKey.ID)

	// A viewer reads but cannot revoke.
	viewer := callerCtx([]string{f.clientID}, []string{"platform:iam:field-worker:view"}, nil, true)
	_, err = f.s.listUnlockKeys(viewer, &apicommon.IDInput{ID: w.ID})
	require.NoError(t, err)
	_, err = f.s.revokeUnlockKey(viewer, &revokeUnlockKeyInput{ID: w.ID, UnlockKeyID: key.ID})
	requireErr(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")

	// Another client's caller sees nothing (404).
	other := newFixture(t, hasher(t))
	foreign := callerCtx([]string{other.clientID}, allFieldPerms, nil, true)
	_, err = f.s.listUnlockKeys(foreign, &apicommon.IDInput{ID: w.ID})
	requireErr(t, err, usecase.KindNotFound, "FieldWorker_NOT_FOUND")

	// A key of another worker is 404 under this worker.
	w2 := f.ensure(f.actx, "W-UK2", "Other", f.codeA).Body
	_, err = f.s.revokeUnlockKey(f.actx, &revokeUnlockKeyInput{ID: w2.ID, UnlockKeyID: key.ID})
	requireErr(t, err, usecase.KindNotFound, "FieldUnlockKey_NOT_FOUND")

	out, err := f.s.revokeUnlockKey(f.actx, &revokeUnlockKeyInput{ID: w.ID, UnlockKeyID: key.ID})
	require.NoError(t, err)
	assert.Equal(t, "REVOKED", out.Body.Status)
	assert.Equal(t, "ADMIN", out.Body.RevokedReason)
	assert.Equal(t, "prn_optestrunner1", out.Body.RevokedBy)
	assert.Equal(t, 1, f.eventCount(fieldidentity.UnlockKeyRevokedType, key.ID))
	// Idempotent: a second revoke answers the key as it is and emits nothing.
	out, err = f.s.revokeUnlockKey(f.actx, &revokeUnlockKeyInput{ID: w.ID, UnlockKeyID: key.ID})
	require.NoError(t, err)
	assert.Equal(t, "REVOKED", out.Body.Status)
	assert.Equal(t, 1, f.eventCount(fieldidentity.UnlockKeyRevokedType, key.ID))

	dev, err = f.s.getDevice(f.actx, &apicommon.IDInput{ID: d.ID})
	require.NoError(t, err)
	assert.Nil(t, dev.Body.UnlockKey, "only a live key is the device's")
}

func TestPolicyUnlockKeyFields(t *testing.T) {
	f := newFixture(t, hasher(t))
	def, err := f.s.getPolicy(f.actx, &policyInput{ClientID: f.clientID})
	require.NoError(t, err)
	assert.False(t, def.Body.UnlockKeysEnabled)
	assert.Equal(t, "NONE", def.Body.UnlockKeyAttestation)

	assert.Equal(t, 0, def.Body.PinReauthDays, "never by default")
	put, err := f.s.putPolicy(f.actx, &putPolicyInput{ClientID: f.clientID, Body: PutFieldPolicyRequest{UnlockKeysEnabled: true}})
	require.NoError(t, err)
	assert.True(t, put.Body.UnlockKeysEnabled)
	for _, days := range []int{-1, 91} {
		_, err = f.s.putPolicy(f.actx, &putPolicyInput{ClientID: f.clientID, Body: PutFieldPolicyRequest{PinReauthDays: &days}})
		requireErr(t, err, usecase.KindValidation, "POLICY_INVALID")
	}
	assert.Equal(t, "NONE", put.Body.UnlockKeyAttestation)

	_, err = f.s.putPolicy(f.actx, &putPolicyInput{ClientID: f.clientID, Body: PutFieldPolicyRequest{
		UnlockKeysEnabled: true, UnlockKeyAttestation: "MAYBE",
	}})
	requireErr(t, err, usecase.KindValidation, "POLICY_INVALID")
	_, err = f.s.putPolicy(f.actx, &putPolicyInput{ClientID: f.clientID, Body: PutFieldPolicyRequest{
		UnlockKeysEnabled: true, UnlockKeyAttestation: "REQUIRED",
	}})
	requireErr(t, err, usecase.KindValidation, "POLICY_INVALID") // no attestation roots configured

	// Switching unlock keys off revokes the client's keys.
	w := f.ensure(f.actx, "W-UKP", "Policy", f.codeA).Body
	key := f.unlockKey(w.ID, f.device(fieldidentity.ModePersonal, nil, f.appA))
	_, err = f.s.putPolicy(f.actx, &putPolicyInput{ClientID: f.clientID, Body: PutFieldPolicyRequest{}})
	require.NoError(t, err)
	got, err := f.s.Ops.R.UnlockKeys.FindByID(context.Background(), key.ID)
	require.NoError(t, err)
	assert.False(t, got.Live())
	assert.Equal(t, fieldidentity.UnlockRevokedPolicyDisabled, got.RevokedReason)
}

func TestUnlockKeyRevokeRouteIsClientBoxedAndConfined(t *testing.T) {
	f := newFixture(t, hasher(t))
	w := f.ensure(f.actx, "W-UKB", "Boxed", f.codeA).Body
	key := f.unlockKey(w.ID, f.device(fieldidentity.ModePersonal, nil, f.appA))

	other := newFixture(t, hasher(t))
	foreign := callerCtx([]string{other.clientID}, allFieldPerms, nil, true)
	_, err := f.s.revokeUnlockKey(foreign, &revokeUnlockKeyInput{ID: w.ID, UnlockKeyID: key.ID})
	requireErr(t, err, usecase.KindNotFound, "FieldWorker_NOT_FOUND")

	// An application-confined account of another application reaches no worker
	// without its application.
	confined := callerCtx([]string{f.clientID}, allFieldPerms, []string{f.appB}, false)
	_, err = f.s.revokeUnlockKey(confined, &revokeUnlockKeyInput{ID: w.ID, UnlockKeyID: key.ID})
	requireErr(t, err, usecase.KindNotFound, "FieldWorker_NOT_FOUND")

	got, err := f.s.Ops.R.UnlockKeys.FindByID(context.Background(), key.ID)
	require.NoError(t, err)
	assert.True(t, got.Live(), "nothing was revoked")
}

// A key's live sessions are on the wire (liveSessions), also for a revoked
// key; revoking a key the phone revoked already ends the sessions it left.
func TestUnlockKeyLiveSessionsAndEndingThemOnARevokedKey(t *testing.T) {
	f := newFixture(t, hasher(t))
	w := f.ensure(f.actx, "W-UKS", "Sessions", f.codeA).Body
	d := f.device(fieldidentity.ModePersonal, nil, f.appA)
	key := f.unlockKey(w.ID, d)
	pool := testpg.Pool(t)
	ctx := context.Background()
	session := func(expiresIn string) string {
		id := "fws_" + tsid.GenerateUntyped()[:13]
		_, err := pool.Exec(ctx, `INSERT INTO field_sessions (id, family_id, identity_id, device_id, application_id,
			refresh_hash, idle_expires_at, absolute_expires_at, unlock_key_id)
			VALUES ($1, $1, $2, $3, $4, $5, NOW() + $6::interval, NOW() + $6::interval, $7)`,
			id, w.ID, d.ID, f.appA, tsid.GenerateUntyped(), expiresIn, key.ID)
		require.NoError(t, err)
		return id
	}
	live := session("1 hour")
	session("-1 minute") // expired: not counted

	list, err := f.s.listUnlockKeys(f.actx, &apicommon.IDInput{ID: w.ID})
	require.NoError(t, err)
	assert.Equal(t, int64(1), list.Body.UnlockKeys[0].LiveSessions)
	dev, err := f.s.getDevice(f.actx, &apicommon.IDInput{ID: d.ID})
	require.NoError(t, err)
	assert.Equal(t, int64(1), dev.Body.UnlockKey.LiveSessions)

	// The phone revokes its key: the session stays, the key shows it.
	_, err = pool.Exec(ctx, `UPDATE field_unlock_keys SET revoked_at = NOW(), revoked_reason = 'DEVICE' WHERE id = $1`, key.ID)
	require.NoError(t, err)
	list, err = f.s.listUnlockKeys(f.actx, &apicommon.IDInput{ID: w.ID})
	require.NoError(t, err)
	assert.Equal(t, "REVOKED", list.Body.UnlockKeys[0].Status)
	assert.Equal(t, int64(1), list.Body.UnlockKeys[0].LiveSessions)

	out, err := f.s.revokeUnlockKey(f.actx, &revokeUnlockKeyInput{ID: w.ID, UnlockKeyID: key.ID})
	require.NoError(t, err)
	assert.Equal(t, "DEVICE", out.Body.RevokedReason, "the key keeps its revocation")
	assert.Zero(t, out.Body.LiveSessions)
	var reason string
	require.NoError(t, pool.QueryRow(ctx, `SELECT revoked_reason FROM field_sessions WHERE id = $1`, live).Scan(&reason))
	assert.Equal(t, fieldidentity.ReasonUnlockKeyRevoked, reason)
	assert.Zero(t, f.eventCount(fieldidentity.UnlockKeyRevokedType, key.ID), "the key was not revoked again")
}
