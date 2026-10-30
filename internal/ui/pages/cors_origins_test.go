package pages

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/cors"
	corsops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/cors/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/go-sdk/usecase"
)

type fakeCorsStore struct{ origins []cors.AllowedOrigin }

func (f *fakeCorsStore) FindAll(context.Context) ([]cors.AllowedOrigin, error) {
	return append([]cors.AllowedOrigin(nil), f.origins...), nil
}

func (f *fakeCorsStore) FindByID(_ context.Context, id string) (*cors.AllowedOrigin, error) {
	for i := range f.origins {
		if f.origins[i].ID == id {
			o := f.origins[i]
			return &o, nil
		}
	}
	return nil, nil
}

type fakeCorsOps struct {
	added   []corsops.AddCommand
	deleted []string
	err     error
	store   *fakeCorsStore
}

func (o *fakeCorsOps) Add(_ context.Context, cmd corsops.AddCommand) (string, error) {
	o.added = append(o.added, cmd)
	if o.err != nil {
		return "", o.err
	}
	o.store.origins = append(o.store.origins, corsOrigin("cor_9", cmd.Origin, deref(cmd.Description)))
	return "cor_9", nil
}

func (o *fakeCorsOps) Delete(_ context.Context, id string) error {
	o.deleted = append(o.deleted, id)
	return o.err
}

func corsOrigin(id, origin, desc string) cors.AllowedOrigin {
	day := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	o := cors.AllowedOrigin{ID: id, Origin: origin, CreatedAt: day, UpdatedAt: day}
	if desc != "" {
		o.Description = &desc
	}
	return o
}

type corsHarness struct {
	store *fakeCorsStore
	ops   *fakeCorsOps
}

func newCorsHarness() *corsHarness {
	s := &fakeCorsStore{origins: []cors.AllowedOrigin{
		corsOrigin("cor_1", "https://app.example.com", "Production <SPA>"),
		corsOrigin("cor_2", "http://localhost:3000", ""),
	}}
	return &corsHarness{store: s, ops: &fakeCorsOps{store: s}}
}

func (h *corsHarness) rec(t *testing.T, ac *auth.AuthContext, q req) *httptest.ResponseRecorder {
	t.Helper()
	return serveMounted(t, ac, q, &CorsOrigins{Store: h.store, Ops: h.ops, Shell: shellForTest()})
}

const (
	corsViewPermTest   = "platform:admin:cors-origin:view"
	corsCreatePermTest = "platform:admin:cors-origin:create"
	corsDeletePermTest = "platform:admin:cors-origin:delete"
)

func TestCorsOriginsList(t *testing.T) {
	h := newCorsHarness()
	body := h.rec(t, anchorAdmin(), req{target: "/ui/cors-origins"}).Body.String()
	mustContain(t, body, "<title>CORS Origins · FlowCatalyst</title>", "Origins are cached for 5 minutes.",
		"https://app.example.com", "Production &lt;SPA&gt;", "http://localhost:3000", `datetime="2026-10-01T09:00:00Z"`,
		`href="/ui/cors-origins/new"`, "Add Origin", `href="/ui/cors-origins/cor_1/delete"`,
		`href="/ui/cors-origins" class="fc-nav-item" title="CORS Origins" aria-current="page"`)
	mustNotContain(t, body, "Production <SPA>")

	// Search reads the origin and the description, any case.
	live := h.rec(t, anchorAdmin(), req{target: "/ui/cors-origins?q=production", datastar: true}).Body.String()
	mustContain(t, live, "https://app.example.com", "data: mode replace", "history.replaceState")
	mustNotContain(t, live, "localhost")
	mustContain(t, h.rec(t, anchorAdmin(), req{target: "/ui/cors-origins?q=zzz"}).Body.String(), "No CORS origins found")
	h.store.origins = nil
	mustContain(t, h.rec(t, anchorAdmin(), req{target: "/ui/cors-origins"}).Body.String(), "No CORS origins configured")
}

