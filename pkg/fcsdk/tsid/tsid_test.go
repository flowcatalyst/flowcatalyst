package tsid_test

import (
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/tsid"
)

func TestGenerateTypedID(t *testing.T) {
	id := tsid.Generate(tsid.Client)
	assert.Len(t, id, 17) // "clt_" + 13 chars
	assert.True(t, strings.HasPrefix(id, "clt_"))
}

func TestGenerateWithCustomPrefix(t *testing.T) {
	id := tsid.GenerateWithPrefix("ord")
	assert.Len(t, id, 17)
	assert.True(t, strings.HasPrefix(id, "ord_"))
}

func TestGenerateUntyped(t *testing.T) {
	id := tsid.GenerateUntyped()
	assert.Len(t, id, 13)
}

func TestUniqueness(t *testing.T) {
	seen := make(map[string]struct{}, 10000)
	for i := 0; i < 10000; i++ {
		id := tsid.Generate(tsid.Client)
		_, dup := seen[id]
		require.False(t, dup, "duplicate TSID at iteration %d: %s", i, id)
		seen[id] = struct{}{}
	}
}

// TestUniquenessUnderCounterOverflow hammers the generator far past 4096 ids
// per millisecond (the 12-bit sequence capacity). The old free-running counter
// wrapped here and collided at p≈1/1024 per wrapped pair — the historical
// TestUniqueness flake. The monotonic (ms, seq) state must instead borrow
// future milliseconds, so duplicates are structurally impossible: this test
// must NEVER flake.
func TestUniquenessUnderCounterOverflow(t *testing.T) {
	const n = 100_000
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		id := tsid.GenerateUntyped()
		_, dup := seen[id]
		require.False(t, dup, "duplicate TSID at iteration %d: %s", i, id)
		seen[id] = struct{}{}
	}
}

// TestUniquenessConcurrent asserts global uniqueness across goroutines — the
// CAS on the shared (ms, seq) state must serialize issuance correctly.
func TestUniquenessConcurrent(t *testing.T) {
	const workers, per = 8, 20_000
	results := make([][]string, workers)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			ids := make([]string, per)
			for i := range ids {
				ids[i] = tsid.GenerateUntyped()
			}
			results[w] = ids
		}(w)
	}
	wg.Wait()
	seen := make(map[string]struct{}, workers*per)
	for _, ids := range results {
		for _, id := range ids {
			_, dup := seen[id]
			require.False(t, dup, "duplicate TSID across goroutines: %s", id)
			seen[id] = struct{}{}
		}
	}
}

func TestPrefixCoverage(t *testing.T) {
	for et := tsid.Client; et <= tsid.Process; et++ {
		p := et.Prefix()
		assert.Len(t, p, 3, "entity %d has bad-length prefix %q", et, p)
		assert.NotEqual(t, "unk", p, "entity %d returned the unknown fallback", et)
	}
}

func TestRoundTripDecode(t *testing.T) {
	raw := tsid.GenerateUntyped()
	n, ok := tsid.DecodeCrockford(raw)
	require.True(t, ok)
	assert.Equal(t, raw, tsid.FromLong(int64(n)))
}

func TestToLongHandlesTypedAndRaw(t *testing.T) {
	typed := tsid.Generate(tsid.Client)
	raw := strings.TrimPrefix(typed, "clt_")

	a, ok := tsid.ToLong(typed)
	require.True(t, ok)
	b, ok := tsid.ToLong(raw)
	require.True(t, ok)
	assert.Equal(t, a, b)
}

func TestDecodeCrockfordRejectsShort(t *testing.T) {
	_, ok := tsid.DecodeCrockford("ABC")
	assert.False(t, ok)
}

func TestDecodeCrockfordRejectsInvalidChars(t *testing.T) {
	_, ok := tsid.DecodeCrockford("IIIIIIIIIIIII")
	assert.False(t, ok)
}
