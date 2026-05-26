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

// TestPrefixesMatchRust enumerates every EntityType and asserts the
// prefix matches the Rust EntityType::prefix() string. This is the
// load-bearing cross-language parity test: if any prefix drifts, the
// TSIDs the Go code produces don't match what consumers expect.
func TestPrefixesMatchRust(t *testing.T) {
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
