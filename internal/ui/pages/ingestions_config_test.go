package pages

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	ingops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// The environment's settings of an ingestion (docs/ingestion-design.md section
// 13): the source's choice in the form, the drawer's section, the settings
// form and the platform config entries it writes.

// ── fakes ──────────────────────────────────────────────────────────────

// ingsEntryKey addresses an entry in the fakes: application code, client id,
// key.
func ingsEntryKey(app string, client ids.ClientID, key string) string {
	return app + "/" + string(client) + "/" + key
}

func (f *fakeIngStore) Resolve(_ context.Context, app string, c ids.ClientID, code string, def ingestion.Definition) (ingestion.Resolved, error) {
	if f.err != nil {
		return ingestion.Resolved{}, f.err
	}
	key := ingestion.ConfigKeyOf(def, code)
	return ingestion.ApplyConfig(def, code, f.entries[ingsEntryKey(app, c, key)]), nil
}

func (f *fakeIngStore) ConfigNamed(_ context.Context, i *ingestion.Ingestion) (ingestion.Definition, string, bool, error) {
	if k := ingestion.ConfigKeyOf(i.Definition, i.Code); k != "" {
		return i.Definition, k, true, nil
	}
	if i.LatestVersion > i.Version {
		for _, v := range f.versions[i.ID] {
			if v.Number == i.LatestVersion {
				if k := ingestion.ConfigKeyOf(v.Definition, i.Code); k != "" {
					return v.Definition, k, true, nil
				}
			}
		}
	}
	return ingestion.Definition{}, "", false, nil
}

func (f *fakeIngStore) ConfigEntry(_ context.Context, app string, c ids.ClientID, key string) (*ingestion.ConfigEntry, error) {
	return f.entries[ingsEntryKey(app, c, key)], f.err
}

func (f *fakeIngStore) SchemaOf(_ context.Context, kind string) (ingestion.KindSchema, error) {
	for _, k := range f.kinds {
		if k.Kind == kind {
			if fields, ok := ingestion.ParseSettingFields(k.Settings); ok {
				return ingestion.KindSchema{Fields: fields, Checked: true}, f.err
			}
		}
	}
	return ingestion.KindSchema{}, f.err
}

func (f *fakeIngStore) NamingConfig(ctx context.Context, appID ids.ApplicationID, c ids.ClientID, key string) ([]*ingestion.Ingestion, error) {
	var out []*ingestion.Ingestion
	for n := range f.items {
		it := f.items[n].Ingestion
		if it.ApplicationID != appID || it.ClientID != c {
			continue
		}
		if def, k, ok, _ := f.ConfigNamed(ctx, &it); ok && k == key && def.Source != nil {
			out = append(out, &it)
		}
	}
	return out, f.err
}

// SetConfig answers as the use case does for what the page can show: the value
// is held against the ingestions that name the entry (ApplyConfig, and the
// kind's schema for an extension), and is stored when it passes.
func (o *fakeIngOps) SetConfig(ctx context.Context, cmd ingops.SetConfigCommand) (ingops.IngestionConfigSet, error) {
	o.rec("config:" + cmd.ID)
	o.configs = append(o.configs, cmd)
	if o.configErr != nil {
		return ingops.IngestionConfigSet{}, o.configErr
	}
	i, _ := o.store.FindByID(ctx, cmd.ID)
	def, key, _, _ := o.store.ConfigNamed(ctx, i)
	raw, _ := json.Marshal(cmd.Value)
	entry := &ingestion.ConfigEntry{Value: string(raw), UpdatedAt: time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC)}
	res := ingestion.ApplyConfig(def, i.Code, entry)
	if res.Status == ingestion.ConfigInvalid {
		return ingops.IngestionConfigSet{}, res.Problems.Err()
	}
	if c, ok := ingestion.ConfigOf(def); ok && c.Base == ingestion.KindCustom {
		if schema, _ := o.store.SchemaOf(ctx, c.Type); schema.Checked {
			settings, _ := res.Definition.Source.(ingestion.Custom)
			if p := ingestion.CheckSettings(schema.Fields, settings.Settings); len(p) > 0 {
				// As the use case names the ingestion each problem is about.
				for n := range p {
					p[n].Message = "ingestion " + i.Code + ": " + p[n].Message
				}
				return ingops.IngestionConfigSet{}, p.Err()
			}
		}
	}
	if o.store.entries == nil {
		o.store.entries = map[string]*ingestion.ConfigEntry{}
	}
	app := ""
	for _, it := range o.store.items {
		if it.ID == i.ID {
			app = it.ApplicationCode
		}
	}
	o.store.entries[ingsEntryKey(app, i.ClientID, key)] = entry
	names, _ := o.store.NamingConfig(ctx, i.ApplicationID, i.ClientID, key)
	var codes []string
	for _, n := range names {
		codes = append(codes, n.Code)
	}
	return ingops.IngestionConfigSet{Code: i.Code, Key: key, Ingestions: codes, Warnings: o.configWarnings}, nil
}

