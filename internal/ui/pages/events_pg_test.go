//go:build integration

package pages

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/event"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// The events page against Postgres (the API's reads): facets derived from
// the distinct types, the filtered list, keyset paging and the drawer.
func TestEventsPageAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	page := &Events{Events: event.NewRepository(pool), Clients: client.NewRepository(pool), Shell: shellForTest()}
	for i, typ := range []string{"uipgev:sales:order:created", "uipgev:sales:order:paid", "uipgev:crm:person:updated"} {
		_, err := pool.Exec(ctx, `INSERT INTO msg_events_read (id, type, source, subject, time, data, application, subdomain, aggregate, created_at)
			VALUES ($1, $2::text, 'svc', 'order/1', NOW(), '{"k":"<v>"}', split_part($2::text, ':', 1), split_part($2::text, ':', 2), split_part($2::text, ':', 3), NOW() - make_interval(secs => $3))`,
			"uipgev"+string(rune('a'+i)), typ, i)
		require.NoError(t, err)
	}
	do := func(q req) string {
		t.Helper()
		rec := serveMounted(t, anchorAdmin(), q, page)
		require.Equal(t, http.StatusOK, rec.Code, "%s: %s", q.target, rec.Body.String())
		return rec.Body.String()
	}
	list := do(req{target: "/ui/events?applications=uipgev&subdomains=sales"})
	assert.Contains(t, list, "uipgeva")
	assert.Contains(t, list, "uipgevb")
	assert.NotContains(t, list, "uipgevc")
	assert.Contains(t, list, `value="uipgev:sales:order:paid"`)
	assert.NotContains(t, list, `value="uipgev:crm:person:updated"`, "types narrowed by the chosen subdomain")

	paged := do(req{target: "/ui/events?applications=uipgev&size=50"})
	assert.Contains(t, paged, "Showing the 3 most recent events")

	drawer := do(req{target: "/ui/events/uipgeva", datastar: true})
	assert.Contains(t, drawer, "uipgev:sales:order:created")
	assert.Contains(t, drawer, "&#34;k&#34;: &#34;&lt;v&gt;&#34;")
}
