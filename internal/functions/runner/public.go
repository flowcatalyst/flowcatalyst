package runner

import (
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
)

// routeTable is the pool's public routes by hostname, longest prefix first.
type routeTable map[string][]control.Route

func buildRoutes(routes []control.Route) routeTable {
	t := routeTable{}
	for _, rt := range routes {
		h := strings.ToLower(strings.TrimSuffix(rt.Hostname, "."))
		rt.PathPrefix = "/" + strings.Trim(rt.PathPrefix, "/")
		t[h] = append(t[h], rt)
	}
	for _, rs := range t {
		slices.SortFunc(rs, func(a, b control.Route) int { return len(b.PathPrefix) - len(a.PathPrefix) })
	}
	return t
}

// match finds the route for host and path and returns the path relative to
// the route's prefix.
func (t routeTable) match(host, path string) (control.Route, string, bool) {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	for _, rt := range t[strings.ToLower(strings.TrimSuffix(host, "."))] {
		if rt.PathPrefix == "/" {
			return rt, path, true
		}
		if path == rt.PathPrefix {
			return rt, "/", true
		}
		if rest, ok := strings.CutPrefix(path, rt.PathPrefix+"/"); ok {
			return rt, "/" + rest, true
		}
	}
	return control.Route{}, "", false
}

// PublicHandler is the runner's public entry: requests are routed by Host and
// path prefix to a function's live version (or a route's alias). Explicit
// versions are not reachable here; each endpoint's own auth applies.
func (r *Runner) PublicHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.mu.RLock()
		rt, path, ok := r.routes.match(req.Host, req.URL.Path)
		r.mu.RUnlock()
		if !ok {
			writeError(w, http.StatusNotFound, "ROUTE_NOT_FOUND", "no function serves this host and path", 0)
			return
		}
		r.invoke(w, req, target{address: rt.Address, alias: rt.Alias}, path, true)
	})
}

// clientAddr is the caller's address. On the public entry, X-Forwarded-For is
// honoured only as far back as a chain of trusted proxies reaches: walking
// from the connection's peer, each trusted hop may name the one before it.
func (r *Runner) clientAddr(req *http.Request, public bool) string {
	peer := req.RemoteAddr
	if h, _, err := net.SplitHostPort(peer); err == nil {
		peer = h
	}
	if !public || len(r.cfg.TrustedProxies) == 0 {
		return peer
	}
	trusted := func(a string) bool {
		ip, err := netip.ParseAddr(strings.TrimSpace(a))
		if err != nil {
			return false
		}
		for _, p := range r.cfg.TrustedProxies {
			if p.Contains(ip.Unmap()) {
				return true
			}
		}
		return false
	}
	if !trusted(peer) {
		return peer
	}
	var hops []string
	for _, v := range req.Header.Values("X-Forwarded-For") {
		for h := range strings.SplitSeq(v, ",") {
			hops = append(hops, strings.TrimSpace(h))
		}
	}
	addr := peer
	for _, hop := range slices.Backward(hops) {
		addr = hop
		if !trusted(addr) {
			return addr
		}
	}
	return addr
}
