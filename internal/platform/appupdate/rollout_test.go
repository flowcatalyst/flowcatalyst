package appupdate

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeClientID(t *testing.T) {
	t.Parallel()
	id := "3F2504E0-4F89-41D3-9A0C-0305E82C3301"
	assert.Equal(t, strings.ToLower(id), NormalizeClientID(id), "iOS sends upper case")
	assert.Equal(t, strings.ToLower(id), NormalizeClientID(strings.ToLower(id)), "Android sends lower case")
	for _, bad := range []string{
		"", "not-a-uuid", "{3f2504e0-4f89-41d3-9a0c-0305e82c3301}", "3f2504e04f8941d39a0c0305e82c3301",
		"urn:uuid:3f2504e0-4f89-41d3-9a0c-0305e82c3301", strings.Repeat("a", 500),
	} {
		assert.Empty(t, NormalizeClientID(bad), bad)
	}
}

func TestBucketIsStableAndInRange(t *testing.T) {
	t.Parallel()
	for range 1000 {
		c := uuid.NewString()
		b := Bucket("upd_0000000000001", c)
		require.GreaterOrEqual(t, b, 0)
		require.Less(t, b, 100)
		require.Equal(t, b, Bucket("upd_0000000000001", c))
	}
}

// Each update samples different phones, and the share in a rollout is close to
// its percentage.
func TestBucketDistribution(t *testing.T) {
	t.Parallel()
	const n = 20000
	in, sameBoth := 0, 0
	for range n {
		c := uuid.NewString()
		a, b := Bucket("upd_A", c) < 30, Bucket("upd_B", c) < 30
		if a {
			in++
		}
		if a && b {
			sameBoth++
		}
	}
	assert.InDelta(t, 0.30, float64(in)/n, 0.02)
	// Independent per update: about 9% of phones are in both 30% rollouts.
	assert.InDelta(t, 0.09, float64(sameBoth)/n, 0.02)
}

// Raising a percentage keeps every phone that was in; lowering keeps a subset.
func TestInRolloutIsMonotone(t *testing.T) {
	t.Parallel()
	u := Update{ID: "upd_MONOTONE00001"}
	for range 2000 {
		c := uuid.NewString()
		prev := false
		for p := 0; p <= 100; p += 5 {
			u.RolloutPercent = p
			now := InRollout(&u, c)
			if prev {
				require.True(t, now, "a phone in at a lower percentage stays in at %d", p)
			}
			prev = now
		}
	}
}

func TestInRolloutEdges(t *testing.T) {
	t.Parallel()
	c := uuid.NewString()
	assert.True(t, InRollout(&Update{ID: "x", RolloutPercent: 100}, ""), "100% reaches a phone without an id")
	assert.False(t, InRollout(&Update{ID: "x", RolloutPercent: 99}, ""), "a partial rollout needs an id")
	assert.False(t, InRollout(&Update{ID: "x", RolloutPercent: 0}, c))
	assert.True(t, InRollout(&Update{ID: "x", RolloutPercent: 100}, c))
}

func TestSelect(t *testing.T) {
	t.Parallel()
	assert.Nil(t, Select(nil, "c"))

	newest := Update{ID: "upd_NEW", Kind: KindUpdate, RolloutPercent: 100}
	older := Update{ID: "upd_OLD", Kind: KindUpdate, RolloutPercent: 100}
	assert.Equal(t, "upd_NEW", Select([]Update{newest, older}, "").ID)

	rb := Update{ID: "upd_RB", Kind: KindRollbackToEmbedded, RolloutPercent: 100}
	assert.Equal(t, "upd_RB", Select([]Update{rb, older}, "").ID)

	// A partial newest: phones in it get it, the rest the one before.
	partial := Update{ID: "upd_PARTIAL", Kind: KindUpdate, RolloutPercent: 50}
	var inID, outID string
	for inID == "" || outID == "" {
		c := uuid.NewString()
		if InRollout(&partial, c) {
			inID = c
		} else {
			outID = c
		}
	}
	assert.Equal(t, "upd_PARTIAL", Select([]Update{partial, older}, inID).ID)
	assert.Equal(t, "upd_OLD", Select([]Update{partial, older}, outID).ID)
	assert.Nil(t, Select([]Update{partial}, outID), "no earlier update: nothing")
	assert.Equal(t, "upd_OLD", Select([]Update{partial, older}, "").ID, "no id: not in a partial rollout")
}

