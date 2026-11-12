//go:build integration

package api

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// A test identity's console sign-in code needs only `manage`. So flipping a
// real worker to a test identity needs the rule for signing in as a real
// worker, and unmarking one expires its outstanding codes.

// humanCtx is an active human principal (a real iam_principals row, so a code
// it mints is redeemable) of the client with the permissions.
func (f *fixture) humanCtx(perms ...string) context.Context {
	f.t.Helper()
	id := fmt.Sprintf("prn_tb%011d", seq.Add(1))
	_, err := testpg.Pool(f.t).Exec(context.Background(),
		`INSERT INTO iam_principals (id, type, scope, name, active) VALUES ($1, 'USER', 'ANCHOR', 'Console Admin', TRUE)`, id)
	require.NoError(f.t, err)
	return testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID: id, Scope: auth.ScopeClient, Clients: []string{f.clientID},
		Permissions: perms, AllApplications: true, PrincipalType: auth.PrincipalTypeUser,
	})
}

func (f *fixture) mintCode(ctx context.Context, workerID string) string {
	f.t.Helper()
	out, err := f.s.createSignInCode(ctx, &createSignInCodeInput{ID: workerID, Body: CreateFieldSignInCodeRequest{AppCode: f.codeA}})
	require.NoError(f.t, err)
	return out.Body.Code
}

func (f *fixture) redeem(code string) error {
	f.t.Helper()
	_, err := usecaseop.RunTx(context.Background(), f.s.UoW, f.s.Ops.RedeemSignInCode(), fieldidentity.RedeemSignInCodeCommand{
		Code: code, JKT: "jkt-" + code[len(code)-6:], RefreshHash: fieldidentity.HashToken("r-" + code),
	}, usecase.NewExecutionContext(fieldidentity.PrincipalSignInCode))
	return err
}

func TestMarkingARealWorkerAsTestNeedsTheRealWorkerRule(t *testing.T) {
	f := newFixture(t, hasher(t))
	yes := true
	manager := f.humanCtx(permManage)
	impersonator := f.humanCtx(permManage, permImpersonate)
	saManage := callerCtx([]string{f.clientID}, []string{permManage}, nil, true)
	saBoth := callerCtx([]string{f.clientID}, []string{permManage, permImpersonate}, nil, true)
	set := func(ctx context.Context, id string) error {
		_, err := f.s.updateWorker(ctx, &updateWorkerInput{ID: id, Body: UpdateFieldWorkerRequest{Name: "Real", IsTest: &yes}})
		return err
	}

	w := f.ensure(f.actx, "TB1-REAL", "Real", f.codeA).Body
	requireErr(t, set(manager, w.ID), usecase.KindAuthorization, "PERMISSION_REQUIRED")
	assert.False(t, f.workerFlag(w.ID, "is_test"), "row unchanged")
	requireErr(t, set(saManage, w.ID), usecase.KindAuthorization, "PERMISSION_REQUIRED")
	requireErr(t, set(saBoth, w.ID), usecase.KindAuthorization, "PRINCIPAL_NOT_HUMAN")
	assert.False(t, f.workerFlag(w.ID, "is_test"), "row unchanged")

	require.NoError(t, set(impersonator, w.ID))
	assert.True(t, f.workerFlag(w.ID, "is_test"))
	// Setting it again is not a change: manage alone is fine.
	require.NoError(t, set(manager, w.ID))
}

func TestUnmarkingATestWorkerExpiresItsSignInCodes(t *testing.T) {
	f := newFixture(t, hasher(t))
	yes, no := true, false
	manager := f.humanCtx(permManage)
	test, err := f.s.ensureWorker(manager, &apicommon.In[EnsureFieldWorkerRequest]{Body: EnsureFieldWorkerRequest{
		ClientID: f.clientID, WorkerCode: "TB2-TEST", Name: "Test", AppCodes: []string{f.codeA}, IsTest: &yes,
	}})
	require.NoError(t, err)
	id := test.Body.ID
	code := f.mintCode(manager, id)

	// A name-only update leaves the code redeemable (TB4): proven with a second code.
	keep := f.mintCode(manager, id)
	_, err = f.s.updateWorker(manager, &updateWorkerInput{ID: id, Body: UpdateFieldWorkerRequest{Name: "Renamed"}})
	require.NoError(t, err)
	assert.Equal(t, 2, f.count(`SELECT COUNT(*) FROM field_sign_in_codes WHERE identity_id = $1 AND used_at IS NULL AND expires_at > NOW()`, id))
	// Setting isTest to its current value is not a change either.
	_, err = f.s.updateWorker(manager, &updateWorkerInput{ID: id, Body: UpdateFieldWorkerRequest{Name: "Renamed", IsTest: &yes}})
	require.NoError(t, err)
	assert.Equal(t, 2, f.count(`SELECT COUNT(*) FROM field_sign_in_codes WHERE identity_id = $1 AND used_at IS NULL AND expires_at > NOW()`, id))
	require.NoError(t, f.redeem(keep), "an update that does not change isTest leaves the code redeemable")

	// What an unknown or expired code answers today.
	unknown := f.redeem("FWS1NOPE")
	ue := usecase.AsError(unknown)
	require.NotNil(t, ue)

	_, err = f.s.updateWorker(manager, &updateWorkerInput{ID: id, Body: UpdateFieldWorkerRequest{Name: "Renamed", IsTest: &no}})
	require.NoError(t, err)
	assert.False(t, f.workerFlag(id, "is_test"))
	err = f.redeem(code)
	got := usecase.AsError(err)
	require.NotNil(t, got, "the code minted under the test rule is refused once the worker is real")
	assert.Equal(t, ue.Kind, got.Kind)
	assert.Equal(t, ue.Code, got.Code)
	assert.Equal(t, ue.Message, got.Message)
	assert.Equal(t, "INVALID_CODE", got.Code)
	assert.Equal(t, 0, f.count(`SELECT COUNT(*) FROM field_sign_in_codes WHERE identity_id = $1 AND used_at IS NULL AND expires_at > NOW()`, id))
}

func TestNewTestWorkerWithManageAloneStillWorks(t *testing.T) {
	f := newFixture(t, hasher(t))
	yes := true
	manager := f.humanCtx(permManage)
	test, err := f.s.ensureWorker(manager, &apicommon.In[EnsureFieldWorkerRequest]{Body: EnsureFieldWorkerRequest{
		ClientID: f.clientID, WorkerCode: "TB3-TEST", Name: "Test", AppCodes: []string{f.codeA}, IsTest: &yes,
	}})
	require.NoError(t, err)
	assert.True(t, test.Body.IsTest)
	code := f.mintCode(manager, test.Body.ID)
	require.NoError(t, f.redeem(code))
}