func (o *fakeIngOps) DeleteConfig(ctx context.Context, id string) (ingops.IngestionConfigDeleted, error) {
	o.rec("config-delete:" + id)
	o.deleted = append(o.deleted, id)
	if o.configErr != nil {
		return ingops.IngestionConfigDeleted{}, o.configErr
	}
	i, _ := o.store.FindByID(ctx, id)
	_, key, _, _ := o.store.ConfigNamed(ctx, i)
	app := ""
	for _, it := range o.store.items {
		if it.ID == i.ID {
			app = it.ApplicationCode
		}
	}
	delete(o.store.entries, ingsEntryKey(app, i.ClientID, key))
	names, _ := o.store.NamingConfig(ctx, i.ApplicationID, i.ClientID, key)
	var codes []string
	for _, n := range names {
		codes = append(codes, n.Code)
	}
	return ingops.IngestionConfigDeleted{Code: i.Code, Key: key, Ingestions: codes}, nil
}

// ── fixtures ───────────────────────────────────────────────────────────

const ingCfgKnownHost = "sftp.vendor.example ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExample"

// ingCfgSftp is an SFTP source whose settings are per environment: `config:
// true`, the ingestion's own code.
func ingCfgSftp(def ingestion.Definition, key ingestion.ConfigRef) ingestion.Definition {
	def.Name = "Vendor feed"
	def.Source = ingestion.Configured{Base: ingestion.KindSFTP, Config: key}
	def.Split = ingestion.CSV{Header: true}
	return def
}

func ingCfgValidSftp() map[string]any {
	return map[string]any{
		"host": "sftp.vendor.example", "port": 22, "directory": "/out", "username": "svc",
		"credential": map[string]any{"name": "vendor-sftp-pass"}, "knownHosts": ingCfgKnownHost, "every": "PT1M",
	}
}

// ingCfgEntry is an entry as it is stored.
func ingCfgEntry(v map[string]any) *ingestion.ConfigEntry {
	raw, _ := json.Marshal(v)
	return &ingestion.ConfigEntry{Value: string(raw), UpdatedAt: time.Date(2026, 10, 25, 8, 30, 0, 0, time.UTC)}
}

// ingCfgFixture adds to the standard fixture:
//
//	ing_6  vendor-feed   sftp, config: true; its entry is SET
//	ing_7  vendor-feed-2 sftp, config: shared-vendor, delivery URL in the entry
//	ing_8  vendor-feed-3 sftp, config: shared-vendor, MISSING for client globex
//	ing_9  mix-cfg       extension "mix", config: mix-positions (no entry)
//	ing_10 set-cfg       sftp, config: true, owned by a set
func ingCfgFixture() *fakeIngStore {
	store := ingFixture()
	base := ingDef("x", nil, nil)
	noURL := base
	noURL.Target.TargetURL = ""
	mix := ingCfgSftp(noURL, ingestion.ConfigRef{Key: "mix-positions"})
	mix.Name = "MiX positions"
	mix.Source = ingestion.Configured{Base: ingestion.KindCustom, Type: "mix", Config: ingestion.ConfigRef{Key: "mix-positions"}}
	mix.Split = ingestion.None{}
	set := "ingset_1"
	owned := ingSample("ing_10", "set-cfg", "orders", "acme", ingCfgSftp(base, ingestion.ConfigRef{Own: true}), true, 1, 1)
	owned.ManagedBy = &set
	for _, it := range []ingestion.ListItem{
		ingSample("ing_6", "vendor-feed", "orders", "acme", ingCfgSftp(base, ingestion.ConfigRef{Own: true}), true, 1, 1),
		ingSample("ing_7", "vendor-feed-2", "orders", "acme", ingCfgSftp(noURL, ingestion.ConfigRef{Key: "shared-vendor"}), true, 1, 1),
		ingSample("ing_8", "vendor-feed-3", "orders", "acme", ingCfgSftp(base, ingestion.ConfigRef{Key: "shared-vendor"}), true, 1, 1),
		ingSample("ing_9", "mix-cfg", "orders", "acme", mix, true, 1, 1),
		owned,
	} {
		store.items = append(store.items, it)
		store.versions[it.ID] = []ingestion.Version{ingVersion(it.ID, 1, it.Definition, "")}
	}
	store.carried = append(store.carried, "mix")
	store.entries = map[string]*ingestion.ConfigEntry{
		ingsEntryKey("orders-sync", "clt_acme", "vendor-feed"): ingCfgEntry(ingCfgValidSftp()),
		ingsEntryKey("orders-sync", "clt_acme", "shared-vendor"): ingCfgEntry(func() map[string]any {
			v := ingCfgValidSftp()
			v["targetUrl"] = "https://vendor.example/hook"
			return v
		}()),
	}
	return store
}

func ingCfgHarness() *ingHarness {
	store := ingCfgFixture()
	return &ingHarness{store: store, ops: &fakeIngOps{store: store}}
}

