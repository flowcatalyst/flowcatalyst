//go:build integration

package ingestion_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Fair share: the polled ingestions spread evenly over the live runtimes. These
// tests use a time base and runtime ids of their own: the runtime table is
// global, and other tests' runtimes must not fall into this lease window.
var tFair0 = time.Date(2031, 6, 1, 12, 0, 0, 0, time.UTC)

// day gives a test a day of its own, so no other test's runtime is live in its window.
func day(n int) time.Time { return tFair0.AddDate(0, 0, n) } // kept distinct from env.t0 days (2030)

func (e *env) polledAll(t *testing.T, n int) []string {
	t.Helper()
	ids := make([]string, 0, n)
	for i := range n {
		ids = append(ids, e.live(t, fmt.Sprintf("fair-%d", i), e.sftp(t)).ID)
	}
	return ids
}

func TestFairShare_AloneTakesEverything(t *testing.T) {
	e := newEnv(t)
	tf := day(1)
	ids := e.polledAll(t, 5)
	got := given(e.poll(t, "fair-solo", tf))
	assert.ElementsMatch(t, ids, got)
}

func TestFairShare_SecondRuntimeTakesHalfAfterTheFirstReleases(t *testing.T) {
	e := newEnv(t)
	tf := day(2)
	ids := e.polledAll(t, 6)

	// a is alone: holds all six.
	assert.Len(t, given(e.poll(t, "fair-a", tf)), 6)

	// b arrives. It cannot take a lease that a still holds, so it gets nothing yet.
	now := tf.Add(time.Minute)
	assert.Empty(t, given(e.poll(t, "fair-b", now)), "a lease is never handed over directly")

	// a's next poll: two runtimes are live, share is 3: it keeps three and releases three.
	a := given(e.poll(t, "fair-a", now.Add(time.Second)))
	assert.Len(t, a, 3)

	// The three it released are still leased until they expire (three minutes after
	// a's first poll), so b gets nothing yet.
	assert.Empty(t, given(e.poll(t, "fair-b", now.Add(2*time.Second))))

	// Once they have expired, both runtimes poll again: a renews its three, b claims the three.
	later := tf.Add(lease + 10*time.Second)
	a = given(e.poll(t, "fair-a", later))
	b := given(e.poll(t, "fair-b", later.Add(2*time.Second)))
	assert.Len(t, a, 3)
	assert.Len(t, b, 3)
	assert.ElementsMatch(t, ids, append(append([]string{}, a...), b...), "together they hold every ingestion, none twice")
	assert.Empty(t, intersect(a, b))
}

func TestFairShare_ConvergesForThreeRuntimes(t *testing.T) {
	e := newEnv(t)
	tf := day(3)
	ids := e.polledAll(t, 7) // share ceil(7/3) = 3
	runtimes := []string{"fair-x", "fair-y", "fair-z"}
	held := map[string][]string{}
	now := tf
	for round := range 8 {
		for _, r := range runtimes {
			held[r] = given(e.poll(t, r, now))
			now = now.Add(20 * time.Second)
		}
		now = now.Add(time.Minute)
		_ = round
	}
	var all []string
	for _, r := range runtimes {
		assert.LessOrEqual(t, len(held[r]), 3, "%s is within its share", r)
		all = append(all, held[r]...)
	}
	assert.ElementsMatch(t, ids, all, "every ingestion is held by exactly one runtime")
}

func TestFairShare_ADeadRuntimeLosesItsShareToTheSurvivor(t *testing.T) {
	e := newEnv(t)
	tf := day(4)
	e.polledAll(t, 4)
	now := tf
	e.poll(t, "fair-p", now)
	e.poll(t, "fair-q", now.Add(time.Second))
	for i := range 4 { // converge
		now = now.Add(lease + time.Minute)
		e.poll(t, "fair-p", now)
		e.poll(t, "fair-q", now.Add(time.Second))
		_ = i
	}
	// q goes silent: after the lease window p is the only live runtime and takes all four.
	now = now.Add(2 * lease)
	e.poll(t, "fair-p", now) // p sees q gone, share 4, claims what has expired
	now = now.Add(lease + time.Second)
	assert.Len(t, given(e.poll(t, "fair-p", now)), 4)
}

func TestFairShare_SharedKindsAreNotShared(t *testing.T) {
	e := newEnv(t)
	tf := day(5)
	httpDef := parse(t, definition(sourceCases[0].source, sourceCases[0].split))
	id := e.live(t, "fair-http", httpDef).ID
	for _, r := range []string{"fair-h1", "fair-h2", "fair-h3"} {
		assert.Contains(t, given(e.poll(t, r, tf)), id, "an HTTP source runs on every runtime")
	}
}

func intersect(a, b []string) []string {
	in := map[string]bool{}
	for _, x := range a {
		in[x] = true
	}
	var out []string
	for _, y := range b {
		if in[y] {
			out = append(out, y)
		}
	}
	return out
}

var _ = require.NoError
