//go:build integration

package pages

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// The environment's settings against Postgres: the page writes the entry
// through the API's use case, so the platform config row, the checks against
// every ingestion that names it and the status the drawer reads are the real
// ones.
func TestIngestionSettingsPerEnvironmentAgainstPostgres(t *testing.T) {
	p := newIngPG(t)
	ctx := context.Background()
	admin := ingAnchor(ingPermView, ingPermCreate, ingPermUpdate, ingPermDelete, ingPermPromote, ingPermSecrets, ingPermIdentity)
	configs := platformconfig.NewRepository(testpg.Pool(t))

	sftpForm := func(code string, key string) url.Values {
		f := p.form(code)
		for _, k := range []string{"http.auth", "http.token", "split.arrayPath"} {
			f.Del(k)
		}
		f.Set("kind", "sftp")
		f.Set("settings", "config")
		f.Set("configKey", key)
		f.Set("split.kind", "csv")
		// The delivery URL is the environment's to give.
		f.Set("target.targetUrl", "")
		return f
	}
	code := "vendor-" + ingUnique()

	// The definition holds a reference and no settings; the entry is not there yet.
	res := p.post(admin, "/ui/ingestions/new", sftpForm(code, ""))
	require.Equal(t, http.StatusOK, res.code, res.body)
	assert.Contains(t, res.body, "Ingestion created")
	i := p.find(code)
	ref, ok := ingestion.ConfigOf(i.Definition)
	require.True(t, ok, "%T", i.Definition.Source)
	assert.True(t, ref.Config.Own)
	drawer := p.do(admin, req{target: "/ui/ingestions/" + i.ID, datastar: true})
	assert.Contains(t, drawer.body, "Needs settings")
	assert.Contains(t, drawer.body, "config ingest/"+code+" is not set for client Acme")
	assert.Contains(t, p.do(admin, req{target: "/ui/ingestions?q=" + code}).body, `data-testid="ings-needs-badge"`)

	// The settings form is drawn from the kind; a value that is not valid is told on its field, nothing is stored.
	form := p.do(admin, req{target: "/ui/ingestions/" + i.ID + "/config", datastar: true})
	require.Equal(t, http.StatusOK, form.code, form.body)
	assert.Contains(t, form.body, `name="sftp.host"`)
	assert.Contains(t, form.body, `name="config.targetUrl"`)
	values := url.Values{
		"sftp.host": {"169.254.169.254"}, "sftp.port": {"22"}, "sftp.directory": {"/out"}, "sftp.username": {"svc"},
		"sftp.credential": {"vendor-pass"}, "sftp.knownHosts": {ingCfgKnownHost}, "sftp.every": {"1m"},
		"config.targetUrl": {"https://app.example.com/hooks/vendor"}, "drawer": {i.ID},
	}
	refused := p.post(admin, "/ui/ingestions/"+i.ID+"/config", values)
	assert.Contains(t, refused.body, `id="ings-config-sftp-host-err"`)
	entry, err := p.repo.ConfigEntry(ctx, p.app.Code, p.client.ID, code)
	require.NoError(t, err)
	assert.Nil(t, entry)

	// A valid value is stored as platform config ingest/<code> at the client, and the drawer reads it back.
	values.Set("sftp.host", "sftp.vendor.example")
	saved := p.post(admin, "/ui/ingestions/"+i.ID+"/config", values)
	require.Equal(t, http.StatusOK, saved.code, saved.body)
	assert.Contains(t, saved.body, "Settings saved for this environment")
	assert.Contains(t, saved.body, "sftp.vendor.example")
	assert.Contains(t, saved.body, "vendor-pass")
	assert.NotContains(t, saved.body, `id="ings-needs-settings"`)
	row, err := configs.FindByCoordinate(ctx, p.app.Code, ingestion.ConfigSection, code, platformconfig.ScopeClient, new(string(p.client.ID)))
	require.NoError(t, err)
	require.NotNil(t, row)
	assert.Equal(t, platformconfig.ValuePlain, row.ValueType)
	assert.Contains(t, row.Value, `"host":"sftp.vendor.example"`)
	assert.Contains(t, p.do(admin, req{target: "/ui/ingestions/" + i.ID, datastar: true}).body, ">Set<")

	// Someone who can only view sees that the entry is set, not what it holds.
	seen := p.do(ingAnchor(ingPermView), req{target: "/ui/ingestions/" + i.ID, datastar: true})
	assert.Contains(t, seen.body, `id="ings-config-hidden"`)
	// (The drawer's overview still shows the effective definition; hiding that is not this section's.)
	assert.NotContains(t, seen.body, `id="ings-config-values"`)

	// Deleting names who stops, and the ingestion waits again.
	del := p.post(admin, "/ui/ingestions/"+i.ID+"/config/delete", url.Values{"drawer": {i.ID}})
	assert.Contains(t, del.body, "Waiting for settings: "+code)
	entry, err = p.repo.ConfigEntry(ctx, p.app.Code, p.client.ID, code)
	require.NoError(t, err)
	assert.Nil(t, entry)
	assert.Contains(t, p.do(admin, req{target: "/ui/ingestions/" + i.ID, datastar: true}).body, "Needs settings")

	// The entry needs the update permission; a viewer is refused and nothing is written.
	viewer := ingAnchor(ingPermView)
	assert.Contains(t, p.post(viewer, "/ui/ingestions/"+i.ID+"/config", values).body, "ermission required")
	entry, err = p.repo.ConfigEntry(ctx, p.app.Code, p.client.ID, code)
	require.NoError(t, err)
	assert.Nil(t, entry)
}

