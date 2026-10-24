package pages

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
)

type namesHarness struct {
	cfg *fakeConfig
	ops *fakeConfigOps
}

func newNamesHarness(stored string) *namesHarness {
	h := &namesHarness{cfg: newFakeConfig(), ops: &fakeConfigOps{}}
	if stored != "" {
		h.cfg.rows = append(h.cfg.rows, cfgRowFixture("pfc_20", "branding", "platform-name", platformconfig.ValuePlain, stored))
	}
	return h
}

func (h *namesHarness) rec(t *testing.T, ac *auth.AuthContext, q req) *httptest.ResponseRecorder {
	t.Helper()
	return serveMounted(t, ac, q, &NamesSettings{Config: h.cfg, Ops: h.ops, Shell: shellForTest()})
}

func (h *namesHarness) serve(t *testing.T, ac *auth.AuthContext, q req) string {
	t.Helper()
	return h.rec(t, ac, q).Body.String()
}

func TestNamesPage(t *testing.T) {
	t.Run("the stored name; the default when there is none or it is blank", func(t *testing.T) {
		mustContain(t, newNamesHarness("  Acme <Portal> ").serve(t, cfgAdmin(), req{target: "/ui/platform/settings/names"}),
			"Names", `name="platformName" value="Acme &lt;Portal&gt;"`, "passkey prompt name updates on the next server restart")
		mustContain(t, newNamesHarness("").serve(t, cfgAdmin(), req{target: "/ui/platform/settings/names"}), `value="FlowCatalyst"`)
		mustContain(t, newNamesHarness("   ").serve(t, cfgAdmin(), req{target: "/ui/platform/settings/names"}), `value="FlowCatalyst"`)
	})
	t.Run("a client-scoped value of the same coordinates is not the platform name", func(t *testing.T) {
		h := newNamesHarness("")
		row := cfgRowFixture("pfc_21", "branding", "platform-name", platformconfig.ValuePlain, "Not Global")
		cid := "clt_acme"
		row.Scope = platformconfig.ScopeClient
		row.ClientID = ids.PtrOf[ids.ClientID](&cid)
		h.cfg.rows = append(h.cfg.rows, row)
		mustContain(t, h.serve(t, cfgAdmin(), req{target: "/ui/platform/settings/names"}), `value="FlowCatalyst"`)
	})
	t.Run("reset to default fills the field and saves nothing", func(t *testing.T) {
		h := newNamesHarness("Acme")
		mustContain(t, h.serve(t, cfgAdmin(), req{target: "/ui/platform/settings/names?default=1"}), `value="FlowCatalyst"`)
		if len(h.ops.sets) != 0 {
			t.Fatal("a GET saved")
		}
	})
}

func TestNamesPermissionMatrix(t *testing.T) {
	t.Run("a viewer sees the name without controls to change it", func(t *testing.T) {
		body := newNamesHarness("Acme").serve(t, cfgUser(permCfgView), req{target: "/ui/platform/settings/names"})
		mustContain(t, body, `value="Acme"`, "disabled")
		mustNotContain(t, body, ">Save<", "Reset to default")
	})
	t.Run("an updater sees them", func(t *testing.T) {
		mustContain(t, newNamesHarness("Acme").serve(t, cfgUser(permCfgView, permCfgUpdate), req{target: "/ui/platform/settings/names"}), "Save", "Reset to default")
	})
	t.Run("without the view permission the page is refused", func(t *testing.T) {
		if rec := newNamesHarness("Acme").rec(t, cfgUser(), req{target: "/ui/platform/settings/names"}); rec.Code != http.StatusForbidden {
			t.Errorf("status %d", rec.Code)
		}
	})
	t.Run("forged posts from a viewer or nobody change nothing", func(t *testing.T) {
		h := newNamesHarness("Acme")
		for _, ac := range []*auth.AuthContext{cfgUser(permCfgView), cfgUser()} {
			form := cfgSaveForm("platformName", "Hacked")
			if rec := h.rec(t, ac, req{method: http.MethodPost, target: "/ui/platform/settings/names", form: form}); rec.Code != http.StatusForbidden {
				t.Errorf("status %d", rec.Code)
			}
			mustContain(t, h.serve(t, ac, req{method: http.MethodPost, target: "/ui/platform/settings/names", form: form, datastar: true}), "fc-flash-error")
		}
		if len(h.ops.sets) != 0 {
			t.Fatalf("forged posts saved: %v", h.ops.sets)
		}
	})
}

func TestNamesSave(t *testing.T) {
	post := func(name string, datastar bool) req {
		return req{method: http.MethodPost, target: "/ui/platform/settings/names", datastar: datastar, form: cfgSaveForm("platformName", name)}
	}
	t.Run("the SPA's PUT: trimmed, PLAIN, at platform/branding/platform-name", func(t *testing.T) {
		h := newNamesHarness("Acme")
		body := h.serve(t, cfgAdmin(), post("  Acme Two  ", true))
		if len(h.ops.sets) != 1 {
			t.Fatalf("sets %d: %s", len(h.ops.sets), body)
		}
		cmd := h.ops.sets[0]
		if cmd.ApplicationCode != "platform" || cmd.Section != "branding" || cmd.Property != "platform-name" || cmd.Value != "Acme Two" ||
			deref(cmd.ValueType) != "PLAIN" || deref(cmd.Description) != "Platform display name" || cmd.ClientID != nil {
			t.Errorf("command = %+v", cmd)
		}
		mustContain(t, body, "Platform name updated.")
	})
	t.Run("an empty name is refused with the SPA's message", func(t *testing.T) {
		h := newNamesHarness("Acme")
		body := h.serve(t, cfgAdmin(), post("   ", true))
		mustContain(t, body, "Platform name cannot be empty.", `id="names-platform-name-err"`, `document.getElementById(&#34;names-platform-name&#34;)?.focus()`)
		if rec := h.rec(t, cfgAdmin(), post("", false)); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "Platform name cannot be empty.") {
			t.Errorf("native: status %d", rec.Code)
		}
		if len(h.ops.sets) != 0 {
			t.Fatal("saved an empty name")
		}
	})
	t.Run("a name too long to print in an email is refused", func(t *testing.T) {
		h := newNamesHarness("Acme")
		mustContain(t, h.serve(t, cfgAdmin(), post(strings.Repeat("x", namesMax+1), true)), "Use at most 200 characters.")
		if len(h.ops.sets) != 0 {
			t.Fatal("saved a long name")
		}
	})
	t.Run("a refusal of the use case is shown on the field", func(t *testing.T) {
		h := newNamesHarness("Acme")
		h.ops.err = httperror.Forbidden("no write access")
		mustContain(t, h.serve(t, cfgAdmin(), post("Acme Two", true)), "No write access")
	})
	t.Run("native post: redirect with a flash", func(t *testing.T) {
		rec := newNamesHarness("Acme").rec(t, cfgAdmin(), post("Acme Two", false))
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/ui/platform/settings/names" || !strings.Contains(rec.Header().Get("Set-Cookie"), "fc_ui_flash=platform-name-saved") {
			t.Errorf("status %d, location %q, cookie %q", rec.Code, rec.Header().Get("Location"), rec.Header().Get("Set-Cookie"))
		}
	})
}
