package common_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

// TestMessageJSONRoundtrip verifies camelCase tags and omitempty match
// the wire contract. If this test diverges, webhook delivery
// payloads diverge.
func TestMessageJSONRoundtrip(t *testing.T) {
	authToken := "bearer-token"
	signingSecret := "shh"
	groupID := "group-1"

	m := common.Message{
		ID:              "msg_01",
		PoolCode:        "pool-a",
		AuthToken:       &authToken,
		SigningSecret:   &signingSecret,
		MediationType:   common.MediationTypeHTTP,
		MediationTarget: "https://example.com/webhook",
		MessageGroupID:  &groupID,
		HighPriority:    true,
		DispatchMode:    common.DispatchNextOnError,
	}

	b, err := json.Marshal(m)
	require.NoError(t, err)
	got := string(b)

	// Field names are camelCase (the wire contract).
	assert.Contains(t, got, `"id":"msg_01"`)
	assert.Contains(t, got, `"poolCode":"pool-a"`)
	assert.Contains(t, got, `"authToken":"bearer-token"`)
	assert.Contains(t, got, `"signingSecret":"shh"`)
	assert.Contains(t, got, `"mediationType":"HTTP"`)
	assert.Contains(t, got, `"mediationTarget":"https://example.com/webhook"`)
	assert.Contains(t, got, `"messageGroupId":"group-1"`)
	assert.Contains(t, got, `"highPriority":true`)
	assert.Contains(t, got, `"dispatchMode":"NEXT_ON_ERROR"`)

	// Round-trip back to struct.
	var back common.Message
	require.NoError(t, json.Unmarshal(b, &back))
	assert.Equal(t, m, back)
}

// TestMessageOmitsEmptyOptionals verifies unset optionals are omitted
// from the JSON entirely (absent, not null).
func TestMessageOmitsEmptyOptionals(t *testing.T) {
	m := common.Message{
		ID:              "msg_02",
		MediationType:   common.MediationTypeHTTP,
		MediationTarget: "https://example.com/webhook",
	}
	b, err := json.Marshal(m)
	require.NoError(t, err)
	got := string(b)

	assert.NotContains(t, got, `"authToken"`)
	assert.NotContains(t, got, `"signingSecret"`)
	assert.NotContains(t, got, `"messageGroupId"`)
	assert.NotContains(t, got, `"highPriority"`)
	assert.NotContains(t, got, `"dispatchMode"`)
}

// An unspecified mode means DefaultDispatchMode — ordering kept, moving on past
// a failure. It used to mean IMMEDIATE, so every producer that omitted the field
// silently gave up sequencing.
func TestDispatchModeParseDefaultsToOrdered(t *testing.T) {
	assert.Equal(t, common.DispatchNextOnError, common.DefaultDispatchMode,
		"the default must be a mode that actually orders")
	assert.Equal(t, common.DefaultDispatchMode, common.ParseDispatchMode(""))
	assert.Equal(t, common.DefaultDispatchMode, common.ParseDispatchMode("UNKNOWN_GIBBERISH"),
		"a typo is a producer bug, not a request for concurrency")
}

// An explicit mode is always honoured — including IMMEDIATE, which is now the
// only way to ask for unordered dispatch.
func TestDispatchModeParseHonoursExplicitModes(t *testing.T) {
	assert.Equal(t, common.DispatchImmediate, common.ParseDispatchMode("IMMEDIATE"))
	assert.Equal(t, common.DispatchNextOnError, common.ParseDispatchMode("NEXT_ON_ERROR"))
	assert.Equal(t, common.DispatchBlockOnError, common.ParseDispatchMode("BLOCK_ON_ERROR"))
}

func TestDispatchModeRequiresOrdering(t *testing.T) {
	assert.False(t, common.DispatchImmediate.RequiresOrdering())
	assert.True(t, common.DispatchNextOnError.RequiresOrdering())
	assert.True(t, common.DispatchBlockOnError.RequiresOrdering())
}

func TestDispatchStatusLifecycle(t *testing.T) {
	assert.True(t, common.DispatchCompleted.IsTerminal())
	assert.True(t, common.DispatchCompleted.IsSuccessful())
	assert.True(t, common.DispatchFailed.IsTerminal())
	assert.False(t, common.DispatchFailed.IsSuccessful())
	assert.False(t, common.DispatchPending.IsTerminal())
}

// TestParseDispatchStatusStrict pins the X-06 conversion: ParseDispatchStatus
// still accepts the legacy aliases (IN_PROGRESS/ERROR — real values
// msg_dispatch_jobs.status has held, see GroupHoldingStatusSQL) but an
// actually-unrecognised value must be rejected (ok=false), never silently
// coerced to PENDING. The PENDING default from the old lenient parser is
// gone.
func TestParseDispatchStatusStrict(t *testing.T) {
	v, ok := common.ParseDispatchStatus("IN_PROGRESS")
	require.True(t, ok)
	assert.Equal(t, common.DispatchProcessing, v)

	v, ok = common.ParseDispatchStatus("ERROR")
	require.True(t, ok)
	assert.Equal(t, common.DispatchFailed, v)

	_, ok = common.ParseDispatchStatus("WHO_KNOWS")
	assert.False(t, ok, "an unrecognised status must be rejected, not coerced to PENDING")
}

func TestOutboxStatusCodes(t *testing.T) {
	for _, s := range []common.OutboxStatus{
		common.OutboxPending, common.OutboxSuccess, common.OutboxBadRequest,
		common.OutboxInternalError, common.OutboxUnauthorized, common.OutboxForbidden,
		common.OutboxGatewayError, common.OutboxInProgress,
	} {
		assert.Equal(t, s, common.FromOutboxCode(s.Code()))
	}
}

func TestOutboxItemTypeAPIPath(t *testing.T) {
	assert.Equal(t, "/api/events/batch", common.OutboxItemEvent.APIPath())
	assert.Equal(t, "/api/dispatch-jobs/batch", common.OutboxItemDispatchJob.APIPath())
	assert.Equal(t, "/api/audit-logs/batch", common.OutboxItemAuditLog.APIPath())
}

func TestParseOutboxItemTypeAliases(t *testing.T) {
	for _, in := range []string{"DISPATCH_JOB", "DISPATCHJOB", "DISPATCH-JOB"} {
		v, ok := common.ParseOutboxItemType(in)
		require.True(t, ok)
		assert.Equal(t, common.OutboxItemDispatchJob, v)
	}
	_, ok := common.ParseOutboxItemType("UNKNOWN")
	assert.False(t, ok)
}
