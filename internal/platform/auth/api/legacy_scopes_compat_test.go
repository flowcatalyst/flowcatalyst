package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
)

type scopeProbeBody struct {
	DefaultScopes []string `json:"defaultScopes,omitempty"`
}

type scopeProbeInput struct {
	Body scopeProbeBody
}

type scopeProbeOutput struct {
	Body struct {
		Got []string `json:"got"`
	}
}

// newScopeProbe registers a minimal huma operation at the real create path whose
// body carries the array-typed defaultScopes, mirroring CreateOAuthClientRequest
// (including the RelaxRequestBodies pass the server applies).
func newScopeProbe(t *testing.T) (http.Handler, *[]string) {
	t.Helper()
	httpcompat.Init()

	mux, api := humatest.New(t)
	seen := new([]string)
	huma.Register(api, huma.Operation{
		OperationID: "scope-probe",
		Method:      http.MethodPost,
		Path:        createOAuthClientPath,
	}, func(_ context.Context, in *scopeProbeInput) (*scopeProbeOutput, error) {
		*seen = in.Body.DefaultScopes
		out := &scopeProbeOutput{}
		out.Body.Got = in.Body.DefaultScopes
		return out, nil
	})
	httpcompat.RelaxRequestBodies(api)
	return mux, seen
}

func postJSON(h http.Handler, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// TestLegacyDefaultScopesRejectedWithoutShim documents WHY this shim is a
// middleware rather than a lenient UnmarshalJSON on the field: huma validates
// the body against the registry schema — the same object the OpenAPI document is
// emitted from — before unmarshaling, so an array-typed field rejects the string
// outright and no custom decoder would ever run.
func TestLegacyDefaultScopesRejectedWithoutShim(t *testing.T) {
	mux, _ := newScopeProbe(t)

	rec := postJSON(mux, createOAuthClientPath, `{"defaultScopes":"openid profile"}`)

	if rec.Code == http.StatusOK {
		t.Fatalf("expected the bare pipeline to reject the string form, got 200")
	}
	if !strings.Contains(rec.Body.String(), "defaultScopes") {
		t.Errorf("expected a defaultScopes validation error, got: %s", rec.Body.String())
	}
}

// TestLegacyDefaultScopesCompat is the shim's contract: the retired string form
// is accepted and normalised, and the documented array form is untouched.
func TestLegacyDefaultScopesCompat(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want []string
	}{
		{"legacy space-delimited string", `{"defaultScopes":"openid profile email"}`, []string{"openid", "profile", "email"}},
		{"irregular whitespace", `{"defaultScopes":"  openid   profile "}`, []string{"openid", "profile"}},
		{"documented array form", `{"defaultScopes":["openid","profile"]}`, []string{"openid", "profile"}},
		{"empty string means no scopes", `{"defaultScopes":""}`, nil},
		{"field absent", `{}`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mux, seen := newScopeProbe(t)
			h := LegacyDefaultScopesCompat(mux)

			rec := postJSON(h, createOAuthClientPath, tc.body)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			if len(*seen) != len(tc.want) {
				t.Fatalf("got %v, want %v", *seen, tc.want)
			}
			for i := range tc.want {
				if (*seen)[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", *seen, tc.want)
				}
			}
		})
	}
}

// TestLegacyDefaultScopesCompatLeavesOtherRoutesAlone: the shim is guarded on
// method and path, so nothing else in the platform has its body buffered or
// rewritten.
func TestLegacyDefaultScopesCompatLeavesOtherRoutesAlone(t *testing.T) {
	var got string
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		b, _ := json.Marshal(json.RawMessage(readAll(t, r)))
		got = string(b)
	})
	h := LegacyDefaultScopesCompat(inner)

	const body = `{"defaultScopes":"openid profile"}`
	postJSON(h, "/api/principals", body)
	if got != body {
		t.Errorf("body on a different path was modified: %s", got)
	}

	// Same path, wrong method — also untouched.
	got = ""
	req := httptest.NewRequest(http.MethodPut, createOAuthClientPath, strings.NewReader(body))
	h.ServeHTTP(httptest.NewRecorder(), req)
	if got != body {
		t.Errorf("body on a non-POST was modified: %s", got)
	}
}

// TestRewriteLegacyDefaultScopesPassthrough: malformed or already-correct bodies
// report ok=false so the caller forwards the original bytes and huma reports the
// real error rather than the shim swallowing it.
func TestRewriteLegacyDefaultScopesPassthrough(t *testing.T) {
	for _, in := range []string{
		`not json`,
		`{"clientName":"x"}`,
		`{"defaultScopes":["openid"]}`,
		`{"defaultScopes":123}`,
	} {
		if _, ok := rewriteLegacyDefaultScopes([]byte(in)); ok {
			t.Errorf("%s: expected ok=false", in)
		}
	}
}

// TestLegacyDefaultScopesCompatPreservesSiblingFields: rewriting the one field
// must not drop or alter the rest of the create body.
func TestLegacyDefaultScopesCompatPreservesSiblingFields(t *testing.T) {
	out, ok := rewriteLegacyDefaultScopes(
		[]byte(`{"clientName":"acme","defaultScopes":"openid profile","pkceRequired":true}`))
	if !ok {
		t.Fatal("expected a rewrite")
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["clientName"] != "acme" || got["pkceRequired"] != true {
		t.Errorf("sibling fields altered: %v", got)
	}
	scopes, _ := got["defaultScopes"].([]any)
	if len(scopes) != 2 || scopes[0] != "openid" || scopes[1] != "profile" {
		t.Errorf("defaultScopes = %v", got["defaultScopes"])
	}
}

func readAll(t *testing.T, r *http.Request) []byte {
	t.Helper()
	b := make([]byte, r.ContentLength)
	n, _ := r.Body.Read(b)
	return b[:n]
}
