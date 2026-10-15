package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeFnPlatform is a minimal platform: token endpoint, function lookup,
// versions, aliases and DELETE. It records every request.
type fakeFnPlatform struct {
	mu       sync.Mutex
	requests []string
	exists   bool
	deleteSt int
	srv      *httptest.Server
}

func newFakeFnPlatform(t *testing.T, exists bool) *fakeFnPlatform {
	f := &fakeFnPlatform{exists: exists, deleteSt: http.StatusNoContent}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.requests = append(f.requests, r.Method+" "+r.URL.Path)
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/oauth/token":
			_, _ = w.Write([]byte(`{"access_token":"t","expires_in":300,"token_type":"Bearer"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/functions":
			if !f.exists {
				_, _ = w.Write([]byte(`{"data":[]}`))
				return
			}
			_, _ = w.Write([]byte(`{"data":[{"id":"fn_1","address":"demo.logger","applicationCode":"demo","clientId":"clt_9"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/functions/fn_1/versions":
			_, _ = w.Write([]byte(`[{"number":1,"digest":"aaaaaaaaaaaaaaaa"},{"number":2,"digest":"bbbbbbbbbbbbbbbb"}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/functions/fn_1/aliases":
			_, _ = w.Write([]byte(`[{"name":"live","version":2}]`))
		case r.Method == http.MethodDelete && r.URL.Path == "/api/functions/fn_1":
			if f.deleteSt != http.StatusNoContent {
				w.WriteHeader(f.deleteSt)
				_, _ = w.Write([]byte(`{"error":"BOOM","message":"nope"}`))
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeFnPlatform) deletes() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, r := range f.requests {
		if r == "DELETE /api/functions/fn_1" {
			n++
		}
	}
	return n
}

func runFnDeleteCmd(t *testing.T, f *fakeFnPlatform, stdin string, tty bool, args ...string) (string, error) {
	t.Helper()
	old := fnStdinIsTerminal
	fnStdinIsTerminal = func() bool { return tty }
	t.Cleanup(func() { fnStdinIsTerminal = old })
	cmd := newFnDeleteCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs(append(args, "--platform", f.srv.URL, "--client-id", "c", "--client-secret", "s", "--runner", f.srv.URL))
	err := cmd.Execute()
	return out.String(), err
}

func TestFnDeleteConfirmedByTypedAddress(t *testing.T) {
	f := newFakeFnPlatform(t, true)
	out, err := runFnDeleteCmd(t, f, "demo.logger\n", true, "demo.logger")
	if err != nil {
		t.Fatal(err, out)
	}
	if f.deletes() != 1 {
		t.Fatalf("want 1 DELETE, got %d", f.deletes())
	}
	for _, want := range []string{"demo.logger", "fn_1", "clt_9", "2 version", "live"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary lacks %q:\n%s", want, out)
		}
	}
}

func TestFnDeleteWrongConfirmationSendsNothing(t *testing.T) {
	for _, in := range []string{"\n", "demo.other\n", "y\n", "yes\n", "", "demo.logger-x\n"} {
		f := newFakeFnPlatform(t, true)
		_, err := runFnDeleteCmd(t, f, in, true, "demo.logger")
		if err == nil {
			t.Errorf("input %q: want an error", in)
		}
		if f.deletes() != 0 {
			t.Errorf("input %q: DELETE sent without confirmation", in)
		}
	}
}

func TestFnDeleteRefusesNonTerminalWithoutYes(t *testing.T) {
	f := newFakeFnPlatform(t, true)
	// Even a correct typed address on stdin must not count when not a terminal.
	_, err := runFnDeleteCmd(t, f, "demo.logger\n", false, "demo.logger")
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("want --yes hint, got %v", err)
	}
	if f.deletes() != 0 {
		t.Fatal("DELETE sent on a non-terminal without --yes")
	}
}

func TestFnDeleteYesSkipsPrompt(t *testing.T) {
	f := newFakeFnPlatform(t, true)
	if _, err := runFnDeleteCmd(t, f, "", false, "demo.logger", "--yes"); err != nil {
		t.Fatal(err)
	}
	if f.deletes() != 1 {
		t.Fatalf("want 1 DELETE, got %d", f.deletes())
	}
}

func TestFnDeleteMissingFunction(t *testing.T) {
	f := newFakeFnPlatform(t, false)
	_, err := runFnDeleteCmd(t, f, "", false, "demo.logger", "--yes")
	if err == nil || !strings.Contains(err.Error(), "no function demo.logger") {
		t.Fatalf("got %v", err)
	}
	if f.deletes() != 0 {
		t.Fatal("DELETE sent for a missing function")
	}
}

func TestFnDeleteSurfacesPlatformError(t *testing.T) {
	f := newFakeFnPlatform(t, true)
	f.deleteSt = http.StatusConflict
	_, err := runFnDeleteCmd(t, f, "", false, "demo.logger", "--yes")
	if err == nil || !strings.Contains(err.Error(), "409") {
		t.Fatalf("got %v", err)
	}
}
