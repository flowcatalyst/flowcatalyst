//go:build integration

package pages

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/openapispecs"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/platformsink"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// The dashboard against Postgres, in a database of its own (the OpenAPI sync
// writes the platform application's spec, which the shared database's other
// tests must not see): the counts are the tables' own, the page shows them,
// and the OpenAPI sync publishes, finds no change, then archives.
func TestDashboardAgainstPostgres(t *testing.T) {
	pool := testpg.ScratchDB(t, "uidashboard")
	ctx := testpg.AnchorCtx()
	uow := usecasepgx.New(pool, platformsink.New())

	// Seed rows so the exact counts are not all zero.
	_, err := pool.Exec(ctx, `INSERT INTO app_applications (id, code, name) VALUES ('app_uidash0000001', 'platform', 'Platform') ON CONFLICT (code) DO NOTHING`)
	require.NoError(t, err)

	count := func(query string) uint64 {
		t.Helper()
		var n uint64
		require.NoError(t, pool.QueryRow(ctx, query).Scan(&n))
		return n
	}
	counts := DashboardCounts{Pool: pool}
	got, err := counts.Stats(ctx)
	require.NoError(t, err)
	assert.Equal(t, count(`SELECT COUNT(*) FROM tnt_clients`), got.TotalClients)
	assert.Equal(t, count(`SELECT COUNT(*) FROM iam_principals WHERE type = 'USER' AND active = TRUE`), got.ActiveUsers)
	assert.Equal(t, count(`SELECT COUNT(*) FROM iam_roles`), got.RolesDefined)

	version := "1.0.0"
	ops := OpenAPISyncOps{
		Apps: application.NewRepository(pool), Specs: openapispecs.NewRepository(pool), UoW: uow,
		Spec: func() (json.RawMessage, error) {
			return json.RawMessage(`{"openapi":"3.1.0","info":{"title":"p","version":"` + version + `"},"paths":{}}`), nil
		},
	}
	h := newDashHarness()
	h.page.Stats = counts
	h.page.OpenAPI = ops
	rec := serveMounted(t, anchorAdmin(), req{target: "/ui/dashboard"}, h.page)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	mustContain(t, rec.Body.String(), ">"+strconv.FormatUint(got.RolesDefined, 10)+"<", "Message Plane (approx.)")

	post := func() string {
		t.Helper()
		_, body := h.do(t, anchorAdmin(), dashPost("/ui/dashboard/sync/openapi"))
		return body
	}
	// The first sync publishes; the same document again changes nothing; a new
	// version archives the one before it.
	mustContain(t, post(), "Published initial version v1.0.0")
	mustContain(t, post(), "No changes (v1.0.0 unchanged)")
	version = "1.1.0"
	mustContain(t, post(), "v1.0.0 → v1.1.0")

	// Without a platform application the sync fails visibly, not silently.
	_, err = pool.Exec(context.Background(), `DELETE FROM app_application_openapi_specs`)
	require.NoError(t, err)
	_, err = pool.Exec(context.Background(), `DELETE FROM app_applications WHERE code = 'platform'`)
	require.NoError(t, err)
	mustContain(t, post(), "fc-flash-error", "Something went wrong.")
}
