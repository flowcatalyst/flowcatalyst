package ui

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/consoleredirect"
)

// Every page the SPA redirect (internal/consoleredirect) can send a browser
// to must be a GET route of the /ui router, so renaming a /ui page breaks this
// build instead of a user's bookmark.
func TestConsoleRedirectTargetsExist(t *testing.T) {
	r := chi.NewRouter()
	if err := Mount(r, Config{
		Authenticate:   fakeAuth,
		DisplayName:    func(context.Context, string) string { return "x" },
		DispatchPools:  noPools{},
		SignInUpstream: http.NotFoundHandler(),
	}); err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	if err := chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if method == http.MethodGet {
			have[strings.ReplaceAll(strings.TrimSuffix(route, "/*"), "/*/", "/")] = true
			have[route] = true
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// {id} and {name} spell the same wildcard in different pages.
	canon := func(p string) string {
		segs := strings.Split(p, "/")
		for i, s := range segs {
			if len(s) > 2 && s[0] == '{' {
				segs[i] = "{}"
			}
		}
		return strings.Join(segs, "/")
	}
	canonHave := map[string]bool{}
	for k := range have {
		canonHave[canon(k)] = true
	}
	for _, target := range consoleredirect.UITargets() {
		if !canonHave[canon(target)] {
			t.Errorf("redirect target %s is not a GET route of /ui", target)
		}
	}
}
