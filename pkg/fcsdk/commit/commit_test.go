package commit_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
)

// TestZeroCommittedHasZeroEvent documents the externally-reachable shape
// of a Committed[E]. External callers cannot inject a non-zero event;
// the only thing they can produce by hand is the zero value, which
// returns the zero E from Event().
func TestZeroCommittedHasZeroEvent(t *testing.T) {
	var c commit.Committed[fakeEvent]
	assert.Equal(t, fakeEvent{}, c.Event())
}

// TestSealCannotBePopulatedExternally documents the compile-time seal.
// The body is empty — the evidence is that the snippets below WOULD NOT
// COMPILE if they were uncommented. Uncomment any line to verify:
//
//	_ = commit.Committed[fakeEvent]{event: fakeEvent{id: "fake"}} // unknown field event in struct literal
//	c := commit.Committed[fakeEvent]{}
//	c.event = fakeEvent{id: "fake"}                                // c.event undefined (cannot refer to unexported field)
//
// The compile errors are the seal. Application code outside this
// package cannot place an event inside a Committed[E] by any means
// except calling Save / Delete / SaveAll / Emit — each of which writes
// the event to the database in the same transaction as the aggregate.
func TestSealCannotBePopulatedExternally(t *testing.T) {
	// The evidence is the uncompilable snippets above. Nothing to run.
}

// fakeEvent is just enough to instantiate Committed[E] in tests. The
// production E type satisfies usecase.DomainEvent, but the seal works
// for any E.
type fakeEvent struct{}
