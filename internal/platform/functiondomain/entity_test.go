package functiondomain

import "testing"

func TestValidZone(t *testing.T) {
	cases := map[string]bool{
		"acme.example.com": true,
		"example.com":      true,
		"localhost":        true,
		"a.b.c.d":          true,
		"":                 false,
		"Acme.example.com": false, // uppercase
		"acme..com":        false, // empty label
		"-acme.com":        false, // leading hyphen
		"acme-.com":        false, // trailing hyphen
		"acme.com:8080":    false, // port
		"*.acme.com":       false, // wildcard
		"acme.com.":        false, // trailing dot (callers normalize first)
	}
	for zone, want := range cases {
		if got := ValidZone(zone); got != want {
			t.Errorf("ValidZone(%q) = %v, want %v", zone, got, want)
		}
	}
}

func TestCovers(t *testing.T) {
	cases := []struct {
		zone, hostname string
		want           bool
	}{
		{"acme.com", "acme.com", true},
		{"acme.com", "api.acme.com", true},
		{"acme.com", "foo.api.acme.com", true},
		{"acme.com", "notacme.com", false},
		{"acme.com", "otheracme.com", false},
		{"api.acme.com", "acme.com", false}, // narrower zone does not cover its parent
		{"acme.com", "acme.company.com", false},
	}
	for _, c := range cases {
		if got := Covers(c.zone, c.hostname); got != c.want {
			t.Errorf("Covers(%q, %q) = %v, want %v", c.zone, c.hostname, got, c.want)
		}
	}
}

func TestSameOwner(t *testing.T) {
	a, b := "clt_a", "clt_a"
	c := "clt_b"
	if !SameOwner(nil, nil) {
		t.Error("nil, nil should be same owner (platform)")
	}
	if SameOwner(nil, &a) {
		t.Error("nil vs non-nil should differ")
	}
	if !SameOwner(&a, &b) {
		t.Error("equal client ids should be same owner")
	}
	if SameOwner(&a, &c) {
		t.Error("different client ids should differ")
	}
}
