package openapispecs

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// httpVerbs is the set of OpenAPI path-item keys we treat as
// "operations" — anything else (parameters, summary, $ref) isn't a
// verb so it doesn't count toward removed-operation breakage.
var httpVerbs = []string{
	"get", "put", "post", "delete", "options", "head", "patch", "trace",
}

// SpecHash returns a stable canonical-JSON sha256 of the spec. Used by
// the no-op short-circuit so re-sending the same document doesn't
// create a new row.
func SpecHash(spec json.RawMessage) string {
	var parsed any
	if err := json.Unmarshal(spec, &parsed); err != nil {
		return ""
	}
	canonical, err := canonicalJSON(parsed)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

// canonicalJSON serialises v with object keys in sorted order so two
// semantically-equal documents hash identically.
func canonicalJSON(v any) ([]byte, error) {
	var b strings.Builder
	if err := writeCanonical(&b, v); err != nil {
		return nil, err
	}
	return []byte(b.String()), nil
}

func writeCanonical(b *strings.Builder, v any) error {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			kj, err := json.Marshal(k)
			if err != nil {
				return err
			}
			b.Write(kj)
			b.WriteByte(':')
			if err := writeCanonical(b, t[k]); err != nil {
				return err
			}
		}
		b.WriteByte('}')
	case []any:
		b.WriteByte('[')
		for i, item := range t {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := writeCanonical(b, item); err != nil {
				return err
			}
		}
		b.WriteByte(']')
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			return err
		}
		b.Write(raw)
	}
	return nil
}

// ComputeChangeNotes diffs two OpenAPI documents and returns the
// structured diff plus a human-readable summary suitable for listings.
// Mirrors the Rust shallow-diff: set-diff `paths` keys + `components.schemas`
// keys + verbs of surviving paths. Anything removed → has_breaking.
func ComputeChangeNotes(prior, current json.RawMessage) (ChangeNotes, string) {
	var priorDoc, currentDoc map[string]any
	_ = json.Unmarshal(prior, &priorDoc)
	_ = json.Unmarshal(current, &currentDoc)

	priorPaths := objectKeys(nestedObject(priorDoc, "paths"))
	currentPaths := objectKeys(nestedObject(currentDoc, "paths"))
	priorSchemas := objectKeys(nestedObject(priorDoc, "components", "schemas"))
	currentSchemas := objectKeys(nestedObject(currentDoc, "components", "schemas"))

	notes := ChangeNotes{
		AddedPaths:     setDifference(currentPaths, priorPaths),
		RemovedPaths:   setDifference(priorPaths, currentPaths),
		AddedSchemas:   setDifference(currentSchemas, priorSchemas),
		RemovedSchemas: setDifference(priorSchemas, currentSchemas),
	}

	// Verbs dropped from paths that survive.
	survivors := setIntersection(priorPaths, currentPaths)
	for _, path := range survivors {
		priorVerbs := verbsForPath(nestedObject(priorDoc, "paths", path))
		currentVerbs := verbsForPath(nestedObject(currentDoc, "paths", path))
		for _, v := range priorVerbs {
			if !contains(currentVerbs, v) {
				notes.RemovedOperations = append(notes.RemovedOperations,
					strings.ToUpper(v)+" "+path)
			}
		}
	}
	sort.Strings(notes.AddedPaths)
	sort.Strings(notes.RemovedPaths)
	sort.Strings(notes.AddedSchemas)
	sort.Strings(notes.RemovedSchemas)
	sort.Strings(notes.RemovedOperations)

	notes.HasBreaking = len(notes.RemovedPaths) > 0 ||
		len(notes.RemovedSchemas) > 0 ||
		len(notes.RemovedOperations) > 0

	return notes, renderSummary(notes)
}

// ── helpers ──────────────────────────────────────────────────────────────

func nestedObject(root map[string]any, path ...string) map[string]any {
	cur := root
	for _, p := range path {
		if cur == nil {
			return nil
		}
		next, ok := cur[p].(map[string]any)
		if !ok {
			return nil
		}
		cur = next
	}
	return cur
}

func objectKeys(o map[string]any) []string {
	if o == nil {
		return nil
	}
	out := make([]string, 0, len(o))
	for k := range o {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func verbsForPath(node map[string]any) []string {
	var out []string
	for _, v := range httpVerbs {
		if _, ok := node[v]; ok {
			out = append(out, v)
		}
	}
	return out
}

func setDifference(a, b []string) []string {
	bset := make(map[string]struct{}, len(b))
	for _, x := range b {
		bset[x] = struct{}{}
	}
	out := []string{}
	for _, x := range a {
		if _, in := bset[x]; !in {
			out = append(out, x)
		}
	}
	return out
}

func setIntersection(a, b []string) []string {
	bset := make(map[string]struct{}, len(b))
	for _, x := range b {
		bset[x] = struct{}{}
	}
	out := []string{}
	for _, x := range a {
		if _, in := bset[x]; in {
			out = append(out, x)
		}
	}
	return out
}

func contains(haystack []string, needle string) bool {
	for _, x := range haystack {
		if x == needle {
			return true
		}
	}
	return false
}

func renderSummary(n ChangeNotes) string {
	if n.IsEmpty() {
		return "No structural changes (descriptions or examples may differ)."
	}
	var parts []string
	if len(n.AddedPaths) > 0 {
		parts = append(parts, fmt.Sprintf("Added %d path(s)", len(n.AddedPaths)))
	}
	if len(n.RemovedPaths) > 0 {
		parts = append(parts, fmt.Sprintf("Removed %d path(s)", len(n.RemovedPaths)))
	}
	if len(n.RemovedOperations) > 0 {
		parts = append(parts, fmt.Sprintf("Removed %d operation(s)", len(n.RemovedOperations)))
	}
	if len(n.AddedSchemas) > 0 {
		parts = append(parts, fmt.Sprintf("Added %d schema(s)", len(n.AddedSchemas)))
	}
	if len(n.RemovedSchemas) > 0 {
		parts = append(parts, fmt.Sprintf("Removed %d schema(s)", len(n.RemovedSchemas)))
	}
	summary := strings.Join(parts, "; ")
	if n.HasBreaking {
		summary += ". Contains breaking changes (removals)."
	} else {
		summary += "."
	}
	var details []string
	if len(n.RemovedPaths) > 0 {
		details = append(details, "removed paths: "+sample(n.RemovedPaths, 5))
	}
	if len(n.RemovedOperations) > 0 {
		details = append(details, "removed ops: "+sample(n.RemovedOperations, 5))
	}
	if len(n.RemovedSchemas) > 0 {
		details = append(details, "removed schemas: "+sample(n.RemovedSchemas, 5))
	}
	if len(details) > 0 {
		summary += " — " + strings.Join(details, "; ")
	}
	return summary
}

func sample(items []string, n int) string {
	if len(items) <= n {
		return strings.Join(items, ", ")
	}
	return fmt.Sprintf("%s, … (+%d more)", strings.Join(items[:n], ", "), len(items)-n)
}
