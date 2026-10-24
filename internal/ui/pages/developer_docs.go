package pages

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/schemacodegen"
)

// The developer portal's API docs: an application's OpenAPI document laid
// out as operations grouped by tag, each with its parameters, request body
// and responses. The SPA mounts Swagger UI; this is the server-side reading
// of the same document (docs/ui/README.md, "Developer portal").

// oaMethods are the operation keys of a path item, in Swagger UI's order.
var oaMethods = []string{"get", "put", "post", "delete", "options", "head", "patch", "trace"}

const (
	// oaUntagged is the group of an operation with no tags (Swagger UI's).
	oaUntagged = "default"

	// A $ref chain is followed to this depth and for this many expansions
	// per schema shown; past either the $ref itself is shown. The limits
	// keep a recursive or very wide schema from producing a huge block.
	oaMaxDepth      = 16
	oaMaxExpansions = 2000
)

// oaDoc is a parsed OpenAPI document.
type oaDoc struct {
	root                        map[string]any
	Title, Version, Description string
	Groups                      []oaGroup
	Total                       int
}

type oaGroup struct {
	Name, Description string
	Ops               []oaOp
}

// oaOp is one row of the operation list.
type oaOp struct {
	// N numbers the row within the list: it is the id of the row's detail.
	N                     int
	Method, Path, Summary string
	Deprecated            bool
}

func parseOpenAPI(raw []byte) (*oaDoc, error) {
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("parse OpenAPI document: %w", err)
	}
	d := &oaDoc{root: root}
	info, _ := root["info"].(map[string]any)
	d.Title, d.Version, d.Description = str(info, "title"), str(info, "version"), str(info, "description")

	// Groups follow the document's tags list, then any other tag by name.
	byName := map[string]*oaGroup{}
	var order []string
	group := func(name string) *oaGroup {
		if g, ok := byName[name]; ok {
			return g
		}
		byName[name] = &oaGroup{Name: name}
		order = append(order, name)
		return byName[name]
	}
	declared, _ := root["tags"].([]any)
	for _, t := range declared {
		if tm, ok := t.(map[string]any); ok && str(tm, "name") != "" {
			group(str(tm, "name")).Description = str(tm, "description")
		}
	}
	extra := map[string]bool{}

	paths, _ := root["paths"].(map[string]any)
	for _, path := range sortedKeys(paths) {
		item, _ := paths[path].(map[string]any)
		for _, method := range oaMethods {
			op, ok := item[method].(map[string]any)
			if !ok {
				continue
			}
			row := oaOp{Method: method, Path: path, Summary: str(op, "summary"), Deprecated: op["deprecated"] == true}
			tags := stringList(op["tags"])
			if len(tags) == 0 {
				tags = []string{oaUntagged}
			}
			for _, tag := range tags {
				if _, known := byName[tag]; !known {
					extra[tag] = true
				}
				g := group(tag)
				g.Ops = append(g.Ops, row)
			}
			d.Total++
		}
	}
	// Tags that only operations mention sort after the declared ones.
	declaredCount := len(order) - len(extra)
	slices.Sort(order[declaredCount:])
	n := 0
	for _, name := range order {
		g := byName[name]
		if len(g.Ops) == 0 {
			continue
		}
		slices.SortStableFunc(g.Ops, func(a, b oaOp) int {
			if c := strings.Compare(a.Path, b.Path); c != 0 {
				return c
			}
			return slices.Index(oaMethods, a.Method) - slices.Index(oaMethods, b.Method)
		})
		for i := range g.Ops {
			n++
			g.Ops[i].N = n
		}
		d.Groups = append(d.Groups, *g)
	}
	return d, nil
}