const ingMixSettings = `[{"name":"region","label":"Region","type":"CHOICE","required":true,"choices":["eu","us"]},` +
	`{"name":"token","label":"API token","type":"SECRET","required":true},{"name":"batch","type":"INTEGER","min":"1","max":"10"}]`

func ingCfgReportMix(h *ingHarness) {
	h.store.kinds = append(h.store.kinds, ingestion.RuntimeKind{Kind: "mix", Description: "MiX Telematics", Settings: json.RawMessage(ingMixSettings)})
}

func ingCfgDrawer(t *testing.T, h *ingHarness, ac *auth.AuthContext, id string) string {
	t.Helper()
	return ingBody(t, h.do(t, ac, req{target: "/ui/ingestions/" + id, datastar: true}))
}

// ── the definition form ────────────────────────────────────────────────

func TestIngestionFormOffersSettingsPerEnvironment(t *testing.T) {
	h := newIngHarness()
	body := ingBody(t, h.do(t, ingAdmin(), req{target: "/ui/ingestions/new", datastar: true}))
	mustContain(t, body,
		`name="settings"`, `Settings in this definition`, `Settings per environment (platform config)`,
		`data-bind:ing-settings`, `name="configKey"`, `placeholder="the ingestion&#39;s code"`,
		// The kind's fields apply only while the settings are inline.
		`data-show="$ingSettings === &#39;inline&#39;"`, `data-show="$ingSettings === &#39;config&#39;"`,
		// The delivery URL says it may be given in the environment instead.
		`this may be empty and given in the environment&#39;s settings instead`,
	)
}

func TestIngestionCreateWithSettingsPerEnvironment(t *testing.T) {
	create := func(t *testing.T, mutate func(url.Values)) (*ingHarness, string) {
		h := newIngHarness()
		form := ingCreateForm()
		form.Set("settings", "config")
		mutate(form)
		return h, ingBody(t, ingPost(h, t, ingAdmin(), "/ui/ingestions/new", form))
	}
	source := func(t *testing.T, h *ingHarness) string {
		t.Helper()
		require.Len(t, h.ops.created, 1)
		raw, err := json.Marshal(h.ops.created[0].Definition)
		require.NoError(t, err)
		var doc map[string]any
		require.NoError(t, json.Unmarshal(raw, &doc))
		out, _ := json.Marshal(doc["source"])
		return string(out)
	}

	t.Run("an empty key is the ingestion's own code", func(t *testing.T) {
		// The posted bearer token is the inline source's field: it is not read.
		h, _ := create(t, func(url.Values) {})
		assert.JSONEq(t, `{"kind":"http","config":true}`, source(t, h))
	})
	t.Run("an explicit key", func(t *testing.T) {
		h, _ := create(t, func(f url.Values) { f.Set("configKey", " shared-vendor ") })
		assert.JSONEq(t, `{"kind":"http","config":"shared-vendor"}`, source(t, h))
	})
	t.Run("an extension kind", func(t *testing.T) {
		h := newIngHarness()
		ingCfgReportMix(&ingHarness{store: h.store})
		form := ingCreateForm()
		form.Set("kind", "mix")
		form.Set("settings", "config")
		form.Set("configKey", "mix-positions")
		ingBody(t, ingPost(h, t, ingAdmin(), "/ui/ingestions/new", form))
		assert.JSONEq(t, `{"kind":"custom","type":"mix","config":"mix-positions"}`, source(t, h))
	})
	t.Run("the delivery URL may be left for the environment", func(t *testing.T) {
		h, body := create(t, func(f url.Values) { f.Set("target.targetUrl", "") })
		require.Len(t, h.ops.created, 1, body)
		assert.Empty(t, h.ops.created[0].Definition.Target.TargetURL)
	})
	t.Run("an inline source still needs its delivery URL", func(t *testing.T) {
		h := newIngHarness()
		form := ingCreateForm()
		form.Set("target.targetUrl", "")
		body := ingPost(h, t, ingAdmin(), "/ui/ingestions/new", form).Body.String()
		mustContain(t, body, `id="ings-form-target-target-url-err"`, `Must be an http or https address`)
	})
	t.Run("a bad key lands on the key", func(t *testing.T) {
		_, body := create(t, func(f url.Values) { f.Set("configKey", "Not A Key") })
		mustContain(t, body, `id="ings-form-config-key-err"`, `Must be true (the ingestion&#39;s own code) or a key`)
	})
}

