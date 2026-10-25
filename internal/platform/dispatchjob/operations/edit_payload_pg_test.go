//go:build integration

package operations_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
)

func TestEditDispatchJobPayload_EditableStatuses(t *testing.T) {
	t.Parallel()
	repo := dispatchjob.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	for _, st := range []common.DispatchStatus{common.DispatchPending, common.DispatchFailed, "ERROR"} {
		t.Run("edits "+string(st), func(t *testing.T) {
			t.Parallel()
			j := seedJob(t, repo, "opstest:edit:ok:"+string(st), st, seedOpts{AttemptCount: 2, LastError: new("boom")})
			ev, err := runAuthorized(uow, operations.EditDispatchJobPayload(repo), operations.EditPayloadCommand{ID: j.ID, Payload: `{"fixed":true}`})
			require.NoError(t, err)
			assert.Equal(t, j.ID, ev.DispatchJobID)

			got, err := repo.FindByID(context.Background(), j.ID)
			require.NoError(t, err)
			require.NotNil(t, got.Payload)
			assert.Equal(t, `{"fixed":true}`, *got.Payload)
			assert.Equal(t, "application/json", got.PayloadContentType, "content type kept when not supplied")
			var raw string
			require.NoError(t, testpg.Pool(t).QueryRow(context.Background(), `SELECT status FROM msg_dispatch_jobs WHERE id = $1`, j.ID).Scan(&raw))
			assert.Equal(t, string(st), raw, "status is untouched (stored value)")
			assert.Equal(t, int32(2), got.AttemptCount, "attempts are untouched")
			require.NotNil(t, got.LastError)
			assert.Equal(t, "boom", *got.LastError)
		})
	}

	for _, st := range []common.DispatchStatus{
		common.DispatchQueued, common.DispatchProcessing, common.DispatchCompleted, common.DispatchCancelled, common.DispatchExpired,
	} {
		t.Run("refuses "+string(st), func(t *testing.T) {
			t.Parallel()
			j := seedJob(t, repo, "opstest:edit:no:"+string(st), st, seedOpts{})
			before, err := repo.FindByID(context.Background(), j.ID)
			require.NoError(t, err)
			_, err = runAuthorized(uow, operations.EditDispatchJobPayload(repo), operations.EditPayloadCommand{ID: j.ID, Payload: `{"x":1}`})
			testpg.RequireUsecaseError(t, err, usecase.KindConflict, "NOT_EDITABLE")
			after, err := repo.FindByID(context.Background(), j.ID)
			require.NoError(t, err)
			assert.Equal(t, before.Payload, after.Payload)
		})
	}
}

func TestEditDispatchJobPayload_NotFound(t *testing.T) {
	t.Parallel()
	repo := dispatchjob.NewRepository(testpg.Pool(t))
	_, err := runAuthorized(testpg.NewUoW(t), operations.EditDispatchJobPayload(repo), operations.EditPayloadCommand{ID: "dpj_doesnotexist01", Payload: "{}"})
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "DispatchJob_NOT_FOUND")
}