func TestCorsOriginsSort(t *testing.T) {
	h := newCorsHarness()
	order := func(target string) string {
		body := h.rec(t, anchorAdmin(), req{target: target}).Body.String()
		return map[bool]string{true: "app-first", false: "localhost-first"}[strings.Index(body, "app.example.com") < strings.Index(body, "localhost:3000")]
	}
	if got := order("/ui/cors-origins"); got != "localhost-first" {
		t.Errorf("by origin: %s", got)
	}
	if got := order("/ui/cors-origins?sort=origin&dir=desc"); got != "app-first" {
		t.Errorf("by origin, descending: %s", got)
	}
	if got := order("/ui/cors-origins?sort=description"); got != "localhost-first" {
		t.Errorf("by description (blank first): %s", got)
	}
}

// The permission matrix: the page needs anchor + view; the controls follow
// create and delete; a forged post without them is refused and runs nothing.
func TestCorsOriginsPermissions(t *testing.T) {
	h := newCorsHarness()
	viewer := connUser(corsViewPermTest)
	body := h.rec(t, viewer, req{target: "/ui/cors-origins"}).Body.String()
	mustContain(t, body, "https://app.example.com")
	mustNotContain(t, body, "Add Origin", "/ui/cors-origins/new", "/delete")

	adder := h.rec(t, connUser(corsViewPermTest, corsCreatePermTest), req{target: "/ui/cors-origins"}).Body.String()
	mustContain(t, adder, "Add Origin")
	mustNotContain(t, adder, "/delete")
	remover := h.rec(t, connUser(corsViewPermTest, corsDeletePermTest), req{target: "/ui/cors-origins"}).Body.String()
	mustContain(t, remover, `href="/ui/cors-origins/cor_1/delete"`)
	mustNotContain(t, remover, "Add Origin")

	if rec := h.rec(t, connUser(corsCreatePermTest), req{target: "/ui/cors-origins"}); rec.Code != http.StatusForbidden {
		t.Errorf("no view permission: %d", rec.Code)
	}
	client := &auth.AuthContext{PrincipalID: "p", Scope: auth.ScopeClient, Clients: []string{"clt_1"}, Roles: []string{"r"}, Permissions: []string{corsViewPermTest, corsCreatePermTest, corsDeletePermTest}, PrincipalType: auth.PrincipalTypeUser}
	forged := []req{
		{target: "/ui/cors-origins"},
		{target: "/ui/cors-origins/new"},
		{target: "/ui/cors-origins/cor_1/delete"},
		{method: http.MethodPost, target: "/ui/cors-origins/new", form: url.Values{"origin": {"https://evil.example"}}},
		{method: http.MethodPost, target: "/ui/cors-origins/cor_1/delete"},
	}
	for _, q := range forged {
		if rec := h.rec(t, client, q); rec.Code != http.StatusForbidden {
			t.Errorf("client-scoped %s %s: %d", q.method, q.target, rec.Code)
		}
	}
	for _, q := range forged[1:] {
		if rec := h.rec(t, viewer, q); rec.Code != http.StatusForbidden {
			t.Errorf("view-only %s %s: %d", q.method, q.target, rec.Code)
		}
		// A Datastar request is answered with the error flash, or in the add form.
		q.datastar = true
		if body := h.rec(t, viewer, q).Body.String(); !strings.Contains(body, "fc-flash-error") && !strings.Contains(body, "Permission required") {
			t.Errorf("view-only (Datastar) %s %s: no refusal in %s", q.method, q.target, body)
		}
	}
	if len(h.ops.added)+len(h.ops.deleted) != 0 {
		t.Errorf("a refused request ran %v %v", h.ops.added, h.ops.deleted)
	}
}

