//go:build integration

// trace_pg_test.go covers the trace-context write path
// (docs/function-trace-design.md sections 1, 4 and 7): EnableTraceContext and
// DisableTraceContext, their caps and validation, the declared-context check,
// the same-transaction pool revision bump, the audit events, the listing, and
// the retention prune.
package operations_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
)

var traceLimits = function.TraceLimits{MaxMinutes: 60, HTTPMaxMinutes: 15, RetentionDays: 30}

// seedTraceVersion inserts a version whose describe declares contexts.
func (f *testFunctionFixture) seedTraceVersion(number int32, status function.VersionStatus, contexts ...string) {
	f.t.Helper()
	describe := `{"abi":1,"endpoints":[{"method":"POST","path":"/x","auth":"webhook"}]`
	if len(contexts) > 0 {
		describe += `,"traceContexts":[`
		for i, c := range contexts {
			if i > 0 {
				describe += ","
			}
			describe += fmt.Sprintf(`{"name":%q,"description":"the %s context"}`, c, c)
		}
		describe += `]`
	}
	describe += `}`
	v := function.Version{
		ID: tsid.Generate(tsid.FunctionVersion), FunctionID: f.fn.ID, Number: number,
		Digest: fmt.Sprintf("%064x", time.Now().UnixNano()+int64(number)), SizeBytes: 1, ABI: 1, Runtime: "js",
		Describe: []byte(describe), Status: status, CreatedAt: time.Now().UTC(),
	}
	require.NoError(f.t, f.repo.InsertVersion(context.Background(), &v))
}

func (f *testFunctionFixture) enableTrace(context_ string, minutes, sample int) (operations.TraceContextState, error) {
	f.t.Helper()
	return usecaseop.RunTx(testpg.AnchorCtx(), f.uow, operations.EnableTraceContext(f.repo, traceLimits),
		operations.EnableTraceCommand{FunctionID: f.fn.ID, Context: context_, Minutes: minutes, SampleRate: sample}, testpg.TestEC())
}

func (f *testFunctionFixture) disableTrace(context_ string) error {
	f.t.Helper()
	_, err := usecaseop.RunTx(testpg.AnchorCtx(), f.uow, operations.DisableTraceContext(f.repo),
		operations.DisableTraceCommand{FunctionID: f.fn.ID, Context: context_}, testpg.TestEC())
	return err
}

func TestEnableTrace_DeclaredContext_OpensWindowBumpsRevisionAndEmitsEvent(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_optrace0001a", "optraceapp1", "trace-fn")
	f.seedTraceVersion(1, function.VersionReady, "orders")
	revBefore := f.poolRevision()
	eventsBefore := len(f.functionEvents())

	before := time.Now()
	st, err := f.enableTrace("orders", 5, 10)
	require.NoError(t, err)
	assert.Equal(t, "orders", st.Name)
	assert.Equal(t, "the orders context", st.Description)
	assert.False(t, st.BuiltIn)
	assert.True(t, st.Live)
	assert.Equal(t, 10, st.SampleRate)
	assert.Equal(t, 60, st.MaxMinutes)
	require.NotNil(t, st.EnabledUntil)
	assert.WithinDuration(t, before.Add(5*time.Minute), *st.EnabledUntil, 5*time.Second)

	assert.Greater(t, f.poolRevision(), revBefore, "an enable must bump the pool revision (wakes the runners' long-poll)")

	row, err := f.repo.GetTraceContext(context.Background(), f.fn.ID, "orders")
	require.NoError(t, err)
	require.NotNil(t, row)
	assert.Equal(t, int32(10), row.SampleRate)
	require.NotNil(t, row.EnabledBy)
	assert.True(t, row.Live(time.Now()))

	evs := f.eventsSince(eventsBefore)
	require.Len(t, evs, 1)
	assert.Equal(t, "platform:function:trace:enabled", evs[0].Type)
	assert.Equal(t, "orders", evs[0].Data["context"])
	assert.EqualValues(t, 10, evs[0].Data["sampleRate"])
	assert.NotEmpty(t, evs[0].Data["enabledUntil"])
	assert.Equal(t, false, evs[0].Data["indefinite"], "indefinite is always present")
}

func TestEnableTrace_SampleRateDefaultsToOne(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_optrace0002a", "optraceapp2", "trace-fn")
	st, err := f.enableTrace("http", 5, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, st.SampleRate)
	assert.True(t, st.BuiltIn)
}

func TestEnableTrace_HTTPNeedsNoDeclaration(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_optrace0003a", "optraceapp3", "trace-fn") // no versions at all
	st, err := f.enableTrace("http", 15, 1)
	require.NoError(t, err)
	assert.Equal(t, 15, st.MaxMinutes)
	assert.True(t, st.Live)
}