func TestIngestionConfiguredSourceReadsBackIntoTheForm(t *testing.T) {
	cat := ingsNewCatalogue([]ingestion.RuntimeKind{{Kind: "mix"}})
	own, err := ingsValuesOf(ingCfgSftp(ingDef("x", nil, nil), ingestion.ConfigRef{Own: true}), cat)
	require.NoError(t, err)
	assert.Equal(t, "config", own["settings"])
	assert.Equal(t, "sftp", own["kind"])
	assert.Empty(t, own["configKey"], "config: true is the ingestion's own code")

	keyed, err := ingsValuesOf(ingCfgSftp(ingDef("x", nil, nil), ingestion.ConfigRef{Key: "shared-vendor"}), cat)
	require.NoError(t, err)
	assert.Equal(t, "shared-vendor", keyed["configKey"])

	ext := ingDef("x", ingestion.Configured{Base: ingestion.KindCustom, Type: "mix", Config: ingestion.ConfigRef{Key: "mix-positions"}}, ingestion.None{})
	values, err := ingsValuesOf(ext, cat)
	require.NoError(t, err)
	assert.Equal(t, "mix", values["kind"])
	assert.Equal(t, "mix-positions", values["configKey"])

	inline, err := ingsValuesOf(ingDef("x", ingestion.Sqs{Region: "eu-west-1", Queue: "q"}, ingestion.None{}), cat)
	require.NoError(t, err)
	assert.Equal(t, "inline", inline["settings"])

	// And the build is the inverse.
	built := ingsBuildDefinition(keyed, cat)
	assert.Empty(t, built.Errs)
	src, _ := json.Marshal(built.Doc["source"])
	assert.JSONEq(t, `{"kind":"sftp","config":"shared-vendor"}`, string(src))
}

func TestIngestionEditFormOfAConfiguredSource(t *testing.T) {
	h := ingCfgHarness()
	body := ingBody(t, h.do(t, ingAdmin(), req{target: "/ui/ingestions/ing_8/edit", datastar: true}))
	mustContain(t, body, `<option value="config" selected>`, `value="shared-vendor"`)
}

func TestIngestionExportKeepsTheReference(t *testing.T) {
	h := ingCfgHarness()
	rec := h.do(t, ingAdmin(), req{target: "/ui/ingestions/ing_6/definition.yaml"})
	body := ingBody(t, rec)
	mustContain(t, body, "kind: sftp", "config: true")
	mustNotContain(t, body, "sftp.vendor.example", "vendor-sftp-pass")
	keyed := ingBody(t, h.do(t, ingAdmin(), req{target: "/ui/ingestions/ing_8/definition.yaml"}))
	mustContain(t, keyed, "config: shared-vendor")
	// The drawer's Definition tab shows the file, the reference too.
	mustContain(t, ingCfgDrawer(t, h, ingAdmin(), "ing_6"), `config: true`)
}

// ── the list and the drawer ────────────────────────────────────────────

func TestIngestionSettingsStates(t *testing.T) {
	t.Run("set: the effective values, the secret by name", func(t *testing.T) {
		h := ingCfgHarness()
		body := ingCfgDrawer(t, h, ingAdmin(), "ing_6")
		mustContain(t, body,
			`Settings (this environment)`, `id="ings-config"`, `>Set<`, `ingest/vendor-feed`,
			`sftp.vendor.example`, `/out`, `<code title="The secret's name, never its value">vendor-sftp-pass</code>`,
			// The effective source is where the drawer says it points, and its secret is in the Secrets section.
			`svc@sftp.vendor.example:22/out`, `id="ings-secret-vendor-sftp-pass"`,
			`href="/ui/ingestions/ing_6/config"`, `Edit…`, `/ui/ingestions/ing_6/config/delete`,
		)
		mustNotContain(t, body, `id="ings-needs-settings"`, `Needs settings`)
	})
	t.Run("not set: needs settings, with the reason", func(t *testing.T) {
		h := ingCfgHarness()
		delete(h.store.entries, ingsEntryKey("orders-sync", "clt_acme", "vendor-feed"))
		body := ingCfgDrawer(t, h, ingAdmin(), "ing_6")
		mustContain(t, body,
			`id="ings-needs-settings"`, `Needs settings`, `config ingest/vendor-feed is not set for client Acme Corp`,
			`>Not set<`, `Nothing is set for this client in this environment.`, `Set…`, `Settings in config ingest/vendor-feed`,
		)
		mustNotContain(t, body, `/config/delete`, `id="ings-secret-`)
		list := ingBody(t, h.do(t, ingAdmin(), req{target: "/ui/ingestions"}))
		mustContain(t, list, `data-testid="ings-needs-badge"`, `config ingest/vendor-feed is not set for client Acme Corp`)
	})
	t.Run("invalid: the problems", func(t *testing.T) {
		h := ingCfgHarness()
		bad := ingCfgValidSftp()
		delete(bad, "host")
		h.store.entries[ingsEntryKey("orders-sync", "clt_acme", "vendor-feed")] = ingCfgEntry(bad)
		body := ingCfgDrawer(t, h, ingAdmin(), "ing_6")
		mustContain(t, body, `>Invalid<`, `id="ings-config-invalid"`, `source.host`, `id="ings-needs-settings"`, `is not valid for client Acme Corp`)
		list := ingBody(t, h.do(t, ingAdmin(), req{target: "/ui/ingestions"}))
		mustContain(t, list, `data-testid="ings-needs-badge"`)
	})
	t.Run("an extension no runtime reports: schema not checked", func(t *testing.T) {
		h := ingCfgHarness()
		h.store.entries[ingsEntryKey("orders-sync", "clt_acme", "mix-positions")] = ingCfgEntry(map[string]any{"region": "eu", "targetUrl": "https://mix.example/hook"})
		body := ingCfgDrawer(t, h, ingAdmin(), "ing_9")
		mustContain(t, body, `id="ings-config-unchecked"`, `Schema not checked`, `https://mix.example/hook`, `>Set<`)
		ingCfgReportMix(h)
		mustNotContain(t, ingCfgDrawer(t, h, ingAdmin(), "ing_9"), `Schema not checked`)
	})
	t.Run("an inline ingestion has no section and no badge", func(t *testing.T) {
		h := ingCfgHarness()
		body := ingCfgDrawer(t, h, ingAdmin(), "ing_1")
		mustNotContain(t, body, `Settings (this environment)`, `Needs settings`)
	})
	t.Run("a draft names the entry, the live version does not", func(t *testing.T) {
		h := ingCfgHarness()
		draft := ingCfgSftp(ingDef("x", nil, nil), ingestion.ConfigRef{Own: true})
		h.store.versions["ing_1"] = append(h.store.versions["ing_1"], ingVersion("ing_1", 4, draft, ""))
		for n := range h.store.items {
			if h.store.items[n].ID == "ing_1" {
				h.store.items[n].LatestVersion = 4
			}
		}
		body := ingCfgDrawer(t, h, ingAdmin(), "ing_1")
		mustContain(t, body, `Settings (this environment)`, `id="ings-config-draft"`, `Named by a draft`)
		mustNotContain(t, body, `id="ings-needs-settings"`)
	})
	t.Run("an entry that is shared names the others", func(t *testing.T) {
		h := ingCfgHarness()
		body := ingCfgDrawer(t, h, ingAdmin(), "ing_7")
		mustContain(t, body, `id="ings-config-shared"`, `Shared with vendor-feed-3`, `https://vendor.example/hook`)
	})
}

