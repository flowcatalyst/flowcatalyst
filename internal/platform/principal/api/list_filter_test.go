package api

import (
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
)

// mkPrincipal builds a minimal USER principal for the list-filter helpers.
func mkPrincipal(name, email string, clientID *string, active bool, roles []string, created time.Time) *principal.Principal {
	ras := make([]serviceaccount.RoleAssignment, 0, len(roles))
	for _, r := range roles {
		ras = append(ras, serviceaccount.RoleAssignment{Role: r})
	}
	return &principal.Principal{
		Name:         name,
		Type:         principal.TypeUser,
		Active:       active,
		ClientID:     clientID,
		UserIdentity: principal.NewUserIdentity(email),
		Roles:        ras,
		CreatedAt:    created,
	}
}

func TestPrincipalMatchesQuery(t *testing.T) {
	// Contract: the caller (handler) lowercases the needle; the helper lowercases
	// only the haystack. Tests pass already-lowercased needles, as the handler does.
	p := mkPrincipal("Alice Smith", "alice@acme.io", nil, true, nil, time.Now())
	cases := []struct {
		q    string
		want bool
	}{
		{"alice", true},   // name match against mixed-case "Alice"
		{"smith", true},   // surname match against mixed-case "Smith"
		{"acme.io", true}, // email
		{"@acme", true},   // partial email
		{"bob", false},
	}
	for _, c := range cases {
		if got := principalMatchesQuery(p, c.q); got != c.want {
			t.Errorf("query %q: got %v want %v", c.q, got, c.want)
		}
	}
}

func TestPrincipalMatchesClient(t *testing.T) {
	home := strptr("clt_home")
	p := mkPrincipal("U", "u@x.io", home, true, nil, time.Now())
	p.AssignedClients = []string{"clt_granted"}

	if !principalMatchesClient(p, "clt_home") {
		t.Error("should match home client")
	}
	if !principalMatchesClient(p, "clt_granted") {
		t.Error("should match granted client")
	}
	if principalMatchesClient(p, "clt_other") {
		t.Error("should not match unrelated client")
	}
}

func TestPrincipalHasAnyRole(t *testing.T) {
	p := mkPrincipal("U", "u@x.io", nil, true, []string{"app:admin", "app:viewer"}, time.Now())
	if !principalHasAnyRole(p, []string{"app:viewer"}) {
		t.Error("should match a held role")
	}
	if !principalHasAnyRole(p, []string{"app:nope", "app:admin"}) {
		t.Error("should match when at least one role is held (OR semantics)")
	}
	if principalHasAnyRole(p, []string{"app:nope"}) {
		t.Error("should not match an unheld role")
	}
}

func TestSplitCSV(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", []string{}},
		{"a", []string{"a"}},
		{"a,b,c", []string{"a", "b", "c"}},
		{" a , ,b ", []string{"a", "b"}}, // trims and drops empties
	}
	for _, c := range cases {
		got := splitCSV(c.in)
		if len(got) != len(c.want) {
			t.Errorf("splitCSV(%q) = %v, want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("splitCSV(%q)[%d] = %q, want %q", c.in, i, got[i], c.want[i])
			}
		}
	}
}

func TestSortPrincipals(t *testing.T) {
	t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)
	t2 := t0.Add(2 * time.Hour)

	newSet := func() []*principal.Principal {
		return []*principal.Principal{
			mkPrincipal("Charlie", "c@x.io", nil, true, nil, t1),
			mkPrincipal("alice", "a@x.io", nil, true, nil, t2),
			mkPrincipal("Bob", "b@x.io", nil, true, nil, t0),
		}
	}

	// name asc is case-insensitive: alice, Bob, Charlie
	ps := newSet()
	sortPrincipals(ps, "name", "asc")
	if ps[0].Name != "alice" || ps[1].Name != "Bob" || ps[2].Name != "Charlie" {
		t.Errorf("name asc order wrong: %s, %s, %s", ps[0].Name, ps[1].Name, ps[2].Name)
	}

	// name desc reverses
	ps = newSet()
	sortPrincipals(ps, "name", "desc")
	if ps[0].Name != "Charlie" || ps[2].Name != "alice" {
		t.Errorf("name desc order wrong: %s ... %s", ps[0].Name, ps[2].Name)
	}

	// createdAt asc (also the empty/default key)
	ps = newSet()
	sortPrincipals(ps, "", "asc")
	if !ps[0].CreatedAt.Equal(t0) || !ps[2].CreatedAt.Equal(t2) {
		t.Errorf("createdAt asc order wrong: %v ... %v", ps[0].CreatedAt, ps[2].CreatedAt)
	}
}

func TestPaginate(t *testing.T) {
	mk := func(n int) []*principal.Principal {
		out := make([]*principal.Principal, n)
		for i := range out {
			out[i] = mkPrincipal("u", "u@x.io", nil, true, nil, time.Now())
		}
		return out
	}

	if got := paginate(mk(10), 0, 0); len(got) != 10 {
		t.Errorf("pageSize<=0 should return all, got %d", len(got))
	}
	if got := paginate(mk(10), 0, 3); len(got) != 3 {
		t.Errorf("page 0 size 3 should return 3, got %d", len(got))
	}
	if got := paginate(mk(10), 3, 3); len(got) != 1 {
		t.Errorf("page 3 size 3 of 10 should return 1 (the remainder), got %d", len(got))
	}
	if got := paginate(mk(10), 99, 3); len(got) != 0 {
		t.Errorf("page past the end should return empty, got %d", len(got))
	}
	if got := paginate(mk(10), -1, 3); len(got) != 3 {
		t.Errorf("negative page should clamp to 0, got %d", len(got))
	}
}
