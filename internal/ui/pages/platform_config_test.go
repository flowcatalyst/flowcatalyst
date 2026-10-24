package pages

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
)

const (
	cfgSecretValue = "TOPSECRET-hunter2"
	cfgNewSecret   = "NEWSECRET-correct-horse"
)

// fakeConfig stands in for platformconfig.Service: an in-memory store whose
// rules are the real permission checks (auth.RequireConfigView and friends,
// platformconfig.RevealReach), so the permission matrix is the platform's.
// The reveal counts as audited when it returns a value.
type fakeConfig struct {
	rows     []platformconfig.Config
	reveals  []string
	deleted  []string
	revealed int
}

func cfgRowFixture(id, section, property string, vt platformconfig.ValueType, value string) platformconfig.Config {
	desc := "About <b>" + property + "</b>"
	return platformconfig.Config{
		ID: id, ApplicationCode: "platform", Section: section, Property: property, Scope: platformconfig.ScopeGlobal,
		ValueType: vt, Value: value, Description: &desc, UpdatedAt: time.Date(2026, 10, 20, 9, 30, 0, 0, time.UTC),
	}
}

func newFakeConfig() *fakeConfig {
	acme := ids.ClientID("clt_acme")
	clientRow := cfgRowFixture("pfc_3", "mail", "smtp-password", platformconfig.ValueSecret, "encrypted:ciphertext-"+cfgSecretValue)
	clientRow.Scope, clientRow.ClientID = platformconfig.ScopeClient, &acme
	return &fakeConfig{rows: []platformconfig.Config{
		cfgRowFixture("pfc_1", "branding", "name", platformconfig.ValuePlain, "Acme <Portal>"),
		cfgRowFixture("pfc_2", "mail", "api-key", platformconfig.ValueSecret, "encrypted:ciphertext-"+cfgSecretValue),
		clientRow,
	}}
}

