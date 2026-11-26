//go:build integration

package api

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// appAccount is an application's service account as the platform makes it:
// ANCHOR scope (no client links), confined to its application, with the field
// permission an application holds for its workers.
func appAccount(appID string) context.Context {
	return testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_appsvc000001", Scope: auth.ScopeAnchor,
		Permissions: []string{"platform:iam:field-worker:manage"}, Applications: []string{appID},
		AllApplications: false, PrincipalType: auth.PrincipalTypeService,
	})
}

// The review's reproduction: an anchor-scoped application account reaches its
// override only in clients its application is enabled for, and never the locks.
func TestAnApplicationAccountReachesOnlyClientsItServes(t *testing.T) {
	f := newFixture(t, hasher(t))
	acct := appAccount(f.appA)
	_, err := f.patch(acct, f.codeA, PatchFieldPolicyOverrideRequest{Set: &FieldPolicyOverrideValues{UnlockKeysEnabled: on()}})
	require.NoError(t, err, "a client its application is enabled for")

	other := newFixture(t, hasher(t)) // A is not enabled for this client
	_, err = other.patch(acct, f.codeA, PatchFieldPolicyOverrideRequest{Set: &FieldPolicyOverrideValues{UnlockKeysEnabled: on()}})
	requireErr(t, err, usecase.KindAuthorization, "APPLICATION_FORBIDDEN")
	_, err = other.s.getPolicyApplication(acct, &policyAppInput{ClientID: other.clientID, AppCode: f.codeA})
	requireErr(t, err, usecase.KindAuthorization, "APPLICATION_FORBIDDEN")
	list, err := other.s.listPolicyApplications(acct, &policyInput{ClientID: other.clientID})
	require.NoError(t, err)
	assert.Empty(t, list.Body.Applications)
	assert.Equal(t, 0, f.count(`SELECT COUNT(*) FROM field_policy_overrides WHERE client_id = $1`, other.clientID))

	_, err = f.s.putPolicyLocks(acct, &putPolicyLocksInput{ClientID: f.clientID, Body: PutFieldPolicyLocksRequest{LockedMembers: []string{}}})
	requireErr(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")
}

// An application's own account may only tighten requireEnrolledDevice,
// badgeOnlyLogin and photosEnabled; a client or platform administrator may
// loosen them. unlockKeysEnabled and the limits it may set either way.
func TestAnApplicationMayOnlyTightenTheCredentialGates(t *testing.T) {
	f := newFixture(t, hasher(t))
	acct := appAccount(f.appA)
	for name, v := range map[string]FieldPolicyOverrideValues{
		"requireEnrolledDevice false": {RequireEnrolledDevice: off()},
		"badgeOnlyLogin true":         {BadgeOnlyLogin: on()},
		"photosEnabled true":          {PhotosEnabled: on()},
	} {
		_, err := f.patch(acct, f.codeA, PatchFieldPolicyOverrideRequest{Set: &v})
		requireErr(t, err, usecase.KindAuthorization, fieldidentity.CodePolicyLooseningForbidden)
		_ = name
	}
	r, err := f.patch(acct, f.codeA, PatchFieldPolicyOverrideRequest{Set: &FieldPolicyOverrideValues{
		RequireEnrolledDevice: on(), BadgeOnlyLogin: off(), PhotosEnabled: off(),
		UnlockKeysEnabled: on(), ShiftMaxSecs: new(86400), PersonalIdleSecs: new(60),
	}})
	require.NoError(t, err, "tightening, and the members it may loosen")
	assert.True(t, r.Effective.RequireEnrolledDevice)
	// Clearing a tightening loosens what is in force, whoever set it (the
	// override does not record who): an administrator clears it.
	_, err = f.patch(acct, f.codeA, PatchFieldPolicyOverrideRequest{Clear: []string{"requireEnrolledDevice", "photosEnabled"}})
	requireErr(t, err, usecase.KindAuthorization, fieldidentity.CodePolicyLooseningForbidden)
	_, err = f.patch(acct, f.codeA, PatchFieldPolicyOverrideRequest{Clear: []string{"badgeOnlyLogin", "shiftMaxSecs"}})
	require.NoError(t, err, "clearing what does not loosen (badge-only is off client-wide too)")
	_, err = f.patch(f.actx, f.codeA, PatchFieldPolicyOverrideRequest{Clear: []string{"requireEnrolledDevice", "photosEnabled"}})
	require.NoError(t, err)

	// photosEnabled loosened while the client has photos off: refused for the
	// application, allowed for an administrator.
	_, err = f.s.putPolicy(f.actx, &putPolicyInput{ClientID: f.clientID, Body: PutFieldPolicyRequest{PhotosEnabled: off()}})
	require.NoError(t, err)
	_, err = f.patch(acct, f.codeA, PatchFieldPolicyOverrideRequest{Set: &FieldPolicyOverrideValues{PhotosEnabled: on()}})
	requireErr(t, err, usecase.KindAuthorization, fieldidentity.CodePolicyLooseningForbidden)
	r, err = f.patch(f.actx, f.codeA, PatchFieldPolicyOverrideRequest{Set: &FieldPolicyOverrideValues{PhotosEnabled: on()}})
	require.NoError(t, err)
	assert.True(t, r.Effective.PhotosEnabled, "an administrator loosens it for A")
	b, err := f.s.getPolicyApplication(f.actx, &policyAppInput{ClientID: f.clientID, AppCode: f.codeB})
	require.NoError(t, err)
	assert.False(t, b.Body.Effective.PhotosEnabled, "B follows the client")
}

// The lock is a platform administrator's: the dedicated permission, not
// anchor scope with field-device manage.
func TestLockingNeedsThePlatformAdministratorsPermission(t *testing.T) {
	f := newFixture(t, hasher(t))
	anchorFieldAdmin := testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_anchorfield01", Scope: auth.ScopeAnchor, AllApplications: true, PrincipalType: auth.PrincipalTypeUser,
		Permissions: []string{"platform:iam:field-device:view", "platform:iam:field-device:manage", "platform:iam:field-worker:manage"},
	})
	_, err := f.s.putPolicyLocks(anchorFieldAdmin, &putPolicyLocksInput{ClientID: f.clientID, Body: PutFieldPolicyLocksRequest{LockedMembers: []string{"badgeOnlyLogin"}}})
	requireErr(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")
	platformAdmin := testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_platadmin0001", Scope: auth.ScopeAnchor, AllApplications: true, PrincipalType: auth.PrincipalTypeUser,
		Permissions: []string{"platform:iam:field-policy:lock"},
	})
	got, err := f.s.putPolicyLocks(platformAdmin, &putPolicyLocksInput{ClientID: f.clientID, Body: PutFieldPolicyLocksRequest{LockedMembers: []string{"badgeOnlyLogin"}}})
	require.NoError(t, err)
	assert.Equal(t, []string{"badgeOnlyLogin"}, got.Body.LockedMembers)
}

