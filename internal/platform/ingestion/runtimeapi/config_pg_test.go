//go:build integration

package runtimeapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	platformconfigops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
)

// Per-environment settings in platform config (docs/ingestion-design.md
// section 13), through the real routes on a real database: a source that names
// an entry, the entry endpoints, and what the runtime's calls make of it.

const (
	permCreate   = "platform:ingestion:ingestion:create"
	permUpdate   = "platform:ingestion:ingestion:update"
	permPromote  = "platform:ingestion:ingestion:promote"
	permBind     = "platform:ingestion:ingestion:bind-secrets"
	permIdentity = "platform:ingestion:ingestion:runtime-identity"
	permApplySet = "platform:ingestion:set:apply"
)

// newConfigEnv is newEnv for a test that leaves ingestions behind: the runtime
// reach is platform-wide and the fair share counts every live polled ingestion,
// so a live sftp ingestion nobody leases would be claimed by the next test's
// runtime ahead of that test's own. The ingestions go when the test ends.
func newConfigEnv(t *testing.T) *env {
	t.Helper()
	e := newEnv(t)
	t.Cleanup(func() {
		_, err := e.pool.Exec(context.Background(), `DELETE FROM ing_ingestions WHERE application_id = $1`, string(e.app.ID))
		assert.NoError(t, err)
	})
	return e
}

// as makes the one principal of the environment hold exactly these permissions
// (it is a runtime's service account, so the runtime calls work too).
func (e *env) as(perms ...string) { e.who = e.principal(append([]string{permRuntime}, perms...)...) }

// operator holds everything an operator needs except bind-secrets and
// runtime-identity, which a test grants on purpose.
func (e *env) operator(extra ...string) {
	e.as(append([]string{permView, permCreate, permUpdate, permPromote, permTest}, extra...)...)
}

// refTarget is a target whose delivery URL is left to the config entry.
func (tg targets) refTarget() string {
	return fmt.Sprintf(`{"code":"%s:orders:order:received","pool":%q,"mode":"IMMEDIATE","batchSize":100}`, tg.app, tg.pool.Code)
}

// createBody is the body of createIngestion for a source and target.
func (e *env) createBody(code, source, target string) map[string]any {
	var body map[string]any
	doc := `{"name":"Orders","source":` + source + `,"split":{"kind":"none"},"target":` + target + `}`
	if err := json.Unmarshal([]byte(doc), &body); err != nil {
		e.t.Fatal(err)
	}
	body["application"], body["client"], body["code"] = e.app.Code, e.client.Identifier, code
	return body
}

// create makes an ingestion through the API as the current principal.
func (e *env) create(code, source, target string) response {
	e.t.Helper()
	return e.do(http.MethodPost, "/api/ingestions", e.createBody(code, source, target))
}

func (e *env) mustCreate(code, source, target string) string {
	e.t.Helper()
	r := e.create(code, source, target)
	require.Equal(e.t, http.StatusCreated, r.code, r.raw)
	return r.str("id")
}

func configPath(id string) string { return "/api/ingestions/" + id + "/config" }

func (e *env) putConfig(id, value string) response {
	e.t.Helper()
	var v map[string]any
	require.NoError(e.t, json.Unmarshal([]byte(value), &v))
	return e.do(http.MethodPut, configPath(id), map[string]any{"value": v})
}

func (r response) obj(key string) map[string]any { m, _ := r.json[key].(map[string]any); return m }

func (r response) strs(key string) []string {
	var out []string
	arr, _ := r.json[key].([]any)
	for _, x := range arr {
		out = append(out, fmt.Sprint(x))
	}
	return out
}

func sftpEntry(host, secret string) string {
	return fmt.Sprintf(`{"host":%q,"port":22,"directory":"/in","username":"feed","credential":{"name":%q},"every":"PT5M","knownHosts":"%s ssh-ed25519 AAAA","targetUrl":"https://hooks.example.com/orders"}`, host, secret, host)
}

func sqsEntry(secret string) string {
	return fmt.Sprintf(`{"region":"eu-west-1","queue":"orders","credential":{"accessKeyId":"AKIAEXAMPLE","secretAccessKey":{"name":%q}},"targetUrl":"https://hooks.example.com/orders"}`, secret)
}

