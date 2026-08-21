// Package docs embeds the platform's Markdown documentation so the running
// binary can serve it to administrators (Platform → Documentation in the
// SPA, via /api/docs). Top-level *.md files only — the adr/ subtree and
// anything non-Markdown stay repo-only.
package docs

import "embed"

// FS holds the embedded documentation pages, keyed by filename.
//
//go:embed *.md
var FS embed.FS