// An application disabled for the client, or deleted, takes its override with
// it, in the operation's own transaction.
func TestAnOverrideGoesWithItsApplication(t *testing.T) {
	f := newFixture(t, hasher(t))
	_, err := f.patch(f.actx, f.codeA, PatchFieldPolicyOverrideRequest{Set: &FieldPolicyOverrideValues{UnlockKeysEnabled: on()}})
	require.NoError(t, err)
	_, err = f.patch(f.actx, f.codeB, PatchFieldPolicyOverrideRequest{Set: &FieldPolicyOverrideValues{UnlockKeysEnabled: on()}})
	require.NoError(t, err)
	pool := testpg.Pool(t)
	ctx := context.Background()
	configs := application.NewClientConfigRepo(pool)
	cfg, err := configs.FindByApplicationAndClient(ctx, f.appA, f.clientID)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	cfg.Disable()
	inTx := func(fn func(tx *usecasepgx.DbTx) error) {
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		require.NoError(t, fn(usecaseTx(tx)))
		require.NoError(t, tx.Commit(ctx))
	}
	inTx(func(tx *usecasepgx.DbTx) error { return configs.Persist(ctx, cfg, tx) })
	assert.Equal(t, 0, f.count(`SELECT COUNT(*) FROM field_policy_overrides WHERE application_id = $1`, f.appA), "disabled for the client")

	apps := application.NewRepository(pool)
	b, err := apps.FindByID(ctx, f.appB)
	require.NoError(t, err)
	inTx(func(tx *usecasepgx.DbTx) error { return apps.Delete(ctx, b, tx) })
	assert.Equal(t, 0, f.count(`SELECT COUNT(*) FROM field_policy_overrides WHERE application_id = $1`, f.appB), "deleted")
}