func TestIngestionSettingsListBadgeOnlyForUnsetEntries(t *testing.T) {
	h := ingCfgHarness()
	list := ingBody(t, h.do(t, ingAdmin(), req{target: "/ui/ingestions"}))
	// ing_9 (no entry) is the only one that needs settings: vendor-feed and the
	// shared one are set, the set-owned one shares vendor-feed's key by code and is missing.
	mustContain(t, list, `config ingest/mix-positions is not set for client Acme Corp`, `config ingest/set-cfg is not set for client Acme Corp`)
	mustNotContain(t, list, `config ingest/vendor-feed is not set`, `config ingest/shared-vendor is not set`)
}

func TestIngestionSettingsAreNotReadWithoutPermission(t *testing.T) {
	h := ingCfgHarness()
	body := ingCfgDrawer(t, h, ingUser(ingPermView), "ing_6")
	mustContain(t, body, `Settings (this environment)`, `sftp.vendor.example`)
	mustNotContain(t, body, `/ui/ingestions/ing_6/config"`, `/config/delete`)
}

// ── the settings form ──────────────────────────────────────────────────

func TestIngestionSettingsFormOfABuiltInKind(t *testing.T) {
	h := ingCfgHarness()
	body := ingBody(t, h.do(t, ingAdmin(), req{target: "/ui/ingestions/ing_6/config", datastar: true}))
	mustContain(t, body,
		`id="ings-config-body"`, `action="/ui/ingestions/ing_6/config"`, `Settings for vendor-feed`, `ingest/vendor-feed for client Acme Corp`,
		// The same controls as the definition's form, with the stored values.
		`name="sftp.host"`, `value="sftp.vendor.example"`, `name="sftp.port"`, `value="22"`, `name="sftp.directory"`,
		`name="sftp.credential"`, `value="vendor-sftp-pass"`, `name="sftp.every"`, `name="sftp.knownHosts"`,
		// The element ids carry the settings form's own prefix.
		`id="ings-config-sftp-host"`,
	)
	// The definition gives the delivery URL, so the entry does not ask for it.
	mustNotContain(t, body, `name="config.targetUrl"`, `name="kind"`, `type="password"`)
	// Neither the definition's identity nor its other sections.
	mustNotContain(t, body, `name="split.kind"`, `name="target.pool"`, `name="code"`)

	noURL := ingBody(t, h.do(t, ingAdmin(), req{target: "/ui/ingestions/ing_7/config", datastar: true}))
	mustContain(t, noURL, `name="config.targetUrl"`, `value="https://vendor.example/hook"`, `Delivery URL`,
		`The entry is shared with vendor-feed-3: the value must suit each of them.`)
}

