package tsid_test

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

func TestGenerateTyped(t *testing.T) {
	id := tsid.Generate(tsid.Client)
	assert.Len(t, id, 17)
	assert.Equal(t, "clt_", id[:4])
}

func TestGenerateCustomPrefix(t *testing.T) {
	id := tsid.GenerateWithPrefix("ord")
	assert.Len(t, id, 17)
	assert.Equal(t, "ord_", id[:4])
}

func TestGenerateUntyped(t *testing.T) {
	id := tsid.GenerateUntyped()
	assert.Len(t, id, 13)
}

func TestUniquenessSerial(t *testing.T) {
	seen := make(map[string]struct{}, 10000)
	for range 10000 {
		id := tsid.Generate(tsid.Event)
		_, dup := seen[id]
		require.False(t, dup, "duplicate TSID generated")
		seen[id] = struct{}{}
	}
}

func TestUniquenessParallel(t *testing.T) {
	const goroutines = 32
	const perGoroutine = 1000
	results := make([]string, goroutines*perGoroutine)
	var wg sync.WaitGroup
	for g := range goroutines {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := range perGoroutine {
				results[g*perGoroutine+i] = tsid.Generate(tsid.Event)
			}
		}(g)
	}
	wg.Wait()

	seen := make(map[string]struct{}, len(results))
	for _, id := range results {
		_, dup := seen[id]
		require.False(t, dup, "duplicate TSID generated")
		seen[id] = struct{}{}
	}
}

func TestRoundTripRaw(t *testing.T) {
	id := tsid.GenerateUntyped()
	num, ok := tsid.ToLong(id)
	require.True(t, ok)
	back := tsid.FromLong(num)
	assert.Equal(t, id, back)
}

func TestRoundTripTyped(t *testing.T) {
	id := tsid.Generate(tsid.Client)
	num, ok := tsid.ToLong(id)
	require.True(t, ok)
	back := tsid.FromLong(num)
	assert.Equal(t, id[4:], back)
}

func TestSortability(t *testing.T) {
	id1 := tsid.Generate(tsid.Client)
	time.Sleep(2 * time.Millisecond)
	id2 := tsid.Generate(tsid.Client)
	assert.Less(t, id1, id2, "TSIDs should be lexicographically sortable")
}

// TestSortabilityWithinAMillisecond is the test that had to exist. Its
// predecessor sleeps 2ms between two ids and so only exercises ordering
// ACROSS milliseconds — the easy half, decided by the timestamp alone. It
// failed intermittently anyway, and only by luck: under load the generator
// borrows milliseconds ahead of the wall clock, which occasionally landed
// both ids in the same millisecond and exposed what this test attacks head on.
//
// Inside one millisecond the ordering came from a random field that outranked
// the sequence, so it was a coin toss per pair. A tight burst forces thousands
// of ids into each millisecond, plus several sequence exhaustions (4096 per
// ms) and the millisecond-borrowing that follows.
func TestSortabilityWithinAMillisecond(t *testing.T) {
	const n = 50_000 // >> 4096, so sequence wrap and ms borrowing both happen

	ids := make([]string, n)
	for i := range ids {
		ids[i] = tsid.GenerateUntyped()
	}

	for i := 1; i < n; i++ {
		require.Lessf(t, ids[i-1], ids[i],
			"id %d sorts before id %d: %q !< %q — generation order must be sort order",
			i, i-1, ids[i-1], ids[i])
	}
}

// The bit layout IS the sort order, because Crockford Base32 is
// order-preserving. Pin it so a future reorder — the exact defect this
// package carried — fails here rather than as an occasional mystery
// elsewhere.
func TestLayoutPutsSequenceAboveRandom(t *testing.T) {
	// Two ids from the same burst: same millisecond, consecutive sequence.
	a, aok := tsid.ToLong(tsid.GenerateUntyped())
	b, bok := tsid.ToLong(tsid.GenerateUntyped())
	require.True(t, aok && bok)

	msA, msB := uint64(a)>>22, uint64(b)>>22
	require.Equal(t, msA, msB, "a tight pair should share a millisecond; rerun if the clock ticked between them")

	seqA, seqB := (uint64(a)>>10)&0xFFF, (uint64(b)>>10)&0xFFF
	assert.Equal(t, seqA+1, seqB, "bits 21..10 must be the incrementing sequence")
	assert.Less(t, a, b, "and the sequence must decide the order")
}

// TestPrefixCatalogStable enumerates every EntityType and asserts the
// prefix matches the canonical prefix table. This is the
// load-bearing compatibility test: if any prefix drifts, the
// TSIDs the Go code produces don't match what consumers expect.
func TestPrefixCatalogStable(t *testing.T) {
	cases := map[tsid.EntityType]string{
		tsid.Client:                  "clt",
		tsid.Principal:               "prn",
		tsid.Application:             "app",
		tsid.ServiceAccount:          "sac",
		tsid.Role:                    "rol",
		tsid.Permission:              "prm",
		tsid.OAuthClient:             "oac",
		tsid.AuthCode:                "acd",
		tsid.LoginAttempt:            "lat",
		tsid.ClientAuthConfig:        "cac",
		tsid.AppClientConfig:         "apc",
		tsid.IdpRoleMapping:          "irm",
		tsid.CorsOrigin:              "cor",
		tsid.AnchorDomain:            "anc",
		tsid.IdentityProvider:        "idp",
		tsid.EmailDomainMapping:      "edm",
		tsid.ClientAccessGrant:       "gnt",
		tsid.EventType:               "evt",
		tsid.Event:                   "evn",
		tsid.EventRead:               "evr",
		tsid.Connection:              "con",
		tsid.Subscription:            "sub",
		tsid.DispatchPool:            "dpl",
		tsid.DispatchJob:             "djb",
		tsid.DispatchJobRead:         "djr",
		tsid.Schema:                  "sch",
		tsid.AuditLog:                "aud",
		tsid.PlatformConfig:          "pcf",
		tsid.ConfigAccess:            "cfa",
		tsid.PasswordResetToken:      "prt",
		tsid.WebauthnCredential:      "pkc",
		tsid.ScheduledJob:            "sjb",
		tsid.ScheduledJobInstance:    "sji",
		tsid.ScheduledJobInstanceLog: "sjl",
		tsid.ApplicationOpenApiSpec:  "oas",
		tsid.Process:                 "prc",
	}
	for et, want := range cases {
		assert.Equal(t, want, et.Prefix(), "entity %v", et)
	}
}

// TestEncodingFormat verifies the Crockford alphabet (no I/L/O/U).
func TestEncodingFormat(t *testing.T) {
	for range 1000 {
		id := tsid.GenerateUntyped()
		for _, c := range id {
			assert.NotContains(t, "ILOU", string(c), "TSID must not contain ambiguous Crockford chars")
		}
	}
}

// TestKnownValueDecode pins a specific TSID→numeric mapping. If the
// alphabet or bit layout ever drifts, this fails immediately.
func TestKnownValueDecode(t *testing.T) {
	// "0000000000001" should decode to 1 in Crockford Base32.
	v, ok := tsid.ToLong("0000000000001")
	require.True(t, ok)
	assert.Equal(t, int64(1), v)

	// And "0000000000010" should decode to 32 (0x20).
	v, ok = tsid.ToLong("0000000000010")
	require.True(t, ok)
	assert.Equal(t, int64(32), v)
}
