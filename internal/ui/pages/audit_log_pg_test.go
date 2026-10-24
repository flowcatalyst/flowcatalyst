//go:build integration

package pages

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// The audit log page against Postgres (the API's reads): facets from the
// distinct values, the filtered list, keyset paging and the drawer.
func TestAuditLogPageAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := audit.NewRepository(pool)
	page := &AuditLogPage{Store: repo, Apps: newAppHarness().store, Clients: client.NewRepository(pool), Shell: shellForTest()}
	now := time.Now().UTC().Truncate(time.Microsecond)
	for i, typ := range []string{"UiPgAlpha", "UiPgBeta", "UiPgAlpha"} {
		require.NoError(t, repo.Insert(ctx, &audit.Log{
			ID: "uipgaud" + string(rune('a'+i)), EntityType: typ, EntityID: "uipg_ent_" + string(rune('a'+i)), Operation: "UiPgDoThing",
			OperationJSON: json.RawMessage(`{"k":"<v>"}`), PerformedAt: now.Add(-time.Duration(i) * time.Second),
		}))
	}
	do := func(q req) string {
		t.Helper()
		rec := serveMounted(t, anchorAdmin(), q, page)
		require.Equal(t, http.StatusOK, rec.Code, "%s: %s", q.target, rec.Body.String())
		return rec.Body.String()
	}
	list := do(req{target: "/ui/audit-log?entityType=UiPgAlpha&operation=UiPgDoThing"})
	assert.Contains(t, list, "uipgauda")
	assert.Contains(t, list, "uipgaudc")
	assert.NotContains(t, list, "uipgaudb")
	assert.Contains(t, list, `value="UiPgBeta"`, "entity types are the distinct values")
	assert.Contains(t, list, "Ui Pg Do Thing")

	drawer := do(req{target: "/ui/audit-log/uipgauda", datastar: true})
	assert.Contains(t, drawer, "Unknown", "the principal name is joined in; none stored here")
	assert.Contains(t, drawer, "&lt;v&gt;")
	assert.Equal(t, http.StatusNotFound, serveMounted(t, anchorAdmin(), req{target: "/ui/audit-log/uipgaud-none"}, page).Code)
	assert.Equal(t, http.StatusForbidden, serveMounted(t, connUser("platform:admin:cors-origin:view"), req{target: "/ui/audit-log"}, page).Code)
}