func TestEditDispatchJobPayload_Validation(t *testing.T) {
	t.Parallel()
	repo := dispatchjob.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	j := seedJob(t, repo, "opstest:edit:validation", common.DispatchFailed, seedOpts{})
	run := func(cmd operations.EditPayloadCommand) error {
		cmd.ID = j.ID
		_, err := runAuthorized(uow, operations.EditDispatchJobPayload(repo), cmd)
		return err
	}

	// JSON content type (the job's own) refuses invalid JSON.
	testpg.RequireUsecaseError(t, run(operations.EditPayloadCommand{Payload: `{not json`}), usecase.KindValidation, "PAYLOAD_NOT_JSON")
	testpg.RequireUsecaseError(t, run(operations.EditPayloadCommand{Payload: strings.Repeat("a", operations.MaxEditedPayloadBytes+1)}), usecase.KindValidation, "PAYLOAD_TOO_LARGE")
	testpg.RequireUsecaseError(t, run(operations.EditPayloadCommand{Payload: "a\xffb", PayloadContentType: new("text/plain")}), usecase.KindValidation, "PAYLOAD_NOT_UTF8")

	// Changing the content type to XML makes a non-JSON payload valid, and is persisted.
	require.NoError(t, run(operations.EditPayloadCommand{Payload: `<order id="1"/>`, PayloadContentType: new("application/xml")}))
	got, err := repo.FindByID(context.Background(), j.ID)
	require.NoError(t, err)
	assert.Equal(t, "application/xml", got.PayloadContentType)
	assert.Equal(t, `<order id="1"/>`, *got.Payload)

	// An XML job takes XML as it is; a switch back to JSON validates against JSON.
	require.NoError(t, run(operations.EditPayloadCommand{Payload: `<order id="2"/>`}))
	testpg.RequireUsecaseError(t, run(operations.EditPayloadCommand{Payload: `<order/>`, PayloadContentType: new("application/json")}), usecase.KindValidation, "PAYLOAD_NOT_JSON")
}

func TestEditDispatchJobPayload_ResourceScope(t *testing.T) {
	t.Parallel()
	repo := dispatchjob.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	own := tsid.Generate(tsid.Client)
	other := tsid.Generate(tsid.Client)
	clientCtx := testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_djeditscope1", Scope: auth.ScopeClient, Clients: []string{own},
	})
	run := func(id string) error {
		_, err := usecaseop.Run(clientCtx, uow, operations.EditDispatchJobPayload(repo), operations.EditPayloadCommand{ID: id, Payload: "{}"}, testpg.TestEC())
		return err
	}
	testpg.RequireUsecaseError(t, run(seedJob(t, repo, "opstest:edit:scope:other", common.DispatchFailed, seedOpts{ClientID: &other}).ID), usecase.KindAuthorization, "SCOPE_FORBIDDEN")
	testpg.RequireUsecaseError(t, run(seedJob(t, repo, "opstest:edit:scope:platform", common.DispatchFailed, seedOpts{}).ID), usecase.KindAuthorization, "SCOPE_FORBIDDEN")
	require.NoError(t, run(seedJob(t, repo, "opstest:edit:scope:own", common.DispatchFailed, seedOpts{ClientID: &own}).ID))
}

// The audit row and the event carry sizes and digests, never the payload text.
func TestEditDispatchJobPayload_AuditHoldsNoPayload(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	repo := dispatchjob.NewRepository(pool)
	uow := testpg.NewUoW(t)
	ctx := context.Background()
	j := seedJob(t, repo, "opstest:edit:audit", common.DispatchFailed, seedOpts{})
	_, err := pool.Exec(ctx, `UPDATE msg_dispatch_jobs SET payload = $2 WHERE id = $1`, j.ID, `{"email":"old@example.com"}`)
	require.NoError(t, err)

	ev, err := runAuthorized(uow, operations.EditDispatchJobPayload(repo), operations.EditPayloadCommand{ID: j.ID, Payload: `{"email":"new@example.com"}`})
	require.NoError(t, err)
	assert.Equal(t, len(`{"email":"old@example.com"}`), ev.PreviousSize)
	assert.Equal(t, len(`{"email":"new@example.com"}`), ev.Size)
	assert.NotEqual(t, ev.PreviousSHA256, ev.SHA256)
	assert.Len(t, ev.SHA256, 64)
	data, err := ev.ToDataJSON()
	require.NoError(t, err)
	assert.NotContains(t, string(data), "example.com")

	var opJSON string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT operation_json::text FROM aud_logs WHERE entity_id = $1 AND operation = 'EditPayloadCommand'`, j.ID).Scan(&opJSON))
	assert.NotContains(t, opJSON, "example.com", "the audit row must not hold the payload")
	assert.Contains(t, opJSON, j.ID)
}