func str(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

func stringList(v any) []string {
	var out []string
	list, _ := v.([]any)
	for _, e := range list {
		if s, ok := e.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// ── one operation ──────────────────────────────────────────────────────

type oaParam struct {
	Name, In, Type, Description string
	Required                    bool
}

// oaContent is one media type of a body: its schema as highlighted JSON
// (escaped text and hl-* spans only, from schemacodegen).
type oaContent struct {
	MediaType, SchemaHTML string
}

type oaBody struct {
	Description string
	Required    bool
	Content     []oaContent
}

type oaResponse struct {
	Status, Description string
	Content             []oaContent
}

// oaOpView is the detail of one operation.
type oaOpView struct {
	Method, Path, Summary, OperationID, Description string
	Deprecated                                      bool
	Params                                          []oaParam
	Body                                            *oaBody
	Responses                                       []oaResponse
}

// operation reads one operation: path-level parameters first, then the
// operation's own (which override by name and location), every $ref
// followed.
func (d *oaDoc) operation(method, path string) (oaOpView, bool) {
	paths, _ := d.root["paths"].(map[string]any)
	item, _ := paths[path].(map[string]any)
	op, ok := item[method].(map[string]any)
	if !slices.Contains(oaMethods, method) || !ok {
		return oaOpView{}, false
	}
	r := &oaResolver{root: d.root}
	v := oaOpView{
		Method: method, Path: path, Summary: str(op, "summary"), OperationID: str(op, "operationId"),
		Description: str(op, "description"), Deprecated: op["deprecated"] == true,
	}

	var params []map[string]any
	for _, list := range []any{item["parameters"], op["parameters"]} {
		entries, _ := list.([]any)
		for _, e := range entries {
			p, _ := r.shallow(e).(map[string]any)
			if p == nil {
				continue
			}
			i := slices.IndexFunc(params, func(q map[string]any) bool {
				return str(q, "name") == str(p, "name") && str(q, "in") == str(p, "in")
			})
			if i >= 0 {
				params[i] = p
			} else {
				params = append(params, p)
			}
		}
	}
	for _, p := range params {
		schema, _ := r.resolve(p["schema"]).(map[string]any)
		v.Params = append(v.Params, oaParam{
			Name: str(p, "name"), In: str(p, "in"), Required: p["required"] == true,
			Type: schemaLabel(schema), Description: str(p, "description"),
		})
	}

	if body, _ := r.shallow(op["requestBody"]).(map[string]any); body != nil {
		v.Body = &oaBody{Description: str(body, "description"), Required: body["required"] == true, Content: r.content(body["content"])}
	}

	responses, _ := op["responses"].(map[string]any)
	for _, status := range sortedKeys(responses) {
		resp, _ := r.shallow(responses[status]).(map[string]any)
		if resp == nil {
			continue
		}
		v.Responses = append(v.Responses, oaResponse{Status: status, Description: str(resp, "description"), Content: r.content(resp["content"])})
	}
	return v, true
}

// content resolves a media type map into highlighted schemas.
func (r *oaResolver) content(v any) []oaContent {
	media, _ := v.(map[string]any)
	var out []oaContent
	for _, mt := range sortedKeys(media) {
		m, _ := media[mt].(map[string]any)
		c := oaContent{MediaType: mt}
		if schema, ok := m["schema"]; ok {
			// A fresh budget per schema: one wide schema must not blank
			// the next.
			r.expansions = 0
			b, err := json.MarshalIndent(r.resolve(schema), "", "  ")
			if err == nil {
				c.SchemaHTML = schemacodegen.HighlightJSON(string(b))
			}
		}
		out = append(out, c)
	}
	return out
}

// schemaLabel is a short type for a parameter: "string", "string (uuid)",
// "array of integer".
func schemaLabel(s map[string]any) string {
	if s == nil {
		return ""
	}
	t := str(s, "type")
	if items, ok := s["items"].(map[string]any); ok && t == "array" {
		if inner := schemaLabel(items); inner != "" {
			return "array of " + inner
		}
	}
	if f := str(s, "format"); f != "" && t != "" {
		return t + " (" + f + ")"
	}
	if t == "" {
		if ref := str(s, "$ref"); ref != "" {
			return ref[strings.LastIndex(ref, "/")+1:]
		}
	}
	return t
}

// ── $ref resolution ────────────────────────────────────────────────────

// oaResolver inlines local $refs (#/components/...). A reference met again
// on the way down (a recursive schema) is left as {"$ref": ...}.
type oaResolver struct {
	root       map[string]any
	stack      []string
	expansions int
}

func (r *oaResolver) resolve(v any) any {
	switch t := v.(type) {
	case map[string]any:
		if ref, ok := t["$ref"].(string); ok {
			return r.follow(ref, t)
		}
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[k] = r.resolve(e)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = r.resolve(e)
		}
		return out
	}
	return v
}

// shallow follows a reference to the object it names and no further: the
// schemas inside are resolved when they are shown, each with its own
// budget and its own stack.
func (r *oaResolver) shallow(v any) any {
	for range oaMaxDepth {
		m, ok := v.(map[string]any)
		ref, isRef := m["$ref"].(string)
		if !ok || !isRef {
			return v
		}
		target, found := r.lookup(ref)
		if !found {
			return v
		}
		v = target
	}
	return v
}

// follow replaces a reference by its target. Keys next to the $ref
// (OpenAPI 3.1 allows them) override the target's.
func (r *oaResolver) follow(ref string, node map[string]any) any {
	target, ok := r.lookup(ref)
	if !ok || slices.Contains(r.stack, ref) || len(r.stack) >= oaMaxDepth || r.expansions >= oaMaxExpansions {
		return map[string]any{"$ref": ref}
	}
	r.expansions++
	r.stack = append(r.stack, ref)
	resolved := r.resolve(target)
	r.stack = r.stack[:len(r.stack)-1]
	if len(node) == 1 {
		return resolved
	}
	merged, isMap := resolved.(map[string]any)
	if !isMap {
		return resolved
	}
	out := make(map[string]any, len(merged)+len(node))
	maps.Copy(out, merged)
	for k, e := range node {
		if k != "$ref" {
			out[k] = r.resolve(e)
		}
	}
	return out
}

// lookup follows a local JSON pointer ("#/components/schemas/Name").
func (r *oaResolver) lookup(ref string) (any, bool) {
	rest, ok := strings.CutPrefix(ref, "#/")
	if !ok {
		return nil, false
	}
	var cur any = r.root
	for seg := range strings.SplitSeq(rest, "/") {
		if dec, err := url.PathUnescape(seg); err == nil {
			seg = dec
		}
		seg = strings.ReplaceAll(strings.ReplaceAll(seg, "~1", "/"), "~0", "~")
		switch c := cur.(type) {
		case map[string]any:
			cur, ok = c[seg]
		case []any:
			i, err := strconv.Atoi(seg)
			if err != nil || i < 0 || i >= len(c) {
				return nil, false
			}
			cur, ok = c[i], true
		default:
			return nil, false
		}
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

// downloadName is the SPA's file name for the spec: the info title as a
// slug (the application id when there is none), the version, then
// "-openapi.json".
func downloadName(title, fallback, version string) string {
	if title == "" {
		title = fallback
	}
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(title) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	name := strings.Trim(b.String(), "-")
	if name == "" {
		name = "openapi"
	}
	if version != "" {
		name += "-" + version
	}
	return name + "-openapi.json"
}
