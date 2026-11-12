package principal

import (
	"testing"
	"time"
)

func TestSessionValidAt(t *testing.T) {
	stamp := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)
	withStamp := func(s time.Time) *Principal { return &Principal{SessionsValidAfter: &s} }

	cases := []struct {
		name string
		p    *Principal
		iat  time.Time
		want bool
	}{
		{"never invalidated accepts anything", &Principal{}, stamp.Add(-1000 * time.Hour), true},
		{"never invalidated accepts a session with no issued-at", &Principal{}, time.Time{}, true},
		{"issued before the stamp is revoked", withStamp(stamp), stamp.Add(-time.Second), false},
		{"issued in the stamp's second counts", withStamp(stamp), stamp, true},
		{"issued after the stamp counts", withStamp(stamp), stamp.Add(time.Second), true},
		{"a sub-second stamp is read at its second", withStamp(stamp.Add(900 * time.Millisecond)), stamp, true},
		{"a sub-second stamp still revokes the second before", withStamp(stamp.Add(900 * time.Millisecond)), stamp.Add(-time.Second), false},
		{"a sub-second issued-at is read at its second", withStamp(stamp), stamp.Add(-100 * time.Millisecond), false},
		{"no issued-at cannot be shown to postdate the stamp", withStamp(stamp), time.Time{}, false},
	}
	for _, c := range cases {
		if got := c.p.SessionValidAt(c.iat); got != c.want {
			t.Errorf("%s: SessionValidAt = %v, want %v", c.name, got, c.want)
		}
	}
	if got := SessionStamp(stamp.Add(900 * time.Millisecond)); !got.Equal(stamp) {
		t.Errorf("SessionStamp = %v, want %v", got, stamp)
	}
}

func TestDeactivateEndsSessions(t *testing.T) {
	p := NewUser("a@b.example", ScopeClient)
	if p.InvalidateSessions {
		t.Fatal("a new user starts with no pending invalidation")
	}
	p.Deactivate()
	if !p.InvalidateSessions {
		t.Fatal("Deactivate must mark the sessions for invalidation")
	}
}
