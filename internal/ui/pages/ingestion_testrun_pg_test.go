//go:build integration

package pages

import (
	"net/http"
	"net/url"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
)

var uitTestIDPattern = regexp.MustCompile(`id="ing-test-(itr_[A-Za-z0-9]+)"`)

// Test connection against Postgres: a saved ingestion's test is queued, a
// runtime takes it (the poll) and answers (the report), and the stream shows
// each state in turn; a test no runtime answers expires after the two minutes;
// the excerpt is shown only for an ingestion that keeps payloads; and the
// permission a runtime-declared kind requires is checked against the kinds the
// runtimes reported.
func TestIngestionTestConnectionAgainstPostgres(t *testing.T) {
	e := newUitPG(t)
	ctx := e.ctx()
	keeping := e.create("test-keep-"+uitUnique(), "q-keep", true)
	plain := e.create("test-plain-"+uitUnique(), "q-plain", false)

	var offset time.Duration // the page's clock runs ahead of the database's by this much
	h := &IngestionTests{
		Store: e.repo, Resolver: e.resolver, Shell: shellForTest(), every: 5 * time.Millisecond,
		now: func() time.Time { return time.Now().UTC().Add(offset) },
		Subject: func(r *http.Request) (IngestionTestSubject, error) {
			def := e.def("q-unsaved")
			if kind := r.PostFormValue("kind"); kind != "" {
				def.Source = ingestion.Custom{Type: kind}
			}
			return IngestionTestSubject{Application: e.app.Code, Client: e.client.Identifier, Definition: def}, nil
		},
	}
	serve := func(ac *auth.AuthContext, q req) string {
		t.Helper()
		return serveMounted(t, ac, q, h).Body.String()
	}
	queue := func(i *ingestion.Ingestion) string {
		body := serve(e.admin(), uitTestPost("/ui/ingestions/"+i.ID+"/test", nil))
		m := uitTestIDPattern.FindStringSubmatch(body)
		require.NotNil(t, m, body)
		return m[1]
	}
	stream := func(id string) string {
		return serve(e.admin(), req{target: "/ui/ingestion-tests/" + id + "/stream", datastar: true})
	}
	// A runtime polls (taking the tests of the kinds it carries) and answers.
	rt := "rt-" + uitUnique()
	poll := func(kinds ...ingestion.RuntimeKind) {
		t.Helper()
		_, err := e.repo.Poll(ctx, ingestion.PollRequest{RuntimeID: rt, ContractVersion: ingestion.ContractVersion, Kinds: kinds, Now: time.Now().UTC(), Lease: ingestion.DefaultLease})
		require.NoError(t, err)
	}
	answer := func(id string, result ingestion.TestResult) {
		t.Helper()
		_, err := e.repo.Record(ctx, ingestion.Report{RuntimeID: rt, Caller: e.admin(), Now: time.Now().UTC(), Tests: []ingestion.TestAnswer{{TestID: id, Result: result}}})
		require.NoError(t, err)
	}

	// Queued, taken, answered. The stream's first state is what the row says.
	id := queue(keeping)
	row, err := e.repo.FindTest(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, row)
	assert.Equal(t, ingestion.TestPending, row.Status)
	require.NotNil(t, row.IngestionID)
	assert.Equal(t, keeping.ID, *row.IngestionID)
	poll()
	records, excerpt := int64(2), "first-bytes-of-the-answer"
	answer(id, ingestion.TestResult{Outcome: "RESULTS", Steps: []string{"Connected to sqs", "Read a message"}, Records: &records, Excerpt: &excerpt})
	body := stream(id)
	mustContain(t, body, `fc-tag-success">Logged in, results`, "on "+rt, "Connected to sqs", "Read a message", excerpt, "The split makes 2 record(s)")

	// An ingestion that does not keep payloads never shows the start of the answer.
	id = queue(plain)
	poll()
	answer(id, ingestion.TestResult{Outcome: "NO_RESULTS", Steps: []string{"Connected"}, Excerpt: &excerpt})
	body = stream(id)
	mustContain(t, body, `fc-tag-info">Logged in, no results`, "Connected")
	mustNotContain(t, body, excerpt)

	// A test nobody takes expires after two minutes (the page's clock moved on).
	id = queue(plain)
	offset = ingestion.TestRequestTTL + time.Second
	body = stream(id)
	mustContain(t, body, "No runtime answered in time.")
	offset = 0

	// Another tenant's client user reads neither the test nor its result.
	foreign := &auth.AuthContext{PrincipalID: "prn_f", Scope: auth.ScopeClient, Clients: []string{string(e.other.ID)}, Roles: []string{"r"}, Permissions: []string{uitPermTest}, AllApplications: true, PrincipalType: auth.PrincipalTypeUser}
	body = serve(foreign, req{target: "/ui/ingestion-tests/" + id + "/stream", datastar: true})
	mustContain(t, body, "no access to this resource")
	body = serve(foreign, uitTestPost("/ui/ingestions/"+plain.ID+"/test", nil))
	mustContain(t, body, "no access to this resource")
	own := &auth.AuthContext{PrincipalID: "prn_o", Scope: auth.ScopeClient, Clients: []string{string(e.client.ID)}, Roles: []string{"r"}, Permissions: []string{uitPermTest}, AllApplications: true, PrincipalType: auth.PrincipalTypeUser}
	body = serve(own, uitTestPost("/ui/ingestions/"+plain.ID+"/test", nil))
	mustContain(t, body, "Waiting for a runtime")

	// An unsaved definition of a kind a runtime reported with a required
	// permission: refused without it, queued with it.
	kind := "pgkind-" + uitUnique()
	poll(ingestion.RuntimeKind{Kind: kind, RequiredPermission: auth.PermIngestionRoute})
	form := url.Values{"kind": {kind}}
	tester := uitUser(uitPermTest)
	body = serve(tester, uitTestPost("/ui/ingestions/test", form))
	mustContain(t, body, "permission required for source kind "+kind+": "+auth.PermIngestionRoute)
	mustNotContain(t, body, `id="ing-test-itr_`)
	body = serve(uitUser(uitPermTest, auth.PermIngestionRoute), uitTestPost("/ui/ingestions/test", form))
	mustContain(t, body, `id="ing-test-itr_`)
	// A built-in kind needs nothing more.
	body = serve(tester, uitTestPost("/ui/ingestions/test", nil))
	mustContain(t, body, `id="ing-test-itr_`)
}
