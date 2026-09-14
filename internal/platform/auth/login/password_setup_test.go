package login

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
)

// str is a small helper to get a *string in table literals.

func TestIsPasswordSetupEligible(t *testing.T) {
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
		{
			name: "nil principal",
			p:    nil,
			want: false,
		},
		{
			name: "eligible: internal user, never set a password",
			p:    activeUser(nil),
			want: true,
		},
		{
			name: "not eligible: password already set",
			p: activeUser(func(p *principal.Principal) {
				p.UserIdentity.PasswordHash = new("$argon2id$...")
			}),
			want: false,
		},
		{
			name: "not eligible: inactive principal",
			p: activeUser(func(p *principal.Principal) {
				p.Active = false
			}),
			want: false,
		},
		{
			name: "not eligible: SERVICE principal, not a USER",
			p: activeUser(func(p *principal.Principal) {
				p.Type = principal.TypeService
			}),
			want: false,
		},
		{
			name: "not eligible: no UserIdentity",
			p: activeUser(func(p *principal.Principal) {
				p.UserIdentity = nil
			}),
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
			p: activeUser(func(p *principal.Principal) {
				p.UserIdentity.Provider = new("OIDC")
			}),
			want: false,
		},
		{
			name: "eligible: provider set but not OIDC",
			p: activeUser(func(p *principal.Principal) {
				p.UserIdentity.Provider = new("legacy-import")
			}),
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, isPasswordSetupEligible(tc.p))
		})
	}
}