func TestServedPercent(t *testing.T) {
	t.Parallel()
	rows := []Update{{RolloutPercent: 30}, {RolloutPercent: 100}, {RolloutPercent: 100}}
	assert.Equal(t, 30, ServedPercent(rows, 0))
	assert.Equal(t, 70, ServedPercent(rows, 1))
	assert.Equal(t, 0, ServedPercent(rows, 2))
	full := []Update{{RolloutPercent: 100}, {RolloutPercent: 100}}
	assert.Equal(t, 0, ServedPercent(full, 1))
}

func TestCommitTimes(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 11, 17, 10, 0, 0, 123456789, time.UTC)
	assert.Equal(t, "2026-11-17T10:00:00.123Z", FormatTime(base), "exactly yyyy-MM-dd'T'HH:mm:ss.SSS'Z'")
	assert.Equal(t, "2026-11-17T10:00:00.000Z", FormatTime(time.Date(2026, 11, 17, 12, 0, 0, 0, time.FixedZone("x", 2*3600))))

	assert.Equal(t, CommitTime(base), NextCommitTime(base, time.Time{}))
	prev := CommitTime(base)
	next := NextCommitTime(base, prev)
	assert.True(t, next.After(prev), "a new row in the same millisecond is still later")
	assert.Equal(t, prev.Add(time.Millisecond), next)
	// A clock that went backwards still moves forward.
	assert.Equal(t, prev.Add(time.Millisecond), NextCommitTime(base.Add(-time.Hour), prev))
	assert.Equal(t, CommitTime(base.Add(time.Second)), NextCommitTime(base.Add(time.Second), prev))
}

func TestValidators(t *testing.T) {
	t.Parallel()
	ok := func(f func(string) bool, vs ...string) {
		for _, v := range vs {
			assert.True(t, f(v), v)
		}
	}
	bad := func(f func(string) bool, vs ...string) {
		for _, v := range vs {
			assert.False(t, f(v), v)
		}
	}
	ok(ValidApplicationCode, "acme", "acme-2", "acme_x")
	bad(ValidApplicationCode, "", "Acme", "1acme", "acme/x", strings.Repeat("a", 51), "acme%2e")
	ok(ValidAppKey, "driver-app", "app2", "9")
	bad(ValidAppKey, "", "-x", "Driver", "a_b", strings.Repeat("a", 41), "..")
	ok(ValidChannel, "production", "staging-2", "v1.2", "a_b")
	bad(ValidChannel, "", "Prod", ".x", strings.Repeat("a", 65), "a b")
	ok(ValidRuntimeVersion, "1.0.0", "exposdk:58.0.0", "abc123", "1.0.0+build.5")
	bad(ValidRuntimeVersion, "", " 1.0", "1.0 ", "../x", "1/0", strings.Repeat("1", 129), "1.0\n")
	ok(ValidPlatform, "android", "ios")
	bad(ValidPlatform, "web", "Android", "")
	ok(ValidSHA256, strings.Repeat("a", 64))
	bad(ValidSHA256, strings.Repeat("A", 64), strings.Repeat("a", 63), strings.Repeat("g", 64))

	for in, want := range map[string]string{"png": ".png", ".ttf": ".ttf", "JPG": ".JPG"} {
		got, ok := NormalizeFileExtension(in)
		assert.True(t, ok, in)
		assert.Equal(t, want, got)
	}
	for _, in := range []string{"", ".", "p/ng", "../x", "a.b", strings.Repeat("a", 17)} {
		_, ok := NormalizeFileExtension(in)
		assert.False(t, ok, in)
	}
	assert.Equal(t, "image/png", ContentTypeFor(".PNG"))
	assert.Equal(t, "application/octet-stream", ContentTypeFor(".weird"))
}
