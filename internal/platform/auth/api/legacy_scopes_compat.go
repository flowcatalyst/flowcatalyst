package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// createOAuthClientPath is the only route carrying the legacy scope shape.
const createOAuthClientPath = "/api/oauth-clients"

// maxLegacyScopeBody caps how much of a create body this shim will buffer.
// Anything larger is passed through untouched — it is not a legitimate
// create-client payload, and huma will reject it on its own terms.
const maxLegacyScopeBody = 1 << 20 // 1 MiB

// LegacyDefaultScopesCompat rewrites a space-delimited `defaultScopes` string in
// the POST /api/oauth-clients body into the documented array form, before huma
// parses and validates the body.
//
// Background: `defaultScopes` used to be a space-delimited string on create
// while update and the response used an array, so a read-modify-write round trip
// silently corrupted scopes. Create is now an array like the other two. This
// shim keeps callers built against the old string shape — notably released
// Laravel/TypeScript SDKs, whose generated CreateOAuthClientRequest types it as
// a string — working without a coordinated deploy.
//
// Why a middleware rather than a lenient UnmarshalJSON on the field: huma
// validates the parsed body against the registry schema BEFORE unmarshaling into
// the Go struct, and that registry schema is the same object the OpenAPI
// document is emitted from. A schema documenting array<string> therefore rejects
// the string with `expected array` at body.defaultScopes and a custom decoder
// never runs. (Verified — see TestLegacyDefaultScopesRejectedWithoutShim.)
// Normalising the raw JSON ahead of validation is what keeps the document strict
// and the field genuinely array-typed while still accepting the legacy shape.
//
// The string form is deliberately absent from the OpenAPI document: it is a
// migration aid, not part of the contract. Retire this together with the legacy
// `scopes` alias on both create and update.
func LegacyDefaultScopesCompat(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != createOAuthClientPath || r.Body == nil {
			next.ServeHTTP(w, r)
			return
		}

		raw, err := io.ReadAll(io.LimitReader(r.Body, maxLegacyScopeBody+1))
		_ = r.Body.Close()
		if err != nil || len(raw) > maxLegacyScopeBody {
			// Hand the bytes back unmodified and let huma produce the error.
			r.Body = io.NopCloser(bytes.NewReader(raw))
			r.ContentLength = int64(len(raw))
			next.ServeHTTP(w, r)
			return
		}

		if rewritten, ok := rewriteLegacyDefaultScopes(raw); ok {
			raw = rewritten
		}
		r.Body = io.NopCloser(bytes.NewReader(raw))
		r.ContentLength = int64(len(raw))
		next.ServeHTTP(w, r)
	})
}

// rewriteLegacyDefaultScopes returns the body with a string-valued
// `defaultScopes` replaced by the equivalent array. ok is false when there is
// nothing to do — malformed JSON, no such field, or the field is already an
// array — in which case the caller must forward the original bytes so huma
// reports the real error.
func rewriteLegacyDefaultScopes(raw []byte) ([]byte, bool) {
	var body map[string]json.RawMessage
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, false
	}
	v, ok := body["defaultScopes"]
	if !ok {
		return nil, false
	}
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		// Already an array (or some other shape) — leave it for validation.
		return nil, false
	}
	arr, err := json.Marshal(strings.Fields(s))
	if err != nil {
		return nil, false
	}
	body["defaultScopes"] = arr
	out, err := json.Marshal(body)
	if err != nil {
		return nil, false
	}
	return out, true
}
