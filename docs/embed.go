// Package docs embeds the platform's PUBLISHED Markdown documentation —
// the curated set under published/ — so the running binary serves it to
// administrators (Platform → Documentation in the SPA, via /api/docs) and
// the pages always match the running build. Deliberately NOT the whole
// docs/ tree: plans, handovers, and other working documents stay
// repo-only. To publish a page, add it to published/ with an "NN-" order
// prefix; the prefix fixes the reading order and is stripped from the
// page's slug.
package docs

import "embed"

// FS holds the embedded published pages under "published/".
//
//go:embed published/*.md
var FS embed.FS
