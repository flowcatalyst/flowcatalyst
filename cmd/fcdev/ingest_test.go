package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const ingestSftpFile = `application: ingest
client: acme
code: orders-sftp
name: Orders over SFTP
source:
  kind: sftp
  host: sftp.example.com
  directory: /in/orders
  username: feed
  credential:
    name: sftp-password
  every: PT5M
split:
  kind: csv
  header: true
target:
  code: acme:orders:order:received
  targetUrl: https://hooks.example.com/orders
  pool: orders
  mode: NEXT_ON_ERROR
  batchSize: 100
`

const ingestRowJSON = `{"id":"ing_1","applicationCode":"ingest","clientId":"clt_1","clientIdentifier":"acme","code":"orders-sftp","name":"Orders","status":"ACTIVE","version":2,"latestVersion":3,"source":{"kind":"sftp"},"split":{"kind":"csv"},"target":{"code":"c","targetUrl":"https://h.example.com","pool":"orders","connection":"conn","mode":"IMMEDIATE","batchSize":1}}`

// ingestPlatform is a fake platform for `fcdev ingest`.
type ingestPlatform struct {
	srv  *httptest.Server
	mu   sync.Mutex
	reqs []string
	// bodies and queries of the writes, by "METHOD path".
	bodies  map[string]string
	queries map[string]string
	// testPolls counts GETs of the queued test; it is DONE from the second.
	testPolls int
	outcome   string
	// configHidden answers the config GET as to a viewer who cannot update.
	configHidden bool
}

