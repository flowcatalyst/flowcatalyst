package pages

import (
	"context"
	"html"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/docsapi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/md"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
)

// DocsSource is the slice of docsapi.State the page reads: the same listing
// and page lookups the HTTP API serves (GET /api/docs and the two page reads).
type DocsSource interface {
	Index(ctx context.Context) (docsapi.DocListResponse, error)
	PlatformPage(slug string) (docsapi.DocResponse, error)
	ApplicationPage(ctx context.Context, appCode, slug string) (docsapi.DocResponse, error)
}

// Docs is the Documentation page (/ui/platform/docs), mirroring the SPA's
// DocsPage: the platform's published pages and each application's synced
// pages, addressed as {source}/{slug}. Markdown is rendered on the server
// through the sanitising policy of internal/ui/md; the pages are shown to
// holders of the docs view permission, as GET /api/docs.
type Docs struct {
	Source DocsSource
	Shell  *shell.Shell
}

const (
	docsPath  = "/ui/platform/docs"
	docsNavID = "docs-nav"
)

// docsNamePattern is what a source (an application code) and a slug may
// contain; it keeps both safe to put in a URL and an expression.
var docsNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)

// Mount registers the page's routes (read-only: no POSTs).
func (h *Docs) Mount(r chi.Router) {
	r.Get("/platform/docs", h.index)
	r.Get("/platform/docs/{source}/{slug}", h.page)
}

type docsLink struct {
	Title, Href string
	Active      bool
}

type docsGroup struct {
	Label string
	Links []docsLink
}

type docsView struct {
	// Q is the navigation filter; Self the address of the page shown, which the
	// filter form submits to.
	Q, Self string
	Groups  []docsGroup
	// Content is the rendered page; Problem the message that stands in for it
	// when the page could not be loaded.
	Content templ.Component
	Problem string
}

func docsURL(source, slug, q string) string {
	u := docsPath + "/" + url.PathEscape(source) + "/" + url.PathEscape(slug)
	if q != "" {
		u += "?q=" + url.QueryEscape(q)
	}
	return u
}

// docsNav lists the pages that match the filter (the SPA's: title or slug,
// case-insensitive), the platform's first, then each application's.
func docsNav(idx docsapi.DocListResponse, q, source, slug string) []docsGroup {
	needle := strings.ToLower(strings.TrimSpace(q))
	match := func(d docsapi.DocSummary) bool {
		return needle == "" || strings.Contains(strings.ToLower(d.Title), needle) || strings.Contains(strings.ToLower(d.Slug), needle)
	}
	group := func(label, src string, docs []docsapi.DocSummary) docsGroup {
		g := docsGroup{Label: label}
		for _, d := range docs {
			if match(d) {
				g.Links = append(g.Links, docsLink{Title: d.Title, Href: docsURL(src, d.Slug, q), Active: src == source && d.Slug == slug})
			}
		}
		return g
	}
	// The platform's label stays when nothing matches, as in the SPA; an
	// application with no match is left out.
	groups := []docsGroup{group("Platform", docsapi.SourcePlatform, idx.Platform)}
	for _, a := range idx.Applications {
		if g := group(a.ApplicationName, a.ApplicationCode, a.Docs); len(g.Links) > 0 {
			groups = append(groups, g)
		}
	}
	return groups
}

// index opens the first published page, as the SPA's redirect does.
func (h *Docs) index(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := auth.CanReadPlatformDocs(auth.FromContext(ctx)); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	idx, err := h.Source.Index(ctx)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	q := r.URL.Query().Get("q")
	if len(idx.Platform) > 0 {
		http.Redirect(w, r, docsURL(docsapi.SourcePlatform, idx.Platform[0].Slug, q), http.StatusFound) //nolint:gosec // G710: a local path; the slug is path-escaped and q query-escaped
		return
	}
	h.render(w, r, http.StatusOK, docsView{Q: q, Self: docsPath, Groups: docsNav(idx, q, "", "")})
}

func (h *Docs) page(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := auth.CanReadPlatformDocs(auth.FromContext(ctx)); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	source, slug := chi.URLParam(r, "source"), chi.URLParam(r, "slug")
	if !docsNamePattern.MatchString(source) || !docsNamePattern.MatchString(slug) {
		h.Shell.Fail(w, r, httperror.NotFound("Doc", slug))
		return
	}
	idx, err := h.Source.Index(ctx)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	q := r.URL.Query().Get("q")
	v := docsView{Q: q, Self: docsURL(source, slug, ""), Groups: docsNav(idx, q, source, slug)}
	// Typing in the filter re-renders the navigation only.
	if ds.IsDatastar(r) {
		if err := ds.NewSSE(w, r).Patch(docsNavigation(v)); err != nil {
			slog.DebugContext(ctx, "ui: documentation patch not delivered", "err", err)
		}
		return
	}
	status := http.StatusOK
	var doc docsapi.DocResponse
	if source == docsapi.SourcePlatform {
		doc, err = h.Source.PlatformPage(slug)
	} else {
		doc, err = h.Source.ApplicationPage(ctx, source, slug)
	}
	if err != nil {
		var code int
		code, v.Problem = shell.Classify(r, err)
		if code != http.StatusNotFound {
			h.Shell.Fail(w, r, err)
			return
		}
		status = code
		v.Problem = "Failed to load document: " + v.Problem
	} else {
		v.Content = docsContent(doc.Content, source)
	}
	h.render(w, r, status, v)
}

