package resetapproval

import (
	"errors"
	"testing"
	"time"
)

func TestParseStatus(t *testing.T) {
	for _, s := range []string{"PENDING", "APPROVED", "DENIED", "EXPIRED"} {
		if got, ok := ParseStatus(s); !ok || string(got) != s {
			t.Errorf("%s: got %q, %v", s, got, ok)
		}
	}
	for _, s := range []string{"", "pending", "DONE"} {
		if _, ok := ParseStatus(s); ok {
			t.Errorf("%q accepted", s)
		}
	}
}

func TestHydrateEnforcesTheStateInvariants(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name    string
		status  string
		by      *string
		at      *time.Time
		corrupt bool
	}{
		{"pending, undecided", "PENDING", nil, nil, false},
		{"approved with decider", "APPROVED", new("prn_admin"), &now, false},
		{"denied with decider", "DENIED", new("prn_admin"), &now, false},
		{"approved without decider", "APPROVED", nil, &now, true},
		{"approved without time", "APPROVED", new("prn_admin"), nil, true},
		{"approved with empty decider", "APPROVED", new(""), &now, true},
		{"denied with nothing", "DENIED", nil, nil, true},
		{"pending but decided", "PENDING", new("prn_admin"), &now, true},
		{"unknown status", "MAYBE", nil, nil, true},
		{"expired is unconstrained", "EXPIRED", nil, nil, false},
	}
	for _, c := range cases {
		r := Request{ID: "rar_1", DecidedBy: c.by, DecidedAt: c.at}
		err := r.hydrate(c.status)
		if (err != nil) != c.corrupt {
			t.Errorf("%s: err = %v, want corrupt=%v", c.name, err, c.corrupt)
		}
		if err != nil && !errors.Is(err, ErrCorruptRow) {
			t.Errorf("%s: error does not wrap ErrCorruptRow: %v", c.name, err)
		}
	}
}
