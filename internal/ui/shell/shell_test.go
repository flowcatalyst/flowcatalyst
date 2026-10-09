package shell

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

func admin() *auth.AuthContext {
	return &auth.AuthContext{
		PrincipalID: "prn_1", Email: "ada@example.com", Scope: auth.ScopeAnchor,
		Roles: []string{"platform:super-admin"}, Permissions: []string{"platform:*:*:*"}, PrincipalType: auth.PrincipalTypeUser,
	}
}

func request(datastar bool) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/ui/x", nil)
	r.Header.Set("Accept", "text/html")
	if datastar {
		r.Header.Set(ds.RequestHeader, "true")
		r.Header.Set("Accept", "text/event-stream, text/html, application/json")
	}
	return r.WithContext(auth.WithContext(r.Context(), admin()))
}

// The page has the flash stack: a polite live region with the fallback
// listener and its three templates, plus the 401 listener on <body>.
func TestPageHasTheFlashStack(t *testing.T) {
	rec := httptest.NewRecorder()
	(&Shell{}).Render(rec, request(false), Page{Title: "T", Path: "/ui/x"}, kit.EmptyState(kit.EmptyStateProps{Message: "x"}))
	body := rec.Body.String()
	for _, want := range []string{
		`<div id="flash-stack" class="fc-flash-stack" aria-live="polite" data-on:datastar-fetch="`,
		`<template data-flash-template="refused">`,
		`<template data-flash-template="failed">`,
		`<template data-flash-template="offline">`,
		`<body data-on:datastar-fetch="evt.detail.type === &#39;error&#39; &amp;&amp; evt.detail.argsRaw.status === &#39;401&#39;`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page lacks %s", want)
		}
	}
	if strings.Index(body, `id="flash-stack"`) > strings.Index(body, "fc-empty") {
		t.Error("the flash stack must come before the page content")
	}
}

func TestFlashAppendsToTheStack(t *testing.T) {
	rec := httptest.NewRecorder()
	sse := ds.NewSSE(rec, request(true))
	if err := Flash(sse, kit.FlashMessageProps{Kind: kit.FlashSuccess, Summary: "Pool <b>saved</b>."}); err != nil {
		t.Fatal(err)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"event: datastar-patch-elements\n", "data: selector #flash-stack\n", "data: mode append\n",
		`class="fc-flash fc-flash-success" data-init__delay.3s="el.remove()"`,
		`Pool &lt;b&gt;saved&lt;/b&gt;.`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("SSE lacks %q:\n%s", want, body)
		}
	}
}

// The Datastar error contract: a handled failure is 200 + an error flash
// (Datastar ignores non-200 bodies); a page request gets the error page.
func TestFail(t *testing.T) {
	sh := &Shell{}
	denied := usecase.Authorization("FORBIDDEN", "You cannot change this pool.")
	t.Run("datastar, use-case 4xx: 200 with the message", func(t *testing.T) {
		rec := httptest.NewRecorder()
		sh.Fail(rec, request(true), denied)
		body := rec.Body.String()
		if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "text/event-stream" {
			t.Fatalf("status %d, Content-Type %q", rec.Code, rec.Header().Get("Content-Type"))
		}
		for _, want := range []string{"data: selector #flash-stack\n", `fc-flash-error" role="alert"`, "Access denied.", "You cannot change this pool."} {
			if !strings.Contains(body, want) {
				t.Errorf("SSE lacks %q:\n%s", want, body)
			}
		}
	})
	t.Run("datastar, internal error: 200, generic, no details", func(t *testing.T) {
		rec := httptest.NewRecorder()
		sh.Fail(rec, request(true), errors.New("pg: password authentication failed"))
		body := rec.Body.String()
		if rec.Code != http.StatusOK || !strings.Contains(body, "Something went wrong.") || strings.Contains(body, "password") {
			t.Fatalf("status %d body %s", rec.Code, body)
		}
	})
	t.Run("on an open stream", func(t *testing.T) {
		rec := httptest.NewRecorder()
		r := request(true)
		sse := ds.NewSSE(rec, r)
		sh.FailSSE(sse, r, usecase.NotFound("NOT_FOUND", "The pool no longer exists."))
		if body := rec.Body.String(); !strings.Contains(body, "Not Found.") || !strings.Contains(body, "The pool no longer exists.") {
			t.Fatalf("body %s", body)
		}
	})
	t.Run("page request: the error page with the status", func(t *testing.T) {
		rec := httptest.NewRecorder()
		sh.Fail(rec, request(false), denied)
		if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "fc-sidebar") || !strings.Contains(rec.Body.String(), "You cannot change this pool.") {
			t.Fatalf("status %d", rec.Code)
		}
	})
}

// Guard refusals (CSRF, no session, role-less) stay non-200 for Datastar:
// the client's flash-stack fallback shows the generic message.
func TestDenyDatastarIsABareStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	(&Shell{}).Deny(rec, request(true), http.StatusForbidden, "blocked")
	if rec.Code != http.StatusForbidden || strings.Contains(rec.Body.String(), "<") {
		t.Fatalf("status %d body %q", rec.Code, rec.Body.String())
	}
}