func (h *Docs) render(w http.ResponseWriter, r *http.Request, status int, v docsView) {
	h.Shell.RenderStatus(w, r, status, shell.Page{Title: "Documentation", Path: docsPath}, docsPage(v))
}

// ── rendering a page ───────────────────────────────────────────────────

// docsSegment is a run of Markdown or one mermaid diagram of a page.
type docsSegment struct {
	Text    string
	Mermaid bool
}

// splitMermaid cuts a page at its ```mermaid fences: the diagrams are drawn
// by the Mermaid island from their source (the SPA upgrades the fences after
// sanitising, because the sanitiser would mangle inline SVG), the rest is
// rendered as Markdown. A fence of another language that mentions mermaid in
// its text is not a diagram.
func splitMermaid(src string) []docsSegment {
	var out []docsSegment
	var text, diagram strings.Builder
	var closing string // the marker that closes the fence being read
	inDiagram := false
	flush := func() {
		if text.Len() > 0 {
			out = append(out, docsSegment{Text: text.String()})
			text.Reset()
		}
	}
	for line := range strings.SplitSeq(src, "\n") {
		switch {
		case inDiagram && fenceCloses(line, closing):
			out = append(out, docsSegment{Text: diagram.String(), Mermaid: true})
			diagram.Reset()
			inDiagram = false
		case inDiagram:
			diagram.WriteString(line + "\n")
		case closing != "":
			text.WriteString(line + "\n")
			if fenceCloses(line, closing) {
				closing = ""
			}
		default:
			if marker, info, ok := fenceOpens(line); ok {
				closing = marker
				if info == "mermaid" {
					flush()
					inDiagram = true
					continue
				}
			}
			text.WriteString(line + "\n")
		}
	}
	if inDiagram {
		out = append(out, docsSegment{Text: diagram.String(), Mermaid: true})
	}
	flush()
	return out
}

// fenceOpens reports a code fence's opening line: its marker (the run of
// backticks or tildes) and info string.
func fenceOpens(line string) (marker, info string, ok bool) {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 || len(trimmed) < 3 || (trimmed[0] != '`' && trimmed[0] != '~') {
		return "", "", false
	}
	run := len(trimmed) - len(strings.TrimLeft(trimmed, trimmed[:1]))
	if run < 3 {
		return "", "", false
	}
	info = strings.TrimSpace(trimmed[run:])
	if trimmed[0] == '`' && strings.Contains(info, "`") {
		return "", "", false
	}
	return trimmed[:run], info, true
}

// fenceCloses reports whether line closes the fence opened with marker.
func fenceCloses(line, marker string) bool {
	trimmed := strings.TrimLeft(line, " ")
	trimmed = strings.TrimRight(trimmed, " \t")
	return len(line)-len(strings.TrimLeft(line, " ")) <= 3 && len(trimmed) >= len(marker) && strings.Trim(trimmed, marker[:1]) == ""
}

var (
	// docsAnchorPattern finds the links of sanitised output (bluemonday writes
	// the href first and double-quoted).
	docsAnchorPattern = regexp.MustCompile(`<a href="([^"]*)"`)
	// docsSiblingPattern is the SPA's cross-document link: a link to another
	// page's .md file stays in the current source.
	docsSiblingPattern = regexp.MustCompile(`^(?:\./)?(?:docs/)?(?:published/)?(?:\d+-)?([a-zA-Z0-9_-]+)\.md(#.*)?$`)
)

// docsLinks rewrites the links of a rendered page as the SPA does on click: a
// link to a sibling .md file opens that page of the same source; an external
// link leaves in a new tab. The input is sanitised output.
func docsLinks(rendered, source string) string {
	return docsAnchorPattern.ReplaceAllStringFunc(rendered, func(a string) string {
		href := docsAnchorPattern.FindStringSubmatch(a)[1]
		if m := docsSiblingPattern.FindStringSubmatch(href); m != nil {
			return `<a href="` + html.EscapeString(docsPath+"/"+url.PathEscape(source)+"/"+m[1]) + m[2] + `"`
		}
		if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
			return a + ` target="_blank" rel="noopener noreferrer"`
		}
		return a
	})
}

// docsContent renders a page: the Markdown through the sanitising policy
// (raw HTML in a page is dropped), its mermaid fences as diagrams.
func docsContent(src, source string) templ.Component {
	var segs []docsSegment
	for _, s := range splitMermaid(src) {
		if !s.Mermaid {
			rendered, err := md.HTML(s.Text)
			if err != nil {
				// A render failure shows the source as text, as kit.Markdown does.
				rendered = "<pre>" + html.EscapeString(s.Text) + "</pre>"
			}
			s.Text = docsLinks(rendered, source)
		}
		segs = append(segs, s)
	}
	return docsBody(segs)
}