func (f *fakeConfig) List(_ context.Context, ac *auth.AuthContext, app string) ([]platformconfig.Config, error) {
	if err := auth.RequireConfigView(ac); err != nil {
		return nil, err
	}
	var out []platformconfig.Config
	for _, r := range f.rows {
		if app == "platform" {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeConfig) AuthorizeUpdate(_ context.Context, ac *auth.AuthContext, _ string) error {
	return auth.RequireConfigUpdate(ac)
}

func (f *fakeConfig) AuthorizeWrite(ctx context.Context, ac *auth.AuthContext, app string) error {
	return f.AuthorizeUpdate(ctx, ac, app)
}

func (f *fakeConfig) AuthorizeRevealApp(_ context.Context, ac *auth.AuthContext, _ string) error {
	if auth.CanRevealPlatformConfigSecret(ac) == nil || auth.CanRevealApplicationConfigSecret(ac) == nil {
		return nil
	}
	return httperror.Forbidden("no reveal permission")
}

func (f *fakeConfig) ResolveClientID(_ context.Context, ref string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(ref)) {
	case "":
		return "", nil
	case "acme", "clt_acme":
		return "clt_acme", nil
	}
	return "", httperror.NotFound("Client", ref)
}

func (f *fakeConfig) Delete(ctx context.Context, ac *auth.AuthContext, app, section, property, _ string) error {
	if err := f.AuthorizeWrite(ctx, ac, app); err != nil {
		return err
	}
	f.deleted = append(f.deleted, section+"/"+property)
	return nil
}

func (f *fakeConfig) Reveal(ctx context.Context, ac *auth.AuthContext, app, section, property, clientRef string) (*platformconfig.Revealed, error) {
	if err := f.AuthorizeRevealApp(ctx, ac, app); err != nil {
		return nil, err
	}
	cid, err := f.ResolveClientID(ctx, clientRef)
	if err != nil {
		return nil, err
	}
	if err := platformconfig.RevealReach(ac, cid); err != nil {
		return nil, err
	}
	f.revealed++
	f.reveals = append(f.reveals, section+"/"+property)
	for i := range f.rows {
		if f.rows[i].Section == section && f.rows[i].Property == property {
			return &platformconfig.Revealed{Config: &f.rows[i], Value: cfgSecretValue}, nil
		}
	}
	return nil, httperror.NotFound("Config", section+"/"+property)
}

type fakeConfigOps struct {
	sets []operations.SetPropertyCommand
	err  error
}

func (o *fakeConfigOps) Set(_ context.Context, cmd operations.SetPropertyCommand) error {
	o.sets = append(o.sets, cmd)
	return o.err
}

type fakeConfigApps struct{}

func (fakeConfigApps) FindActive(context.Context) ([]application.Application, error) {
	return []application.Application{{ID: "app_orders", Code: "orders"}, {ID: "app_billing", Code: "billing"}}, nil
}

type cfgHarness struct {
	cfg *fakeConfig
	ops *fakeConfigOps
}

func newCfgHarness() *cfgHarness { return &cfgHarness{cfg: newFakeConfig(), ops: &fakeConfigOps{}} }

func (h *cfgHarness) rec(t *testing.T, ac *auth.AuthContext, q req) *httptest.ResponseRecorder {
	t.Helper()
	page := &PlatformConfig{Config: h.cfg, Ops: h.ops, Apps: fakeConfigApps{}, Shell: &shell.Shell{Version: "test", Flashes: FlashMessages}}
	return serveMounted(t, ac, q, page)
}

func (h *cfgHarness) do(t *testing.T, ac *auth.AuthContext, q req) string {
	t.Helper()
	return h.rec(t, ac, q).Body.String()
}

func cfgUser(perms ...string) *auth.AuthContext {
	return &auth.AuthContext{PrincipalID: "prn_cfg", Email: "cfg@x.y", Scope: auth.ScopeAnchor, AllApplications: true, Roles: []string{"r"}, Permissions: perms, PrincipalType: auth.PrincipalTypeUser}
}

// cfgAdmin is a super-admin with access to every application.
func cfgAdmin() *auth.AuthContext {
	ac := anchorAdmin()
	ac.AllApplications = true
	return ac
}

const (
	permCfgView   = "platform:admin:config:view"
	permCfgUpdate = "platform:admin:config:update"
	permCfgReveal = "platform:admin:config:reveal"
)

func TestConfigList(t *testing.T) {
	h := newCfgHarness()
	body := h.do(t, cfgAdmin(), req{target: "/ui/platform/config"})
	mustContain(t, body,
		`<title>Configuration · FlowCatalyst</title>`,
		`<a href="/ui/platform/config" class="fc-nav-child" aria-current="page">`,
		`<h1 class="fc-page-title">Configuration</h1>`, `href="/ui/platform/config/new?app=platform"`, "Add property",
		`<option value="orders">orders</option>`, `<option value="platform" selected>platform</option>`,
		`id="cfg-table"`, "Search section, property, description...",
		"Acme &lt;Portal&gt;", "About &lt;b&gt;name&lt;/b&gt;", "Global", "acme",
		"••••••••", `<dialog id="cfg-reveal"`, `<dialog id="cfg-delete"`,
	)
	mustNotContain(t, body, cfgSecretValue, "ciphertext", "Acme <Portal>")

	// Another application is a different list; search narrows it.
	mustNotContain(t, h.do(t, cfgAdmin(), req{target: "/ui/platform/config?app=orders"}), `id="cfg-pfc_1"`)
	filtered := h.do(t, cfgAdmin(), req{target: "/ui/platform/config?q=smtp"})
	mustContain(t, filtered, `id="cfg-pfc_3"`)
	mustNotContain(t, filtered, `id="cfg-pfc_1"`, `id="cfg-pfc_2"`)

	live := h.do(t, cfgAdmin(), req{target: "/ui/platform/config?app=platform&q=name", datastar: true})
	mustContain(t, live, `id="cfg-actions"`, `id="cfg-table"`, `id="cfg-pager"`, `id="config-list-state"`)
}

// The viewer's controls follow the permissions, and a forged post is refused
// on the server whatever the page offered.
func TestConfigPermissionMatrix(t *testing.T) {
	forged := func(h *cfgHarness, ac *auth.AuthContext, path string, form url.Values) string {
		return h.do(t, ac, req{method: http.MethodPost, target: path, datastar: true, multipart: true, form: form})
	}
	row := url.Values{"app": {"platform"}, "section": {"branding"}, "property": {"name"}}
	save := url.Values{"app": {"platform"}, "section": {"s"}, "property": {"p"}, "client": {""}, "valueType": {"PLAIN"}, "plainValue": {"v"}}

	t.Run("view only", func(t *testing.T) {
		h := newCfgHarness()
		viewer := cfgUser(permCfgView)
		body := h.do(t, viewer, req{target: "/ui/platform/config"})
		mustContain(t, body, `id="cfg-pfc_2"`, "••••••••")
		mustNotContain(t, body, "Add property", "/ui/platform/config/edit", "Reveal…", "Delete…", `<dialog id="cfg-reveal"`, `<dialog id="cfg-delete"`, cfgSecretValue)

		mustContain(t, forged(h, viewer, "/ui/platform/config/save", save), "Permission required: platform:admin:config:update")
		for _, path := range []string{"/ui/platform/config/delete", "/ui/platform/config/reveal"} {
			out := forged(h, viewer, path, save)
			mustContain(t, out, "fc-flash-error")
			mustNotContain(t, out, cfgSecretValue)
		}
		forged(h, viewer, "/ui/platform/config/delete", row)
		if len(h.ops.sets) != 0 || len(h.cfg.deleted) != 0 || h.cfg.revealed != 0 {
			t.Fatalf("a view-only caller changed something: sets=%d deleted=%d revealed=%d", len(h.ops.sets), len(h.cfg.deleted), h.cfg.revealed)
		}
		mustContain(t, h.do(t, viewer, req{target: "/ui/platform/config/new?app=platform", datastar: true}), "fc-flash-error")
		mustContain(t, h.do(t, viewer, req{target: "/ui/platform/config/edit?" + row.Encode(), datastar: true}), "fc-flash-error")
	})

	t.Run("update without reveal", func(t *testing.T) {
		h := newCfgHarness()
		updater := cfgUser(permCfgView, permCfgUpdate)
		body := h.do(t, updater, req{target: "/ui/platform/config"})
		mustContain(t, body, "Add property", `aria-label="Edit branding/name"`, "Delete…", `<dialog id="cfg-delete"`)
		mustNotContain(t, body, "Reveal…", `<dialog id="cfg-reveal"`)

		out := forged(h, updater, "/ui/platform/config/reveal", url.Values{"app": {"platform"}, "section": {"mail"}, "property": {"api-key"}})
		mustContain(t, out, "fc-flash-error")
		mustNotContain(t, out, cfgSecretValue)
		if h.cfg.revealed != 0 {
			t.Fatal("revealed without the reveal permission")
		}
	})

	t.Run("update and reveal", func(t *testing.T) {
		h := newCfgHarness()
		body := h.do(t, cfgUser(permCfgView, permCfgUpdate, permCfgReveal), req{target: "/ui/platform/config"})
		mustContain(t, body, "Reveal…", `<dialog id="cfg-reveal"`, `data-action="/ui/platform/config/reveal?app=platform&amp;property=api-key&amp;section=mail"`)
		// Plain rows have no reveal button.
		mustNotContain(t, menuOrActions(t, body, "pfc_1"), "Reveal")
	})

	t.Run("below anchor tier, reveal reaches only a client's own value", func(t *testing.T) {
		h := newCfgHarness()
		tenant := &auth.AuthContext{PrincipalID: "prn_t", Scope: auth.ScopeClient, Clients: []string{"clt_acme"}, Roles: []string{"r"}, Permissions: []string{"platform:application-service:config:reveal"}, PrincipalType: auth.PrincipalTypeUser}
		// (the fake's List needs the view rule, which below anchor tier is the role grants: here passed)
		body := h.do(t, tenant, req{target: "/ui/platform/config"})
		mustContain(t, body, `aria-label="Reveal… mail/smtp-password"`)
		mustNotContain(t, body, `aria-label="Reveal… mail/api-key"`)
	})
}

func menuOrActions(t *testing.T, body, id string) string {
	t.Helper()
	_, rest, ok := strings.Cut(body, `id="cfg-`+id+`"`)
	if !ok {
		t.Fatalf("no row %s", id)
	}
	rest, _, _ = strings.Cut(rest, "</tr>")
	return rest
}

// A secret's plaintext is in no response but the reveal's: not the list, not
// the edit dialog, not a failed or a successful save, not the page that follows
// a reveal.
func TestConfigSecretNeverRendered(t *testing.T) {
	h := newCfgHarness()
	admin := cfgAdmin()
	responses := map[string]string{
		"list":            h.do(t, admin, req{target: "/ui/platform/config"}),
		"live list":       h.do(t, admin, req{target: "/ui/platform/config", datastar: true}),
		"edit dialog":     h.do(t, admin, req{target: "/ui/platform/config/edit?app=platform&section=mail&property=api-key", datastar: true}),
		"native edit":     h.do(t, admin, req{target: "/ui/platform/config/edit?app=platform&section=mail&property=api-key"}),
		"add dialog":      h.do(t, admin, req{target: "/ui/platform/config/new?app=platform", datastar: true}),
		"failed save":     h.do(t, admin, req{method: http.MethodPost, target: "/ui/platform/config/save", datastar: true, form: cfgSaveForm("section", "", "valueType", "SECRET", "secretValue", cfgNewSecret)}),
		"native failed":   h.do(t, admin, req{method: http.MethodPost, target: "/ui/platform/config/save", form: cfgSaveForm("section", "", "valueType", "SECRET", "secretValue", cfgNewSecret)}),
		"successful save": h.do(t, admin, req{method: http.MethodPost, target: "/ui/platform/config/save", datastar: true, form: cfgSaveForm("valueType", "SECRET", "secretValue", cfgNewSecret)}),
	}
	for name, body := range responses {
		if strings.Contains(body, cfgSecretValue) || strings.Contains(body, cfgNewSecret) || strings.Contains(body, "ciphertext") {
			t.Errorf("%s response carries a secret", name)
		}
	}
	mustContain(t, responses["edit dialog"], "Enter a new value to replace the secret", `data-show="$cfgSecret"`)
	mustNotContain(t, responses["edit dialog"], `value="encrypted`)
	if len(h.ops.sets) != 1 || h.ops.sets[0].Value != cfgNewSecret {
		t.Fatalf("the secret reached the command: %+v", h.ops.sets)
	}

	revealed := h.do(t, admin, req{method: http.MethodPost, target: "/ui/platform/config/reveal?app=platform&section=mail&property=api-key", datastar: true})
	if strings.Count(revealed, cfgSecretValue) != 1 {
		t.Errorf("the reveal response should carry the value once, got %d", strings.Count(revealed, cfgSecretValue))
	}
	after := h.do(t, admin, req{target: "/ui/platform/config"})
	mustNotContain(t, after, cfgSecretValue)
}

func cfgSaveForm(kv ...string) url.Values {
	v := url.Values{"app": {"platform"}, "section": {"mail"}, "property": {"api-key"}, "client": {""}, "valueType": {"PLAIN"}, "plainValue": {"v"}, "description": {""}}
	for i := 0; i+1 < len(kv); i += 2 {
		v.Set(kv[i], kv[i+1])
	}
	return v
}

func TestConfigReveal(t *testing.T) {
	h := newCfgHarness()
	post := req{method: http.MethodPost, target: "/ui/platform/config/reveal?app=platform&section=mail&property=api-key", datastar: true}

	rec := h.rec(t, cfgAdmin(), post)
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	body := rec.Body.String()
	mustContain(t, body, "data: mode replace\n", `<div id="cfg-reveal-slot"><dialog id="cfg-reveal-value-dialog"`, "Secret value",
		"mail/api-key: this reveal was recorded in the audit log.", cfgSecretValue, ">Copy<", ">Close<", `data-on:cancel="evt.preventDefault(); el.remove()"`)
	// The value is on the page as text only: not in a signal, a URL, an attribute or a flash.
	mustNotContain(t, body, `"`+cfgSecretValue, `=`+cfgSecretValue, "fc-flash")
	if h.cfg.revealed != 1 {
		t.Errorf("reveals = %d", h.cfg.revealed)
	}

	// A native post gets the page with the dialog, also uncacheable.
	native := h.rec(t, cfgAdmin(), req{method: http.MethodPost, target: post.target})
	if native.Code != http.StatusOK || native.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("native: status %d, Cache-Control %q", native.Code, native.Header().Get("Cache-Control"))
	}
	mustContain(t, native.Body.String(), `<dialog id="cfg-reveal-value-dialog"`, cfgSecretValue, `<h1 class="fc-page-title">Configuration</h1>`)

	// A refused reveal is uncacheable too, and shows no value.
	refused := h.rec(t, cfgUser(permCfgView, permCfgUpdate), post)
	if refused.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("refusal Cache-Control = %q", refused.Header().Get("Cache-Control"))
	}
	mustNotContain(t, refused.Body.String(), cfgSecretValue)

	// An unknown client is refused, not revealed.
	unknown := h.do(t, cfgAdmin(), req{method: http.MethodPost, target: post.target + "&client=nobody", datastar: true})
	mustContain(t, unknown, "fc-flash-error")
	mustNotContain(t, unknown, cfgSecretValue)
	if h.cfg.revealed != 2 {
		t.Errorf("reveals = %d, want 2 (the unknown client must not count)", h.cfg.revealed)
	}
}

