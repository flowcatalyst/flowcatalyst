//go:build integration

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// docs/field-policy-application-overrides.md: the three states, the locks,
// the limits and who may change what.

func on() *bool  { v := true; return &v }
func off() *bool { v := false; return &v }

func (f *fixture) patch(ctx context.Context, appCode string, body PatchFieldPolicyOverrideRequest) (*FieldPolicyApplicationResponse, error) {
	out, err := f.s.patchPolicyApplication(ctx, &patchPolicyOverrideInput{ClientID: f.clientID, AppCode: appCode, Body: body})
	if err != nil {
		return nil, err
	}
	return &out.Body, nil
}

func TestPolicyOverrideThreeStates(t *testing.T) {
	f := newFixture(t, hasher(t))
	got, err := f.s.getPolicyApplication(f.actx, &policyAppInput{ClientID: f.clientID, AppCode: f.codeA})
	require.NoError(t, err)
	m := got.Body.Members.UnlockKeysEnabled
	assert.Equal(t, FieldPolicyBoolMember{Effective: false, Source: "DEFAULT"}, m, "no policy, no override: the default")

	// On for A: effective on, from the application, its own value on.
	r, err := f.patch(f.actx, f.codeA, PatchFieldPolicyOverrideRequest{Set: &FieldPolicyOverrideValues{UnlockKeysEnabled: on(), PersonalIdleSecs: new(3600)}})
	require.NoError(t, err)
	assert.Equal(t, FieldPolicyBoolMember{Effective: true, Source: "APPLICATION", Override: on()}, r.Members.UnlockKeysEnabled)
	assert.Equal(t, FieldPolicyIntMember{Effective: 3600, Source: "APPLICATION", Override: new(3600)}, r.Members.PersonalIdleSecs)
	assert.True(t, r.Effective.UnlockKeysEnabled)
	assert.Equal(t, 3600, r.Effective.PersonalIdleSecs)
	assert.NotNil(t, r.UpdatedAt)

	// B follows the client.
	b, err := f.s.getPolicyApplication(f.actx, &policyAppInput{ClientID: f.clientID, AppCode: f.codeB})
	require.NoError(t, err)
	assert.False(t, b.Body.Members.UnlockKeysEnabled.Effective)
	assert.Nil(t, b.Body.Members.UnlockKeysEnabled.Override)

	// The client's own policy: the source is CLIENT where A follows it.
	_, err = f.s.putPolicy(f.actx, &putPolicyInput{ClientID: f.clientID, Body: PutFieldPolicyRequest{BadgeOnlyLogin: true}})
	require.NoError(t, err)
	a, err := f.s.getPolicyApplication(f.actx, &policyAppInput{ClientID: f.clientID, AppCode: f.codeA})
	require.NoError(t, err)
	assert.Equal(t, FieldPolicyBoolMember{Effective: true, Source: "CLIENT"}, a.Body.Members.BadgeOnlyLogin)
	assert.True(t, a.Body.Members.UnlockKeysEnabled.Effective, "the client-wide PUT leaves A's override alone")

	// Clear one member: it follows the client again; the other stays.
	r, err = f.patch(f.actx, f.codeA, PatchFieldPolicyOverrideRequest{Clear: []string{"personalIdleSecs"}})
	require.NoError(t, err)
	assert.Equal(t, "CLIENT", r.Members.PersonalIdleSecs.Source)
	assert.Nil(t, r.Members.PersonalIdleSecs.Override)
	assert.Equal(t, "APPLICATION", r.Members.UnlockKeysEnabled.Source)

	list, err := f.s.listPolicyApplications(f.actx, &policyInput{ClientID: f.clientID})
	require.NoError(t, err)
	require.Len(t, list.Body.Applications, 1)
	assert.Equal(t, f.codeA, list.Body.Applications[0].AppCode)
	assert.Equal(t, on(), list.Body.Applications[0].Overrides.UnlockKeysEnabled)

	// DELETE clears every member; the row goes.
	d, err := f.s.deletePolicyApplication(f.actx, &policyAppInput{ClientID: f.clientID, AppCode: f.codeA})
	require.NoError(t, err)
	assert.Nil(t, d.Body.UpdatedAt)
	assert.Equal(t, 0, f.count(`SELECT COUNT(*) FROM field_policy_overrides WHERE client_id = $1`, f.clientID))
	assert.Equal(t, 3, f.eventCount(fieldidentity.PolicyOverriddenType, f.clientID))
}

