package loginattempt

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
)

// rowWithOutcome builds a minimal dbq.IamLoginAttempt row for testing
// rowToLoginAttempt's outcome handling in isolation.
func rowWithOutcome(id, outcome string) dbq.IamLoginAttempt {
	return dbq.IamLoginAttempt{
		ID:          id,
		AttemptType: string(AttemptUserLogin),
		Outcome:     outcome,
		AttemptedAt: time.Now().UTC(),
	}
}

// TestParseOutcome_KnownValuesRoundTrip pins the happy path.
func TestParseOutcome_KnownValuesRoundTrip(t *testing.T) {
	for _, want := range []Outcome{OutcomeSuccess, OutcomeFailure} {
		got, ok := ParseOutcome(string(want))
		assert.True(t, ok, "%s should parse", want)
		assert.Equal(t, want, got)
	}
}

// TestParseOutcome_UnknownValueRejectsLoudly is the X-06 fix: a corrupted or
// unrecognised outcome must never silently become SUCCESS (that undercounts
// lockout failures) — it must report ok=false so the caller decides what to
// do, rather than defaulting.
func TestParseOutcome_UnknownValueRejectsLoudly(t *testing.T) {
	for _, bad := range []string{"", "success", "FAIL", "SUCCES", "UNKNOWN", " SUCCESS"} {
		got, ok := ParseOutcome(bad)
		assert.False(t, ok, "%q must not parse", bad)
		assert.Equal(t, Outcome(""), got)
	}
}

// TestRowToLoginAttempt_UnknownOutcomeFailsSafe pins the read-boundary
// behaviour: a row whose outcome column holds something other than SUCCESS
// or FAILURE must display as FAILURE, never SUCCESS — an unreadable row
// must never make a login look more successful than it was. This is a
// display/audit path (FindPage, FindRecentByIdentifier), not the backoff
// counting path (which runs raw `outcome = 'FAILURE'` SQL and is unaffected
// by this Go-level parse), so it degrades to FAILURE rather than failing
// the whole read.
func TestRowToLoginAttempt_UnknownOutcomeFailsSafe(t *testing.T) {
	got := rowToLoginAttempt(rowWithOutcome("sa_row_corrupt01", "GARBAGE"))
	assert.Equal(t, OutcomeFailure, got.Outcome, "an unrecognised outcome must read as FAILURE, never SUCCESS")
}

// TestRowToLoginAttempt_KnownOutcomesPassThrough pins the happy path for the
// same helper.
func TestRowToLoginAttempt_KnownOutcomesPassThrough(t *testing.T) {
	assert.Equal(t, OutcomeSuccess, rowToLoginAttempt(rowWithOutcome("id1", "SUCCESS")).Outcome)
	assert.Equal(t, OutcomeFailure, rowToLoginAttempt(rowWithOutcome("id2", "FAILURE")).Outcome)
}
