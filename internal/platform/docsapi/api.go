// Package docsapi serves the platform's embedded Markdown documentation
// (the repo's docs/*.md, compiled into the binary — see docs/embed.go) to
// administrators: GET /api/docs lists the pages, GET /api/docs/{slug}
// returns one page's raw Markdown for the SPA to render. Content is
// read-only and ships with the build, so the pages always match the
// running platform version.
package docsapi

import (
	"context"
	"regexp"
	"sort"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/docs"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// State carries no dependencies — the content is the embedded FS.
type State struct{}

const tag = "docs"

// Register mounts the documentation endpoints.
func Register(api huma.API, s *State) {
	g := apiroute.New(api, tag)
	apiroute.Get(g, "listDocs", "/api/docs", "List platform documentation pages", s.list)
	apiroute.Get(g, "getDoc", "/api/docs/{slug}", "Fetch one documentation page as Markdown", s.get)
}

// DocSummary is one row of GET /api/docs.
type DocSummary struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
}

// DocListResponse is the GET /api/docs envelope.
type DocListResponse struct {
	Docs []DocSummary `json:"docs"`
}

// DocResponse is the GET /api/docs/{slug} body. Content is raw Markdown
// (Mermaid fences included) — rendering is the SPA's job.
type DocResponse struct {
	Slug    string `json:"slug"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

// slugPattern is the whole filename-derived slug alphabet. Anything else
// (path separators, dots beyond the stripped extension) is rejected before
// touching the FS — defense in depth on top of embed.FS's own sandbox.
var slugPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func (s *State) list(ctx context.Context, _ *apicommon.Empty) (*apicommon.Out[DocListResponse], error) {
	if err := auth.CanReadPlatformDocs(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	entries, err := docs.FS.ReadDir(".")
	if err != nil {
		return nil, usecase.Internal("DOCS", "embedded docs unreadable", err)
	}
	out := make([]DocSummary, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		slug := strings.TrimSuffix(e.Name(), ".md")
		out = append(out, DocSummary{Slug: slug, Title: titleOf(e.Name(), slug)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Title < out[j].Title })
	return &apicommon.Out[DocListResponse]{Body: DocListResponse{Docs: out}}, nil
}

type getDocInput struct {
	Slug string `path:"slug"`
}

func (s *State) get(ctx context.Context, in *getDocInput) (*apicommon.Out[DocResponse], error) {
	if err := auth.CanReadPlatformDocs(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	if !slugPattern.MatchString(in.Slug) {
		return nil, httperror.NotFound("Doc", in.Slug)
	}
	name := in.Slug + ".md"
	content, err := docs.FS.ReadFile(name)
	if err != nil {
		return nil, httperror.NotFound("Doc", in.Slug)
	}
	return &apicommon.Out[DocResponse]{Body: DocResponse{
		Slug:    in.Slug,
		Title:   titleOf(name, in.Slug),
		Content: string(content),
	}}, nil
}

// titleOf returns the page's first `# ` heading, falling back to the slug.
func titleOf(name, fallback string) string {
	content, err := docs.FS.ReadFile(name)
	if err != nil {
		return fallback
	}
	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(trimmed, "# "); ok {
			return strings.TrimSpace(after)
		}
	}
	return fallback
}