// A definition that names an entry that does not exist yet is saved, with a
// CONFIG_MISSING warning (so a pipeline applying a set never depends on someone
// having entered the environment's values first), and runs nowhere until the
// entry is valid. Every answer that saves says so.
func TestConfig_SaveWithMissingEntryIsSavedWithAWarning(t *testing.T) {
	e := newConfigEnv(t)
	tg := e.makeTargets()
	e.operator()

	r := e.create("feed", `{"kind":"sftp","config":"vendor"}`, tg.refTarget())
	require.Equal(t, http.StatusCreated, r.code, r.raw)
	id := r.str("id")
	assert.Equal(t, "MISSING", r.obj("config")["status"])
	assert.Equal(t, "vendor", r.obj("config")["key"])
	require.Len(t, r.strs("warnings"), 1)
	assert.True(t, strings.HasPrefix(r.strs("warnings")[0], "CONFIG_MISSING: "), r.strs("warnings")[0])
	assert.Equal(t, map[string]any{"kind": "sftp", "config": "vendor"}, r.obj("source"), "the stored definition is the reference")
	assert.Empty(t, e.poll("rt-"+unique(), ingestion.ContractVersion).list("ingestions"), "and it does not run")

	// Update and save-version say it too.
	upd := e.createBody("feed", `{"kind":"sftp","config":true}`, tg.refTarget())
	upd["expectedVersion"] = 1
	r = e.do(http.MethodPut, "/api/ingestions/"+id, upd)
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.Contains(t, strings.Join(r.strs("warnings"), "\n"), "CONFIG_MISSING")
	assert.Equal(t, "SAVED", r.obj("save")["outcome"])

	ver := e.createBody("feed", `{"kind":"sftp","config":"another"}`, tg.refTarget())
	r = e.do(http.MethodPost, "/api/ingestions/"+id+"/versions", ver)
	require.Equal(t, http.StatusCreated, r.code, r.raw)
	assert.Contains(t, strings.Join(r.strs("warnings"), "\n"), "CONFIG_MISSING")

	// A dry run of a set that applies it says so per item.
	e.operator(permApplySet)
	item := e.createBody("set-feed", `{"kind":"sftp","config":"vendor"}`, tg.refTarget())
	r = e.do(http.MethodPut, "/api/ingestion-sets/cfg-"+unique()[:8]+"?dryRun=true", map[string]any{"items": []any{item}})
	require.Equal(t, http.StatusOK, r.code, r.raw)
	changes := r.list("changes")
	require.Len(t, changes, 1)
	assert.Equal(t, "CREATE", changes[0]["action"])
	assert.Contains(t, fmt.Sprint(changes[0]["warnings"]), "CONFIG_MISSING")

	// The list and the drawer show it as needing configuration.
	r = e.do(http.MethodGet, "/api/ingestions/"+id, nil)
	assert.Equal(t, "MISSING", r.obj("config")["status"])
}

