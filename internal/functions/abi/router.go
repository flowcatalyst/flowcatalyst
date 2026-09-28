package abi

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
)

// Router matches a request to one of a describe document's endpoints with
// net/http's own ServeMux, so pattern syntax, precedence, conflicts and the
// 404/405 split are exactly Go's.
type Router struct {
	mux       *http.ServeMux
	endpoints []Endpoint
	params    [][]string // wildcard names per endpoint
}

// Match is a request's endpoint and the values of its path wildcards.
type Match struct {
	Index      int
	Endpoint   Endpoint
	PathParams map[string]string
}

var wildcard = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_]*)(?:\.\.\.)?\}`)

// NewRouter builds a router. A malformed or conflicting pattern is an error,
// not a panic.
func NewRouter(endpoints []Endpoint) (r *Router, err error) {
	r = &Router{mux: http.NewServeMux(), endpoints: endpoints, params: make([][]string, len(endpoints))}
	for i, e := range endpoints {
		for _, m := range wildcard.FindAllStringSubmatch(e.Path, -1) {
			r.params[i] = append(r.params[i], m[1])
		}
		if err := r.handle(i, e.Pattern()); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (r *Router) handle(i int, pattern string) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("pattern %q: %v", pattern, p)
		}
	}()
	r.mux.Handle(pattern, matchHandler(i))
	return nil
}

// matchHandler records which endpoint the mux chose, via the capture writer.
type matchHandler int

func (h matchHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	c := w.(*capture)
	c.index = int(h)
	c.req = req
}

// capture is the ResponseWriter a match runs against. The mux writes 404/405
// through it when no endpoint takes the request.
type capture struct {
	index  int
	req    *http.Request
	status int
	header http.Header
}

func (c *capture) Header() http.Header {
	if c.header == nil {
		c.header = http.Header{}
	}
	return c.header
}
func (c *capture) Write(b []byte) (int, error) { return len(b), nil }
func (c *capture) WriteHeader(s int)           { c.status = s }

// Match finds the endpoint for method and path. The path must be the
// request's path as received (already percent-decoded by the listener).
func (r *Router) Match(method, path string) (*Match, error) {
	req := &http.Request{Method: method, URL: &url.URL{Path: path}, Host: "fn", Header: http.Header{}}
	c := &capture{index: -1}
	r.mux.ServeHTTP(c, req)
	if c.index < 0 {
		if c.status == http.StatusMethodNotAllowed {
			return nil, ErrMethodNotAllowed
		}
		return nil, ErrNoRoute
	}
	m := &Match{Index: c.index, Endpoint: r.endpoints[c.index]}
	if names := r.params[c.index]; len(names) > 0 {
		m.PathParams = make(map[string]string, len(names))
		for _, n := range names {
			m.PathParams[n] = c.req.PathValue(n)
		}
	}
	return m, nil
}
