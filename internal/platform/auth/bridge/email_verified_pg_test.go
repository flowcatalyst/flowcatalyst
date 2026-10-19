//go:build integration

package bridge

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// email_verified policy: off by default (today's behaviour); with the switch
// on, only an EXPLICIT false is refused - absent, true and "true" sign in.
func TestOidcLogin_EmailVerifiedPolicy(t *testing.T) {
	cases := []struct {
		name     string
		verified any // nil = claim absent
		enforce  bool
		wantOK   bool
	}{
		{"default: false is accepted", false, false, true},
		{"enforced: false refused", false, true, false},
		{"enforced: string false refused", "false", true, false},
		{"enforced: absent accepted", nil, true, true},
		{"enforced: true accepted", true, true, true},
		{"enforced: string true accepted", "true", true, true},
	}
	for i, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newOidcTestFixture(t)
			f.endpoint.RejectUnverifiedEmail = c.enforce
			nonce := "nonce-ev-" + string(rune('a'+i))
			claims := map[string]any{"nonce": nonce, "email": "ev" + string(rune('a'+i)) + "@" + f.domain}
			if c.verified != nil {
				claims["email_verified"] = c.verified
			}
			rec := f.login(t, claims, "opaque")
			if c.wantOK {
				require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
				assert.True(t, f.lastSessionCalled)
			} else {
				require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
				assert.Equal(t, "EMAIL_NOT_VERIFIED", envelopeCode(t, rec))
				assert.False(t, f.lastSessionCalled)
			}
		})
	}
}
