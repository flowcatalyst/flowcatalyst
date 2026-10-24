//go:build integration

package pages

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/artifact"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

// uitPG is the world of the ingestion page tests against Postgres: an
// INTEGRATION application, two clients, a dispatch pool, and the real
// repository, resolver and set archive.
type uitPG struct {
	t        *testing.T
	uow      *usecasepgx.UnitOfWork
	repo     *ingestion.Repository
	resolver ingestion.Resolver
	archive  artifact.Store
	audit    *audit.Repository
	app      *application.Application
	client   *client.Client
	other    *client.Client
	poolCode string
}

func uitUnique() string { return strings.ToLower(tsid.GenerateUntyped()) }

func newUitPG(t *testing.T) *uitPG {
	t.Helper()
	ctx := context.Background()
	pool := testpg.Pool(t)
	e := &uitPG{t: t, uow: testpg.NewUoW(t), repo: ingestion.NewRepository(pool), audit: audit.NewRepository(pool)}
	e.app = application.NewIntegration("uiing-"+uitUnique(), "UI ingestion")
	e.client = client.New("Acme", "acme-"+uitUnique())
	e.other = client.New("Beta", "beta-"+uitUnique())
	e.poolCode = "pool-" + uitUnique()
	apps, clients := application.NewRepository(pool), client.NewRepository(pool)
	tx, err := e.uow.Pool().Begin(ctx)
	require.NoError(t, err)
	btx := usecasepgx.WrapTxForBootstrap(tx)
	require.NoError(t, apps.Persist(ctx, e.app, btx))
	require.NoError(t, clients.Persist(ctx, e.client, btx))
	require.NoError(t, clients.Persist(ctx, e.other, btx))
	require.NoError(t, dispatchpool.NewRepository(pool).Persist(ctx, dispatchpool.New(e.poolCode, e.poolCode), btx))
	require.NoError(t, tx.Commit(ctx))
	e.archive, err = artifact.NewFileStore(t.TempDir())
	require.NoError(t, err)
	e.resolver = ingestion.NewResolver(apps, clients, dispatchpool.NewRepository(pool), connection.NewRepository(pool), serviceaccount.NewRepository(pool), nil)
	return e
}

// admin may do everything to ingestions, reaching every client and application.
func (e *uitPG) admin() *auth.AuthContext {
	return &auth.AuthContext{
		PrincipalID: "prn_uiingtester01", Email: "t@x.y", Scope: auth.ScopeAnchor, Roles: []string{"r"}, AllApplications: true,
		Permissions:   []string{uitPermView, uitPermTest, uitPermApply, "platform:ingestion:ingestion:create", "platform:ingestion:ingestion:update"},
		PrincipalType: auth.PrincipalTypeUser,
	}
}

func (e *uitPG) ctx() context.Context { return testpg.WithAuth(context.Background(), e.admin()) }

func (e *uitPG) def(queue string) ingestion.Definition {
	return ingestion.Definition{
		Name: "Orders " + queue, Source: ingestion.Sqs{Region: "eu-west-1", Queue: queue}, Split: ingestion.None{},
		Target: ingestion.Target{Code: "acme:orders:order:received", TargetURL: "https://hooks.example.com/orders", Pool: e.poolCode, Mode: ingestion.ModeImmediate, BatchSize: 100},
	}
}

// create makes a live ingestion of the application and the first client.
func (e *uitPG) create(code, queue string, keep bool) *ingestion.Ingestion {
	e.t.Helper()
	ctx := e.ctx()
	def := e.def(queue)
	def.KeepFailedPayloads = keep
	ev, err := usecaseop.Run(ctx, e.uow, operations.CreateIngestion(e.repo, e.resolver),
		operations.CreateCommand{Application: e.app.Code, ClientID: e.client.ID, Code: code, Definition: def, Live: true}, auth.NewExecutionContext(ctx))
	require.NoError(e.t, err)
	i, err := e.repo.FindByID(ctx, ev.IngestionID)
	require.NoError(e.t, err)
	require.NotNil(e.t, i)
	return i
}

func (e *uitPG) tabs() *IngestionTabs {
	return &IngestionTabs{Store: e.repo, Audit: e.audit, Shell: shellForTest()}
}