func newIngestPlatform(t *testing.T) *ingestPlatform {
	p := &ingestPlatform{bodies: map[string]string{}, queries: map[string]string{}, outcome: "RESULTS"}
	p.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		defer p.mu.Unlock()
		key := r.Method + " " + r.URL.Path
		p.reqs = append(p.reqs, key)
		body, _ := io.ReadAll(r.Body)
		p.bodies[key], p.queries[key] = string(body), r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		switch key {
		case "POST /oauth/token":
			_, _ = fmt.Fprintf(w, `{"access_token":%q,"expires_in":300,"token_type":"Bearer"}`, jwt(`{"sub":"prn_T","tier":"ANCHOR"}`))
		case "GET /api/ingestions":
			q := r.URL.Query()
			switch {
			case q.Get("q") != "":
				_, _ = fmt.Fprintf(w, `{"ingestions":[%s],"hasMore":false}`, ingestRowJSON)
			case q.Get("after") == "":
				_, _ = fmt.Fprintf(w, `{"ingestions":[%s],"hasMore":true,"nextCursor":"c1"}`, ingestRowJSON)
			default:
				_, _ = w.Write([]byte(`{"ingestions":[],"hasMore":false}`))
			}
		case "GET /api/ingestions/ing_1":
			_, _ = w.Write([]byte(ingestRowJSON))
		case "GET /api/ingestions/ing_1/definition":
			w.Header().Set("Content-Type", "application/yaml")
			_, _ = w.Write([]byte("application: ingest\n"))
		case "GET /api/ingestions/ing_1/versions":
			_, _ = w.Write([]byte(`{"versions":[{"id":"v","version":3,"live":false,"createdAt":"2026-10-24T10:00:00Z","createdBy":"prn_A","note":"tweak","name":"n"},{"id":"w","version":2,"live":true,"createdAt":"2026-10-23T10:00:00Z","createdBy":"prn_A","name":"n"}]}`))
		case "POST /api/ingestions/ing_1/pause", "POST /api/ingestions/ing_1/resume", "POST /api/ingestions/ing_1/versions/3/live":
			_, _ = w.Write([]byte(strings.Replace(ingestRowJSON, `"ACTIVE"`, `"PAUSED"`, 1)))
		case "PUT /api/ingestion-sets/orders", "POST /api/ingestion-sets/orders/applies/ap_1/reapply":
			dry := r.URL.Query().Get("dryRun") == "true"
			apply := `,"applyId":"ap_9"`
			if dry {
				apply = ""
			}
			_, _ = fmt.Fprintf(w, `{"name":"orders","dryRun":%v%s,"changes":[`+
				`{"application":"ingest","client":"acme","code":"orders-sftp","action":"CHANGE","changed":["target","split"]},`+
				`{"application":"ingest","client":"acme","code":"new-one","action":"CREATE","changed":[],"warnings":["CONFIG_MISSING: config entry orders-feed is not set"]},`+
				`{"application":"ingest","client":"acme","code":"same","action":"UNCHANGED","changed":[]},`+
				`{"application":"ingest","client":"acme","code":"old","action":"REMOVE","changed":[]}]}`, dry, apply)
		case "GET /api/ingestions/ing_1/config":
			if p.configHidden {
				_, _ = w.Write([]byte(`{"key":"orders-feed","status":"SET","schemaChecked":true,"valueHidden":true}`))
				return
			}
			_, _ = w.Write([]byte(`{"key":"orders-feed","status":"INVALID","problems":[{"field":"host","code":"NOT_NULL","message":"is required"}],"updatedAt":"2026-10-26T08:00:00Z","schemaChecked":false,"value":{"username":"feed"}}`))
		case "PUT /api/ingestions/ing_1/config":
			if strings.Contains(string(body), "bad") {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"INGESTION_INVALID","message":"host is required","details":{"fields":[{"field":"source.host","code":"NOT_NULL","message":"is required"}]}}`))
				return
			}
			_, _ = w.Write([]byte(`{"key":"orders-feed","status":"SET","schemaChecked":false,"value":{"host":"h"},"ingestions":["orders-sftp","stock"],"warnings":["SCHEMA_UNCHECKED: no live runtime reports sftp"]}`))
		case "DELETE /api/ingestions/ing_1/config":
			_, _ = w.Write([]byte(`{"key":"orders-feed","ingestions":["orders-sftp","stock"]}`))
		case "GET /api/ingestion-sets":
			_, _ = w.Write([]byte(`{"sets":[{"id":"s","name":"orders","ingestions":4,"createdAt":"2026-10-01T00:00:00Z","updatedAt":"2026-10-02T00:00:00Z"}]}`))
		case "GET /api/ingestion-sets/orders/applies":
			_, _ = w.Write([]byte(`{"applies":[{"id":"ap_1","set":"orders","appliedAt":"2026-10-02T00:00:00Z","appliedBy":"prn_A","items":4,"files":2,"changes":[{"application":"a","client":"c","code":"x","action":"CREATE","changed":[]}]}]}`))
		case "POST /api/ingestion-sets/orders/release":
			_, _ = w.Write([]byte(`{"name":"orders","released":4}`))
		case "POST /api/ingestion-tests":
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"id":"tst_1","kind":"sftp","status":"PENDING","createdAt":"x","expiresAt":"y","notice":"no runtime carries sftp yet"}`))
		case "GET /api/ingestion-tests/tst_1":
			p.testPolls++
			if p.testPolls < 2 {
				_, _ = w.Write([]byte(`{"id":"tst_1","kind":"sftp","status":"RUNNING","createdAt":"x","expiresAt":"y"}`))
				return
			}
			_, _ = fmt.Fprintf(w, `{"id":"tst_1","kind":"sftp","status":"DONE","outcome":%q,"records":12,"steps":["connected to sftp.example.com","listed /in/orders: 2 files"],"createdAt":"x","expiresAt":"y"}`, p.outcome)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"NOT_FOUND","message":"no such route"}`))
		}
	}))
	t.Cleanup(p.srv.Close)
	return p
}

func (p *ingestPlatform) writes() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := 0
	for _, r := range p.reqs {
		if strings.HasPrefix(r, "PUT ") || (strings.HasPrefix(r, "POST /api") && r != "POST /api/ingestion-tests") || strings.HasPrefix(r, "DELETE ") {
			n++
		}
	}
	return n
}

func (p *ingestPlatform) flags() []string {
	return []string{"--platform", p.srv.URL, "--client-id", "c", "--client-secret", "s"}
}

func ingestFolder(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "Orders")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sftp.yaml"), []byte(ingestSftpFile), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("ignored"), 0o600))
	return dir
}

func TestIngestApplyPrintsThePlan(t *testing.T) {
	newFnTestEnv(t)
	p := newIngestPlatform(t)
	dir := ingestFolder(t)
	out, _, err := runCobra(t, newIngestApplyCmd(), "", append([]string{dir}, p.flags()...)...)
	require.NoError(t, err)
	require.Equal(t, "set orders applied as apply ap_9, 1 definition(s)\n"+
		"  change    ingest/acme/orders-sftp (target, split)\n"+
		"  create    ingest/acme/new-one\n"+
		"    warning: CONFIG_MISSING: config entry orders-feed is not set\n"+
		"  unchanged ingest/acme/same\n"+
		"  remove    ingest/acme/old\n", out)
	require.Empty(t, p.queries["PUT /api/ingestion-sets/orders"])

	var sent struct {
		Items []map[string]any `json:"items"`
		Files []setFile        `json:"files"`
	}
	require.NoError(t, json.Unmarshal([]byte(p.bodies["PUT /api/ingestion-sets/orders"]), &sent))
	require.Len(t, sent.Items, 1)
	require.Equal(t, "ingest", sent.Items[0]["application"])
	require.Equal(t, "acme", sent.Items[0]["client"])
	require.Equal(t, "orders-sftp", sent.Items[0]["code"])
	require.Equal(t, "https://hooks.example.com/orders", sent.Items[0]["target"].(map[string]any)["targetUrl"])
	require.Len(t, sent.Files, 1)
	require.Equal(t, "sftp.yaml", sent.Files[0].Path)
	require.Equal(t, ingestSftpFile, sent.Files[0].Content, "the archived file is as written")
}

func TestIngestApplyDryRunWritesNothing(t *testing.T) {
	newFnTestEnv(t)
	p := newIngestPlatform(t)
	dir := ingestFolder(t)
	out, _, err := runCobra(t, newIngestApplyCmd(), "", append([]string{dir, "--set", "orders", "--dry-run"}, p.flags()...)...)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(out, "set orders, dry run: nothing was changed, 1 definition(s)\n"), out)
	require.Equal(t, "dryRun=true", p.queries["PUT /api/ingestion-sets/orders"])
}

func TestIngestApplyJSONShape(t *testing.T) {
	newFnTestEnv(t)
	p := newIngestPlatform(t)
	dir := ingestFolder(t)
	out, errOut, err := runCobra(t, newIngestApplyCmd(), "", append([]string{dir, "--json"}, p.flags()...)...)
	require.NoError(t, err)
	var plan ingestPlan
	require.NoError(t, json.Unmarshal([]byte(out), &plan), "stdout is one JSON object")
	require.Equal(t, "orders", plan.Name)
	require.Equal(t, "ap_9", plan.ApplyID)
	require.Len(t, plan.Changes, 4)
	require.Equal(t, []string{"target", "split"}, plan.Changes[0].Changed)
	require.Contains(t, errOut, "  change    ingest/acme/orders-sftp", "human output moves to stderr")
}

func TestIngestApplyRefusals(t *testing.T) {
	newFnTestEnv(t)
	p := newIngestPlatform(t)
	dir := ingestFolder(t)

	// A leftover ${env:} is refused, naming the file and line; a comment line may mention it.
	envFile := filepath.Join(t.TempDir(), "env.yaml")
	require.NoError(t, os.WriteFile(envFile, []byte("# was ${env:OLD}\n"+ingestSftpFile+"# ${env:IN_COMMENT}\n"), 0o600))
	_, _, err := runCobra(t, newIngestApplyCmd(), "", append([]string{envFile, "--set", "orders"}, p.flags()...)...)
	require.NoError(t, err, "comment lines are not read")
	sent := p.writes()
	stale := strings.Replace(ingestSftpFile, "https://hooks.example.com/orders", "${env:ORDERS_HOOK}", 1)
	require.NoError(t, os.WriteFile(envFile, []byte(stale), 0o600))
	_, _, err = runCobra(t, newIngestApplyCmd(), "", append([]string{envFile, "--set", "orders"}, p.flags()...)...)
	require.ErrorContains(t, err, "env.yaml line 18 holds ${env:...}")
	require.ErrorContains(t, err, "fcdev ingest config set")

	empty := t.TempDir()
	_, _, err = runCobra(t, newIngestApplyCmd(), "", append([]string{empty}, p.flags()...)...)
	require.ErrorContains(t, err, "no definitions found")

	_, _, err = runCobra(t, newIngestApplyCmd(), "", append([]string{filepath.Join(dir, "sftp.yaml")}, p.flags()...)...)
	require.ErrorContains(t, err, "--set")

	bad := filepath.Join(t.TempDir(), "bad.yaml")
	require.NoError(t, os.WriteFile(bad, []byte("application: [x\n"), 0o600))
	_, _, err = runCobra(t, newIngestApplyCmd(), "", append([]string{bad, "--set", "orders"}, p.flags()...)...)
	require.ErrorContains(t, err, "bad.yaml")
	require.Equal(t, sent, p.writes(), "a refused file writes nothing")
}

func TestIngestApplyRefusedByThePlatformIsAnError(t *testing.T) {
	newFnTestEnv(t)
	p := newIngestPlatform(t)
	dir := ingestFolder(t)
	_, _, err := runCobra(t, newIngestApplyCmd(), "", append([]string{dir, "--set", "other"}, p.flags()...)...)
	require.ErrorContains(t, err, "HTTP 404")
}

func TestIngestProdApplyIsGuardedButDryRunIsNot(t *testing.T) {
	e := newFnTestEnv(t)
	pl := newProfilePlatform(t, testSecret)
	profileWith(t, e, pl, "prod")
	dir := ingestFolder(t)
	_, _, err := runCobra(t, newIngestApplyCmd(), "", dir, "--profile", "p")
	require.ErrorContains(t, err, "without a terminal")
	require.Zero(t, pl.writes())
	// A dry run asks nothing: it reaches the platform (which here has no such route).
	_, _, err = runCobra(t, newIngestApplyCmd(), "", dir, "--profile", "p", "--dry-run")
	require.ErrorContains(t, err, "HTTP 404")
	require.Equal(t, 1, pl.count("PUT /api/ingestion-sets/orders"))
}

func TestIngestListAndGet(t *testing.T) {
	newFnTestEnv(t)
	p := newIngestPlatform(t)
	out, _, err := runCobra(t, newIngestListCmd(), "", append([]string{"--application", "ingest", "--client", "acme"}, p.flags()...)...)
	require.NoError(t, err)
	require.Contains(t, out, "ingest/acme/orders-sftp")
	require.Contains(t, out, "sftp")
	require.Equal(t, 2, countReq(p, "GET /api/ingestions"), "the cursor is followed")

	out, _, err = runCobra(t, newIngestListCmd(), "", append([]string{"--json"}, p.flags()...)...)
	require.NoError(t, err)
	var list struct {
		Ingestions []map[string]any `json:"ingestions"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &list))
	require.Len(t, list.Ingestions, 1)
	require.Equal(t, "ing_1", list.Ingestions[0]["id"])

	out, _, err = runCobra(t, newIngestGetCmd(), "", append([]string{"ingest/acme/orders-sftp"}, p.flags()...)...)
	require.NoError(t, err)
	require.Contains(t, out, "ingest/acme/orders-sftp (ing_1)")
	require.Contains(t, out, "live v2 of 3")
	require.Contains(t, out, "c → https://h.example.com (pool orders, connection conn)")

	_, _, err = runCobra(t, newIngestGetCmd(), "", append([]string{"ingest/acme/nothing"}, p.flags()...)...)
	require.ErrorContains(t, err, "no ingestion ingest/acme/nothing")
	_, _, err = runCobra(t, newIngestGetCmd(), "", append([]string{"orders-sftp"}, p.flags()...)...)
	require.ErrorContains(t, err, "<application>/<client>/<code>")
}