// An ingestion a set owns keeps its definition read-only, but its entry is the
// environment's part: the update permission writes it.
func TestIngestionSettingsOfASetsIngestionAgainstPostgres(t *testing.T) {
	p := newIngPG(t)
	ctx := context.Background()
	admin := ingAnchor(ingPermView, ingPermCreate, ingPermUpdate, ingPermDelete, ingPermPromote, ingPermSecrets, ingPermIdentity)
	code := "owned-cfg-" + ingUnique()
	f := p.form(code)
	f.Set("settings", "config")
	f.Set("configKey", "")
	require.Equal(t, http.StatusOK, p.post(admin, "/ui/ingestions/new", f).code)
	i := p.find(code)

	setID := tsid.Generate(tsid.IngestionSet)
	pool := testpg.Pool(t)
	now := time.Now().UTC()
	_, err := pool.Exec(ctx, `INSERT INTO ing_sets (id, name, created_at, updated_at) VALUES ($1, $2, $3, $3)`, setID, "set-"+ingUnique(), now)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE ing_ingestions SET managed_by = $2 WHERE id = $1`, i.ID, setID)
	require.NoError(t, err)

	updater := ingAnchor(ingPermView, ingPermUpdate, ingPermSecrets)
	assert.Contains(t, p.do(updater, req{target: "/ui/ingestions/" + i.ID + "/config", datastar: true}).body, `name="http.auth"`)
	saved := p.post(updater, "/ui/ingestions/"+i.ID+"/config", url.Values{
		"http.auth": {"bearer"}, "http.token": {"push-token"}, "config.targetUrl": {"https://app.example.com/hooks/x"}, "drawer": {i.ID},
	})
	assert.Contains(t, saved.body, "Settings saved for this environment")
	entry, err := p.repo.ConfigEntry(ctx, p.app.Code, p.client.ID, code)
	require.NoError(t, err)
	require.NotNil(t, entry)
}

// The generic Configuration page cannot create a property in section ingest:
// the platform refuses it (CONFIG_SECTION_RESERVED) and the page says so on the
// section field, and an entry written through an ingestion is listed read-only.
func TestConfigPageRefusesTheIngestSectionAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	uow := testpg.NewUoW(t)
	repo := platformconfig.NewRepository(pool)
	const app = "uicfgingest"
	testpg.EnsureApplication(t, app)
	page := &PlatformConfig{Config: &fakeConfig{}, Ops: ConfigOps{Repo: repo, UoW: uow}, Apps: fakeConfigApps{}, Shell: shellForTest()}
	admin := cfgAdmin()
	admin.Permissions = []string{"platform:*:*:*"}
	form := url.Values{"app": {app}, "section": {"Ingest"}, "property": {"sneaky"}, "client": {""}, "valueType": {"PLAIN"}, "plainValue": {"{}"}, "description": {""}}
	rec := serveMounted(t, admin, req{method: http.MethodPost, target: "/ui/platform/config/save", datastar: true, multipart: true, form: form}, page)
	assert.Contains(t, rec.Body.String(), `id="cfg-form-section-err"`)
	assert.Contains(t, rec.Body.String(), "holds ingestion settings, which are changed on the ingestion")
	c, err := repo.FindByCoordinate(ctx, app, "Ingest", "sneaky", platformconfig.ScopeGlobal, nil)
	require.NoError(t, err)
	assert.Nil(t, c, "nothing was written")
}
