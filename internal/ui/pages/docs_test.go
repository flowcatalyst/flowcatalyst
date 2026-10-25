package pages

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/docsapi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/assets"
)

const permDocsView = "platform:admin:docs:view"

type fakeDocSource struct {
	index docsapi.DocListResponse
	pages map[string]docsapi.DocResponse // "source/slug"
	err   error
}

func (f fakeDocSource) Index(context.Context) (docsapi.DocListResponse, error) { return f.index, f.err }

func (f fakeDocSource) page(source, slug string) (docsapi.DocResponse, error) {
	if p, ok := f.pages[source+"/"+slug]; ok {
		return p, nil
	}
	return docsapi.DocResponse{}, httperror.NotFound("Doc", slug)
}

func (f fakeDocSource) PlatformPage(slug string) (docsapi.DocResponse, error) {
	return f.page("platform", slug)
}

func (f fakeDocSource) ApplicationPage(_ context.Context, appCode, slug string) (docsapi.DocResponse, error) {
	return f.page(appCode, slug)
}

func newFakeDocSource() fakeDocSource {
	return fakeDocSource{
		index: docsapi.DocListResponse{
			Platform: []docsapi.DocSummary{{Slug: "overview", Title: "Overview"}, {Slug: "events", Title: "Events <guide>"}},
			Applications: []docsapi.AppDocsGroup{
				{ApplicationCode: "orders", ApplicationName: "Orders", Docs: []docsapi.DocSummary{{Slug: "intro", Title: "Orders intro"}}},
				{ApplicationCode: "billing", ApplicationName: "Billing", Docs: []docsapi.DocSummary{{Slug: "invoices", Title: "Invoices"}}},
			},
		},
		pages: map[string]docsapi.DocResponse{
			"platform/overview": {Slug: "overview", Title: "Overview", Content: "# Overview\n\nSee [events](./02-events.md#flow) and [the site](https://example.com/x?a=1&b=2).\n\n" +
				"<script>alert('raw')</script>\n\n[bad](javascript:alert(1)) <img src=x onerror=alert(2)>\n\n" +
				"```mermaid\ngraph TD\n  A[<b>Start</b>] --> B\n  B --> C[\"<i data-init='alert(2)'>x</i>\"]\n```\n\nAfter. A literal `<a href=\"https://evil.example\">` in code.\n\n```go\n// ```mermaid\nfmt.Println(\"in a go fence\")\n```\n"},
			"platform/events": {Slug: "events", Title: "Events", Content: "# Events\n\nbody"},
			"orders/intro":    {Slug: "intro", Title: "Orders intro", Content: "# Orders\n\nsee [next](next-page.md) and [billing](../billing.md)"},
		},
	}
}

func docsReader() *auth.AuthContext { return cfgUser(permDocsView) }

func serveDocs(t *testing.T, d fakeDocSource, ac *auth.AuthContext, q req) string {
	t.Helper()
	return serveDocsRec(t, d, ac, q).Body.String()
}

func TestDocsIndexOpensTheFirstPlatformPage(t *testing.T) {
	rec := serveDocsRec(t, newFakeDocSource(), docsReader(), req{target: "/ui/platform/docs"})
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/ui/platform/docs/platform/overview" {
		t.Errorf("status %d, location %q", rec.Code, rec.Header().Get("Location"))
	}
	empty := serveDocsRec(t, fakeDocSource{}, docsReader(), req{target: "/ui/platform/docs"})
	if empty.Code != http.StatusOK || !strings.Contains(empty.Body.String(), "Documentation") {
		t.Errorf("no pages: status %d", empty.Code)
	}
}