// An override of the idle and shift limits stays within the client-wide
// bounds; a member that cannot be overridden is refused, and has no column
// to be stored in.
func TestPolicyOverrideLimitsAndMembers(t *testing.T) {
	f := newFixture(t, hasher(t))
	for name, v := range map[string]FieldPolicyOverrideValues{
		"shift below 3600":  {ShiftMaxSecs: new(3599)},
		"shift above 86400": {ShiftMaxSecs: new(86401)},
		"idle below 60":     {SharedIdleSecs: new(59)},
		"unenrolled idle":   {UnenrolledIdleSecs: new(86401)},
	} {
		_, err := f.patch(f.actx, f.codeA, PatchFieldPolicyOverrideRequest{Set: &v})
		requireErr(t, err, usecase.KindValidation, "POLICY_INVALID")
		_ = name
	}
	_, err := f.patch(f.actx, f.codeA, PatchFieldPolicyOverrideRequest{Set: &FieldPolicyOverrideValues{ShiftMaxSecs: new(3600)}, Clear: []string{"shiftMaxSecs"}})
	requireErr(t, err, usecase.KindValidation, "POLICY_INVALID")

	// Through the use case, as an older or careless caller might: a member
	// that is not overridable is refused, set or cleared.
	for _, cmd := range []fieldidentity.PatchPolicyOverrideCommand{
		{ClientID: f.clientID, ApplicationID: f.appA, Set: map[fieldidentity.PolicyMember]any{"pinMinLength": 4}},
		{ClientID: f.clientID, ApplicationID: f.appA, Set: map[fieldidentity.PolicyMember]any{"unlockKeyAttestation": "NONE"}},
		{ClientID: f.clientID, ApplicationID: f.appA, Clear: []fieldidentity.PolicyMember{"hardLockThreshold"}},
		{ClientID: f.clientID, ApplicationID: f.appA, Set: map[fieldidentity.PolicyMember]any{"unlockKeysEnabled": "yes"}},
	} {
		_, err := usecaseop.RunTx(f.actx, f.s.UoW, f.s.Ops.PatchPolicyOverride(), cmd, testpg.TestEC())
		requireErr(t, err, usecase.KindValidation, "POLICY_INVALID")
	}
	assert.Equal(t, 0, f.count(`SELECT COUNT(*) FROM field_policy_overrides WHERE client_id = $1`, f.clientID))
	// The table has a column for each overridable member and nothing else.
	cols := f.count(`SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'field_policy_overrides'
		AND column_name NOT IN ('client_id','application_id','updated_at','updated_by')`)
	assert.Equal(t, len(fieldidentity.OverridableMembers()), cols)
}

