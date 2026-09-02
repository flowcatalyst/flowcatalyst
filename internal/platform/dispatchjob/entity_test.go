package dispatchjob_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
)

// TestParseKindStrict pins the X-06 (T, bool) idiom: known values parse
// with ok=true, and an unrecognised value (including empty — callers that
// want "unspecified defaults to EVENT" branch on emptiness themselves
// before calling this, see dispatch_jobs_batch.go's jobFromItem) is
// rejected (ok=false) rather than silently coerced to EVENT.
func TestParseKindStrict(t *testing.T) {
	got, ok := dispatchjob.ParseKind("EVENT")
	require.True(t, ok)
	assert.Equal(t, dispatchjob.KindEvent, got)

	got, ok = dispatchjob.ParseKind("TASK")
	require.True(t, ok)
	assert.Equal(t, dispatchjob.KindTask, got)

	_, ok = dispatchjob.ParseKind("NOT_A_REAL_KIND")
	assert.False(t, ok, "an unrecognised kind must not silently coerce to EVENT")

	_, ok = dispatchjob.ParseKind("")
	assert.False(t, ok, "empty is not a known kind — callers handle unspecified themselves")
}

// TestParseRetryStrategyStrict pins the accepted legacy aliases
// (IMMEDIATE/FIXED_DELAY — real values retry_strategy has held) alongside
// the X-06 rejection of anything else, including empty.
func TestParseRetryStrategyStrict(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want dispatchjob.RetryStrategy
	}{
		{"immediate", dispatchjob.RetryImmediate},
		{"IMMEDIATE", dispatchjob.RetryImmediate},
		{"fixed", dispatchjob.RetryFixed},
		{"FIXED_DELAY", dispatchjob.RetryFixed},
		{"exponential", dispatchjob.RetryExponentialBackoff},
	} {
		got, ok := dispatchjob.ParseRetryStrategy(tc.in)
		require.True(t, ok, "input %q", tc.in)
		assert.Equal(t, tc.want, got, "input %q", tc.in)
	}

	_, ok := dispatchjob.ParseRetryStrategy("NOT_A_REAL_STRATEGY")
	assert.False(t, ok, "an unrecognised retry strategy must not silently coerce to exponential")

	_, ok = dispatchjob.ParseRetryStrategy("")
	assert.False(t, ok, "empty is not a known retry strategy — callers handle unspecified themselves")
}

// TestParseErrorTypeStrict pins the five known constants and rejects
// anything else, including empty.
func TestParseErrorTypeStrict(t *testing.T) {
	for _, want := range []dispatchjob.ErrorType{
		dispatchjob.ErrorConnection, dispatchjob.ErrorTimeout, dispatchjob.ErrorHTTPError,
		dispatchjob.ErrorValidation, dispatchjob.ErrorUnknown,
	} {
		got, ok := dispatchjob.ParseErrorType(string(want))
		require.True(t, ok, "input %q", want)
		assert.Equal(t, want, got, "input %q", want)
	}

	_, ok := dispatchjob.ParseErrorType("NOT_A_REAL_ERROR_TYPE")
	assert.False(t, ok, "an unrecognised error type must not silently coerce to UNKNOWN")

	_, ok = dispatchjob.ParseErrorType("")
	assert.False(t, ok, "empty is not a known error type")
}

// TestCompleteFailure_EmptyErrorTypeLeavesItNil pins the fix for a real
// defect X-06's migration 052 surfaced: the processing handler's two
// cooperative-deferral outcomes (2xx ack=false, HTTP 429) call
// CompleteFailure with the zero-value ErrorType because a deferral isn't a
// real error. Before the fix, CompleteFailure blindly stored that zero
// value, so RecordAttempt persisted error_type as the literal empty string
// instead of NULL — invisible until migration 052's CHECK constraint
// started rejecting it outright (see
// internal/platform/dispatchjob/processing/processing_pg_test.go's
// TestProcess_Deferral429DoesNotSpendBudget, which failed against that
// constraint before this fix). ErrorType must stay nil for the zero value
// and only be set for a real, non-empty error type.
func TestCompleteFailure_EmptyErrorTypeLeavesItNil(t *testing.T) {
	a := dispatchjob.NewAttempt(1)
	a.CompleteFailure("rate limited (429)", "", nil)
	assert.False(t, a.Success)
	assert.Nil(t, a.ErrorType, "a deferral's zero-value error type must persist as NULL, not the empty string")

	a2 := dispatchjob.NewAttempt(1)
	a2.CompleteFailure("boom", dispatchjob.ErrorHTTPError, nil)
	require.NotNil(t, a2.ErrorType)
	assert.Equal(t, dispatchjob.ErrorHTTPError, *a2.ErrorType)
}