func TestDocsPage(t *testing.T) {
	d := newFakeDocSource()
	body := serveDocs(t, d, docsReader(), req{target: "/ui/platform/docs/platform/overview"})
	t.Run("navigation: the platform's pages then each application's, the current one marked", func(t *testing.T) {
		mustContain(t, body, `aria-label="Documentation pages"`, ">Platform<", ">Orders<", ">Billing<",
			`href="/ui/platform/docs/platform/events"`, "Events &lt;guide&gt;", `href="/ui/platform/docs/orders/intro"`,
			`href="/ui/platform/docs/platform/overview" class="block rounded-md px-2.5 py-1.5 text-sm hover:bg-foreground/5 bg-foreground/10 font-semibold" aria-current="page"`)
		if strings.Index(body, ">Orders<") > strings.Index(body, ">Billing<") {
			t.Error("applications are listed in the index's order (docsapi sorts them by name)")
		}
	})
	t.Run("markdown is rendered and sanitised", func(t *testing.T) {
		mustContain(t, body, `<h1 id="overview">Overview</h1>`, "After.")
		mustNotContain(t, body, "alert('raw')", "javascript:", "onerror", "src=x", `<script>`)
	})
	t.Run("text that looks like a link is not rewritten", func(t *testing.T) {
		mustContain(t, body, "&lt;a href=&#34;https://evil.example&#34;&gt;")
		if strings.Count(body, `target="_blank"`) != 1 {
			t.Errorf("target=_blank %d times, want the one real external link", strings.Count(body, `target="_blank"`))
		}
	})
	t.Run("a link to a sibling page stays in the source; an external link opens in a new tab", func(t *testing.T) {
		mustContain(t, body, `href="/ui/platform/docs/platform/events#flow"`, `href="https://example.com/x?a=1&amp;b=2" target="_blank" rel="noopener noreferrer"`)
	})
	t.Run("a mermaid fence is a diagram with its source escaped; another fence stays code", func(t *testing.T) {
		mustContain(t, body, "<fc-diagram", `data-bundle="`+assets.URL("mermaid.min.js")+`"`, "A[&lt;b&gt;Start&lt;/b&gt;] --&gt; B", assets.URL("fc-diagram.js"), "in a go fence")
		mustContain(t, body, "&lt;i data-init=&#39;alert(2)&#39;&gt;x&lt;/i&gt;", ` hidden data-ignore></div>`)
		mustNotContain(t, body, "language-mermaid", "<b>Start", "<i data-init")
		if strings.Count(body, "<fc-diagram") != 1 {
			t.Errorf("diagrams: %d", strings.Count(body, "<fc-diagram"))
		}
	})
	t.Run("an application's page", func(t *testing.T) {
		app := serveDocs(t, d, docsReader(), req{target: "/ui/platform/docs/orders/intro"})
		mustContain(t, app, `href="/ui/platform/docs/orders/next-page"`, `href="../billing.md"`)
	})
}

func TestDocsNavigationFilter(t *testing.T) {
	d := newFakeDocSource()
	t.Run("a native request filters by title or slug, keeps the label, drops groups without a match", func(t *testing.T) {
		body := serveDocs(t, d, docsReader(), req{target: "/ui/platform/docs/platform/overview?q=INVOICE"})
		mustContain(t, body, ">Billing<", "Invoices", `value="INVOICE"`)
		mustNotContain(t, body, ">Orders<", "Orders intro", "Events &lt;guide&gt;")
		mustContain(t, body, ">Platform<")
		by := serveDocs(t, d, docsReader(), req{target: "/ui/platform/docs/platform/overview?q=intro"})
		mustContain(t, by, "Orders intro")
	})
	t.Run("a live request patches the navigation only", func(t *testing.T) {
		body := serveDocs(t, d, docsReader(), req{target: "/ui/platform/docs/platform/overview?q=events", datastar: true})
		mustContain(t, body, `id="docs-nav"`, "Events &lt;guide&gt;")
		mustNotContain(t, body, "<article", "<h1")
	})
	t.Run("the filter is kept in the links", func(t *testing.T) {
		mustContain(t, serveDocs(t, d, docsReader(), req{target: "/ui/platform/docs/platform/overview?q=events"}), `href="/ui/platform/docs/platform/events?q=events"`)
	})
}