func countReq(p *ingestPlatform, key string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := 0
	for _, r := range p.reqs {
		if r == key {
			n++
		}
	}
	return n
}

func TestIngestExportWritesTheFileForm(t *testing.T) {
	newFnTestEnv(t)
	p := newIngestPlatform(t)
	out := t.TempDir()
	stdout, _, err := runCobra(t, newIngestExportCmd(), "", append([]string{"ingest/acme/orders-sftp", "--out", out}, p.flags()...)...)
	require.NoError(t, err)
	file := filepath.Join(out, "ingest", "acme", "orders-sftp.yaml")
	data, err := os.ReadFile(file)
	require.NoError(t, err)
	require.Equal(t, "application: ingest\n", string(data))
	require.Contains(t, stdout, "1 ingestion(s) written")

	_, _, err = runCobra(t, newIngestExportCmd(), "", p.flags()...)
	require.ErrorContains(t, err, "--all")
	stdout, _, err = runCobra(t, newIngestExportCmd(), "", append([]string{"--all", "--out", out, "--json"}, p.flags()...)...)
	require.NoError(t, err)
	var res struct {
		Written []string `json:"written"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &res))
	require.Equal(t, []string{file}, res.Written)
}

func TestIngestVersionsLivePauseResume(t *testing.T) {
	newFnTestEnv(t)
	p := newIngestPlatform(t)
	ref := "ingest/acme/orders-sftp"
	out, _, err := runCobra(t, newIngestVersionsCmd(), "", append([]string{ref}, p.flags()...)...)
	require.NoError(t, err)
	require.Contains(t, out, "v3")
	require.Contains(t, out, "tweak")
	require.Contains(t, out, "v2    2026-10-23T10:00:00Z  prn_A  ← live")

	out, _, err = runCobra(t, newIngestLiveCmd(), "", append([]string{ref, "3"}, p.flags()...)...)
	require.NoError(t, err)
	require.Contains(t, out, "PAUSED, live v2")
	require.Equal(t, 1, countReq(p, "POST /api/ingestions/ing_1/versions/3/live"))

	_, _, err = runCobra(t, newIngestLiveCmd(), "", append([]string{ref, "x"}, p.flags()...)...)
	require.ErrorContains(t, err, "not a version number")

	_, _, err = runCobra(t, newIngestPauseCmd(), "", append([]string{ref}, p.flags()...)...)
	require.NoError(t, err)
	out, _, err = runCobra(t, newIngestResumeCmd(), "", append([]string{"ing_1", "--json"}, p.flags()...)...)
	require.NoError(t, err)
	var row map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &row))
	require.Equal(t, "ing_1", row["id"])
	require.Equal(t, 1, countReq(p, "POST /api/ingestions/ing_1/pause"))
	require.Equal(t, 1, countReq(p, "POST /api/ingestions/ing_1/resume"))
}

func TestIngestProdPauseNeedsConfirmation(t *testing.T) {
	e := newFnTestEnv(t)
	pl := newProfilePlatform(t, testSecret)
	profileWith(t, e, pl, "prod")
	_, _, err := runCobra(t, newIngestPauseCmd(), "", "ing_1", "--profile", "p")
	require.Error(t, err)
	require.Zero(t, pl.writes())
}

func TestIngestTestWaitsAndPrintsSteps(t *testing.T) {
	newFnTestEnv(t)
	old := ingestTestPoll
	ingestTestPoll = time.Millisecond
	t.Cleanup(func() { ingestTestPoll = old })
	p := newIngestPlatform(t)

	out, errOut, err := runCobra(t, newIngestTestCmd(), "", append([]string{"ingest/acme/orders-sftp", "--wait"}, p.flags()...)...)
	require.NoError(t, err)
	require.Equal(t, "test tst_1 queued\n  no runtime carries sftp yet\ntest tst_1: RESULTS, 12 record(s)\n  connected to sftp.example.com\n  listed /in/orders: 2 files\n", out)
	require.JSONEq(t, `{"ingestionId":"ing_1"}`, p.bodies["POST /api/ingestion-tests"])
	require.NotContains(t, errOut, "tst_1")

	// Without --wait it queues and says so.
	p.testPolls = 0
	out, _, err = runCobra(t, newIngestTestCmd(), "", append([]string{"ingest/acme/orders-sftp"}, p.flags()...)...)
	require.NoError(t, err)
	require.NotContains(t, out, "RESULTS")
}

func TestIngestTestFailureIsExitOne(t *testing.T) {
	newFnTestEnv(t)
	old := ingestTestPoll
	ingestTestPoll = time.Millisecond
	t.Cleanup(func() { ingestTestPoll = old })
	for outcome, fails := range map[string]bool{"RESULTS": false, "NO_RESULTS": false, "FAILED": true, "REFUSED": true, "NOT_SUPPORTED": true} {
		p := newIngestPlatform(t)
		p.outcome = outcome
		out, _, err := runCobra(t, newIngestTestCmd(), "", append([]string{"ingest/acme/orders-sftp", "--wait", "--json"}, p.flags()...)...)
		require.Equal(t, fails, err != nil, outcome)
		var res ingestTest
		// A failed command prints cobra's usage after the JSON; read the first value.
		require.NoError(t, json.NewDecoder(strings.NewReader(out)).Decode(&res), "the result is printed even when the test failed")
		require.Equal(t, outcome, res.Outcome)
	}
}

func TestIngestTestAFileSendsItsDefinition(t *testing.T) {
	newFnTestEnv(t)
	old := ingestTestPoll
	ingestTestPoll = time.Millisecond
	t.Cleanup(func() { ingestTestPoll = old })
	p := newIngestPlatform(t)
	dir := ingestFolder(t)
	_, _, err := runCobra(t, newIngestTestCmd(), "", append([]string{filepath.Join(dir, "sftp.yaml"), "--wait"}, p.flags()...)...)
	require.NoError(t, err)
	var sent map[string]any
	require.NoError(t, json.Unmarshal([]byte(p.bodies["POST /api/ingestion-tests"]), &sent))
	require.Equal(t, "ingest", sent["application"])
	require.Equal(t, "acme", sent["client"])
	require.NotContains(t, sent, "ingestionId")
	def := sent["definition"].(map[string]any)
	require.Equal(t, "Orders over SFTP", def["name"])
	require.Equal(t, "sftp", def["source"].(map[string]any)["kind"])
}

func TestIngestSets(t *testing.T) {
	newFnTestEnv(t)
	p := newIngestPlatform(t)
	out, _, err := runCobra(t, newIngestSetsCmd(), "", append([]string{"list"}, p.flags()...)...)
	require.NoError(t, err)
	require.Contains(t, out, "orders")
	require.Contains(t, out, "4 ingestion(s)")

	out, _, err = runCobra(t, newIngestSetsCmd(), "", append([]string{"applies", "orders"}, p.flags()...)...)
	require.NoError(t, err)
	require.Contains(t, out, "ap_1  2026-10-02T00:00:00Z  by prn_A  4 item(s), 1 change(s)")

	out, _, err = runCobra(t, newIngestSetsCmd(), "", append([]string{"reapply", "orders", "ap_1", "--dry-run"}, p.flags()...)...)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(out, "set orders, dry run: nothing was changed\n"), out)
	require.Equal(t, "dryRun=true", p.queries["POST /api/ingestion-sets/orders/applies/ap_1/reapply"])

	out, _, err = runCobra(t, newIngestSetsCmd(), "", append([]string{"reapply", "orders", "ap_1"}, p.flags()...)...)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(out, "set orders applied as apply ap_9\n"), out)

	out, _, err = runCobra(t, newIngestSetsCmd(), "", append([]string{"release", "orders"}, p.flags()...)...)
	require.NoError(t, err)
	require.Contains(t, out, "set orders released: 4 ingestion(s)")
}

func TestIngestConfigGet(t *testing.T) {
	newFnTestEnv(t)
	p := newIngestPlatform(t)
	out, _, err := runCobra(t, newIngestConfigGetCmd(), "", append([]string{"ingest/acme/orders-sftp"}, p.flags()...)...)
	require.NoError(t, err)
	for _, want := range []string{"orders-feed", "INVALID", "problem host: is required", "not checked against them", `"username": "feed"`} {
		require.Contains(t, out, want)
	}
	out, errOut, err := runCobra(t, newIngestConfigGetCmd(), "", append([]string{"ingest/acme/orders-sftp", "--json"}, p.flags()...)...)
	require.NoError(t, err)
	var c ingestConfig
	require.NoError(t, json.Unmarshal([]byte(out), &c))
	require.Equal(t, "INVALID", c.Status)
	require.Contains(t, errOut, "orders-feed")
}

func TestIngestConfigGetHiddenValue(t *testing.T) {
	newFnTestEnv(t)
	p := newIngestPlatform(t)
	p.configHidden = true
	out, _, err := runCobra(t, newIngestConfigGetCmd(), "", append([]string{"ingest/acme/orders-sftp"}, p.flags()...)...)
	require.NoError(t, err)
	require.Contains(t, out, "the value is hidden: it needs ingestion:update")
}

func TestIngestConfigSet(t *testing.T) {
	newFnTestEnv(t)
	p := newIngestPlatform(t)
	file := filepath.Join(t.TempDir(), "v.json")
	require.NoError(t, os.WriteFile(file, []byte(`{"host":"h","targetUrl":"https://h.example.com"}`), 0o600))
	out, _, err := runCobra(t, newIngestConfigSetCmd(), "", append([]string{"ingest/acme/orders-sftp", file}, p.flags()...)...)
	require.NoError(t, err)
	require.JSONEq(t, `{"value":{"host":"h","targetUrl":"https://h.example.com"}}`, p.bodies["PUT /api/ingestions/ing_1/config"])
	require.Contains(t, out, "checked against orders-sftp, stock")
	require.Contains(t, out, "warning: SCHEMA_UNCHECKED")

	// stdin
	_, _, err = runCobra(t, newIngestConfigSetCmd(), `{"host":"x"}`, append([]string{"ingest/acme/orders-sftp", "-"}, p.flags()...)...)
	require.NoError(t, err)
	require.JSONEq(t, `{"value":{"host":"x"}}`, p.bodies["PUT /api/ingestions/ing_1/config"])

	// a problem names its field path
	_, errOut, err := runCobra(t, newIngestConfigSetCmd(), `{"bad":1}`, append([]string{"ingest/acme/orders-sftp", "-"}, p.flags()...)...)
	require.ErrorContains(t, err, "HTTP 400")
	require.Contains(t, errOut, "problem source.host: is required")
}

func TestIngestConfigSetRefusesANonObject(t *testing.T) {
	newFnTestEnv(t)
	p := newIngestPlatform(t)
	before := p.writes()
	for _, in := range []string{`[1]`, `"x"`, `null`, `not json`} {
		_, _, err := runCobra(t, newIngestConfigSetCmd(), in, append([]string{"ingest/acme/orders-sftp", "-"}, p.flags()...)...)
		require.ErrorContains(t, err, "not a JSON object", in)
	}
	require.Equal(t, before, p.writes())
}

func TestIngestConfigDeleteAndProdGuard(t *testing.T) {
	newFnTestEnv(t)
	p := newIngestPlatform(t)
	out, _, err := runCobra(t, newIngestConfigDeleteCmd(), "", append([]string{"ingest/acme/orders-sftp", "--yes"}, p.flags()...)...)
	require.NoError(t, err)
	require.Contains(t, out, "stopped until they are set again: orders-sftp, stock")

	// On a prod profile both writes stop at the confirmation, after the ingestion is found.
	e := newFnTestEnv(t)
	pp := newIngestPlatform(t)
	t.Setenv("FC_TEST_SECRET", testSecret)
	e.writeProfiles(t, 0o600, profileTOML("p", pp.srv.URL, "prod", "env:FC_TEST_SECRET"))
	_, _, err = runCobra(t, newIngestConfigDeleteCmd(), "", "ing_1", "--profile", "p")
	require.ErrorContains(t, err, "without a terminal")
	file := filepath.Join(t.TempDir(), "v.json")
	require.NoError(t, os.WriteFile(file, []byte(`{}`), 0o600))
	_, _, err = runCobra(t, newIngestConfigSetCmd(), "", "ing_1", file, "--profile", "p")
	require.ErrorContains(t, err, "without a terminal")
	require.Zero(t, pp.writes())
}

// A path that does not exist says so: run from the wrong directory, "is not a
// definition file" sent the user to look at the file's contents.
func TestReadDefinitionFilesNamesAMissingPath(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "sftp.yaml")
	require.NoError(t, os.WriteFile(good, []byte(ingestSftpFile), 0o600))

	missing := filepath.Join(dir, "nowhere", "orders.yaml")
	_, _, err := readDefinitionFiles([]string{missing})
	require.EqualError(t, err, missing+": no such file or folder")
	_, _, err = readDefinitionFiles([]string{good, missing})
	require.EqualError(t, err, missing+": no such file or folder")

	// A folder among several arguments is still not a definition file.
	_, _, err = readDefinitionFiles([]string{good, dir})
	require.EqualError(t, err, dir+" is not a definition file")
}