func TestEnableTrace_WindowCaps(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_optrace0004a", "optraceapp4", "trace-fn")
	f.seedTraceVersion(1, function.VersionReady, "orders")

	_, err := f.enableTrace("orders", 61, 1)
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, operations.TraceWindowTooLong)
	_, err = f.enableTrace("http", 16, 1)
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, operations.TraceWindowTooLong)
	_, err = f.enableTrace("orders", 0, 1)
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "TRACE_MINUTES_INVALID")

	_, err = f.enableTrace("orders", 60, 1)
	require.NoError(t, err, "the cap itself is allowed")
	_, err = f.enableTrace("http", 15, 1)
	require.NoError(t, err)
}

func TestEnableTrace_SampleRateBounds(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_optrace0005a", "optraceapp5", "trace-fn")
	for _, bad := range []int{-1, 1001} {
		_, err := f.enableTrace("http", 5, bad)
		testpg.RequireUsecaseError(t, err, usecase.KindValidation, "TRACE_SAMPLE_RATE_INVALID")
	}
	_, err := f.enableTrace("http", 5, 1000)
	require.NoError(t, err)
}

func TestEnableTrace_UnknownContext(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_optrace0006a", "optraceapp6", "trace-fn")
	// Declared only by a PUBLISHED version: not usable until it is READY.
	f.seedTraceVersion(1, function.VersionPublished, "orders")
	_, err := f.enableTrace("orders", 5, 1)
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, operations.TraceContextUnknown)
	_, err = f.enableTrace("nope", 5, 1)
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, operations.TraceContextUnknown)
	_, err = f.enableTrace("Bad Name", 5, 1)
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, operations.TraceContextUnknown)

	f.seedTraceVersion(2, function.VersionReady, "orders")
	_, err = f.enableTrace("orders", 5, 1)
	require.NoError(t, err, "a READY version declares it")
}

func TestEnableTrace_FunctionNotFound(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_optrace0007a", "optraceapp7", "trace-fn")
	_, err := usecaseop.RunTx(testpg.AnchorCtx(), f.uow, operations.EnableTraceContext(f.repo, traceLimits),
		operations.EnableTraceCommand{FunctionID: "fnc_doesnotexist", Context: "http", Minutes: 5}, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "Function_NOT_FOUND")
}

func TestDisableTrace_ClosesWindowIsIdempotentAndEmitsEvent(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_optrace0008a", "optraceapp8", "trace-fn")
	_, err := f.enableTrace("http", 5, 1)
	require.NoError(t, err)
	revAfterEnable := f.poolRevision()
	eventsBefore := len(f.functionEvents())

	offAt := time.Now()
	require.NoError(t, f.disableTrace("http"))
	row, err := f.repo.GetTraceContext(context.Background(), f.fn.ID, "http")
	require.NoError(t, err)
	require.NotNil(t, row)
	require.NotNil(t, row.EnabledUntil, "switching off ends the window now, so queued records are still accepted")
	assert.WithinDuration(t, offAt, *row.EnabledUntil, 5*time.Second)
	assert.False(t, row.Indefinite)
	assert.False(t, row.Live(time.Now().Add(time.Second)))
	assert.Greater(t, f.poolRevision(), revAfterEnable, "a disable must bump the pool revision")

	evs := f.eventsSince(eventsBefore)
	require.Len(t, evs, 1)
	assert.Equal(t, "platform:function:trace:disabled", evs[0].Type)
	assert.Equal(t, "http", evs[0].Data["context"])

	require.NoError(t, f.disableTrace("http"), "a second disable is fine")
	again, err := f.repo.GetTraceContext(context.Background(), f.fn.ID, "http")
	require.NoError(t, err)
	assert.Equal(t, row.EnabledUntil.UnixMicro(), again.EnabledUntil.UnixMicro(), "a second disable keeps the first end")
	require.NoError(t, f.disableTrace("orders"), "a context that was never on is fine")
	none, err := f.repo.GetTraceContext(context.Background(), f.fn.ID, "orders")
	require.NoError(t, err)
	assert.Nil(t, none, "disabling a context that was never on creates no row")
}

func TestListTraceContexts_UnionOfReadyVersionsPlusHTTP(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_optrace0009a", "optraceapp9", "trace-fn")
	f.seedTraceVersion(1, function.VersionReady, "orders", "billing")
	f.seedTraceVersion(2, function.VersionReady, "orders", "audit")
	f.seedTraceVersion(3, function.VersionPublished, "pending")
	f.seedTraceVersion(4, function.VersionRetired, "retired-ctx")
	_, err := f.enableTrace("audit", 5, 1)
	require.NoError(t, err)

	_, states, err := operations.ListTraceContexts(testpg.AnchorCtx(), f.repo, traceLimits, f.fn.ID)
	require.NoError(t, err)
	var names []string
	byName := map[string]operations.TraceContextState{}
	for _, s := range states {
		names = append(names, s.Name)
		byName[s.Name] = s
	}
	assert.Equal(t, []string{"http", "audit", "billing", "orders"}, names, "http first, then declared contexts by name")
	assert.True(t, byName["http"].BuiltIn)
	assert.Equal(t, 15, byName["http"].MaxMinutes)
	assert.Equal(t, 60, byName["orders"].MaxMinutes)
	assert.True(t, byName["audit"].Live)
	require.NotNil(t, byName["audit"].EnabledUntil)
	assert.False(t, byName["orders"].Live)
	assert.Nil(t, byName["orders"].EnabledUntil)
	assert.Equal(t, "the orders context", byName["orders"].Description)
}