// R1 holds for clearing too: an application cannot undo an administrator's
// tightening by clearing it or by deleting its override; clearing that does
// not loosen what is in force stays allowed. The whole call is refused,
// naming the members, so nothing half-happens.
func TestAnApplicationCannotClearAnAdministratorsTightening(t *testing.T) {
	f := newFixture(t, hasher(t))
	acct := appAccount(f.appA)
	_, err := f.s.putPolicy(f.actx, &putPolicyInput{ClientID: f.clientID, Body: PutFieldPolicyRequest{BadgeOnlyLogin: true}})
	require.NoError(t, err) // client: enrolled devices not required, badge-only on
	_, err = f.patch(f.actx, f.codeA, PatchFieldPolicyOverrideRequest{Set: &FieldPolicyOverrideValues{
		RequireEnrolledDevice: on(), BadgeOnlyLogin: off(), UnlockKeysEnabled: on(),
	}})
	require.NoError(t, err, "an administrator tightens A")

	for name, body := range map[string]PatchFieldPolicyOverrideRequest{
		"clear requireEnrolledDevice": {Clear: []string{"requireEnrolledDevice"}},
		"clear badgeOnlyLogin":        {Clear: []string{"badgeOnlyLogin", "unlockKeysEnabled"}},
	} {
		_, err := f.patch(acct, f.codeA, body)
		requireErr(t, err, usecase.KindAuthorization, fieldidentity.CodePolicyLooseningForbidden)
		_ = name
	}
	_, err = f.s.deletePolicyApplication(acct, &policyAppInput{ClientID: f.clientID, AppCode: f.codeA})
	requireErr(t, err, usecase.KindAuthorization, fieldidentity.CodePolicyLooseningForbidden)
	ue := usecase.AsError(err)
	require.NotNil(t, ue)
	assert.Equal(t, []string{"requireEnrolledDevice", "badgeOnlyLogin"}, ue.Details["fields"])
	a, err := f.s.getPolicyApplication(f.actx, &policyAppInput{ClientID: f.clientID, AppCode: f.codeA})
	require.NoError(t, err)
	assert.True(t, a.Body.Effective.RequireEnrolledDevice, "nothing half-happened")
	assert.False(t, a.Body.Effective.BadgeOnlyLogin)
	assert.True(t, a.Body.Effective.UnlockKeysEnabled)

	// Clearing what does not loosen is allowed: unlockKeysEnabled, and a
	// tightening the client now makes itself.
	_, err = f.patch(acct, f.codeA, PatchFieldPolicyOverrideRequest{Clear: []string{"unlockKeysEnabled"}})
	require.NoError(t, err)
	_, err = f.s.putPolicy(f.actx, &putPolicyInput{ClientID: f.clientID, Body: PutFieldPolicyRequest{RequireEnrolledDevice: true}})
	require.NoError(t, err) // badge-only off client-wide too now
	_, err = f.s.deletePolicyApplication(acct, &policyAppInput{ClientID: f.clientID, AppCode: f.codeA})
	require.NoError(t, err, "judged at the time of the call: nothing it clears loosens what is in force")

	// An administrator may clear anything.
	_, err = f.patch(f.actx, f.codeA, PatchFieldPolicyOverrideRequest{Set: &FieldPolicyOverrideValues{PhotosEnabled: off()}})
	require.NoError(t, err)
	_, err = f.s.deletePolicyApplication(f.actx, &policyAppInput{ClientID: f.clientID, AppCode: f.codeA})
	require.NoError(t, err)
}

// An application is told the same for a client that does not exist as for
// one it does not serve.
func TestAnApplicationLearnsNothingAboutUnknownClients(t *testing.T) {
	f := newFixture(t, hasher(t))
	other := newFixture(t, hasher(t)) // exists, A not enabled for it
	acct := appAccount(f.appA)
	for _, ref := range []string{other.clientID, "no-such-client"} {
		list, err := f.s.listPolicyApplications(acct, &policyInput{ClientID: ref})
		require.NoError(t, err, ref)
		assert.Empty(t, list.Body.Applications, ref)
		_, err = f.s.getPolicyApplication(acct, &policyAppInput{ClientID: ref, AppCode: f.codeA})
		requireErr(t, err, usecase.KindAuthorization, "APPLICATION_FORBIDDEN")
	}
}

// Disabling the application for the client takes the client's field-policy
// lock: an override change racing it runs wholly before (and its row is
// deleted with the disable) or after (and is refused); no override survives
// for an application that no longer serves the client. Deleting the
// configuration row takes the override too.
func TestDisablingAnApplicationRacingAnOverrideChange(t *testing.T) {
	f := newFixture(t, hasher(t))
	pool := testpg.Pool(t)
	ctx := context.Background()
	configs := application.NewClientConfigRepo(pool)
	cfg, err := configs.FindByApplicationAndClient(ctx, f.appA, f.clientID)
	require.NoError(t, err)
	cfg.Disable()
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, configs.Persist(ctx, cfg, usecaseTx(tx))) // holds the policy lock until commit

	done := make(chan error, 1)
	go func() {
		_, err := f.patch(f.actx, f.codeA, PatchFieldPolicyOverrideRequest{Set: &FieldPolicyOverrideValues{UnlockKeysEnabled: on()}})
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("the override change did not wait for the disable (err %v)", err)
	case <-time.After(300 * time.Millisecond):
	}
	require.NoError(t, tx.Commit(ctx))
	requireErr(t, <-done, usecase.KindConflict, fieldidentity.CodeApplicationNotEnabled)
	assert.Equal(t, 0, f.count(`SELECT COUNT(*) FROM field_policy_overrides WHERE application_id = $1`, f.appA))

	// The configuration row deleted: its override goes with it.
	_, err = f.patch(f.actx, f.codeB, PatchFieldPolicyOverrideRequest{Set: &FieldPolicyOverrideValues{UnlockKeysEnabled: on()}})
	require.NoError(t, err)
	cfgB, err := configs.FindByApplicationAndClient(ctx, f.appB, f.clientID)
	require.NoError(t, err)
	tx, err = pool.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, configs.Delete(ctx, cfgB, usecaseTx(tx)))
	require.NoError(t, tx.Commit(ctx))
	assert.Equal(t, 0, f.count(`SELECT COUNT(*) FROM field_policy_overrides WHERE application_id = $1`, f.appB))
}