func TestIngestionSettingsFormOfAnApiPollIsDrawnFromItsFields(t *testing.T) {
	h := ingCfgHarness()
	poll := ingDef("Stock poll", ingestion.Configured{Base: ingestion.KindAPIPoll, Config: ingestion.ConfigRef{Own: true}}, ingestion.None{})
	h.store.items = append(h.store.items, ingSample("ing_11", "stock-cfg", "orders", "acme", poll, true, 1, 1))
	h.store.entries[ingsEntryKey("orders-sync", "clt_acme", "stock-cfg")] = ingCfgEntry(map[string]any{
		"url": "https://api.vendor.example/v1/stock", "method": "GET",
		"auth": map[string]any{"kind": "login", "url": "https://api.vendor.example/login", "username": "svc", "password": map[string]any{"name": "api-pass"}},
	})
	body := ingBody(t, h.do(t, ingAdmin(), req{target: "/ui/ingestions/ing_11/config", datastar: true}))
	mustContain(t, body, `name="api.url"`, `value="https://api.vendor.example/v1/stock"`, `name="api.auth"`, `<option value="login" selected>`,
		`name="api.loginUser"`, `name="api.loginPassword"`, `value="api-pass"`, `data-bind:ing-api-auth`)
	// The drawer shows the effective values, the secret by name.
	drawer := ingCfgDrawer(t, h, ingAdmin(), "ing_11")
	mustContain(t, drawer, `https://api.vendor.example/v1/stock`, `api-pass`)
}

func TestIngestionSettingsFormOfAnExtensionIsDrawnFromTheReportedFields(t *testing.T) {
	h := ingCfgHarness()
	ingCfgReportMix(h)
	h.store.entries[ingsEntryKey("orders-sync", "clt_acme", "mix-positions")] = ingCfgEntry(map[string]any{"region": "us", "token": "mix-token", "targetUrl": "https://mix.example/hook"})
	body := ingBody(t, h.do(t, ingAdmin(), req{target: "/ui/ingestions/ing_9/config", datastar: true}))
	mustContain(t, body,
		`name="mix.s.region"`, `<option value="us" selected>`, `<option value="eu"`, `name="mix.s.token"`, `value="mix-token"`, `name="mix.s.batch"`,
		`name="config.targetUrl"`, `value="https://mix.example/hook"`,
	)
	mustNotContain(t, body, `Schema not checked`)
	// And an unreported kind is edited as JSON, with the schema not checked.
	h2 := ingCfgHarness()
	unchecked := ingBody(t, h2.do(t, ingAdmin(), req{target: "/ui/ingestions/ing_9/config", datastar: true}))
	mustContain(t, unchecked, `Schema not checked`, `name="mix.settings"`, `<fc-code-editor`)
}

func TestIngestionSettingsFormNeedsTheUpdatePermissionAndTheKindsPermission(t *testing.T) {
	h := ingCfgHarness()
	rec := h.do(t, ingUser(ingPermView), req{target: "/ui/ingestions/ing_6/config"})
	assert.Equal(t, http.StatusForbidden, rec.Code)
	// An ingestion that is not configured has no entry to edit.
	rec = h.do(t, ingAdmin(), req{target: "/ui/ingestions/ing_1/config"})
	assert.NotEqual(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "holds its settings inline")
	// The kind's permission still applies to the entry.
	camel := ingDef("Route", ingestion.Configured{Base: ingestion.KindCustom, Type: "camel-route", Config: ingestion.ConfigRef{Own: true}}, ingestion.None{})
	h.store.items = append(h.store.items, ingSample("ing_12", "route-cfg", "orders", "acme", camel, true, 1, 1))
	rec = h.do(t, ingWithout(ingPermRoute), req{target: "/ui/ingestions/ing_12/config"})
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), ingPermRoute)
	drawer := ingCfgDrawer(t, h, ingWithout(ingPermRoute), "ing_12")
	mustContain(t, drawer, `Changing a camel-route ingestion needs `+ingPermRoute)
	mustNotContain(t, drawer, `href="/ui/ingestions/ing_12/config"`)
}

// A set owns the definition, not the environment's part of it: the update
// permission is enough to write the entry of an ingestion a set owns.
func TestIngestionSettingsOfASetsIngestionAreEditable(t *testing.T) {
	h := ingCfgHarness()
	drawer := ingCfgDrawer(t, h, ingAdmin(), "ing_10")
	mustContain(t, drawer, `id="ings-managed"`, `href="/ui/ingestions/ing_10/config"`)
	mustNotContain(t, drawer, `href="/ui/ingestions/ing_10/edit"`)
	form := url.Values{"sftp.host": {"sftp.vendor.example"}, "sftp.port": {"22"}, "sftp.directory": {"/out"}, "sftp.username": {"svc"}, "sftp.credential": {"vendor-sftp-pass"}, "sftp.knownHosts": {ingCfgKnownHost}, "sftp.every": {"1m"}}
	body := ingBody(t, ingPost(h, t, ingAdmin(), "/ui/ingestions/ing_10/config", form))
	require.Len(t, h.ops.configs, 1, body)
	// The definition itself stays read-only.
	assert.Equal(t, http.StatusForbidden, h.do(t, ingAdmin(), req{target: "/ui/ingestions/ing_10/edit"}).Code)
}

