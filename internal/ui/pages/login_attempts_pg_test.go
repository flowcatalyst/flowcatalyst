//go:build integration

package pages

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/loginattempt"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// The login attempts page against Postgres: filters, whole-day bounds, keyset
// paging and the drawer's lookup by the row's own time.
func TestLoginAttemptsPageAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	repo := loginattempt.NewRepository(testpg.Pool(t))
	page := &LoginAttempts{Store: repo, Shell: shellForTest()}
	now := time.Now().UTC().Truncate(time.Microsecond)
	var kept []*loginattempt.LoginAttempt
	for i, out := range []loginattempt.Outcome{loginattempt.OutcomeFailure, loginattempt.OutcomeSuccess, loginattempt.OutcomeFailure} {
		a := loginattempt.New(loginattempt.AttemptUserLogin, out)
		ident, agent := "uipg-user@example.com", "UiPg <agent>"
		a.Identifier, a.UserAgent = &ident, &agent
		a.AttemptedAt = now.Add(-time.Duration(i) * time.Millisecond)
		if out == loginattempt.OutcomeFailure {
			reason := "BAD_PASSWORD"
			a.FailureReason = &reason
		}
		require.NoError(t, repo.Record(ctx, a))
		kept = append(kept, a)
	}
	do := func(q req) string {
		t.Helper()
		rec := serveMounted(t, anchorAdmin(), q, page)
		require.Equal(t, http.StatusOK, rec.Code, "%s: %s", q.target, rec.Body.String())
		return rec.Body.String()
	}
	day := now.Format(time.DateOnly)
	list := do(req{target: "/ui/login-attempts?q=uipg-user@example.com&outcome=FAILURE&from=" + day + "&to=" + day})
	assert.Contains(t, list, keyOf(*kept[0]))
	assert.Contains(t, list, keyOf(*kept[2]))
	assert.NotContains(t, list, keyOf(*kept[1]))
	assert.Contains(t, list, "Bad password")
	assert.NotContains(t, do(req{target: "/ui/login-attempts?q=uipg-user@example.com&to=2001-01-01"}), keyOf(*kept[0]), "the day range bounds the list")

	drawer := do(req{target: "/ui/login-attempts/" + keyOf(*kept[0]), datastar: true})
	assert.Contains(t, drawer, "UiPg &lt;agent&gt;")
	assert.Equal(t, http.StatusNotFound, serveMounted(t, anchorAdmin(), req{target: "/ui/login-attempts/" + keyOf(*kept[0]) + "x"}, page).Code)
	assert.Equal(t, http.StatusForbidden, serveMounted(t, connUser("platform:admin:audit-log:view"), req{target: "/ui/login-attempts"}, page).Code)
}