// A locked member: an override of it is kept, does not apply and reads as
// overruled; setting it is refused, clearing it is not; unlocking restores
// it. Only a platform administrator locks.
func TestPolicyOverrideLocks(t *testing.T) {
	f := newFixture(t, hasher(t))
	_, err := f.patch(f.actx, f.codeA, PatchFieldPolicyOverrideRequest{Set: &FieldPolicyOverrideValues{RequireEnrolledDevice: on(), PhotosEnabled: off()}})
	require.NoError(t, err)

	clientAdmin := callerCtx([]string{f.clientID}, allFieldPerms, nil, true)
	_, err = f.s.putPolicyLocks(clientAdmin, &putPolicyLocksInput{ClientID: f.clientID, Body: PutFieldPolicyLocksRequest{LockedMembers: []string{"requireEnrolledDevice"}}})
	requireErr(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")

	pol, err := f.s.putPolicyLocks(f.actx, &putPolicyLocksInput{ClientID: f.clientID, Body: PutFieldPolicyLocksRequest{LockedMembers: []string{"requireEnrolledDevice"}}})
	require.NoError(t, err)
	assert.Equal(t, []string{"requireEnrolledDevice"}, pol.Body.LockedMembers)
	a, err := f.s.getPolicyApplication(f.actx, &policyAppInput{ClientID: f.clientID, AppCode: f.codeA})
	require.NoError(t, err)
	assert.Equal(t, FieldPolicyBoolMember{Effective: false, Source: "DEFAULT", Locked: true, Override: on(), Overruled: true}, a.Body.Members.RequireEnrolledDevice)
	assert.False(t, a.Body.Effective.RequireEnrolledDevice, "the lock keeps the override from applying")
	assert.Equal(t, "APPLICATION", a.Body.Members.PhotosEnabled.Source, "the other member still applies")
	eff, err := f.s.Ops.EffectivePolicy(context.Background(), f.clientID, f.appA, nil)
	require.NoError(t, err)
	assert.False(t, eff.RequireEnrolledDevice)
	assert.False(t, eff.PhotosEnabled)
	list, err := f.s.listPolicyApplications(f.actx, &policyInput{ClientID: f.clientID})
	require.NoError(t, err)
	assert.Equal(t, []string{"requireEnrolledDevice"}, list.Body.Applications[0].Overruled)

	_, err = f.patch(f.actx, f.codeA, PatchFieldPolicyOverrideRequest{Set: &FieldPolicyOverrideValues{RequireEnrolledDevice: off()}})
	requireErr(t, err, usecase.KindConflict, fieldidentity.CodePolicyMemberLocked)

	// The client-wide PUT never changes the locks.
	_, err = f.s.putPolicy(f.actx, &putPolicyInput{ClientID: f.clientID, Body: PutFieldPolicyRequest{}})
	require.NoError(t, err)
	got, err := f.s.getPolicy(f.actx, &policyInput{ClientID: f.clientID})
	require.NoError(t, err)
	assert.Equal(t, []string{"requireEnrolledDevice"}, got.Body.LockedMembers)

	// Unlocking restores the kept override.
	_, err = f.s.putPolicyLocks(f.actx, &putPolicyLocksInput{ClientID: f.clientID, Body: PutFieldPolicyLocksRequest{LockedMembers: []string{}}})
	require.NoError(t, err)
	a, err = f.s.getPolicyApplication(f.actx, &policyAppInput{ClientID: f.clientID, AppCode: f.codeA})
	require.NoError(t, err)
	assert.Equal(t, FieldPolicyBoolMember{Effective: true, Source: "APPLICATION", Override: on()}, a.Body.Members.RequireEnrolledDevice)
	assert.Equal(t, 2, f.eventCount(fieldidentity.PolicyLocksChangedType, f.clientID))

	// Locked again, clearing the override is allowed.
	_, err = f.s.putPolicyLocks(f.actx, &putPolicyLocksInput{ClientID: f.clientID, Body: PutFieldPolicyLocksRequest{LockedMembers: []string{"requireEnrolledDevice"}}})
	require.NoError(t, err)
	r, err := f.patch(f.actx, f.codeA, PatchFieldPolicyOverrideRequest{Clear: []string{"requireEnrolledDevice"}})
	require.NoError(t, err)
	assert.Nil(t, r.Members.RequireEnrolledDevice.Override)
}

// An application-confined caller reads and changes only its own
// application's override, for clients it serves; a client administrator only
// their client's.
func TestPolicyOverrideTenancy(t *testing.T) {
	f := newFixture(t, hasher(t))
	appA := callerCtx([]string{f.clientID}, []string{"platform:iam:field-worker:manage"}, []string{f.appA}, false)
	r, err := f.patch(appA, f.codeA, PatchFieldPolicyOverrideRequest{Set: &FieldPolicyOverrideValues{UnlockKeysEnabled: on()}})
	require.NoError(t, err, "an application changes its own override with the permission it holds")
	assert.True(t, r.Members.UnlockKeysEnabled.Effective)

	_, err = f.patch(appA, f.codeB, PatchFieldPolicyOverrideRequest{Set: &FieldPolicyOverrideValues{UnlockKeysEnabled: on()}})
	requireErr(t, err, usecase.KindAuthorization, "APPLICATION_FORBIDDEN")
	_, err = f.s.getPolicyApplication(appA, &policyAppInput{ClientID: f.clientID, AppCode: f.codeB})
	requireErr(t, err, usecase.KindAuthorization, "APPLICATION_FORBIDDEN")

	// An administrator sets B's; A's account lists only its own.
	_, err = f.patch(f.actx, f.codeB, PatchFieldPolicyOverrideRequest{Set: &FieldPolicyOverrideValues{PhotosEnabled: off()}})
	require.NoError(t, err)
	list, err := f.s.listPolicyApplications(appA, &policyInput{ClientID: f.clientID})
	require.NoError(t, err)
	require.Len(t, list.Body.Applications, 1)
	assert.Equal(t, f.codeA, list.Body.Applications[0].AppCode)

	// The client-wide PUT stays refused to an application, exactly as released.
	_, err = f.s.putPolicy(appA, &putPolicyInput{ClientID: f.clientID, Body: PutFieldPolicyRequest{UnlockKeysEnabled: true}})
	require.Error(t, err)

	// Another client's caller, application or administrator, reaches nothing.
	other := newFixture(t, hasher(t))
	foreignApp := callerCtx([]string{other.clientID}, []string{"platform:iam:field-worker:manage"}, []string{f.appA}, false)
	_, err = f.patch(foreignApp, f.codeA, PatchFieldPolicyOverrideRequest{Set: &FieldPolicyOverrideValues{UnlockKeysEnabled: off()}})
	requireErr(t, err, usecase.KindAuthorization, "SCOPE_FORBIDDEN")
	foreignAdmin := callerCtx([]string{other.clientID}, allFieldPerms, nil, true)
	_, err = f.s.listPolicyApplications(foreignAdmin, &policyInput{ClientID: f.clientID})
	requireErr(t, err, usecase.KindAuthorization, "SCOPE_FORBIDDEN")
	// Without a field permission, nothing.
	nobody := callerCtx([]string{f.clientID}, []string{"platform:iam:portal-user:view"}, []string{f.appA}, false)
	_, err = f.s.getPolicyApplication(nobody, &policyAppInput{ClientID: f.clientID, AppCode: f.codeA})
	require.Error(t, err)
}
