package client_test

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// lockPath is the platform's committed OpenAPI contract, relative to this
// package. It lives outside the fcsdk module, so the conformance tests skip
// (loudly) when the module is checked out on its own.
const lockPath = "../../../api/openapi.lock.json"

// lockSpec is api/openapi.lock.json, loaded once.
type lockSpec struct {
	doc   map[string]any
	paths map[string]map[string]any // path template -> lower-case method -> operation
}

var (
	lockOnce sync.Once
	lockVal  *lockSpec
	lockErr  error
)

func loadLock(t *testing.T) *lockSpec {
	t.Helper()
	lockOnce.Do(func() {
		abs, err := filepath.Abs(lockPath)
		if err != nil {
			lockErr = err
			return
		}
		raw, err := os.ReadFile(abs)
		if err != nil {
			lockErr = err
			return
		}
		doc := map[string]any{}
		if err := json.Unmarshal(raw, &doc); err != nil {
			lockErr = err
			return
		}
		s := &lockSpec{doc: doc, paths: map[string]map[string]any{}}
		for p, v := range asMap(doc["paths"]) {
			ops := map[string]any{}
			maps.Copy(ops, asMap(v))
			s.paths[p] = ops
		}
		lockVal = s
	})
	if lockErr != nil {
		if os.IsNotExist(lockErr) {
			t.Skipf("OpenAPI lock not found (%v): the fcsdk module is checked out without the platform repo", lockErr)
		}
		t.Fatalf("load OpenAPI lock: %v", lockErr)
	}
	return lockVal
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func asSlice(v any) []any {
	s, _ := v.([]any)
	return s
}

// match resolves a concrete request path to its templated lock path. A
// literal segment beats a {param}, so /api/clients/search is never read as
// /api/clients/{id}.
func (s *lockSpec) match(path string) (string, bool) {
	segs := strings.Split(strings.Trim(path, "/"), "/")
	best, bestScore := "", -1
	for tpl := range s.paths {
		ts := strings.Split(strings.Trim(tpl, "/"), "/")
		if len(ts) != len(segs) {
			continue
		}
		score, ok := 0, true
		for i := range ts {
			switch {
			case strings.HasPrefix(ts[i], "{"):
				if segs[i] == "" {
					ok = false
				}
			case ts[i] == segs[i]:
				score++
			default:
				ok = false
			}
		}
		if ok && score > bestScore {
			best, bestScore = tpl, score
		}
	}
	return best, bestScore >= 0
}

func (s *lockSpec) operation(method, path string) (op map[string]any, tpl string, err error) {
	tpl, ok := s.match(path)
	if !ok {
		return nil, "", fmt.Errorf("route %s %s is not in the OpenAPI lock", method, path)
	}
	o, ok := s.paths[tpl][strings.ToLower(method)]
	if !ok {
		have := make([]string, 0, 4)
		for m := range s.paths[tpl] {
			have = append(have, strings.ToUpper(m))
		}
		sort.Strings(have)
		return nil, tpl, fmt.Errorf("route %s %s: the lock has %s only for %s", method, tpl, strings.Join(have, ","), path)
	}
	return asMap(o), tpl, nil
}

// resolve follows a $ref into components.schemas.
func (s *lockSpec) resolve(schema map[string]any) map[string]any {
	for range 16 {
		ref, _ := schema["$ref"].(string)
		if ref == "" {
			return schema
		}
		name := ref[strings.LastIndex(ref, "/")+1:]
		next := asMap(asMap(asMap(s.doc["components"])["schemas"])[name])
		if next == nil {
			return schema
		}
		schema = next
	}
	return schema
}

// successStatus is the lock's declared 2xx for the operation.
func successStatus(op map[string]any) (int, map[string]any) {
	codes := []string{}
	for code := range asMap(op["responses"]) {
		if strings.HasPrefix(code, "2") {
			codes = append(codes, code)
		}
	}
	sort.Strings(codes)
	if len(codes) == 0 {
		return http.StatusOK, nil
	}
	n, _ := strconv.Atoi(codes[0])
	resp := asMap(asMap(op["responses"])[codes[0]])
	content := asMap(resp["content"])
	for _, c := range content {
		return n, asMap(asMap(c)["schema"])
	}
	return n, nil
}

// sample builds a canned value for a schema with every property populated:
// strings "x", integers 1, booleans true, one element per array. Free-form
// objects (no declared properties) come back empty.
func (s *lockSpec) sample(schema map[string]any, depth int) any {
	schema = s.resolve(schema)
	if depth > 6 {
		return nil
	}
	if props := asMap(schema["properties"]); props != nil {
		out := map[string]any{}
		for k, v := range props {
			if k == "$schema" {
				continue
			}
			out[k] = s.sample(asMap(v), depth+1)
		}
		return out
	}
	typ, _ := schema["type"].(string)
	if tl := asSlice(schema["type"]); len(tl) > 0 {
		typ, _ = tl[0].(string)
	}
	switch typ {
	case "string":
		return "x"
	case "integer":
		return 1
	case "number":
		return 1.5
	case "boolean":
		return true
	case "array":
		return []any{s.sample(asMap(schema["items"]), depth+1)}
	case "object":
		return map[string]any{}
	}
	return map[string]any{}
}

// checkBody validates a request body against a schema: every key must be a
// declared property, every required property must be present, and scalar
// types must agree. It recurses through nested objects and arrays.
func (s *lockSpec) checkBody(schema map[string]any, v any, at string) []string {
	schema = s.resolve(schema)
	var errs []string
	props := asMap(schema["properties"])
	switch x := v.(type) {
	case map[string]any:
		if props == nil {
			return nil // free-form object
		}
		for k, val := range x {
			ps, ok := props[k]
			if !ok {
				errs = append(errs, fmt.Sprintf("%s.%s: not a property of the request schema", at, k))
				continue
			}
			errs = append(errs, s.checkBody(asMap(ps), val, at+"."+k)...)
		}
		for _, r := range asSlice(schema["required"]) {
			name, _ := r.(string)
			if _, ok := x[name]; !ok {
				errs = append(errs, fmt.Sprintf("%s.%s: required by the request schema but missing", at, name))
			}
		}
	case []any:
		items := asMap(schema["items"])
		for i, el := range x {
			errs = append(errs, s.checkBody(items, el, fmt.Sprintf("%s[%d]", at, i))...)
		}
	case string:
		if t := scalarType(schema); t != "" && t != "string" {
			errs = append(errs, fmt.Sprintf("%s: sent a string, schema wants %s", at, t))
		}
	case float64:
		if t := scalarType(schema); t != "" && t != "integer" && t != "number" {
			errs = append(errs, fmt.Sprintf("%s: sent a number, schema wants %s", at, t))
		}
	case bool:
		if t := scalarType(schema); t != "" && t != "boolean" {
			errs = append(errs, fmt.Sprintf("%s: sent a boolean, schema wants %s", at, t))
		}
	}
	return errs
}

func scalarType(schema map[string]any) string {
	if t, ok := schema["type"].(string); ok {
		return t
	}
	return ""
}

// lockCall is one request the lock server saw.
type lockCall struct {
	Method, Path, Tpl string
	Status            int
	Query             map[string][]string
	Body              string
	Problems          []string
	Response          any // the canned body, nil for 204
	ResponseSchema    map[string]any
}

// lockServer answers every request from the lock: it finds the operation,
// validates query and body against it, and replies with the declared success
// status and a fully populated body of the declared schema. Violations are
// recorded on the call and answered with 418 so the client call fails too.
type lockServer struct {
	*httptest.Server
	mu    sync.Mutex
	calls []*lockCall
}

func newLockServer(t *testing.T) *lockServer {
	t.Helper()
	spec := loadLock(t)
	ls := &lockServer{}
	ls.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := &lockCall{Method: r.Method, Path: r.URL.Path, Query: r.URL.Query()}
		raw, _ := io.ReadAll(r.Body)
		call.Body = string(raw)
		defer func() {
			ls.mu.Lock()
			ls.calls = append(ls.calls, call)
			ls.mu.Unlock()
		}()

		op, tpl, err := spec.operation(r.Method, r.URL.Path)
		call.Tpl = tpl
		if err != nil {
			call.Problems = append(call.Problems, err.Error())
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		declared := map[string]bool{}
		for _, p := range asSlice(op["parameters"]) {
			pm := asMap(p)
			if pm["in"] != "query" {
				continue
			}
			name, _ := pm["name"].(string)
			declared[name] = true
			if req, _ := pm["required"].(bool); req && r.URL.Query().Get(name) == "" {
				call.Problems = append(call.Problems, "query "+name+" is required by the lock but not sent")
			}
		}
		for k := range r.URL.Query() {
			if !declared[k] {
				call.Problems = append(call.Problems, "query "+k+" is not declared by the lock for "+tpl)
			}
		}

		rb := asMap(op["requestBody"])
		switch {
		case rb != nil && call.Body != "":
			var v any
			if err := json.Unmarshal(raw, &v); err != nil {
				call.Problems = append(call.Problems, "body is not JSON: "+err.Error())
			} else {
				for _, c := range asMap(rb["content"]) {
					call.Problems = append(call.Problems, spec.checkBody(asMap(asMap(c)["schema"]), v, "body")...)
					break
				}
			}
		case rb != nil:
			if req, _ := rb["required"].(bool); req {
				call.Problems = append(call.Problems, "request body required by the lock but none sent")
			}
		case rb == nil && call.Body != "":
			call.Problems = append(call.Problems, "sent a body but the lock declares none for "+tpl)
		}

		status, schema := successStatus(op)
		call.Status = status
		call.ResponseSchema = schema
		if len(call.Problems) > 0 {
			http.Error(w, strings.Join(call.Problems, "; "), http.StatusTeapot)
			return
		}
		if schema == nil || status == http.StatusNoContent {
			w.WriteHeader(status)
			return
		}
		call.Response = spec.sample(schema, 0)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(call.Response)
	}))
	t.Cleanup(ls.Close)
	return ls
}

func (ls *lockServer) last() *lockCall {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	if len(ls.calls) == 0 {
		return nil
	}
	return ls.calls[len(ls.calls)-1]
}

// missingKeys lists the keys of want (a canned platform body) that are absent
// from got (the client's returned value, re-marshalled) — fields the client
// type drops. Free-form (empty) objects and nulls are skipped.
func missingKeys(want, got any, at string) []string {
	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			if len(w) == 0 {
				return nil
			}
			return []string{at + ": want an object"}
		}
		var out []string
		for k, wv := range w {
			gv, ok := g[k]
			if !ok {
				out = append(out, at+"."+k+" (dropped by the client type)")
				continue
			}
			out = append(out, missingKeys(wv, gv, at+"."+k)...)
		}
		return out
	case []any:
		g, ok := got.([]any)
		if !ok || len(g) < len(w) {
			return []string{at + ": array lost"}
		}
		var out []string
		for i := range w {
			out = append(out, missingKeys(w[i], g[i], fmt.Sprintf("%s[%d]", at, i))...)
		}
		return out
	}
	return nil
}

// newRecordingServer answers every request with {} and records the route.
func newRecordingServer(t *testing.T, method, path *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*method, *path = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}