// The entry is written through the ingestion, after the same check a save
// makes of the effective definition: pointing a secret at a new host needs
// bind-secrets, exactly as a save of an inline definition does.
func TestConfig_PutNeedsBindSecretsToPointASecretSomewhereNew(t *testing.T) {
	e := newConfigEnv(t)
	tg := e.makeTargets()
	e.operator()
	id := e.mustCreate("feed", `{"kind":"sftp","config":"vendor"}`, tg.refTarget())

	// The first value names a secret at a host: new to the live version.
	r := e.putConfig(id, sftpEntry("sftp.example.com", "sftp-password"))
	assert.Equal(t, http.StatusForbidden, r.code, r.raw)
	assert.Equal(t, "INGESTION_BIND_SECRETS_PERMISSION_REQUIRED", r.errCode())
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM app_platform_configs WHERE application_code = $1 AND section = 'ingest'`, e.app.Code), "nothing was written")

	e.operator(permBind)
	r = e.putConfig(id, sftpEntry("sftp.example.com", "sftp-password"))
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.Equal(t, "SET", r.json["status"])
	assert.Equal(t, "vendor", r.json["key"])
	assert.Equal(t, true, r.json["schemaChecked"])
	assert.Equal(t, []string{"feed"}, r.strs("ingestions"))
	assert.Equal(t, "sftp.example.com", r.obj("value")["host"])

	// Another host for the same secret: new again.
	e.operator()
	r = e.putConfig(id, sftpEntry("other.example.com", "sftp-password"))
	assert.Equal(t, http.StatusForbidden, r.code, r.raw)
	assert.Equal(t, "INGESTION_BIND_SECRETS_PERMISSION_REQUIRED", r.errCode())
	assert.Equal(t, "sftp.example.com", e.do(http.MethodGet, configPath(id), nil).obj("value")["host"], "the refused write changed nothing")

	e.operator(permBind)
	r = e.putConfig(id, sftpEntry("other.example.com", "sftp-password"))
	require.Equal(t, http.StatusOK, r.code, r.raw)

	// A change that points nothing new needs no more than update.
	e.operator()
	changed := strings.Replace(sftpEntry("other.example.com", "sftp-password"), `"every":"PT5M"`, `"every":"PT10M"`, 1)
	r = e.putConfig(id, changed)
	require.Equal(t, http.StatusOK, r.code, r.raw)

	// Without update it is refused outright.
	e.as(permView)
	assert.Equal(t, http.StatusForbidden, e.putConfig(id, changed).code)
	assert.Equal(t, http.StatusForbidden, e.do(http.MethodDelete, configPath(id), nil).code)
}

// A runtime is given the effective source, the entry's settings in the shape
// an inline source has, with no config key. (A shared kind: a polled one is
// subject to the fair share of every runtime the other tests left live.)
func TestConfig_PollGivesTheRuntimeTheEffectiveSource(t *testing.T) {
	e := newConfigEnv(t)
	tg := e.makeTargets()
	e.operator(permBind, permIdentity)
	id := e.mustCreate("queue", `{"kind":"sqs","config":"vendor-queue"}`, tg.refTarget())
	rt := "rt-" + unique()
	assert.Empty(t, e.poll(rt, ingestion.ContractVersion).list("ingestions"), "while the entry is missing")

	require.Equal(t, http.StatusOK, e.putConfig(id, `{"region":"eu-west-1","queue":"orders","credential":{"accessKeyId":"AKIAEXAMPLE","secretAccessKey":{"name":"aws-secret"}},"targetUrl":"https://hooks.example.com/orders"}`).code)
	got := e.poll(rt, ingestion.ContractVersion).list("ingestions")
	require.Len(t, got, 1)
	assert.Equal(t, id, got[0]["id"])
	assert.Equal(t, map[string]any{
		"kind": "sqs", "region": "eu-west-1", "queue": "orders",
		"credential": map[string]any{"accessKeyId": "AKIAEXAMPLE", "secretAccessKey": map[string]any{"name": "aws-secret"}},
	}, got[0]["source"])
	assert.Equal(t, "https://hooks.example.com/orders", got[0]["target"].(map[string]any)["targetUrl"])

	// An entry that stops being valid takes the ingestion off the poll again
	// (here by deleting it).
	require.Equal(t, http.StatusOK, e.do(http.MethodDelete, configPath(id), nil).code)
	assert.Empty(t, e.poll(rt, ingestion.ContractVersion).list("ingestions"))
}

// An entry is shared by every ingestion of the application and client that
// names its key, so a value must suit each: the check names the one it does not
// suit, and nothing is written.
func TestConfig_PutIsCheckedAgainstEveryIngestionNamingTheKey(t *testing.T) {
	e := newConfigEnv(t)
	tg := e.makeTargets()
	e.operator(permBind, permIdentity)
	a := e.mustCreate("feed-a", `{"kind":"sftp","config":"vendor"}`, tg.refTarget())
	b := e.mustCreate("feed-b", `{"kind":"sftp","config":"vendor"}`, tg.refTarget())
	queue := e.mustCreate("queue-c", `{"kind":"sqs","config":"vendor"}`, tg.refTarget())

	// What suits an sftp source does not suit the sqs ingestion on the same key.
	r := e.putConfig(a, sftpEntry("sftp.example.com", "sftp-password"))
	assert.Equal(t, http.StatusBadRequest, r.code, r.raw)
	assert.Equal(t, "INGESTION_INVALID", r.errCode())
	assert.Contains(t, r.raw, "queue-c", "the problem names the ingestion")
	assert.NotContains(t, r.raw, "feed-a: ")
	assert.Equal(t, "MISSING", e.do(http.MethodGet, configPath(a), nil).json["status"], "nothing was written")

	// Remove the sqs ingestion from the key (a new version of it names another).
	other := e.createBody("queue-c", `{"kind":"sqs","config":"queue-key"}`, tg.refTarget())
	other["expectedVersion"] = 1
	require.Equal(t, http.StatusOK, e.do(http.MethodPut, "/api/ingestions/"+queue, other).code)

	r = e.putConfig(a, sftpEntry("sftp.example.com", "sftp-password"))
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.Equal(t, []string{"feed-a", "feed-b"}, r.strs("ingestions"), "every ingestion that names the key was checked")

	// The entry is read through either of them.
	assert.Equal(t, "SET", e.do(http.MethodGet, configPath(b), nil).json["status"])
	assert.Equal(t, "SET", e.do(http.MethodGet, "/api/ingestions/"+b, nil).obj("config")["status"])

	// A value valid for one is refused when the other does not suit it: here
	// the second ingestion's draft names an sftp setting the value now drops.
	// (Both are sftp, so the value must stay a valid sftp source.)
	r = e.putConfig(b, `{"host":"sftp.example.com","port":0,"targetUrl":"https://hooks.example.com/orders"}`)
	assert.Equal(t, http.StatusBadRequest, r.code, r.raw)
}

// An ingestion's settings can be inline or in an entry, never both, and the
// endpoints say so for one that holds them inline.
func TestConfig_InlineIngestionHasNoEntry(t *testing.T) {
	e := newConfigEnv(t)
	tg := e.makeTargets()
	e.operator(permBind, permIdentity)
	id := e.mustCreate("inline", sqsSource, tg.target(""))

	r := e.do(http.MethodGet, configPath(id), nil)
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.Equal(t, "INLINE", r.json["status"])
	assert.Equal(t, "INLINE", e.do(http.MethodGet, "/api/ingestions/"+id, nil).obj("config")["status"])

	r = e.putConfig(id, `{"region":"eu-west-1","queue":"q"}`)
	assert.Equal(t, http.StatusBadRequest, r.code, r.raw)
	assert.Equal(t, "INGESTION_CONFIG_NOT_NAMED", r.errCode())
	assert.Equal(t, http.StatusBadRequest, e.do(http.MethodDelete, configPath(id), nil).code)

	// A source that holds its settings and names an entry is refused.
	both := e.createBody("both", `{"kind":"sqs","config":"k","region":"eu-west-1","queue":"q"}`, tg.target(""))
	r = e.do(http.MethodPost, "/api/ingestions", both)
	assert.Equal(t, http.StatusBadRequest, r.code, r.raw)
	assert.Contains(t, r.raw, "source.config")
}

// The secrets call answers the secrets of the effective live source: a name the
// entry's value binds, and no other.
func TestConfig_SecretsCallAllowsOnlyNamesOfTheEffectiveLiveSource(t *testing.T) {
	e := newConfigEnv(t)
	tg := e.makeTargets()
	e.operator(permBind)
	id := e.mustCreate("feed", `{"kind":"sqs","config":"vendor"}`, tg.refTarget())
	cid := string(e.client.ID)
	e.setSecret("vendor-password", "pw-1", &cid)
	e.setSecret("other-password", "pw-2", &cid)
	rt := "rt-" + unique()

	// While the entry is missing the ingestion is not listed, and a runtime
	// that asks anyway is told the entry is not ready.
	r := e.do(http.MethodGet, secretPath(id, "vendor-password", rt), nil)
	assert.Equal(t, http.StatusConflict, r.code, r.raw)

	require.Equal(t, http.StatusOK, e.putConfig(id, sqsEntry("vendor-password")).code)
	require.Len(t, e.poll(rt, ingestion.ContractVersion).list("ingestions"), 1)

	r = e.do(http.MethodGet, secretPath(id, "vendor-password", rt), nil)
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.Equal(t, "pw-1", r.str("value"))
	r = e.do(http.MethodGet, secretPath(id, "other-password", rt), nil)
	assert.Equal(t, http.StatusNotFound, r.code, "a secret the effective source does not name: %s", r.raw)
	assert.Equal(t, "SECRET_NOT_FOUND", r.errCode())

	// The drawer's list of bound secrets is the effective source's too.
	r = e.do(http.MethodGet, "/api/ingestions/"+id+"/secrets", nil)
	require.Equal(t, http.StatusOK, r.code, r.raw)
	require.Len(t, r.list("secrets"), 1)
	assert.Equal(t, "vendor-password", r.list("secrets")[0]["name"])

	// The value changes which secret is bound; the old name stops answering.
	require.Equal(t, http.StatusOK, e.putConfig(id, sqsEntry("other-password")).code)
	assert.Equal(t, http.StatusNotFound, e.do(http.MethodGet, secretPath(id, "vendor-password", rt), nil).code)
	assert.Equal(t, "pw-2", e.do(http.MethodGet, secretPath(id, "other-password", rt), nil).str("value"))
}

// The jobs call builds jobs from the effective target: the delivery URL is the
// entry's when the definition leaves it out.
func TestConfig_JobsUseTheValuesTargetUrl(t *testing.T) {
	e := newConfigEnv(t)
	tg := e.makeTargets()
	e.operator(permBind, permIdentity)
	id := e.mustCreate("queue", `{"kind":"sqs","config":true}`, tg.refTarget())
	rt := "rt-" + unique()

	require.Equal(t, http.StatusOK, e.putConfig(id, `{"region":"eu-west-1","queue":"orders","targetUrl":"https://hooks.example.com/from-the-entry"}`).code)
	require.Len(t, e.poll(rt, ingestion.ContractVersion).list("ingestions"), 1)

	r := e.do(http.MethodPost, jobsPath(id, rt), map[string]any{"records": records(1, "cfg")})
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.EqualValues(t, 1, r.json["created"])
	j := e.job(r.list("results")[0]["id"].(string))
	assert.Equal(t, "https://hooks.example.com/from-the-entry", j.targetURL)
	assert.Equal(t, e.app.Code+":orders:order:received", j.code)

	// The delivery URL is the platform's outbound guard's to judge, as a
	// target's in the definition is.
	e.operator(permBind, permIdentity)
	r = e.putConfig(id, `{"region":"eu-west-1","queue":"orders","targetUrl":"http://169.254.169.254/latest"}`)
	assert.Equal(t, http.StatusBadRequest, r.code, r.raw)
	assert.Equal(t, "INGESTION_TARGET_INVALID", r.errCode())

	// The URL in the definition and in the entry: exactly one.
	both := e.mustCreate("both", `{"kind":"sqs","config":true}`, tg.target(""))
	r = e.putConfig(both, `{"region":"eu-west-1","queue":"orders","targetUrl":"https://hooks.example.com/two"}`)
	assert.Equal(t, http.StatusBadRequest, r.code, r.raw)
	assert.Contains(t, r.raw, "target.targetUrl")
}

// Deleting the entry makes every ingestion that names it wait for it again.
func TestConfig_DeleteMakesTheNamingIngestionsMissing(t *testing.T) {
	e := newConfigEnv(t)
	tg := e.makeTargets()
	e.operator(permBind, permIdentity)
	a := e.mustCreate("feed-a", `{"kind":"sqs","config":"shared"}`, tg.refTarget())
	b := e.mustCreate("feed-b", `{"kind":"sqs","config":"shared"}`, tg.refTarget())
	require.Equal(t, http.StatusOK, e.putConfig(a, `{"region":"eu-west-1","queue":"q","targetUrl":"https://hooks.example.com/x"}`).code)
	require.Len(t, e.poll("rt-"+unique(), ingestion.ContractVersion).list("ingestions"), 2)

	r := e.do(http.MethodDelete, configPath(b), nil)
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.Equal(t, "shared", r.json["key"])
	assert.Equal(t, []string{"feed-a", "feed-b"}, r.strs("ingestions"))
	assert.Empty(t, e.poll("rt-"+unique(), ingestion.ContractVersion).list("ingestions"))
	assert.Equal(t, "MISSING", e.do(http.MethodGet, "/api/ingestions/"+a, nil).obj("config")["status"])

	assert.Equal(t, http.StatusNotFound, e.do(http.MethodDelete, configPath(b), nil).code, "nothing left to delete")
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM msg_events WHERE subject = $1 AND type = 'platform:ingestion:ingestion:config-deleted'`, "platform.ingestion."+b))
}

