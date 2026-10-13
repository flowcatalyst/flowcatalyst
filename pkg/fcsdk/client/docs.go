package client

import "context"

// SyncDocInput — one documentation page. Slug is the page id, kebab-case and
// unique within the application; Content is Markdown (Mermaid fences render
// as diagrams); Title defaults to the content's first "# " heading, then the
// slug.
type SyncDocInput struct {
	Slug    string `json:"slug"`
	Title   string `json:"title,omitempty"`
	Content string `json:"content"`
}

// SyncDocsRequest — body for POST /api/applications/{appCode}/docs/sync.
type SyncDocsRequest struct {
	Docs []SyncDocInput `json:"docs"`
}

// DocsResource — an application's documentation pages, shown to
// administrators under Platform → Documentation.
type DocsResource struct {
	c *FlowCatalystClient
}

// Docs returns the documentation accessor —
// /api/applications/{appCode}/docs/*.
func (c *FlowCatalystClient) Docs() *DocsResource {
	return &DocsResource{c: c}
}

// Sync — POST /api/applications/{appCode}/docs/sync. Declarative full
// replace: the pages sent ARE the application's set (there is no
// removeUnlisted flag; an empty list removes every page). At most 100 pages,
// 512 KiB each, 4 MiB in all.
func (r *DocsResource) Sync(ctx context.Context, appCode string, req *SyncDocsRequest) (*SyncResult, error) {
	var out SyncResult
	if err := r.c.Post(ctx, syncPath(appCode, "docs", false), req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
