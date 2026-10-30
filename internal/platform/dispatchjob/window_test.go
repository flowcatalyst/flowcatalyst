package dispatchjob

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/go-sdk/usecase"
)

var windowNow = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

func at(h int) *time.Time { t := windowNow.Add(time.Duration(h) * time.Hour); return &t }

func TestEnforceWindow_DefaultsToTheLast24Hours(t *testing.T) {
	p, w, err := EnforceWindow(FilterParams{}, windowNow, 0)
	require.NoError(t, err)
	assert.Equal(t, Window{From: windowNow.Add(-24 * time.Hour), To: windowNow, Defaulted: true}, w)
	require.NotNil(t, p.Since)
	assert.Equal(t, w.From, *p.Since)
	assert.Nil(t, p.Until, "no upper bound in SQL: the reported 'to' is now")
}

func TestEnforceWindow_HonoursWhatTheCallerGave(t *testing.T) {
	// since only: since .. now, nothing defaulted.
	p, w, err := EnforceWindow(FilterParams{Since: at(-72)}, windowNow, 0)
	require.NoError(t, err)
	assert.Equal(t, Window{From: *at(-72), To: windowNow}, w)
	assert.Nil(t, p.Until)

	// both.
	p, w, err = EnforceWindow(FilterParams{Since: at(-48), Until: at(-24)}, windowNow, 0)
	require.NoError(t, err)
	assert.Equal(t, Window{From: *at(-48), To: *at(-24)}, w)
	assert.Equal(t, *at(-24), *p.Until)

	// until only: the 24 hours before it, and that is a default.
	p, w, err = EnforceWindow(FilterParams{Until: at(-100)}, windowNow, 0)
	require.NoError(t, err)
	assert.Equal(t, Window{From: at(-100).Add(-24 * time.Hour), To: *at(-100), Defaulted: true}, w)
	assert.Equal(t, w.From, *p.Since)
}

func TestEnforceWindow_Limits(t *testing.T) {
	day := func(n int) *time.Time { t := windowNow.AddDate(0, 0, -n); return &t }

	// Exactly the maximum is fine; one hour more is not.
	_, _, err := EnforceWindow(FilterParams{Since: day(31)}, windowNow, 31)
	require.NoError(t, err)
	_, _, err = EnforceWindow(FilterParams{Since: new(day(31).Add(-time.Hour))}, windowNow, 31)
	ue := usecase.AsError(err)
	require.NotNil(t, ue)
	assert.Equal(t, WindowTooLargeCode, ue.Code)
	assert.Equal(t, usecase.KindValidation, ue.Kind)

	// The default maximum is 31 days; a configured one applies.
	_, _, err = EnforceWindow(FilterParams{Since: day(32)}, windowNow, 0)
	assert.Equal(t, WindowTooLargeCode, usecase.AsError(err).Code)
	_, _, err = EnforceWindow(FilterParams{Since: day(10)}, windowNow, 7)
	assert.Equal(t, WindowTooLargeCode, usecase.AsError(err).Code)
	_, _, err = EnforceWindow(FilterParams{Since: day(10)}, windowNow, 90)
	require.NoError(t, err)

	// A window ending in the past is measured from its own bounds, not from now.
	_, _, err = EnforceWindow(FilterParams{Since: day(200), Until: day(190)}, windowNow, 31)
	require.NoError(t, err)

	// until before since.
	_, _, err = EnforceWindow(FilterParams{Since: at(-1), Until: at(-2)}, windowNow, 31)
	assert.Equal(t, "INVALID_WINDOW", usecase.AsError(err).Code)
}

func TestClassifyForRequeue(t *testing.T) {
	want := map[common.DispatchStatus]Verdict{
		common.DispatchPending: VerdictRequeue, common.DispatchFailed: VerdictRequeue, "ERROR": VerdictRequeue,
		common.DispatchCancelled: VerdictRequeue, common.DispatchExpired: VerdictRequeue,
		common.DispatchQueued: VerdictInFlight, common.DispatchProcessing: VerdictInFlight, "IN_PROGRESS": VerdictInFlight,
		common.DispatchCompleted: VerdictCompleted,
	}
	for s, v := range want {
		assert.Equal(t, v, ClassifyForRequeue(s), string(s))
	}
	// The SQL guard (TRequeue) and the classifier agree on what a plain requeue may reset.
	for s, v := range want {
		inTable := false
		for _, f := range TRequeue.From {
			inTable = inTable || f == s
		}
		assert.Equal(t, v == VerdictRequeue, inTable, "TRequeue.From for %s", s)
	}
}

func TestRedriveConfirmed(t *testing.T) {
	assert.True(t, RedriveConfirmed(true, "REDRIVE"))
	assert.False(t, RedriveConfirmed(true, ""), "the flag alone is not enough")
	assert.False(t, RedriveConfirmed(false, "REDRIVE"), "the word alone is not enough")
	assert.False(t, RedriveConfirmed(true, "redrive"), "the word is exact")
}
