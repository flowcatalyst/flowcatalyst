package api

import (
	"encoding/json"
	"testing"
)

// TestOAuthClientScopesWireNameIsUniform pins the single wire name for a client's
// scope list across create, update and the response. These three drifted once —
// create took a space-delimited string while the other two used arrays, so a
// read-modify-write round trip silently corrupted the scopes — and the retired
// `scopes` alias is gone from both request bodies.
func TestOAuthClientScopesWireNameIsUniform(t *testing.T) {
	for _, tc := range []struct {
		name string
		v    any
	}{
		{"create", CreateOAuthClientRequest{DefaultScopes: []string{"openid", "profile"}}},
		{"update", UpdateOAuthClientRequest{DefaultScopes: []string{"openid", "profile"}}},
		{"response", OAuthClientResponse{DefaultScopes: []string{"openid", "profile"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(tc.v)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var got map[string]json.RawMessage
			if err := json.Unmarshal(b, &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if _, ok := got["scopes"]; ok {
				t.Errorf("the legacy `scopes` alias is still on the wire: %s", b)
			}
			raw, ok := got["defaultScopes"]
			if !ok {
				t.Fatalf("defaultScopes missing: %s", b)
			}
			// Array-shaped everywhere — never the old space-delimited string.
			var arr []string
			if err := json.Unmarshal(raw, &arr); err != nil {
				t.Fatalf("defaultScopes is not an array (%s): %v", raw, err)
			}
			if len(arr) != 2 || arr[0] != "openid" || arr[1] != "profile" {
				t.Errorf("defaultScopes = %v", arr)
			}
		})
	}
}

// TestOAuthClientRequestsBindScopes: defaultScopes is the only source for the
// command's scope list now that the alias is gone.
func TestOAuthClientRequestsBindScopes(t *testing.T) {
	want := []string{"openid", "email"}

	create := CreateOAuthClientRequest{ClientName: "acme", DefaultScopes: want}.toCommand()
	if len(create.Scopes) != len(want) || create.Scopes[0] != want[0] || create.Scopes[1] != want[1] {
		t.Errorf("create scopes = %v, want %v", create.Scopes, want)
	}

	update := UpdateOAuthClientRequest{DefaultScopes: want}.toCommand("oac_1")
	if len(update.Scopes) != len(want) || update.Scopes[0] != want[0] || update.Scopes[1] != want[1] {
		t.Errorf("update scopes = %v, want %v", update.Scopes, want)
	}
}