func TestIngestionSettingsSaveRunsTheUseCaseWithTheEntry(t *testing.T) {
	h := ingCfgHarness()
	delete(h.store.entries, ingsEntryKey("orders-sync", "clt_acme", "vendor-feed"))
	form := url.Values{
		"sftp.host": {" sftp.vendor.example "}, "sftp.port": {"2222"}, "sftp.directory": {"/out"}, "sftp.username": {"svc"},
		"sftp.credential": {"vendor-sftp-pass"}, "sftp.knownHosts": {ingCfgKnownHost}, "sftp.every": {"5m"}, "sftp.include": {""},
		"drawer": {"ing_6"}, "tab": {"overview"},
	}
	body := ingBody(t, ingPost(h, t, ingAdmin(), "/ui/ingestions/ing_6/config", form))
	require.Len(t, h.ops.configs, 1, body)
	cmd := h.ops.configs[0]
	assert.Equal(t, "ing_6", cmd.ID)
	raw, _ := json.Marshal(cmd.Value)
	assert.JSONEq(t, `{"host":"sftp.vendor.example","port":2222,"directory":"/out","username":"svc","credential":{"name":"vendor-sftp-pass"},"knownHosts":"`+ingCfgKnownHost+`","acceptAnyHostKey":false,"every":"PT5M"}`, string(raw))
	// The secret is a name, never a value; the delivery URL stays the definition's.
	assert.NotContains(t, cmd.Value, "targetUrl")
	// The dialog closes, the drawer is redrawn with the entry set, the user is told.
	mustContain(t, body, `ings-config`, `Settings saved for this environment`, `id="ings-config"`, `>Set<`)
	mustNotContain(t, body, `id="ings-needs-settings"`)
}

func TestIngestionSettingsSaveCarriesTheDeliveryURLWhenTheDefinitionHasNone(t *testing.T) {
	h := ingCfgHarness()
	ingCfgReportMix(h)
	form := url.Values{"mix.s.region": {"eu"}, "mix.s.token": {"mix-token"}, "mix.s.batch": {"5"}, "config.targetUrl": {" https://mix.example/hook "}, "drawer": {"ing_9"}}
	body := ingBody(t, ingPost(h, t, ingAdmin(), "/ui/ingestions/ing_9/config", form))
	require.Len(t, h.ops.configs, 1, body)
	raw, _ := json.Marshal(h.ops.configs[0].Value)
	assert.JSONEq(t, `{"region":"eu","token":"mix-token","batch":5,"targetUrl":"https://mix.example/hook"}`, string(raw))
	// A definition that gives the delivery URL gets none from a forged post.
	h2 := ingCfgHarness()
	f2 := url.Values{"sftp.host": {"sftp.vendor.example"}, "sftp.port": {"22"}, "sftp.directory": {"/out"}, "sftp.username": {"svc"}, "sftp.credential": {"x"}, "sftp.knownHosts": {ingCfgKnownHost}, "sftp.every": {"1m"}, "config.targetUrl": {"https://evil.example"}}
	ingBody(t, ingPost(h2, t, ingAdmin(), "/ui/ingestions/ing_6/config", f2))
	require.Len(t, h2.ops.configs, 1)
	assert.NotContains(t, h2.ops.configs[0].Value, "targetUrl")
}

func TestIngestionSettingsSaveWarnsWhenTheSchemaWasNotChecked(t *testing.T) {
	h := ingCfgHarness()
	h.ops.configWarnings = []string{"SCHEMA_UNCHECKED: no live runtime reports the settings of mix"}
	form := url.Values{"mix.settings": {`{"region":"eu"}`}, "config.targetUrl": {"https://mix.example/hook"}, "drawer": {"ing_9"}}
	body := ingBody(t, ingPost(h, t, ingAdmin(), "/ui/ingestions/ing_9/config", form))
	require.Len(t, h.ops.configs, 1, body)
	assert.Equal(t, map[string]any{"region": "eu", "targetUrl": "https://mix.example/hook"}, h.ops.configs[0].Value)
	mustContain(t, body, `SCHEMA_UNCHECKED: no live runtime reports the settings of mix`, `fc-flash-warn`)
}