func TestListTraceContexts_ExpiredWindowIsNotLive(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_optrace0010a", "optraceapp10", "trace-fn")
	past := time.Now().Add(-time.Minute)
	withTx(t, f, func(c *function.TraceContext) { c.EnabledUntil = &past })

	_, states, err := operations.ListTraceContexts(testpg.AnchorCtx(), f.repo, traceLimits, f.fn.ID)
	require.NoError(t, err)
	require.Equal(t, "http", states[0].Name)
	assert.False(t, states[0].Live, "live is compared when read")
	assert.NotNil(t, states[0].EnabledUntil, "the closed window is still reported")
}

// withTx writes an http context row directly, adjusted by mutate.
func withTx(t *testing.T, f *testFunctionFixture, mutate func(*function.TraceContext)) {
	t.Helper()
	c := function.TraceContext{FunctionID: f.fn.ID, Context: "http", SampleRate: 1, UpdatedAt: time.Now().UTC()}
	mutate(&c)
	require.NoError(t, f.repo.WithPoolTx(context.Background(), func(tx pgx.Tx) error {
		return f.repo.UpsertTraceContextTx(context.Background(), &c, tx)
	}))
}

func TestPruneTraceLog_DeletesOnlyOldRecordsOfTheFunctionInChunks(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_optrace0011a", "optraceapp11", "trace-fn")
	other := newTestFunctionFixture(t, "app_optrace0012a", "optraceapp12", "trace-fn")
	now := time.Now().UTC()
	// Age is the platform's created_at; the runner-supplied at is the
	// opposite on purpose, to prove retention never reads it.
	insert := func(fnID string, age time.Duration, n int) {
		recs := make([]function.TraceLogRecord, n)
		for i := range recs {
			recs[i] = function.TraceLogRecord{
				ID: tsid.GenerateWithPrefix("ftl"), FunctionID: fnID, Version: 1, InvocationID: "inv",
				Context: "http", Kind: "log", Level: "info", Message: "m", At: now.Add(age - 41*24*time.Hour), CreatedAt: now.Add(-age),
			}
		}
		require.NoError(t, f.repo.InsertTraceRecords(context.Background(), recs))
	}
	insert(f.fn.ID, 40*24*time.Hour, 25) // old (stored 40 days ago, at "fresh")
	insert(f.fn.ID, 1*time.Hour, 3)      // fresh (stored an hour ago, at 40 days back)
	insert(other.fn.ID, 40*24*time.Hour, 4)

	// Chunk of 10 and enough chunks: the 25 old rows go in three statements.
	n, err := f.repo.PruneTraceLog(context.Background(), f.fn.ID, now.Add(-30*24*time.Hour), 10, 10)
	require.NoError(t, err)
	assert.EqualValues(t, 25, n)
	left, err := f.repo.CountTraceLog(context.Background(), f.fn.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 3, left)
	otherLeft, err := f.repo.CountTraceLog(context.Background(), other.fn.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 4, otherLeft, "another function's records are untouched by a per-function prune")

	// The bounded loop stops at maxChunks.
	insert(f.fn.ID, 40*24*time.Hour, 25)
	n, err = f.repo.PruneTraceLog(context.Background(), f.fn.ID, now.Add(-30*24*time.Hour), 10, 2)
	require.NoError(t, err)
	assert.EqualValues(t, 20, n, "two chunks of ten")

	// The backstop sweeps every function.
	n, err = f.repo.PruneTraceLog(context.Background(), "", now.Add(-30*24*time.Hour), 10, 100)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, n, int64(5+4), "5 left of ours and the other function's 4 (parallel tests may add more)")
	otherLeft, err = f.repo.CountTraceLog(context.Background(), other.fn.ID)
	require.NoError(t, err)
	assert.Zero(t, otherLeft)
}

