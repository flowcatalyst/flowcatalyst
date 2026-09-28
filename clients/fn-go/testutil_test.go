//go:build !wasip1

package fn

import "net/http"

// resetRegistry replaces the package-global registry with a fresh one, so
// each test that declares endpoints/subscriptions/etc. starts from a clean
// slate. reg is package state populated by init()-time declarations in real
// guest code; tests stand in for that by calling Webhook/Platform/... (or
// Subscribe/Schedule/Config/...) directly after resetRegistry.
func resetRegistry() {
	reg = newRegistry()
}

func noopHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