// The write is audited on the ingestion and emits config-set; the value is in
// neither.
func TestConfig_PutIsAuditedOnTheIngestionAndEmitsAnEvent(t *testing.T) {
	e := newConfigEnv(t)
	tg := e.makeTargets()
	e.operator(permBind)
	id := e.mustCreate("feed", `{"kind":"sftp","config":"vendor"}`, tg.refTarget())
	require.Equal(t, http.StatusOK, e.putConfig(id, sftpEntry("sftp.example.com", "sftp-password")).code)

	subject := "platform.ingestion." + id
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM msg_events WHERE subject = $1 AND type = 'platform:ingestion:ingestion:config-set'`, subject))
	var data string
	require.NoError(t, e.pool.QueryRow(context.Background(), `SELECT data::text FROM msg_events WHERE subject = $1 AND type = 'platform:ingestion:ingestion:config-set'`, subject).Scan(&data))
	assert.Contains(t, data, `"key": "vendor"`)
	assert.NotContains(t, data, "sftp.example.com", "the event names the settings, not their values")

	r := e.do(http.MethodGet, "/api/ingestions/"+id+"/history", nil)
	require.Equal(t, http.StatusOK, r.code, r.raw)
	var found bool
	for _, entry := range r.list("entries") {
		if strings.Contains(fmt.Sprint(entry["operation"]), "SetConfigCommand") {
			found = true
			assert.NotContains(t, fmt.Sprint(entry["details"]), "sftp.example.com", "the audit row does not keep the value")
		}
	}
	assert.True(t, found, r.raw)
	assert.Equal(t, 0, e.count(`SELECT count(*) FROM ing_versions WHERE ingestion_id = $1 AND version > 1`, id), "an entry is not a version")
}

// Making a draft live is judged against the version live at that moment:
// a draft whose effective source points secrets somewhere new needs
// bind-secrets of whoever makes it live, not only of whoever saved it.
func TestMakeLive_JudgedOnTheEffectiveSourceAgainstWhatIsLiveNow(t *testing.T) {
	e := newConfigEnv(t)
	tg := e.makeTargets()
	e.operator(permBind, permIdentity)
	id := e.mustCreate("feed", `{"kind":"sftp","config":"vendor"}`, tg.refTarget())
	require.Equal(t, http.StatusOK, e.putConfig(id, sftpEntry("sftp.example.com", "sftp-password")).code)
	// A draft that names another entry, which points the secret elsewhere. The
	// entry is written through another ingestion that names it.
	helper := e.mustCreate("helper", `{"kind":"sftp","config":"vendor-2"}`, tg.refTarget())
	require.Equal(t, http.StatusOK, e.putConfig(helper, sftpEntry("elsewhere.example.com", "sftp-password")).code)
	draft := e.createBody("feed", `{"kind":"sftp","config":"vendor-2"}`, tg.refTarget())
	draft["live"] = false
	r := e.do(http.MethodPost, "/api/ingestions/"+id+"/versions", draft)
	require.Equal(t, http.StatusCreated, r.code, r.raw)
	version := int(r.json["version"].(float64))

	live := fmt.Sprintf("/api/ingestions/%s/versions/%d/live", id, version)
	e.as(permView, permPromote) // promote, but not bind-secrets
	r = e.do(http.MethodPost, live, nil)
	assert.Equal(t, http.StatusForbidden, r.code, r.raw)
	assert.Equal(t, "INGESTION_BIND_SECRETS_PERMISSION_REQUIRED", r.errCode())
	assert.Equal(t, 1, int(e.do(http.MethodGet, "/api/ingestions/"+id, nil).json["version"].(float64)), "still the first version")

	e.as(permView, permPromote, permBind)
	r = e.do(http.MethodPost, live, nil)
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.EqualValues(t, version, r.json["version"])
}

// A test of a saved ingestion is given the effective definition; so is a test
// of an unsaved one, from the entry read when it is queued. A missing entry is
// a 400, not a test no runtime could run.
func TestConfig_TestsAreQueuedWithTheEffectiveDefinition(t *testing.T) {
	e := newConfigEnv(t)
	tg := e.makeTargets()
	e.operator(permBind)
	id := e.mustCreate("feed", `{"kind":"sftp","config":"vendor"}`, tg.refTarget())

	r := e.do(http.MethodPost, "/api/ingestion-tests", map[string]any{"ingestionId": id})
	assert.Equal(t, http.StatusBadRequest, r.code, r.raw)
	assert.Equal(t, "INGESTION_CONFIG_MISSING", r.errCode())

	unsaved := e.createBody("unsaved", `{"kind":"sftp","config":"vendor"}`, tg.refTarget())
	delete(unsaved, "application")
	delete(unsaved, "client")
	delete(unsaved, "code")
	body := map[string]any{"application": e.app.Code, "client": e.client.Identifier, "definition": unsaved}
	r = e.do(http.MethodPost, "/api/ingestion-tests", body)
	assert.Equal(t, http.StatusBadRequest, r.code, r.raw)
	assert.Equal(t, "INGESTION_CONFIG_MISSING", r.errCode())

	require.Equal(t, http.StatusOK, e.putConfig(id, sftpEntry("sftp.example.com", "sftp-password")).code)

	for name, body := range map[string]map[string]any{
		"saved":   {"ingestionId": id},
		"unsaved": body,
	} {
		r = e.do(http.MethodPost, "/api/ingestion-tests", body)
		require.Equal(t, http.StatusAccepted, r.code, "%s: %s", name, r.raw)
		var def string
		require.NoError(t, e.pool.QueryRow(context.Background(), `SELECT definition::text FROM ing_test_requests WHERE id = $1`, r.str("id")).Scan(&def))
		assert.Contains(t, def, `"host": "sftp.example.com"`, "%s: the request holds the effective definition", name)
		assert.NotContains(t, def, "config", name)
		assert.Contains(t, def, "https://hooks.example.com/orders", name)
	}

	// An unsaved test is judged as a save would be: bind-secrets for the secret
	// the entry points at a host.
	e.operator()
	r = e.do(http.MethodPost, "/api/ingestion-tests", body)
	assert.Equal(t, http.StatusForbidden, r.code, r.raw)
	assert.Equal(t, "INGESTION_BIND_SECRETS_PERMISSION_REQUIRED", r.errCode())

	// config: true in an unsaved definition needs the code.
	own := e.createBody("unsaved", `{"kind":"sftp","config":true}`, tg.refTarget())
	delete(own, "application")
	delete(own, "client")
	delete(own, "code")
	e.operator(permBind)
	r = e.do(http.MethodPost, "/api/ingestion-tests", map[string]any{"application": e.app.Code, "client": e.client.Identifier, "definition": own})
	assert.Equal(t, http.StatusBadRequest, r.code, r.raw)
	assert.Equal(t, "INGESTION_CONFIG_KEY_UNKNOWN", r.errCode())
}

// An extension kind's value is checked against the field list a live runtime
// reports for it; with none reporting the kind it is stored with a warning.
func TestConfig_ExtensionKindSchemaIsCheckedWhileARuntimeReportsIt(t *testing.T) {
	e := newConfigEnv(t)
	tg := e.makeTargets()
	e.operator(permBind)
	kind := "acme-" + unique()
	id := e.mustCreate("erp", `{"kind":"custom","type":"`+kind+`","config":"erp"}`, tg.refTarget())

	// Nobody reports the kind: accepted, unchecked, and said so.
	r := e.putConfig(id, `{"baseUrl":"https://erp.example.com","count":"not a number","targetUrl":"https://hooks.example.com/orders"}`)
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.Equal(t, false, r.json["schemaChecked"])
	assert.Contains(t, strings.Join(r.strs("warnings"), "\n"), "SCHEMA_UNCHECKED")

	// A runtime reports the kind with its fields.
	fields := []map[string]any{
		{"name": "baseUrl", "label": "Base URL", "type": "URL", "required": true},
		{"name": "count", "label": "Count", "type": "INTEGER", "min": "1", "max": "10"},
		{"name": "mode", "label": "Mode", "type": "CHOICE", "choices": []string{"fast", "slow"}, "defaultValue": "fast"},
	}
	rt := "rt-" + unique()
	require.Equal(t, http.StatusOK, e.poll(rt, ingestion.ContractVersion,
		map[string]any{"kind": kind, "description": "ERP", "settings": fields}).code)

	assert.Equal(t, true, e.do(http.MethodGet, configPath(id), nil).json["schemaChecked"])

	r = e.putConfig(id, `{"count":11,"mood":"x","targetUrl":"https://hooks.example.com/orders"}`)
	assert.Equal(t, http.StatusBadRequest, r.code, r.raw)
	for _, want := range []string{"config.baseUrl", "config.count", "config.mood"} {
		assert.Contains(t, r.raw, want)
	}
	assert.Contains(t, r.raw, "ingestion erp")

	r = e.putConfig(id, `{"baseUrl":"https://erp.example.com","count":3,"mode":"slow","targetUrl":"https://hooks.example.com/orders"}`)
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.Equal(t, true, r.json["schemaChecked"])
	assert.NotContains(t, strings.Join(r.strs("warnings"), "\n"), "SCHEMA_UNCHECKED")

	// The effective source is what a runtime is given.
	got := e.poll(rt, ingestion.ContractVersion, map[string]any{"kind": kind, "settings": fields}).list("ingestions")
	require.Len(t, got, 1)
	src := got[0]["source"].(map[string]any)
	assert.Equal(t, "custom", src["kind"])
	assert.Equal(t, kind, src["type"])
	assert.Equal(t, "https://erp.example.com", src["settings"].(map[string]any)["baseUrl"])
	assert.NotContains(t, src, "config")
}

// The generic config surfaces cannot write section ingest, and show it
// read-only (reads are as for any section).
func TestConfig_GenericSetPropertyRefusesTheSection(t *testing.T) {
	e := newConfigEnv(t)
	tg := e.makeTargets()
	e.operator(permBind)
	id := e.mustCreate("feed", `{"kind":"sftp","config":"vendor"}`, tg.refTarget())
	require.Equal(t, http.StatusOK, e.putConfig(id, sftpEntry("sftp.example.com", "sftp-password")).code)

	admin := testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_optestrunner1", Scope: auth.ScopeAnchor, AllApplications: true, Permissions: []string{"platform:*:*:*"},
	})
	_, err := usecaseop.Run(admin, testpg.NewUoW(t), platformconfigops.SetProperty(e.config.Repo), platformconfigops.SetPropertyCommand{
		ApplicationCode: e.app.Code, Section: "ingest", Property: "vendor", Value: `{}`, ClientID: new(string(e.client.ID)),
	}, testpg.TestEC())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CONFIG_SECTION_RESERVED")

	// The entry is unchanged and readable through the config service.
	cid := string(e.client.ID)
	got, err := e.config.Repo.FindByCoordinate(context.Background(), e.app.Code, "ingest", "vendor", platformconfig.ScopeClient, &cid)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Contains(t, got.Value, "sftp.example.com")
}

// A set owns its ingestions' definitions, not the environment's values: the
// entry is written through a set-owned ingestion with the same permission as
// through any other, while its definition stays read-only.
func TestConfig_PutWorksOnAnIngestionASetOwns(t *testing.T) {
	e := newConfigEnv(t)
	tg := e.makeTargets()
	e.operator(permBind)
	id := e.mustCreate("owned", `{"kind":"sftp","config":"vendor"}`, tg.refTarget())
	set := "set_" + unique()[:12]
	_, err := e.pool.Exec(context.Background(), `INSERT INTO ing_sets (id, name, created_at, updated_at) VALUES ($1, $2, now(), now())`, set, "owner-"+unique()[:8])
	require.NoError(t, err)
	_, err = e.pool.Exec(context.Background(), `UPDATE ing_ingestions SET managed_by = $1 WHERE id = $2`, set, id)
	require.NoError(t, err)

	upd := e.createBody("owned", `{"kind":"sftp","config":"other"}`, tg.refTarget())
	upd["expectedVersion"] = 1
	r := e.do(http.MethodPut, "/api/ingestions/"+id, upd)
	require.Equal(t, http.StatusConflict, r.code, r.raw)
	assert.Equal(t, "INGESTION_SET_OWNED", r.errCode(), "its definition is the set's")

	r = e.putConfig(id, sftpEntry("sftp.example.com", "sftp-password"))
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.Equal(t, http.StatusOK, e.do(http.MethodDelete, configPath(id), nil).code)
}