func TestIngestionSettingsSaveMapsTheProblemsOntoTheFields(t *testing.T) {
	t.Run("a built-in kind: the definition's paths", func(t *testing.T) {
		h := ingCfgHarness()
		form := url.Values{"sftp.host": {""}, "sftp.port": {"22"}, "sftp.directory": {"/out"}, "sftp.username": {"svc"}, "sftp.credential": {"x"}, "sftp.knownHosts": {ingCfgKnownHost}, "sftp.every": {"1m"}}
		body := ingPost(h, t, ingAdmin(), "/ui/ingestions/ing_6/config", form).Body.String()
		require.Len(t, h.ops.configs, 1)
		mustContain(t, body, `id="ings-config-sftp-host-err"`, `ings-config-body`)
		// Nothing was stored: the entry is as it was.
		mustNotContain(t, body, `Settings saved`)
	})
	t.Run("an extension: the entry's paths, from the reported schema", func(t *testing.T) {
		h := ingCfgHarness()
		ingCfgReportMix(h)
		form := url.Values{"mix.s.region": {"mars"}, "mix.s.token": {"t"}, "mix.s.batch": {"50"}, "config.targetUrl": {"https://mix.example/hook"}}
		body := ingPost(h, t, ingAdmin(), "/ui/ingestions/ing_9/config", form).Body.String()
		mustContain(t, body, `id="ings-config-mix-s-region-err"`, `Ingestion mix-cfg: must be one of eu, us`, `id="ings-config-mix-s-batch-err"`)
	})
	t.Run("a bad number is the field's own", func(t *testing.T) {
		h := ingCfgHarness()
		form := url.Values{"sftp.host": {"h.example"}, "sftp.port": {"many"}, "sftp.directory": {"/out"}, "sftp.username": {"svc"}, "sftp.credential": {"x"}, "sftp.every": {"1m"}}
		body := ingPost(h, t, ingAdmin(), "/ui/ingestions/ing_6/config", form).Body.String()
		assert.Empty(t, h.ops.configs, "nothing is sent for a value that cannot be read")
		mustContain(t, body, `id="ings-config-sftp-port-err"`, `Enter a whole number.`)
	})
	t.Run("a refusal that names no field is told in the form", func(t *testing.T) {
		h := ingCfgHarness()
		h.ops.configErr = usecase.Validation("INGESTION_CONFIG_INVALID", "value is required: a JSON object of the kind's settings")
		form := url.Values{"sftp.host": {"h.example"}, "sftp.port": {"22"}, "sftp.directory": {"/out"}, "sftp.username": {"svc"}, "sftp.credential": {"x"}, "sftp.every": {"1m"}}
		body := ingPost(h, t, ingAdmin(), "/ui/ingestions/ing_6/config", form).Body.String()
		mustContain(t, body, `ings-config-error`)
	})
}

func TestIngestionSettingsSaveNeedsTheUpdatePermission(t *testing.T) {
	h := ingCfgHarness()
	form := url.Values{"sftp.host": {"h.example"}, "sftp.port": {"22"}, "sftp.directory": {"/out"}, "sftp.username": {"svc"}, "sftp.credential": {"x"}, "sftp.every": {"1m"}}
	rec := ingPost(h, t, ingUser(ingPermView), "/ui/ingestions/ing_6/config", form)
	assert.Empty(t, h.ops.configs)
	assert.Contains(t, rec.Body.String(), "ermission required")
}

// ── delete ─────────────────────────────────────────────────────────────

func TestIngestionSettingsDeleteConfirmSaysWhichIngestionsStop(t *testing.T) {
	h := ingCfgHarness()
	body := ingCfgDrawer(t, h, ingAdmin(), "ing_7")
	mustContain(t, body,
		`id="ings-config-delete"`, `action="/ui/ingestions/ing_7/config/delete"`, `Delete these settings?`,
		`Delete ingest/shared-vendor for client Acme Corp? These ingestions stop: vendor-feed-2, vendor-feed-3`,
		`Delete…`,
	)
	alone := ingCfgDrawer(t, h, ingAdmin(), "ing_6")
	mustContain(t, alone, `The ingestion vendor-feed stops and takes no jobs until the settings are set again`)
	// Not offered where there is no entry to delete.
	delete(h.store.entries, ingsEntryKey("orders-sync", "clt_acme", "vendor-feed"))
	mustNotContain(t, ingCfgDrawer(t, h, ingAdmin(), "ing_6"), `id="ings-config-delete"`)
}

func TestIngestionSettingsDeleteRunsTheUseCase(t *testing.T) {
	h := ingCfgHarness()
	body := ingBody(t, ingPost(h, t, ingAdmin(), "/ui/ingestions/ing_7/config/delete", url.Values{"drawer": {"ing_7"}, "tab": {"overview"}}))
	assert.Equal(t, []string{"ing_7"}, h.ops.deleted)
	mustContain(t, body, `Settings deleted`, `Waiting for settings: vendor-feed-2, vendor-feed-3`, `>Not set<`)
	assert.Nil(t, h.store.entries[ingsEntryKey("orders-sync", "clt_acme", "shared-vendor")])

	h2 := ingCfgHarness()
	rec := ingPost(h2, t, ingUser(ingPermView), "/ui/ingestions/ing_7/config/delete", url.Values{})
	assert.Empty(t, h2.ops.deleted)
	assert.Contains(t, rec.Body.String(), "ermission required")
	// The set's ingestion may be deleted from, too.
	h3 := ingCfgHarness()
	h3.store.entries[ingsEntryKey("orders-sync", "clt_acme", "set-cfg")] = ingCfgEntry(ingCfgValidSftp())
	ingBody(t, ingPost(h3, t, ingAdmin(), "/ui/ingestions/ing_10/config/delete", url.Values{}))
	assert.Equal(t, []string{"ing_10"}, h3.ops.deleted)
}
