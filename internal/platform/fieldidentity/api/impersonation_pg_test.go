//go:build integration

package api

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
)

const (
	permManage      = "platform:iam:field-worker:manage"
	permImpersonate = "platform:iam:field-worker:impersonate"
)

// userCtx is a human principal of the client with the permissions.
func userCtx(clientID string, perms ...string) context.Context {
	return testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_fieldhuman001", Scope: auth.ScopeClient, Clients: []string{clientID},
		Permissions: perms, AllApplications: true, PrincipalType: auth.PrincipalTypeUser,
	})
}

func TestCanImpersonateNeedsTheImpersonatePermission(t *testing.T) {
	f := newFixture(t, hasher(t))
	manager := userCtx(f.clientID, permManage, "platform:iam:field-worker:view")
	both := userCtx(f.clientID, permManage, permImpersonate)
	yes := true

	// Manage alone cannot create a supervisor, nor promote one.
	_, err := f.s.ensureWorker(manager, &apicommon.In[EnsureFieldWorkerRequest]{Body: EnsureFieldWorkerRequest{
		ClientID: f.clientID, WorkerCode: "SUP-X", Name: "N", CanImpersonate: &yes,
	}})
	requireErr(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")
	plain := f.ensure(manager, "PL-1", "Plain", f.codeA).Body
	assert.False(t, plain.CanImpersonate)
	_, err = f.s.updateWorker(manager, &updateWorkerInput{ID: plain.ID, Body: UpdateFieldWorkerRequest{Name: "Plain", CanImpersonate: &yes}})
	requireErr(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")
	assert.False(t, f.workerFlag(plain.ID, "can_impersonate"))
	// Manage alone may still rename, but may not mark an existing real worker
	// as a test identity (see TestMarkingARealWorkerAsTestNeedsTheRealWorkerRule).
	_, err = f.s.updateWorker(manager, &updateWorkerInput{ID: plain.ID, Body: UpdateFieldWorkerRequest{Name: "Renamed", IsTest: &yes}})
	requireErr(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")
	up, err := f.s.updateWorker(manager, &updateWorkerInput{ID: plain.ID, Body: UpdateFieldWorkerRequest{Name: "Renamed"}})
	require.NoError(t, err)
	assert.False(t, up.Body.IsTest)
	assert.Equal(t, 1, f.eventCount(fieldidentity.WorkerUpdatedType, plain.ID))

	// With the impersonate permission it works, on create and update.
	sup, err := f.s.ensureWorker(both, &apicommon.In[EnsureFieldWorkerRequest]{Body: EnsureFieldWorkerRequest{
		ClientID: f.clientID, WorkerCode: "SUP-1", Name: "Sup", CanImpersonate: &yes, AppCodes: []string{f.codeA},
	}})
	require.NoError(t, err)
	assert.True(t, sup.Body.CanImpersonate)
	got, err := f.s.updateWorker(both, &updateWorkerInput{ID: plain.ID, Body: UpdateFieldWorkerRequest{Name: "Renamed", CanImpersonate: &yes}})
	require.NoError(t, err)
	assert.True(t, got.Body.CanImpersonate)

	// Clearing is a change too (manage alone may not), and ends the sessions the
	// worker holds open as an actor.
	target := f.ensure(f.actx, "TG-1", "Target", f.codeA).Body
	imp := f.session(target.ID, "", f.appA)
	_, err = testpg.Pool(t).Exec(context.Background(),
		`UPDATE field_sessions SET actor_type = 'FIELD_WORKER', actor_id = $2, jkt = 'k' WHERE id = $1`, imp.ID, plain.ID)
	require.NoError(t, err)
	no := false
	_, err = f.s.updateWorker(manager, &updateWorkerInput{ID: plain.ID, Body: UpdateFieldWorkerRequest{Name: "Renamed", CanImpersonate: &no}})
	requireErr(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")
	revoked, _ := f.sessionState(imp.ID)
	assert.False(t, revoked)
	_, err = f.s.updateWorker(both, &updateWorkerInput{ID: plain.ID, Body: UpdateFieldWorkerRequest{Name: "Renamed", CanImpersonate: &no}})
	require.NoError(t, err)
	revoked, reason := f.sessionState(imp.ID)
	assert.True(t, revoked)
	assert.Equal(t, fieldidentity.ReasonActorSuspended, reason)
}

func (f *fixture) workerFlag(id, col string) bool {
	f.t.Helper()
	var v bool
	require.NoError(f.t, testpg.Pool(f.t).QueryRow(context.Background(), `SELECT `+col+` FROM field_identities WHERE id = $1`, id).Scan(&v))
	return v
}

func TestTestIdentityFilterAndEvents(t *testing.T) {
	f := newFixture(t, hasher(t))
	yes := true
	test, err := f.s.ensureWorker(f.actx, &apicommon.In[EnsureFieldWorkerRequest]{Body: EnsureFieldWorkerRequest{
		ClientID: f.clientID, WorkerCode: "T-1", Name: "Tester", IsTest: &yes,
	}})
	require.NoError(t, err)
	assert.True(t, test.Body.IsTest)
	f.ensure(f.actx, "R-1", "Real")
	assert.Equal(t, 1, f.count(`SELECT COUNT(*) FROM msg_events WHERE type = $1 AND data::jsonb->>'isTest' = 'true' AND data::text LIKE '%' || $2 || '%'`,
		fieldidentity.WorkerCreatedType, test.Body.ID))

	names := func(isTest string) []string {
		out, err := f.s.listWorkers(f.actx, &listWorkersInput{ClientID: f.clientID, IsTest: isTest})
		require.NoError(t, err)
		var codes []string
		for _, w := range out.Body.Workers {
			codes = append(codes, w.WorkerCode)
		}
		return codes
	}
	assert.ElementsMatch(t, []string{"T-1", "R-1"}, names(""))
	assert.Equal(t, []string{"T-1"}, names("true"))
	assert.Equal(t, []string{"R-1"}, names("false"))
}

func TestSignInCodeCreationPermissionsAndTargets(t *testing.T) {
	f := newFixture(t, hasher(t))
	real := f.ensure(f.actx, "SC-REAL", "Real", f.codeA).Body
	yes := true
	test, err := f.s.ensureWorker(f.actx, &apicommon.In[EnsureFieldWorkerRequest]{Body: EnsureFieldWorkerRequest{
		ClientID: f.clientID, WorkerCode: "SC-TEST", Name: "Test", AppCodes: []string{f.codeA}, IsTest: &yes,
	}})
	require.NoError(t, err)
	req := func(id, app string) *createSignInCodeInput {
		return &createSignInCodeInput{ID: id, Body: CreateFieldSignInCodeRequest{AppCode: app}}
	}

	manager := userCtx(f.clientID, permManage)
	impersonator := userCtx(f.clientID, permImpersonate)

	// A real worker needs the impersonate permission; manage is not enough.
	_, err = f.s.createSignInCode(manager, req(real.ID, f.codeA))
	requireErr(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")
	// A test identity needs only manage.
	out, err := f.s.createSignInCode(manager, req(test.Body.ID, f.codeA))
	require.NoError(t, err)
	assert.Len(t, out.Body.Code, 30)
	assert.Contains(t, out.Body.Code, "FWS1")
	assert.True(t, out.Body.ExpiresAt.Underlying().After(time.Now().Add(9*time.Minute)))
	assert.Equal(t, test.Body.ID, out.Body.WorkerID)
	assert.Equal(t, f.codeA, out.Body.AppCode)
	assert.Equal(t, 1, f.count(`SELECT COUNT(*) FROM field_sign_in_codes WHERE code_hash = $1 AND created_by = $2`,
		fieldidentity.HashToken(out.Body.Code), "prn_fieldhuman001"))
	assert.Equal(t, 1, f.eventCount(fieldidentity.WorkerSignInCodeCreatedType, test.Body.ID))
	assert.Equal(t, 0, f.count(`SELECT (SELECT COUNT(*) FROM msg_events WHERE data::text LIKE '%' || $1 || '%')
		+ (SELECT COUNT(*) FROM aud_logs WHERE operation_json::text LIKE '%' || $1 || '%')`, out.Body.Code), "the code is never audited")
	// The audit row names the calling admin.
	assert.Equal(t, 1, f.count(`SELECT COUNT(*) FROM aud_logs WHERE principal_id = 'prn_fieldhuman001' AND entity_id = $1
		AND operation = 'CreateSignInCodeCommand'`, test.Body.ID))
	// An application the worker is not granted: 409; its own, an impersonator may.
	_, err = f.s.createSignInCode(impersonator, req(real.ID, f.codeB))
	requireErr(t, err, usecase.KindConflict, "NO_APP_ACCESS")
	ok, err := f.s.createSignInCode(impersonator, req(real.ID, f.codeA))
	require.NoError(t, err)
	assert.NotEqual(t, out.Body.Code, ok.Body.Code)
	// Suspended worker: 409.
	_, err = f.s.suspendWorker(f.actx, &apicommon.IDInput{ID: real.ID})
	require.NoError(t, err)
	_, err = f.s.createSignInCode(impersonator, req(real.ID, f.codeA))
	requireErr(t, err, usecase.KindConflict, "WORKER_UNAVAILABLE")

	// A service account: refused for a real worker even with the permission;
	// allowed for a test identity with manage alone.
	f.ensure(f.actx, "SC-REAL2", "Real2", f.codeA)
	real2, err := f.s.Ops.R.Identities.FindByClientAndCode(f.actx, f.clientID, "SC-REAL2")
	require.NoError(t, err)
	sa := callerCtx([]string{f.clientID}, []string{permManage, permImpersonate}, nil, true)
	_, err = f.s.createSignInCode(sa, req(real2.ID, f.codeA))
	requireErr(t, err, usecase.KindAuthorization, "PRINCIPAL_NOT_HUMAN")
	saManage := callerCtx([]string{f.clientID}, []string{permManage}, nil, true)
	_, err = f.s.createSignInCode(saManage, req(test.Body.ID, f.codeA))
	require.NoError(t, err)

	// Client-boxed: another client's worker is a 404.
	other := newFixture(t, hasher(t))
	theirs := other.ensure(other.actx, "SC-THEIRS", "T", other.codeA).Body
	_, err = f.s.createSignInCode(impersonator, req(theirs.ID, other.codeA))
	requireErr(t, err, usecase.KindNotFound, "FieldWorker_NOT_FOUND")
}

func TestPolicyCarriesTheSignInOptionFields(t *testing.T) {
	f := newFixture(t, hasher(t))
	def, err := f.s.getPolicy(f.actx, &policyInput{ClientID: f.clientID})
	require.NoError(t, err)
	assert.False(t, def.Body.RequireEnrolledDevice)
	assert.Equal(t, 900, def.Body.UnenrolledIdleSecs)
	assert.False(t, def.Body.BadgeOnlyLogin)
	assert.Equal(t, 14400, def.Body.ImpersonationMaxSecs)

	idle, imp := 120, 600
	put, err := f.s.putPolicy(f.actx, &putPolicyInput{ClientID: f.clientID, Body: PutFieldPolicyRequest{
		RequireEnrolledDevice: true, UnenrolledIdleSecs: &idle, BadgeOnlyLogin: true, ImpersonationMaxSecs: &imp,
	}})
	require.NoError(t, err)
	assert.True(t, put.Body.RequireEnrolledDevice)
	assert.Equal(t, 120, put.Body.UnenrolledIdleSecs)
	assert.True(t, put.Body.BadgeOnlyLogin)
	assert.Equal(t, 600, put.Body.ImpersonationMaxSecs)
	assert.Equal(t, 1, f.count(`SELECT COUNT(*) FROM msg_events WHERE type = $1 AND data::jsonb->>'requireEnrolledDevice' = 'true'
		AND data::jsonb->>'unenrolledIdleSecs' = '120' AND data::text LIKE '%' || $2 || '%'`, fieldidentity.PolicyUpdatedType, f.clientID))

	for name, body := range map[string]PutFieldPolicyRequest{
		"unenrolledIdleSecs low":    {UnenrolledIdleSecs: new(59)},
		"unenrolledIdleSecs high":   {UnenrolledIdleSecs: new(86401)},
		"impersonationMaxSecs low":  {ImpersonationMaxSecs: new(299)},
		"impersonationMaxSecs high": {ImpersonationMaxSecs: new(43201)},
	} {
		_, err = f.s.putPolicy(f.actx, &putPolicyInput{ClientID: f.clientID, Body: body})
		requireErr(t, err, usecase.KindValidation, "POLICY_INVALID")
		_ = name
	}
}

func TestSignInsListShowsMethodAndActor(t *testing.T) {
	f := newFixture(t, hasher(t))
	w := f.ensure(f.actx, "SI-1", "W", f.codeA).Body
	require.NoError(t, f.s.Ops.R.Sessions.RecordLoginAttempt(context.Background(), fieldidentity.LoginAttempt{
		IdentityID: w.ID, AuthMethod: fieldidentity.MethodSignInCode, Outcome: fieldidentity.OutcomeSuccess, ActorID: "prn_someadmin0001",
	}))
	out, err := f.s.signIns(f.actx, &signInsInput{ID: w.ID})
	require.NoError(t, err)
	require.Len(t, out.Body.SignIns, 1)
	assert.Equal(t, "SIGN_IN_CODE", out.Body.SignIns[0].AuthMethod)
	assert.Equal(t, "prn_someadmin0001", out.Body.SignIns[0].ActorID)
}
