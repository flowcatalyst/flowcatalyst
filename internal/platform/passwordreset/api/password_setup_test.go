package api

import (
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/passwordreset"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
)

func TestPasswordSetupEligible(t *testing.T) {
	activeUser := func(mutate func(*principal.Principal)) *principal.Principal {
		p := &principal.Principal{
			Type:   principal.TypeUser,
			Active: true,
			UserIdentity: &principal.UserIdentity{
				Email: "new.hire@example.com",
			},
		}
		if mutate != nil {
			mutate(p)
		}
		return p
	}

	tests := []struct {
		name string
		p    *principal.Principal
		want bool
	}{
		{name: "nil principal", p: nil, want: false},
		{name: "eligible: internal user, never set a password", p: activeUser(nil), want: true},
		{
			name: "not eligible: password already set",
			p: activeUser(func(p *principal.Principal) {
				p.UserIdentity.PasswordHash = new("$argon2id$...")
			}),
			want: false,
		},
		{
			name: "not eligible: inactive principal",
			p:    activeUser(func(p *principal.Principal) { p.Active = false }),
			want: false,
		},
		{
			name: "not eligible: SERVICE principal, not a USER",
			p:    activeUser(func(p *principal.Principal) { p.Type = principal.TypeService }),
			want: false,
		},
		{
			name: "not eligible: no UserIdentity",
			p:    activeUser(func(p *principal.Principal) { p.UserIdentity = nil }),
			want: false,
		},
		{
			name: "not eligible: linked external identity",
			p: activeUser(func(p *principal.Principal) {
				p.ExternalIdentity = &principal.ExternalIdentity{ProviderID: "idp1", ExternalID: "ext1"}
			}),
			want: false,
		},
		{
			name: "not eligible: provisioned via OIDC",
			p:    activeUser(func(p *principal.Principal) { p.UserIdentity.Provider = new("OIDC") }),
			want: false,
		},
		{
			name: "eligible: provider set but not OIDC",
			p:    activeUser(func(p *principal.Principal) { p.UserIdentity.Provider = new("legacy-import") }),
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := passwordSetupEligible(tc.p); got != tc.want {
				t.Errorf("passwordSetupEligible() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSafeRelativeReturnURL(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"/dashboard", "/dashboard"},
		{"/a/b?c=d#frag", "/a/b?c=d#frag"},
		{"//evil.example.com", ""},
		{`/\evil.example.com`, ""},
		{"https://evil.example.com", ""},
		{"evil.example.com", ""},
		{"javascript:alert(1)", ""},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			if got := safeRelativeReturnURL(tc.in); got != tc.want {
				t.Errorf("safeRelativeReturnURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestShouldAttemptSessionMint(t *testing.T) {
	tests := []struct {
		name          string
		purpose       passwordreset.Purpose
		status        string
		sessionsWired bool
		want          bool
	}{
		{"invite + ok + wired -> mint", passwordreset.PurposeInvite, "ok", true, true},
		{"reset + ok + wired -> no (reset purpose excluded)", passwordreset.PurposeReset, "ok", true, false},
		{"invite + enrollment_required + wired -> no", passwordreset.PurposeInvite, "enrollment_required", true, false},
		{"invite + ok + not wired -> no", passwordreset.PurposeInvite, "ok", false, false},
		{"reset + enrollment_required + not wired -> no", passwordreset.PurposeReset, "enrollment_required", false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldAttemptSessionMint(tc.purpose, tc.status, tc.sessionsWired); got != tc.want {
				t.Errorf("shouldAttemptSessionMint(%q, %q, %v) = %v, want %v",
					tc.purpose, tc.status, tc.sessionsWired, got, tc.want)
			}
		})
	}
}
