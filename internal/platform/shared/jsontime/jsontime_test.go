package jsontime

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarshalFixedPrecision(t *testing.T) {
	cases := []struct {
		name string
		in   time.Time
		want string
	}{
		{
			name: "exact second pads to .000000",
			in:   time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC),
			want: `"2026-05-26T12:00:00.000000Z"`,
		},
		{
			name: "nanoseconds truncate to microseconds",
			in:   time.Date(2026, 5, 26, 12, 0, 0, 123_456_789, time.UTC),
			want: `"2026-05-26T12:00:00.123456Z"`,
		},
		{
			name: "millisecond precision preserved + padded",
			in:   time.Date(2026, 5, 26, 12, 0, 0, 123_000_000, time.UTC),
			want: `"2026-05-26T12:00:00.123000Z"`,
		},
		{
			name: "non-UTC zone emitted as offset",
			in:   time.Date(2026, 5, 26, 12, 0, 0, 0, time.FixedZone("EST", -5*3600)),
			want: `"2026-05-26T12:00:00.000000-05:00"`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b, err := json.Marshal(New(c.in))
			require.NoError(t, err)
			assert.Equal(t, c.want, string(b))
		})
	}
}

func TestUnmarshalAcceptsAnyRFC3339Precision(t *testing.T) {
	cases := []struct {
		in   string
		want time.Time
	}{
		{`"2026-05-26T12:00:00Z"`, time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)},
		{`"2026-05-26T12:00:00.123Z"`, time.Date(2026, 5, 26, 12, 0, 0, 123_000_000, time.UTC)},
		{`"2026-05-26T12:00:00.123456Z"`, time.Date(2026, 5, 26, 12, 0, 0, 123_456_000, time.UTC)},
		// nanosecond input truncates to microsecond resolution
		{`"2026-05-26T12:00:00.123456789Z"`, time.Date(2026, 5, 26, 12, 0, 0, 123_456_000, time.UTC)},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			var jt Time
			require.NoError(t, jt.UnmarshalJSON([]byte(c.in)))
			assert.True(t, jt.Underlying().Equal(c.want), "got %v want %v", jt.Underlying(), c.want)
		})
	}
}

func TestRoundTrip(t *testing.T) {
	orig := New(time.Date(2026, 5, 26, 12, 34, 56, 789_012_000, time.UTC))
	b, err := json.Marshal(orig)
	require.NoError(t, err)

	var back Time
	require.NoError(t, json.Unmarshal(b, &back))
	assert.Equal(t, orig.Underlying(), back.Underlying())
	assert.Equal(t, orig.String(), back.String())
}

func TestUnmarshalNull(t *testing.T) {
	var jt Time
	require.NoError(t, jt.UnmarshalJSON([]byte("null")))
	assert.True(t, jt.IsZero())
}

func TestRejectsUnquotedAndGarbage(t *testing.T) {
	cases := []string{
		`2026-05-26T12:00:00Z`,
		`""`,
		`"not-a-date"`,
	}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			var jt Time
			assert.Error(t, jt.UnmarshalJSON([]byte(c)))
		})
	}
}

func TestNowTruncatesToMicroseconds(t *testing.T) {
	jt := Now()
	assert.Equal(t, 0, jt.Underlying().Nanosecond()%1000, "Now() must zero the sub-microsecond nanoseconds")
}
