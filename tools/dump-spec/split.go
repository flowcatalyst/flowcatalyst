package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// splitSpec returns the spec restricted to the operations whose x-sdk is
// class ("app" or "admin"), with components.schemas pruned to the schemas
// those operations reference, directly or through other schemas. Everything
// else (info, servers, other component kinds) is kept as is.
func splitSpec(spec any, class string) ([]byte, error) {
	if class != classApp && class != classAdmin {
		return nil, fmt.Errorf("-split must be %q or %q, got %q", classApp, classAdmin, class)
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	doc, err = splitDocument(doc, class)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(doc, "", "  ")
}

var httpMethods = []string{"get", "put", "post", "delete", "options", "head", "patch", "trace"}

func splitDocument(doc map[string]any, class string) (map[string]any, error) {
	paths, _ := doc["paths"].(map[string]any)
	keptPaths := map[string]any{}
	usedTags := map[string]bool{}
	for p, v := range paths {
		item, _ := v.(map[string]any)
		kept := map[string]any{}
		nOps := 0
		for k, val := range item {
			isOp := false
			for _, m := range httpMethods {
				if k == m {
					isOp = true
				}
			}
			if !isOp {
				kept[k] = val // path-level parameters, summary, ...
				continue
			}
			op, _ := val.(map[string]any)
			if op[sdkExtension] != class {
				continue
			}
			kept[k] = val
			nOps++
			if tags, ok := op["tags"].([]any); ok {
				for _, t := range tags {
					if s, ok := t.(string); ok {
						usedTags[s] = true
					}
				}
			}
		}
		if nOps > 0 {
			keptPaths[p] = kept
		}
	}
	doc["paths"] = keptPaths

	// Prune tags to the ones still used.
	if tags, ok := doc["tags"].([]any); ok {
		var keep []any
		for _, t := range tags {
			if m, ok := t.(map[string]any); ok {
				if name, _ := m["name"].(string); usedTags[name] {
					keep = append(keep, t)
				}
			}
		}
		doc["tags"] = keep
	}

	// Reachable schemas: refs from the kept paths, then transitively.
	comps, _ := doc["components"].(map[string]any)
	schemas, _ := comps["schemas"].(map[string]any)
	reach := map[string]bool{}
	var queue []string
	collectRefs(keptPaths, func(name string) {
		if !reach[name] {
			reach[name] = true
			queue = append(queue, name)
		}
	})
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		s, ok := schemas[name]
		if !ok {
			return nil, fmt.Errorf("dangling $ref: #/components/schemas/%s is not defined", name)
		}
		collectRefs(s, func(n string) {
			if !reach[n] {
				reach[n] = true
				queue = append(queue, n)
			}
		})
	}
	pruned := map[string]any{}
	for name := range reach {
		pruned[name] = schemas[name]
	}
	if comps == nil {
		comps = map[string]any{}
	}
	comps["schemas"] = pruned
	doc["components"] = comps
	return doc, nil
}

const schemaRefPrefix = "#/components/schemas/"

// collectRefs calls f with the schema name of every "$ref" under v.
func collectRefs(v any, f func(name string)) {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if k == "$ref" {
				if ref, ok := t[k].(string); ok && strings.HasPrefix(ref, schemaRefPrefix) {
					f(strings.TrimPrefix(ref, schemaRefPrefix))
				}
				continue
			}
			collectRefs(t[k], f)
		}
	case []any:
		for _, e := range t {
			collectRefs(e, f)
		}
	}
}
