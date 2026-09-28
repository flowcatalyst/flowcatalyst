package function

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidName(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"single lowercase letter", "a", true},
		{"typical name", "order-created", true},
		{"digits and hyphens", "a1-b2-3", true},
		{"max length (63 chars)", "a" + repeat("b", 62), true},
		{"empty", "", false},
		{"too long (64 chars)", "a" + repeat("b", 63), false},
		{"uppercase rejected", "Order-Created", false},
		{"starts with digit", "1abc", false},
		{"starts with hyphen", "-abc", false},
		{"underscore rejected", "order_created", false},
		{"space rejected", "order created", false},
		{"dot rejected (single-label name, not the address)", "order.created", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, ValidName(tc.in), "ValidName(%q)", tc.in)
		})
	}
}

func TestBuildAddress(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "orders.order-created", BuildAddress("orders", "order-created"))
}

func TestDefaultLimits(t *testing.T) {
	t.Parallel()
	l := DefaultLimits()
	assert.Equal(t, DefaultMemoryMB, l.MemoryMB)
	assert.Equal(t, DefaultMaxConcurrency, l.MaxConcurrency)
	assert.Equal(t, DefaultTimeoutMs, l.TimeoutMs)
	assert.Equal(t, DefaultMaxBodyBytes, l.MaxBodyBytes)
}

func TestNew(t *testing.T) {
	t.Parallel()
	f := New("app_123", "orders", "order-created")
	assert.NotEmpty(t, f.ID)
	assert.Equal(t, "app_123", f.ApplicationID)
	assert.Equal(t, "orders", f.ApplicationCode)
	assert.Equal(t, "order-created", f.Name)
	assert.Equal(t, "orders.order-created", f.Address)
	assert.False(t, f.Warm)
	assert.Equal(t, DefaultLimits(), f.Limits)
	assert.Equal(t, f.ID, f.IDStr())
}

func TestParseVersionStatus(t *testing.T) {
	t.Parallel()
	for _, s := range []VersionStatus{VersionPublished, VersionReady, VersionFailed, VersionRetired} {
		got, ok := ParseVersionStatus(string(s))
		assert.True(t, ok)
		assert.Equal(t, s, got)
	}
	_, ok := ParseVersionStatus("NOT_A_REAL_STATUS")
	assert.False(t, ok)
}

func TestParseSettingKind(t *testing.T) {
	t.Parallel()
	for _, k := range []SettingKind{SettingConfig, SettingSecret, SettingDB} {
		got, ok := ParseSettingKind(string(k))
		assert.True(t, ok)
		assert.Equal(t, k, got)
	}
	_, ok := ParseSettingKind("NOT_A_REAL_KIND")
	assert.False(t, ok)
}

func repeat(s string, n int) string {
	out := make([]byte, 0, n*len(s))
	for range n {
		out = append(out, s...)
	}
	return string(out)
}