func TestCorsOriginsAdd(t *testing.T) {
	h := newCorsHarness()
	form := h.rec(t, anchorAdmin(), req{target: "/ui/cors-origins/new", datastar: true}).Body.String()
	mustContain(t, form, "Add CORS Origin", `action="/ui/cors-origins/new"`, "Origin URL", "https://app.example.com", "No trailing slash or path", "Description", `data: elements <div id="cors-dialog-slot">`)
	page := h.rec(t, anchorAdmin(), req{target: "/ui/cors-origins/new"}).Body.String()
	mustContain(t, page, `id="cors-add"`, "Add CORS Origin")

	post := func(origin, desc string, datastar bool) *httptest.ResponseRecorder {
		return h.rec(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/cors-origins/new", datastar: datastar, multipart: datastar, form: url.Values{"origin": {origin}, "description": {desc}}})
	}
	// The SPA's own checks answer on the field, before the use case runs.
	for origin, want := range map[string]string{"": "Origin URL is required", "   ": "Origin URL is required", "ftp://x.example": "Origin must start with http:// or https://", "app.example.com": "Origin must start with http:// or https://"} {
		mustContain(t, post(origin, "", true).Body.String(), `id="cors-add-origin-err"`, want)
	}
	if rec := post("ftp://x", "", false); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("native post: %d", rec.Code)
	}
	if len(h.ops.added) != 0 {
		t.Fatalf("invalid input reached the use case: %v", h.ops.added)
	}

	// Lower-cased and trimmed; a blank description is none.
	ok := post("  HTTPS://New.Example.com:8443 ", "  ", true).Body.String()
	mustContain(t, ok, "CORS origin &#34;https://new.example.com:8443&#34; added successfully", "data: mode replace", "new.example.com:8443")
	if len(h.ops.added) != 1 || h.ops.added[0].Origin != "https://new.example.com:8443" || h.ops.added[0].Description != nil {
		t.Errorf("command %+v", h.ops.added)
	}
	post("https://described.example.com", " Staging ", true)
	if got := h.ops.added[1]; deref(got.Description) != "Staging" {
		t.Errorf("description %+v", got)
	}
	native := post("https://native.example.com", "", false)
	if native.Code != http.StatusSeeOther || native.Header().Get("Location") != "/ui/cors-origins" {
		t.Errorf("native add: %d %s", native.Code, native.Header().Get("Location"))
	}

	// The use case's refusals land on the field (a duplicate) or the form.
	h.ops.err = usecase.Conflict("ORIGIN_ALREADY_EXISTS", "CORS origin 'https://app.example.com' already exists")
	mustContain(t, post("https://app.example.com", "", true).Body.String(), `id="cors-add-origin-err"`, "already exists")
	h.ops.err = usecase.Validation("INVALID_ORIGIN_FORMAT", "Origin must be a valid URL (e.g. https://example.com or http://localhost:3000)")
	mustContain(t, post("https://exa mple", "", true).Body.String(), `id="cors-add-origin-err"`, "must be a valid URL")
}

func TestCorsOriginsRemove(t *testing.T) {
	h := newCorsHarness()
	dialog := h.rec(t, anchorAdmin(), req{target: "/ui/cors-origins/cor_1/delete", datastar: true}).Body.String()
	mustContain(t, dialog, "Remove CORS Origin", "Are you sure you want to remove this CORS origin?", "https://app.example.com",
		`action="/ui/cors-origins/cor_1/delete"`, "up to 5 minutes", `data: elements <div id="cors-dialog-slot">`)
	mustContain(t, h.rec(t, anchorAdmin(), req{target: "/ui/cors-origins/cor_1/delete"}).Body.String(), `id="cors-delete"`, "Cached for up to 5 minutes")
	if rec := h.rec(t, anchorAdmin(), req{target: "/ui/cors-origins/cor_404/delete"}); rec.Code != http.StatusNotFound {
		t.Errorf("unknown origin: %d", rec.Code)
	}

	gone := h.rec(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/cors-origins/cor_1/delete", datastar: true, multipart: true, referer: "/ui/cors-origins"}).Body.String()
	mustContain(t, gone, "CORS origin &#34;https://app.example.com&#34; removed", "data: mode replace")
	if len(h.ops.deleted) != 1 || h.ops.deleted[0] != "cor_1" {
		t.Errorf("deleted %v", h.ops.deleted)
	}
	native := h.rec(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/cors-origins/cor_2/delete"})
	if native.Code != http.StatusSeeOther {
		t.Errorf("native remove: %d", native.Code)
	}
	if rec := h.rec(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/cors-origins/bad'id/delete"}); rec.Code != http.StatusNotFound {
		t.Errorf("an id that is not an id: %d", rec.Code)
	}
}
