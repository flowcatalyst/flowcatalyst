//go:build !wasip1

package fn

import (
	"context"
	"testing"
)

func TestPrincipalHasPermissionWildcard(t *testing.T) {
	cases := []struct {
		name string
		held []string
		want string
		ok   bool
	}{
		{"exact match", []string{"platform:function:view"}, "platform:function:view", true},
		{"no match, different segment", []string{"platform:function:view"}, "platform:function:manage", false},
		{"trailing wildcard segment", []string{"platform:messaging:*:*"}, "platform:messaging:queue:read", true},
		{"super-admin wildcard", []string{"platform:*:*:*"}, "platform:messaging:queue:read", true},
		{"wildcard requires same segment count (fewer)", []string{"platform:*"}, "platform:messaging:queue:read", false},
		{"wildcard requires same segment count (more)", []string{"platform:*:*:*:*"}, "platform:messaging:queue", false},
		{"middle wildcard segment", []string{"platform:*:queue:read"}, "platform:messaging:queue:read", true},
		{"middle wildcard segment mismatch", []string{"platform:*:queue:read"}, "platform:messaging:topic:read", false},
		{"empty permissions", nil, "platform:function:view", false},
		{"one of several", []string{"a:b", "platform:function:*"}, "platform:function:view", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &Principal{Permissions: tc.held}
			if got := p.HasPermission(tc.want); got != tc.ok {
				t.Errorf("HasPermission(%q) with held=%v = %v, want %v", tc.want, tc.held, got, tc.ok)
			}
		})
	}
}

func TestPrincipalHasPermissionNilSafe(t *testing.T) {
	var p *Principal
	if p.HasPermission("a:b") {
		t.Error("nil Principal.HasPermission: want false")
	}
	if p.HasRole("x") {
		t.Error("nil Principal.HasRole: want false")
	}
	if p.IsAnchor() {
		t.Error("nil Principal.IsAnchor: want false")
	}
	if p.CanAccessClient("c1") {
		t.Error("nil Principal.CanAccessClient: want false")
	}
	if p.CanAccessApplication("a1") {
		t.Error("nil Principal.CanAccessApplication: want false")
	}
}

func TestPrincipalHasRole(t *testing.T) {
	p := &Principal{Roles: []string{"operant:admin", "billing:viewer"}}
	if !p.HasRole("operant:admin") {
		t.Error("HasRole(operant:admin): want true")
	}
	if p.HasRole("operant:*") {
		t.Error("HasRole does exact match only, must not wildcard")
	}
}

func TestPrincipalCanAccessClient(t *testing.T) {
	anchor := &Principal{Tier: "ANCHOR"}
	if !anchor.CanAccessClient("any-client") {
		t.Error("anchor principal must access any client")
	}
	scoped := &Principal{Tier: "CLIENT", Clients: []string{"clt_a", "clt_b"}}
	if !scoped.CanAccessClient("clt_a") {
		t.Error("scoped principal should access clt_a")
	}
	if scoped.CanAccessClient("clt_z") {
		t.Error("scoped principal should not access clt_z")
	}
}

func TestPrincipalCanAccessApplication(t *testing.T) {
	all := &Principal{AllApplications: true}
	if !all.CanAccessApplication("app_1") {
		t.Error("AllApplications principal must access any application")
	}
	scoped := &Principal{Applications: []string{"app_1"}}
	if !scoped.CanAccessApplication("app_1") {
		t.Error("scoped principal should access app_1")
	}
	if scoped.CanAccessApplication("app_2") {
		t.Error("scoped principal should not access app_2")
	}
}

func TestCallerFromDefaultsToAnonymous(t *testing.T) {
	c := CallerFrom(context.Background())
	if c.Kind() != "anonymous" {
		t.Errorf("Kind() = %q, want anonymous", c.Kind())
	}
	if c.Principal() != nil {
		t.Error("Principal() should be nil for a default Caller")
	}
}
