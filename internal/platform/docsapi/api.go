// Package docsapi serves the platform's documentation surface to
// administrators. Two sources feed it:
//
//   - PLATFORM pages: the curated, published set under docs/published/
//     (compiled into the binary — see docs/embed.go), so the platform's own
//     documentation always matches the running build. Deliberately NOT the
//     whole docs/ tree: internal plans and handovers stay repo-only.
//   - APPLICATION pages: Markdown each application pushes through the SDK
//     sync surface (POST /api/applications/{appCode}/docs/sync — see
//     sdksync), stored in app_docs.
//
// GET /api/docs lists both, grouped; the get endpoints return raw Markdown
// for the SPA to render (Mermaid fences included).
package docsapi

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/docs"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appdocs"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// State bundles deps. AppDocs/Apps are optional — nil serves the platform
// pages alone (the applications section comes back empty).
type State struct {
	AppDocs *appdocs.Repository
	Apps    *application.Repository
}

const tag = "docs"

// Register mounts the documentation endpoints.
func Register(api huma.API, s *State) {
	g := apiroute.New(api, tag)
	apiroute.Get(g, "listDocs", "/api/docs", "List documentation: the platform's published pages plus each application's synced pages", s.list)
	apiroute.Get(g, "getPlatformDoc", "/api/docs/platform/{slug}", "Fetch one published platform page as Markdown", s.getPlatform)
	apiroute.Get(g, "getApplicationDoc", "/api/docs/applications/{appCode}/{slug}", "Fetch one application-synced page as Markdown", s.getApplication)
}

// DocSummary is one page reference.
type DocSummary struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
}

// AppDocsGroup is one application's synced pages.
type AppDocsGroup struct {
	ApplicationCode string       `json:"applicationCode"`
	ApplicationName string       `json:"applicationName"`
	Docs            []DocSummary `json:"docs"`
}

// DocListResponse is the GET /api/docs envelope.
type DocListResponse struct {
	Platform     []DocSummary   `json:"platform"`
	Applications []AppDocsGroup `json:"applications"`
}

// DocResponse is a single page. Content is raw Markdown — rendering is the
// SPA's job.
type DocResponse struct {
	Slug    string `json:"slug"`
	Title   string `json:"title"`
	Content string `json:"content"`
}

// ── platform pages (embedded, curated) ──────────────────────────────────

// publishedDoc is one embedded page: the "NN-" filename prefix fixes the
// reading order and is stripped from the slug.
type publishedDoc struct {
	file  string
	slug  string
	title string
}

var (
	publishedOnce sync.Once
	published     []publishedDoc
	publishedBy   map[string]publishedDoc
	orderPrefix   = regexp.MustCompile(`^\d+-`)
)

func publishedIndex() ([]publishedDoc, map[string]publishedDoc) {
	publishedOnce.Do(func() {
		publishedBy = map[string]publishedDoc{}
		entries, err := docs.FS.ReadDir("published")
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			slug := orderPrefix.ReplaceAllString(strings.TrimSuffix(e.Name(), ".md"), "")
			d := publishedDoc{
				file:  "published/" + e.Name(),
				slug:  slug,
				title: firstHeading("published/"+e.Name(), slug),
			}
			published = append(published, d)
			publishedBy[slug] = d
		}
		// ReadDir is name-sorted, so the NN- prefixes are the order.
	})
	return published, publishedBy
}

// firstHeading returns the page's first `# ` heading, else the fallback.
func firstHeading(file, fallback string) string {
	content, err := docs.FS.ReadFile(file)
	if err != nil {
		return fallback
	}
	for _, line := range strings.Split(string(content), "\n") {
		if after, ok := strings.CutPrefix(strings.TrimSpace(line), "# "); ok {
			return strings.TrimSpace(after)
		}
	}
	return fallback
}

// ── handlers ─────────────────────────────────────────────────────────────

func (s *State) list(ctx context.Context, _ *apicommon.Empty) (*apicommon.Out[DocListResponse], error) {
	if err := auth.CanReadPlatformDocs(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	pages, _ := publishedIndex()
	resp := DocListResponse{Platform: make([]DocSummary, 0, len(pages)), Applications: []AppDocsGroup{}}
	for _, p := range pages {
		resp.Platform = append(resp.Platform, DocSummary{Slug: p.slug, Title: p.title})
	}

	if s.AppDocs != nil && s.Apps != nil {
		appIDs, err := s.AppDocs.ApplicationIDsWithDocs(ctx)
		if err != nil {
			return nil, usecase.Internal("DOCS", "listing application docs failed", err)
		}
		for _, appID := range appIDs {
			app, aerr := s.Apps.FindByID(ctx, appID)
			if aerr != nil || app == nil {
				continue // orphaned rows never break the index
			}
			summaries, derr := s.AppDocs.ListByApplication(ctx, appID)
			if derr != nil {
				return nil, usecase.Internal("DOCS", "listing application docs failed", derr)
			}
			group := AppDocsGroup{ApplicationCode: app.Code, ApplicationName: app.Name, Docs: make([]DocSummary, 0, len(summaries))}
			for _, d := range summaries {
				group.Docs = append(group.Docs, DocSummary{Slug: d.Slug, Title: d.Title})
			}
			resp.Applications = append(resp.Applications, group)
		}
		sort.Slice(resp.Applications, func(i, j int) bool {
			return resp.Applications[i].ApplicationName < resp.Applications[j].ApplicationName
		})
	}
	return &apicommon.Out[DocListResponse]{Body: resp}, nil
}

type getPlatformDocInput struct {
	Slug string `path:"slug"`
}

func (s *State) getPlatform(ctx context.Context, in *getPlatformDocInput) (*apicommon.Out[DocResponse], error) {
	if err := auth.CanReadPlatformDocs(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	_, bySlug := publishedIndex()
	d, ok := bySlug[in.Slug]
	if !ok {
		return nil, httperror.NotFound("Doc", in.Slug)
	}
	content, err := docs.FS.ReadFile(d.file)
	if err != nil {
		return nil, httperror.NotFound("Doc", in.Slug)
	}
	return &apicommon.Out[DocResponse]{Body: DocResponse{Slug: d.slug, Title: d.title, Content: string(content)}}, nil
}

type getApplicationDocInput struct {
	AppCode string `path:"appCode"`
	Slug    string `path:"slug"`
}

func (s *State) getApplication(ctx context.Context, in *getApplicationDocInput) (*apicommon.Out[DocResponse], error) {
	if err := auth.CanReadPlatformDocs(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	if s.AppDocs == nil || s.Apps == nil {
		return nil, httperror.NotFound("Doc", in.Slug)
	}
	app, err := s.Apps.FindByCode(ctx, in.AppCode)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_application failed", err)
	}
	if app == nil {
		return nil, httperror.NotFound("Application", in.AppCode)
	}
	doc, err := s.AppDocs.GetByApplicationSlug(ctx, app.ID, in.Slug)
	if err != nil {
		return nil, usecase.Internal("DOCS", "doc lookup failed", err)
	}
	if doc == nil {
		return nil, httperror.NotFound("Doc", in.Slug)
	}
	return &apicommon.Out[DocResponse]{Body: DocResponse{Slug: doc.Slug, Title: doc.Title, Content: doc.Content}}, nil
}
