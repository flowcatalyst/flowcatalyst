package serviceaccount_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
)

// Work done as an application (a function: its application and its own
// account) may be signed by the application's accounts and by its own
// account, and by no other (owner's decision D4, 2026-11-22).
func TestSigningProblem(t *testing.T) {
	app := func(id string) *ids.ApplicationID { a := ids.ApplicationID(id); return &a }
	fn := serviceaccount.SigningIdentity{ApplicationID: "app_mine"}
	withOwn := serviceaccount.SigningIdentity{ApplicationID: "app_mine", OwnAccountID: "sac_own"}

	for _, c := range []struct {
		name    string
		id      serviceaccount.SigningIdentity
		signer  *serviceaccount.Signer
		refused string
	}{
		{"an account that does not exist signs nothing", fn, nil, ""},
		{"the application's account", fn, &serviceaccount.Signer{ID: "sac_a", Code: "a", ApplicationID: app("app_mine")}, ""},
		{"the application's account, beside an own one", withOwn, &serviceaccount.Signer{ID: "sac_a", Code: "a", ApplicationID: app("app_mine")}, ""},
		{"its own account, bound to no application", withOwn, &serviceaccount.Signer{ID: "sac_own", Code: "own"}, ""},
		{"its own account, bound to another application", withOwn, &serviceaccount.Signer{ID: "sac_own", Code: "own", ApplicationID: app("app_theirs")}, ""},
		{"another application's account", fn, &serviceaccount.Signer{ID: "sac_t", Code: "theirs", ApplicationID: app("app_theirs")}, "belongs to another application"},
		{"another application's account, beside an own one", withOwn, &serviceaccount.Signer{ID: "sac_t", Code: "theirs", ApplicationID: app("app_theirs")}, "belongs to another application"},
		{"an account of no application", fn, &serviceaccount.Signer{ID: "sac_s", Code: "shared"}, "belongs to no application"},
		{"an account of no application that is not its own", withOwn, &serviceaccount.Signer{ID: "sac_s", Code: "shared"}, "belongs to no application"},
		{"an empty application id is no application", fn, &serviceaccount.Signer{ID: "sac_e", Code: "empty", ApplicationID: app("")}, "belongs to no application"},
		{"an identity with no application has none to match", serviceaccount.SigningIdentity{}, &serviceaccount.Signer{ID: "sac_e", Code: "empty", ApplicationID: app("")}, "belongs to no application"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := c.id.SigningProblem(c.signer)
			if c.refused == "" {
				assert.Empty(t, got)
				return
			}
			assert.Contains(t, got, c.refused)
			assert.Contains(t, got, "service account "+c.signer.Code)
		})
	}
}