func TestDocsErrorsAndPermission(t *testing.T) {
	d := newFakeDocSource()
	t.Run("a page that is not there is a message in the pane, the list stays", func(t *testing.T) {
		rec := serveDocsRec(t, d, docsReader(), req{target: "/ui/platform/docs/platform/missing"})
		mustContain(t, rec.Body.String(), "Failed to load document", `role="alert"`, "Overview")
		if rec.Code != http.StatusNotFound {
			t.Errorf("status %d", rec.Code)
		}
	})
	t.Run("a source or slug that is not a name is not found", func(t *testing.T) {
		for _, target := range []string{"/ui/platform/docs/pla'tform/x", "/ui/platform/docs/platform/a%22b", "/ui/platform/docs/platform/%3Cscript%3E"} {
			if rec := serveDocsRec(t, d, docsReader(), req{target: target}); rec.Code != http.StatusNotFound {
				t.Errorf("%s: status %d", target, rec.Code)
			}
		}
	})
	t.Run("the docs view permission gates every route", func(t *testing.T) {
		for _, ac := range []*auth.AuthContext{cfgUser(), cfgUser(permCfgView, permCfgUpdate)} {
			for _, target := range []string{"/ui/platform/docs", "/ui/platform/docs/platform/overview", "/ui/platform/docs/orders/intro"} {
				for _, datastar := range []bool{false, true} {
					rec := serveDocsRec(t, d, ac, req{target: target, datastar: datastar})
					body := rec.Body.String()
					if datastar {
						mustContain(t, body, "fc-flash-error")
					} else if rec.Code != http.StatusForbidden {
						t.Errorf("%s: status %d", target, rec.Code)
					}
					mustNotContain(t, body, "Overview", "Orders intro")
				}
			}
		}
	})
	t.Run("a failing index is an error page, not a half page", func(t *testing.T) {
		if rec := serveDocsRec(t, fakeDocSource{err: httperror.NotFound("Doc", "x")}, docsReader(), req{target: "/ui/platform/docs/platform/overview"}); rec.Code != http.StatusNotFound {
			t.Errorf("status %d", rec.Code)
		}
	})
}

func TestSplitMermaid(t *testing.T) {
	cases := map[string]struct {
		src  string
		want []docsSegment
	}{
		"none":       {"# T\n\ntext\n", []docsSegment{{Text: "# T\n\ntext\n\n"}}},
		"one":        {"a\n```mermaid\ngraph TD\n```\nb", []docsSegment{{Text: "a\n"}, {Text: "graph TD\n", Mermaid: true}, {Text: "b\n"}}},
		"tildes":     {"~~~mermaid\nx\n~~~\n", []docsSegment{{Text: "x\n", Mermaid: true}, {Text: "\n"}}},
		"longer":     {"````mermaid\n```\nx\n````\n", []docsSegment{{Text: "```\nx\n", Mermaid: true}, {Text: "\n"}}},
		"in a fence": {"```text\n```mermaid\n```\nz", []docsSegment{{Text: "```text\n```mermaid\n```\nz\n"}}},
		"unclosed":   {"```mermaid\nx", []docsSegment{{Text: "x\n", Mermaid: true}}},
		"indented":   {"    ```mermaid\n    x\n", []docsSegment{{Text: "    ```mermaid\n    x\n\n"}}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := splitMermaid(tc.src)
			if len(got) != len(tc.want) {
				t.Fatalf("segments %+v, want %+v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("segment %d = %+v, want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func serveDocsRec(t *testing.T, d fakeDocSource, ac *auth.AuthContext, q req) *httptest.ResponseRecorder {
	t.Helper()
	return serveMounted(t, ac, q, &Docs{Source: d, Shell: shellForTest()})
}
