package pages

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
)

// A whole-set save must never be truncated: facetValues caps URL filters at
// maxFacetValues, formSet keeps every value.
func TestFormSetKeepsTheWholeSet(t *testing.T) {
	var raw []string
	for i := range maxFacetValues + 50 {
		raw = append(raw, fmt.Sprintf("app:role-%d", i))
	}
	raw = append(raw, " app:role-0 ", "", "  ", raw[7])
	got := formSet(raw)
	if len(got) != maxFacetValues+50 {
		t.Fatalf("formSet kept %d values, want %d", len(got), maxFacetValues+50)
	}
	if !slices.Equal(got, raw[:maxFacetValues+50]) {
		t.Errorf("formSet changed the order or the values")
	}
	if len(facetValues(raw)) != maxFacetValues {
		t.Errorf("facetValues is still the capped filter reader")
	}
	if formSet(nil) != nil || formSet([]string{" ", ""}) != nil {
		t.Errorf("an empty set reads as nil")
	}
}

// A form that cannot be read never reaches the handler: it would see an
// empty form and a set-replacing save would write an empty set.
func TestParseFormRefusesAnUnreadableForm(t *testing.T) {
	reached := false
	h := parseForm(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusNoContent)
	}))
	post := func(contentType, body string, datastar bool) *httptest.ResponseRecorder {
		reached = false
		r := httptest.NewRequest(http.MethodPost, "/ui/users/u1/roles", strings.NewReader(body))
		r.Header.Set("Content-Type", contentType)
		if datastar {
			r.Header.Set(ds.RequestHeader, "true")
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}

	cases := map[string]struct{ contentType, body string }{
		"multipart without a boundary": {"multipart/form-data", "roles=a"},
		"broken multipart":             {"multipart/form-data; boundary=x", "--x\r\nContent-Disposition: form-data; name=\"roles\"\r\n\r\na"},
		"bad urlencoding":              {"application/x-www-form-urlencoded", "roles=%zz"},
		"too large":                    {"application/x-www-form-urlencoded", "roles=" + strings.Repeat("a", maxFormBytes+1)},
	}
	for name, c := range cases {
		t.Run(name+" (native)", func(t *testing.T) {
			w := post(c.contentType, c.body, false)
			if reached || w.Code != http.StatusBadRequest {
				t.Fatalf("reached=%v status=%d, want refused with 400", reached, w.Code)
			}
		})
		t.Run(name+" (datastar)", func(t *testing.T) {
			w := post(c.contentType, c.body, true)
			if reached || w.Code != http.StatusOK {
				t.Fatalf("reached=%v status=%d, want refused on the 200 stream", reached, w.Code)
			}
			mustContain(t, w.Body.String(), "fc-flash-error", "Bad Request.", "The form could not be read")
		})
	}

	t.Run("a readable form reaches the handler", func(t *testing.T) {
		w := post("application/x-www-form-urlencoded", url.Values{"roles": {"a", "b"}}.Encode(), true)
		if !reached || w.Code != http.StatusNoContent {
			t.Fatalf("reached=%v status=%d", reached, w.Code)
		}
	})
}
