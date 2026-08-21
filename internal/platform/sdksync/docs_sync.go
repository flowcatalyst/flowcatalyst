package sdksync

import (
	"context"
	"regexp"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appdocs"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// Documentation sync: POST /api/applications/{appCode}/docs/sync. The app's
// repo is the source of truth — every call replaces the app's whole doc set
// (declarative; there is no removeUnlisted flag because the payload IS the
// set). Pages surface to administrators under Platform → Documentation.

// docSlugPattern bounds slugs to URL-safe kebab-case.
var docSlugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

const (
	maxDocsPerApp  = 100
	maxDocBytes    = 512 * 1024 // one page
	maxDocsPayload = 4 << 20    // whole sync
)

type syncDocInputRequest struct {
	Slug string `json:"slug" doc:"URL-safe page id, unique within the application (kebab-case)"`
	// Title is optional — derived from the content's first `# ` heading,
	// then the slug, when absent.
	Title   *string `json:"title,omitempty"`
	Content string  `json:"content" doc:"The page body, Markdown (Mermaid fences render as diagrams)"`
}

type syncDocsRequest struct {
	Docs []syncDocInputRequest `json:"docs"`
}

type syncDocsInput struct {
	AppCode string `path:"appCode" doc:"Application code"`
	Body    syncDocsRequest
}

func (s *State) syncAppDocs(ctx context.Context, in *syncDocsInput) (*syncResultOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanSyncAppDocs(ac); err != nil {
		return nil, err
	}
	if s.AppDocs == nil {
		return nil, usecase.Internal("DOCS", "documentation sync is not wired", nil)
	}
	app, err := s.resolveApp(ctx, in.AppCode)
	if err != nil {
		return nil, err
	}
	if err := s.requireAppAccess(ac, app); err != nil {
		return nil, err
	}

	if len(in.Body.Docs) > maxDocsPerApp {
		return nil, usecase.Validation("TOO_MANY_DOCS", "an application may sync at most 100 documentation pages")
	}
	total := 0
	seen := map[string]bool{}
	inputs := make([]appdocs.Input, 0, len(in.Body.Docs))
	for _, d := range in.Body.Docs {
		slug := strings.TrimSpace(d.Slug)
		if !docSlugPattern.MatchString(slug) {
			return nil, usecase.Validation("SLUG_INVALID",
				"doc slug "+slug+" must be kebab-case (lowercase letters, digits, hyphens)")
		}
		if seen[slug] {
			return nil, usecase.Validation("SLUG_DUPLICATE", "doc slug "+slug+" appears more than once")
		}
		seen[slug] = true
		if len(d.Content) > maxDocBytes {
			return nil, usecase.Validation("DOC_TOO_LARGE", "doc "+slug+" exceeds 512KB")
		}
		total += len(d.Content)
		if total > maxDocsPayload {
			return nil, usecase.Validation("PAYLOAD_TOO_LARGE", "documentation sync exceeds 4MB total")
		}
		inputs = append(inputs, appdocs.Input{
			Slug:    slug,
			Title:   docTitle(d, slug),
			Content: d.Content,
		})
	}

	res, err := s.AppDocs.ReplaceForApplication(ctx, app.ID, inputs)
	if err != nil {
		return nil, usecase.Internal("DOCS", "documentation sync failed", err)
	}
	return &syncResultOutput{Body: SyncResultResponse{
		ApplicationCode: app.Code,
		Created:         uint32(res.Created),
		Updated:         uint32(res.Updated),
		Deleted:         uint32(res.Deleted),
		SyncedCodes:     res.Slugs,
	}}, nil
}

// docTitle resolves a page title: explicit, else the first `# ` heading,
// else the slug.
func docTitle(d syncDocInputRequest, slug string) string {
	if d.Title != nil && strings.TrimSpace(*d.Title) != "" {
		return strings.TrimSpace(*d.Title)
	}
	for _, line := range strings.Split(d.Content, "\n") {
		if after, ok := strings.CutPrefix(strings.TrimSpace(line), "# "); ok {
			return strings.TrimSpace(after)
		}
	}
	return slug
}