// Activity, failures and history against the real queries: the windows are
// summed per period, the failures page by id, and the history reads the audit
// log the use cases wrote.
func TestIngestionTabsAgainstPostgres(t *testing.T) {
	e := newUitPG(t)
	i := e.create("tabs-"+uitUnique(), "q-one", true)
	ctx := e.ctx()
	now := time.Now().UTC().Truncate(time.Second)

	rt := "rt-" + uitUnique()
	errText := "queue <missing>"
	window := func(ago time.Duration, completed, failed int64) ingestion.WindowReport {
		from := now.Add(-ago)
		return ingestion.WindowReport{IngestionID: i.ID, From: from, To: from.Add(time.Minute), Completed: completed, Failed: failed, Records: completed * 10, Dispatch: completed}
	}
	rep := ingestion.Report{
		RuntimeID: rt, Caller: e.admin(), Now: now,
		Routes: []ingestion.RouteReport{{IngestionID: i.ID, Version: 1, Status: "FAILING", Error: &errText}},
		Metrics: []ingestion.WindowReport{
			window(time.Hour, 5, 1),        // in the day and the week
			window(3*24*time.Hour, 20, 2),  // in the week only
			window(10*24*time.Hour, 99, 9), // in neither
		},
	}
	for n := 25; n >= 1; n-- {
		excerpt := "payload-" + uitUnique()
		rep.Failures = append(rep.Failures, ingestion.FailureReport{
			IngestionID: i.ID, Version: 1, At: now.Add(-time.Duration(n) * time.Minute), Kind: "SOURCE", Message: "failure " + uitUnique(),
			AfterRecords: new(int64(n)), Excerpt: &excerpt,
		})
	}
	sum, err := e.repo.Record(ctx, rep)
	require.NoError(t, err)
	require.Equal(t, 1+3+25, sum.Accepted, "nothing was ignored")

	page := e.tabs()
	as := func(ac *auth.AuthContext, target string) (int, string) {
		rec := serveMounted(t, ac, req{target: target, datastar: true}, page)
		return rec.Code, rec.Body.String()
	}
	code, body := as(e.admin(), "/ui/ingestions/"+i.ID+"/activity")
	require.Equal(t, http.StatusOK, code, body)
	mustContain(t, body,
		rt, `fc-tag-warn">FAILING`, "queue &lt;missing&gt;",
		"5 completed, 1 failed, 50 records, 5 jobs",                 // last 24 hours: the first window
		"25 completed, 3 failed, 250 records, 25 jobs",              // last 7 days: two windows, not the old one
		`datetime="`+now.Add(-time.Hour).Format(time.RFC3339)+`"`,   // last exchange: the newest window
		`datetime="`+now.Add(-time.Minute).Format(time.RFC3339)+`"`, // last failure
	)
	mustNotContain(t, body, "124 completed", "queue <missing>")

	// An ingestion nobody has reported has empty activity.
	quiet := e.create("quiet-"+uitUnique(), "q-two", false)
	_, body = as(e.admin(), "/ui/ingestions/"+quiet.ID+"/activity")
	mustContain(t, body, "No runtime has reported it yet.", "None in 7 days", "0 completed, 0 failed, 0 records, 0 jobs")

	// Failures: twenty a page, newest first, the excerpt of a payload-keeping ingestion.
	_, body = as(e.admin(), "/ui/ingestions/"+i.ID+"/failures")
	assert.Equal(t, 20, strings.Count(body, `id="ing-failure-`), body)
	mustContain(t, body, "payload-", "After 1 record of the payload.", `data-more="/ui/ingestions/`+i.ID+`/failures?after=ifl_`)
	cursor := body[strings.Index(body, "failures?after=")+len("failures?after="):]
	cursor = cursor[:strings.IndexAny(cursor, `"`)]
	_, body = as(e.admin(), "/ui/ingestions/"+i.ID+"/failures?after="+cursor)
	assert.Equal(t, 5, strings.Count(body, `id="ing-failure-`), body)
	mustNotContain(t, body, "data-more=")

	// History: the create, then an update that changed the source.
	def := e.def("q-changed")
	def.KeepFailedPayloads = true
	_, err = usecaseop.RunTx(ctx, e.uow, operations.UpdateIngestion(e.repo, e.resolver),
		operations.UpdateCommand{ID: i.ID, ExpectedVersion: 1, Definition: def}, testpg.TestEC())
	require.NoError(t, err)
	_, body = as(e.admin(), "/ui/ingestions/"+i.ID+"/history")
	mustContain(t, body, "Created", "Changed", "Changed: name, source")
	assert.Less(t, strings.Index(body, "Changed"), strings.Index(body, "Created"), "newest first")

	// Reach: another tenant's client user, and a caller without the permission.
	foreign := &auth.AuthContext{PrincipalID: "prn_x", Scope: auth.ScopeClient, Clients: []string{string(e.other.ID)}, Roles: []string{"r"}, Permissions: []string{uitPermView}, AllApplications: true, PrincipalType: auth.PrincipalTypeUser}
	own := &auth.AuthContext{PrincipalID: "prn_y", Scope: auth.ScopeClient, Clients: []string{string(e.client.ID)}, Roles: []string{"r"}, Permissions: []string{uitPermView}, AllApplications: true, PrincipalType: auth.PrincipalTypeUser}
	for _, tab := range []string{"activity", "failures", "history"} {
		rec := serveMounted(t, foreign, req{target: "/ui/ingestions/" + i.ID + "/" + tab}, page)
		assert.Equal(t, http.StatusForbidden, rec.Code, tab)
		rec = serveMounted(t, uitUser(uitPermTest), req{target: "/ui/ingestions/" + i.ID + "/" + tab}, page)
		assert.Equal(t, http.StatusForbidden, rec.Code, tab)
		code, _ = as(own, "/ui/ingestions/"+i.ID+"/"+tab)
		assert.Equal(t, http.StatusOK, code, tab)
	}
}
