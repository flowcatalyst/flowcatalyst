//go:build integration

package control

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
	fncontrol "github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// seedEmittingFunction seeds an application (code appCode, derived from the
// test name) + function + a PUBLISHED version 1 whose describe declares
// emits verbatim (pass an already-qualified type, e.g. "<appCode>:a:b:c"
// for an owned one, or a foreign one to exercise EVENT_TYPE_NOT_OWNED).
// Returns the function and its owning application's code.
func seedEmittingFunction(t *testing.T, s *State, emits ...string) (fn *function.Function, appCode string) {
	t.Helper()
	appID := "app_" + shortID(t)
	appCode = "evtapp" + shortID(t)
	seedApplication(t, testpg.Pool(t), appID, appCode)
	fn = seedFunction(t, testpg.Pool(t), s.Repo, appID, appCode, "fn", nil)
	seedVersion(t, s.Repo, fn.ID, 1, function.VersionPublished, "wasm", describeJSON(t, emits...))
	return fn, appCode
}

func TestEmit_Success(t *testing.T) {
	t.Parallel()
	s := newTestState(t)
	appID := "app_" + shortID(t)
	appCode := "evtokapp" + shortID(t)
	eventType := appCode + ":order:order:shipped"
	seedApplication(t, testpg.Pool(t), appID, appCode)
	fn := seedFunction(t, testpg.Pool(t), s.Repo, appID, appCode, "fn", nil)
	seedVersion(t, s.Repo, fn.ID, 1, function.VersionPublished, "wasm", describeJSON(t, eventType))

	ctx := anchorCtx()
	out, err := s.emit(ctx, &apicommon.In[fncontrol.EmitRequest]{Body: fncontrol.EmitRequest{
		FunctionID: fn.ID, Version: 1,
		Event: abi.Event{Type: eventType, DedupID: "dedup-" + shortID(t)},
		Data:  json.RawMessage(`{"k":"v"}`),
	}})
	require.NoError(t, err)
	require.NotEmpty(t, out.Body.EventID)

	stored, err := s.Events.FindRawByID(context.Background(), out.Body.EventID)
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, "function:"+fn.Address, stored.Source, "source defaults to function:<address>")
	assert.Equal(t, eventType, stored.Type)
}

func TestEmit_EventNotDeclared(t *testing.T) {
	t.Parallel()
	s := newTestState(t)
	fn, appCode := seedEmittingFunction(t, s /* version 1's describe declares no emits */)

	ctx := anchorCtx()
	_, err := s.emit(ctx, &apicommon.In[fncontrol.EmitRequest]{Body: fncontrol.EmitRequest{
		FunctionID: fn.ID, Version: 1,
		Event: abi.Event{Type: appCode + ":order:order:shipped", DedupID: "dedup-" + shortID(t)},
	}})
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "EVENT_NOT_DECLARED")
}

func TestEmit_EventTypeNotOwned(t *testing.T) {
	t.Parallel()
	s := newTestState(t)
	// Declares an event type owned by a DIFFERENT application than fn's own.
	fn, _ := seedEmittingFunction(t, s, "someother:order:order:shipped")

	ctx := anchorCtx()
	_, err := s.emit(ctx, &apicommon.In[fncontrol.EmitRequest]{Body: fncontrol.EmitRequest{
		FunctionID: fn.ID, Version: 1,
		Event: abi.Event{Type: "someother:order:order:shipped", DedupID: "dedup-" + shortID(t)},
	}})
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "EVENT_TYPE_NOT_OWNED")
}

func TestEmit_IdempotentOnDedupID(t *testing.T) {
	t.Parallel()
	s := newTestState(t)
	appID := "app_" + shortID(t)
	appCode := "evtidemapp" + shortID(t)
	eventType := appCode + ":order:order:shipped"
	seedApplication(t, testpg.Pool(t), appID, appCode)
	fn := seedFunction(t, testpg.Pool(t), s.Repo, appID, appCode, "fn", nil)
	seedVersion(t, s.Repo, fn.ID, 1, function.VersionPublished, "wasm", describeJSON(t, eventType))

	dedup := "dedup-fixed-" + shortID(t)
	ctx := anchorCtx()
	req := fncontrol.EmitRequest{
		FunctionID: fn.ID, Version: 1,
		Event: abi.Event{Type: eventType, DedupID: dedup},
		Data:  json.RawMessage(`{"n":1}`),
	}
	out1, err := s.emit(ctx, &apicommon.In[fncontrol.EmitRequest]{Body: req})
	require.NoError(t, err)

	req.Data = json.RawMessage(`{"n":2}`) // a retried emit; same dedupId, different body
	out2, err := s.emit(ctx, &apicommon.In[fncontrol.EmitRequest]{Body: req})
	require.NoError(t, err)

	assert.Equal(t, out1.Body.EventID, out2.Body.EventID, "same dedupId must answer with the same event id")

	stored, err := s.Events.FindRawByID(context.Background(), out1.Body.EventID)
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.JSONEq(t, `{"n":1}`, string(stored.Data), "the FIRST write wins; the retry is a no-op")
}