func TestPruneTraceAfterEnable_NeverFails(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_optrace0013a", "optraceapp13", "trace-fn")
	now := time.Now().UTC()
	require.NoError(t, f.repo.InsertTraceRecords(context.Background(), []function.TraceLogRecord{{
		ID: tsid.GenerateWithPrefix("ftl"), FunctionID: f.fn.ID, Version: 1, InvocationID: "i", Context: "http",
		Kind: "log", Level: "info", Message: "old", At: now, CreatedAt: now.Add(-31 * 24 * time.Hour),
	}}))
	// Detached from the caller: a cancelled request does not stop it.
	reqCtx, cancel := context.WithCancel(context.Background())
	done := operations.PruneTraceAfterEnable(reqCtx, f.repo, traceLimits, f.fn.ID)
	cancel()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("background prune did not finish")
	}
	n, err := f.repo.CountTraceLog(context.Background(), f.fn.ID)
	require.NoError(t, err)
	assert.Zero(t, n)
}

// The live prune visits only functions with an open window.
func TestPruneLiveTraceFunctions(t *testing.T) {
	t.Parallel()
	live := newTestFunctionFixture(t, "app_optrace0014a", "optraceapp14", "trace-fn")
	idle := newTestFunctionFixture(t, "app_optrace0015a", "optraceapp15", "trace-fn")
	now := time.Now().UTC()
	for _, fx := range []*testFunctionFixture{live, idle} {
		require.NoError(t, fx.repo.InsertTraceRecords(context.Background(), []function.TraceLogRecord{{
			ID: tsid.GenerateWithPrefix("ftl"), FunctionID: fx.fn.ID, Version: 1, InvocationID: "i", Context: "http",
			Kind: "log", Level: "info", Message: "old", At: now, CreatedAt: now.Add(-31 * 24 * time.Hour),
		}}))
	}
	_, err := live.enableTrace("http", 5, 1)
	require.NoError(t, err)
	ids, err := live.repo.ListLiveTraceFunctionIDs(context.Background(), time.Now())
	require.NoError(t, err)
	assert.Contains(t, ids, live.fn.ID)
	assert.NotContains(t, ids, idle.fn.ID)

	function.PruneLiveTraceFunctions(context.Background(), live.repo, traceLimits)
	n, err := live.repo.CountTraceLog(context.Background(), live.fn.ID)
	require.NoError(t, err)
	assert.Zero(t, n)
	n, err = idle.repo.CountTraceLog(context.Background(), idle.fn.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 1, n, "a function without a live window is left to the sweep")
}

func TestEnableTrace_Indefinite_CeilingPermissionAndNeverHTTP(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_optrace0020a", "optraceapp20", "trace-fn")
	f.seedTraceVersion(1, function.VersionReady, "orders")
	eventsBefore := len(f.functionEvents())
	enable := func(ctx context.Context, context_ string) (operations.TraceContextState, error) {
		return usecaseop.RunTx(ctx, f.uow, operations.EnableTraceContext(f.repo, traceLimits),
			operations.EnableTraceCommand{FunctionID: f.fn.ID, Context: context_, Indefinite: true}, testpg.TestEC())
	}

	// http never stays on until switched off: its minute cap always applies.
	_, err := enable(testpg.AnchorCtx(), "http")
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, operations.TraceIndefiniteNotAllowed)

	// It needs platform:function:trace:indefinite, beyond trace:manage.
	manager := testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_optracemgr01", Scope: auth.ScopeAnchor,
		Permissions: []string{"platform:function:trace:manage", "platform:function:trace:view"},
	})
	_, err = enable(manager, "orders")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "platform:function:trace:indefinite")

	// Granted: minutes is ignored, and the window has the hard ceiling.
	before := time.Now()
	st, err := enable(testpg.AnchorCtx(), "orders")
	require.NoError(t, err)
	assert.True(t, st.Live)
	assert.True(t, st.Indefinite)
	assert.Equal(t, function.DefaultTraceIndefiniteMaxDays, st.IndefiniteMaxDays)
	require.NotNil(t, st.EnabledUntil)
	assert.WithinDuration(t, before.Add(7*24*time.Hour), *st.EnabledUntil, 10*time.Second)
	evs := f.eventsSince(eventsBefore)
	require.Len(t, evs, 1)
	assert.Equal(t, true, evs[0].Data["indefinite"])
	assert.NotEmpty(t, evs[0].Data["enabledUntil"], "the ceiling is in the event")

	// Re-enabling with a time limit replaces it.
	timed, err := f.enableTrace("orders", 5, 1)
	require.NoError(t, err)
	assert.False(t, timed.Indefinite)
	assert.True(t, timed.EnabledUntil.Before(time.Now().Add(time.Hour)))

	// Disabling ends an indefinite window like any other.
	_, err = enable(testpg.AnchorCtx(), "orders")
	require.NoError(t, err)
	require.NoError(t, f.disableTrace("orders"))
	row, err := f.repo.GetTraceContext(context.Background(), f.fn.ID, "orders")
	require.NoError(t, err)
	assert.False(t, row.Indefinite)
	assert.False(t, row.Live(time.Now().Add(time.Second)))
}
