package api

import (
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
)

func strptr(s string) *string { return &s }

// TestDeriveUserScope covers the create-user scope-derivation ported from Rust
// create_user: anchor-domain, ANCHOR/PARTNER/CLIENT mappings, and unmapped.
func TestDeriveUserScope(t *testing.T) {
	cases := []struct {
		name        string
		isAnchorDom bool
		mapping     *emaildomainmapping.EmailDomainMapping
		reqClient   *string
		wantScope   string
		wantClient  *string
		wantErr     bool
	}{
		{
			name: "anchor domain ignores client", isAnchorDom: true,
			reqClient: strptr("clt_x"), wantScope: "ANCHOR", wantClient: nil,
		},
		{
			name:    "unmapped domain → client, verbatim clientId",
			mapping: nil, reqClient: strptr("clt_x"), wantScope: "CLIENT", wantClient: strptr("clt_x"),
		},
		{
			name:      "ANCHOR mapping → anchor, no client",
			mapping:   &emaildomainmapping.EmailDomainMapping{ScopeType: emaildomainmapping.ScopeAnchor},
			reqClient: strptr("clt_x"), wantScope: "ANCHOR", wantClient: nil,
		},
		{
			name:      "CLIENT mapping uses request clientId",
			mapping:   &emaildomainmapping.EmailDomainMapping{ScopeType: emaildomainmapping.ScopeClient, PrimaryClientID: strptr("clt_primary")},
			reqClient: strptr("clt_req"), wantScope: "CLIENT", wantClient: strptr("clt_req"),
		},
		{
			name:      "CLIENT mapping falls back to primary when no request clientId",
			mapping:   &emaildomainmapping.EmailDomainMapping{ScopeType: emaildomainmapping.ScopeClient, PrimaryClientID: strptr("clt_primary")},
			reqClient: nil, wantScope: "CLIENT", wantClient: strptr("clt_primary"),
		},
		{
			name:      "PARTNER requires clientId",
			mapping:   &emaildomainmapping.EmailDomainMapping{ScopeType: emaildomainmapping.ScopePartner},
			reqClient: nil, wantErr: true,
		},
		{
			name: "PARTNER rejects clientId not in mapping",
			mapping: &emaildomainmapping.EmailDomainMapping{
				ScopeType: emaildomainmapping.ScopePartner, GrantedClientIDs: []string{"clt_a"},
			},
			reqClient: strptr("clt_b"), wantErr: true,
		},
		{
			name: "PARTNER accepts granted clientId",
			mapping: &emaildomainmapping.EmailDomainMapping{
				ScopeType: emaildomainmapping.ScopePartner, GrantedClientIDs: []string{"clt_a", "clt_b"},
			},
			reqClient: strptr("clt_b"), wantScope: "PARTNER", wantClient: strptr("clt_b"),
		},
		{
			name: "PARTNER accepts primary clientId",
			mapping: &emaildomainmapping.EmailDomainMapping{
				ScopeType: emaildomainmapping.ScopePartner, PrimaryClientID: strptr("clt_primary"),
			},
			reqClient: strptr("clt_primary"), wantScope: "PARTNER", wantClient: strptr("clt_primary"),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			scope, client, err := deriveUserScope(c.isAnchorDom, c.mapping, c.reqClient)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got scope=%q client=%v", scope, client)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if scope != c.wantScope {
				t.Errorf("scope: got %q want %q", scope, c.wantScope)
			}
			switch {
			case c.wantClient == nil && client != nil:
				t.Errorf("client: got %q want nil", *client)
			case c.wantClient != nil && client == nil:
				t.Errorf("client: got nil want %q", *c.wantClient)
			case c.wantClient != nil && client != nil && *client != *c.wantClient:
				t.Errorf("client: got %q want %q", *client, *c.wantClient)
			}
		})
	}
}