func TestConfigSave(t *testing.T) {
	t.Run("plain create: the SPA's PUT body, dialog closed, table, flash", func(t *testing.T) {
		h := newCfgHarness()
		body := h.do(t, cfgAdmin(), req{method: http.MethodPost, target: "/ui/platform/config/save", datastar: true, multipart: true, form: cfgSaveForm(
			"section", " web ", "property", "title", "plainValue", "Hello", "description", " the title ", "client", "ACME")})
		if len(h.ops.sets) != 1 {
			t.Fatalf("sets %d: %s", len(h.ops.sets), body)
		}
		cmd := h.ops.sets[0]
		if cmd.ApplicationCode != "platform" || cmd.Section != "web" || cmd.Property != "title" || cmd.Value != "Hello" ||
			deref(cmd.ValueType) != "PLAIN" || deref(cmd.Description) != "the title" || deref(cmd.ClientID) != "clt_acme" {
			t.Errorf("command = %+v", cmd)
		}
		mustContain(t, body, `document.getElementById(&#34;cfg-form&#34;)?.close()`, `id="cfg-table"`, "The property was saved")
	})
	t.Run("secret: the password input's value, a blank description is none", func(t *testing.T) {
		h := newCfgHarness()
		h.do(t, cfgAdmin(), req{method: http.MethodPost, target: "/ui/platform/config/save", datastar: true, form: cfgSaveForm("valueType", "SECRET", "secretValue", cfgNewSecret, "plainValue", "ignored")})
		cmd := h.ops.sets[0]
		if cmd.Value != cfgNewSecret || deref(cmd.ValueType) != "SECRET" || cmd.Description != nil || cmd.ClientID != nil {
			t.Errorf("command = %+v", cmd)
		}
	})
	t.Run("a secret needs a new value, a plain value needs a value", func(t *testing.T) {
		h := newCfgHarness()
		body := h.do(t, cfgAdmin(), req{method: http.MethodPost, target: "/ui/platform/config/save", datastar: true, form: cfgSaveForm("edit", "1", "valueType", "SECRET", "secretValue", "")})
		mustContain(t, body, "Enter the secret value.", `id="cfg-form-value-err"`, `document.getElementById(&#34;cfg-form-secret-value&#34;)?.focus()`)
		body = h.do(t, cfgAdmin(), req{method: http.MethodPost, target: "/ui/platform/config/save", datastar: true, form: cfgSaveForm("plainValue", "", "section", "", "property", "")})
		mustContain(t, body, "A value is required.", "Section is required.", "Property is required.")
		if len(h.ops.sets) != 0 {
			t.Fatal("saved an invalid form")
		}
	})
	t.Run("an unknown client is refused on its field", func(t *testing.T) {
		h := newCfgHarness()
		body := h.do(t, cfgAdmin(), req{method: http.MethodPost, target: "/ui/platform/config/save", datastar: true, form: cfgSaveForm("client", "nobody")})
		mustContain(t, body, `id="cfg-form-client-err"`, "No client with this identifier.")
		if len(h.ops.sets) != 0 {
			t.Fatal("saved for an unknown client")
		}
	})
	t.Run("the edit dialog: coordinates read-only, plain value prefilled, secret not", func(t *testing.T) {
		h := newCfgHarness()
		plain := h.do(t, cfgAdmin(), req{target: "/ui/platform/config/edit?app=platform&section=branding&property=name", datastar: true})
		mustContain(t, plain, "Edit property", `name="section" value="branding" required autocomplete="off" readonly`, `name="edit" value="1"`, "Acme &lt;Portal&gt;")
		secret := h.do(t, cfgAdmin(), req{target: "/ui/platform/config/edit?app=platform&section=mail&property=api-key", datastar: true})
		mustContain(t, secret, `data-signals="{cfgSecret: true}"`, `type="password"`)
		mustNotContain(t, secret, cfgSecretValue)
		if got := h.do(t, cfgAdmin(), req{target: "/ui/platform/config/edit?app=platform&section=x&property=y", datastar: true}); !strings.Contains(got, "fc-flash-error") {
			t.Error("editing a property that is not there should fail")
		}
	})
	t.Run("native post: redirect with a flash", func(t *testing.T) {
		h := newCfgHarness()
		rec := h.rec(t, cfgAdmin(), req{method: http.MethodPost, target: "/ui/platform/config/save", form: cfgSaveForm()})
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/ui/platform/config?app=platform" ||
			!strings.Contains(rec.Header().Get("Set-Cookie"), "fc_ui_flash=config-saved") {
			t.Errorf("status %d, location %q, cookie %q", rec.Code, rec.Header().Get("Location"), rec.Header().Get("Set-Cookie"))
		}
	})
}

func TestConfigDelete(t *testing.T) {
	h := newCfgHarness()
	body := h.do(t, cfgAdmin(), req{method: http.MethodPost, target: "/ui/platform/config/delete?app=platform&section=branding&property=name", datastar: true})
	mustContain(t, body, "The property was deleted", `id="cfg-table"`)
	if strings.Join(h.cfg.deleted, ",") != "branding/name" {
		t.Errorf("deleted = %v", h.cfg.deleted)
	}
}
