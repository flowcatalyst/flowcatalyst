package md

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func render(t *testing.T, src string) string {
	t.Helper()
	out, err := HTML(src)
	require.NoError(t, err)
	return out
}

func TestHTMLSanitises(t *testing.T) {
	cases := map[string]struct {
		src  string
		gone []string
	}{
		"script tag":        {"<script>alert(1)</script>hello", []string{"<script", "alert(1)"}},
		"javascript url":    {"[x](javascript:alert(1))", []string{"javascript:"}},
		"javascript upper":  {"[x](JaVaScRiPt:alert(1))", []string{"avascript"}},
		"data url link":     {"[x](data:text/html;base64,PHNjcmlwdD4=)", []string{"data:"}},
		"on attribute":      {`<a href="/x" onclick="alert(1)">x</a>`, []string{"onclick"}},
		"img onerror":       {`<img src=x onerror=alert(1)>`, []string{"onerror"}},
		"raw html iframe":   {`<iframe src="https://evil.example"></iframe>`, []string{"<iframe"}},
		"style tag":         {`<style>body{display:none}</style>`, []string{"<style"}},
		"svg onload":        {`<svg onload=alert(1)></svg>`, []string{"onload", "<svg"}},
		"form":              {`<form action="/x"><input name=a></form>`, []string{"<form"}},
		"id attr on para":   {`<p id="x" onmouseover="a()">t</p>`, []string{"onmouseover"}},
		"autolink js":       {"<javascript:alert(1)>", []string{"href=\"javascript"}},
		"code class inject": {"<code class=\"x\" onmouseover=\"a()\">c</code>", []string{"onmouseover"}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			out := render(t, c.src)
			for _, g := range c.gone {
				assert.NotContains(t, strings.ToLower(out), strings.ToLower(g), out)
			}
		})
	}
}

func TestHTMLKeepsStructure(t *testing.T) {
	out := render(t, "# Title\n\n| a | b |\n|---|:-:|\n| 1 | 2 |\n\n```go\nfmt.Println(\"x\")\n```\n\n[ok](https://example.com) and `inline`\n\n- [x] done\n")
	assert.Contains(t, out, `<h1 id="title">Title</h1>`)
	assert.Contains(t, out, "<table>")
	assert.Contains(t, out, `<th style="text-align:center">b</th>`)
	assert.Contains(t, out, `<code class="language-go">`)
	assert.Contains(t, out, `href="https://example.com"`)
	assert.Contains(t, out, "<code>inline</code>")
	assert.Contains(t, out, `type="checkbox"`)
}

var headingLine = regexp.MustCompile(`(?m)^#{1,6} `)

// The docs pages' real inputs render without losing tables, code fences or
// headings with ids.
func TestPublishedDocs(t *testing.T) {
	files, err := filepath.Glob("../../../docs/published/*.md")
	require.NoError(t, err)
	require.NotEmpty(t, files)
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			raw, err := os.ReadFile(f)
			require.NoError(t, err)
			src := string(raw)
			out := render(t, src)

			assert.Equal(t, strings.Count(inFences(src, headingLine), "\n#"), strings.Count(out, "<h")-strings.Count(out, "<hr"), "heading count")
			assert.Equal(t, strings.Count(out, "<h"), strings.Count(out, ` id="`)-idsOutsideHeadings(out), "every heading has an id")
			assert.Equal(t, countTables(src), strings.Count(out, "<table>"), "tables")
			assert.Equal(t, countFences(src), strings.Count(out, "<pre>"), "code fences")
			assert.NotContains(t, out, "<script")
		})
	}
}

// inFences returns the lines of src outside fenced code that match re, each
// prefixed with "\n#" so the caller can count them.
func inFences(src string, re *regexp.Regexp) string {
	var b strings.Builder
	in := false
	for line := range strings.SplitSeq(src, "\n") {
		if strings.HasPrefix(line, "```") {
			in = !in
			continue
		}
		if !in && re.MatchString(line) {
			b.WriteString("\n#")
		}
	}
	return b.String()
}

var tableSep = regexp.MustCompile(`^\|?\s*:?-{3,}:?\s*(\|\s*:?-{3,}:?\s*)*\|?\s*$`)

func countTables(src string) int {
	n, in := 0, false
	for line := range strings.SplitSeq(src, "\n") {
		if strings.HasPrefix(line, "```") {
			in = !in
		}
		if !in && strings.Contains(line, "|") && tableSep.MatchString(strings.TrimSpace(line)) {
			n++
		}
	}
	return n
}

func idsOutsideHeadings(out string) int {
	return strings.Count(out, ` id="`) - len(regexp.MustCompile(`<h[1-6] id="`).FindAllString(out, -1))
}

// countFences counts opening fence lines (a fence may be indented).
func countFences(src string) int {
	n, in := 0, false
	for line := range strings.SplitSeq(src, "\n") {
		if strings.HasPrefix(strings.TrimLeft(line, " "), "```") {
			if !in {
				n++
			}
			in = !in
		}
	}
	return n
}
