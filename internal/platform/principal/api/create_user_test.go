package api

import (
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
)

// TestDeriveUserScope covers the create-user scope resolution: the requested
// scope wins (ANCHOR/PARTNER only when the domain setup backs them), and an
// absent scope defaults to CLIENT — the domain never upgrades it.
func TestDeriveUserScope(t *testing.T) {
	cases := []struct {
		name        string
		reqScope    *string
		isAnchorDom bool
		mapping     *emaildomainmapping.EmailDomainMapping
		reqClient   *string
		wantScope   string
		wantClient  *string
		wantErr     bool
	}{
		// ── no scope sent → CLIENT, never promoted ─────────────────────
		{
			name:      "no scope, unmapped domain → client, verbatim clientId",
			reqClient: new("clt_x"), wantScope: "CLIENT", wantClient: new("clt_x"),
		},
		{
			name: "no scope, anchor domain NOT promoted → client", isAnchorDom: true,
			reqClient: new("clt_x"), wantScope: "CLIENT", wantClient: new("clt_x"),
		},
		{
			name:      "no scope, ANCHOR mapping NOT promoted → client",
			mapping:   &emaildomainmapping.EmailDomainMapping{ScopeType: emaildomainmapping.ScopeAnchor},
			reqClient: new("clt_x"), wantScope: "CLIENT", wantClient: new("clt_x"),
		},
		{
			name: "no scope, PARTNER mapping NOT promoted → client",
			mapping: &emaildomainmapping.EmailDomainMapping{
				ScopeType: emaildomainmapping.ScopePartner, GrantedClientIDs: []string{"clt_x"},
			},
			reqClient: new("clt_x"), wantScope: "CLIENT", wantClient: new("clt_x"),
		},
		{
			name:      "no scope, CLIENT mapping falls back to primary when no clientId",
			mapping:   &emaildomainmapping.EmailDomainMapping{ScopeType: emaildomainmapping.ScopeClient, PrimaryClientID: new("clt_primary")},
			wantScope: "CLIENT", wantClient: new("clt_primary"),
		},
		{
			name:      "no scope, no clientId → client with nil client (op rejects downstream)",
			wantScope: "CLIENT", wantClient: nil,
		},

		// ── explicit CLIENT ────────────────────────────────────────────
		{
			name: "explicit CLIENT uses request clientId over mapping primary", reqScope: new("CLIENT"),
			mapping:   &emaildomainmapping.EmailDomainMapping{ScopeType: emaildomainmapping.ScopeClient, PrimaryClientID: new("clt_primary")},
			reqClient: new("clt_req"), wantScope: "CLIENT", wantClient: new("clt_req"),
		},
		{
			name: "explicit CLIENT on anchor domain is a downgrade, allowed", reqScope: new("CLIENT"),
			isAnchorDom: true, reqClient: new("clt_x"), wantScope: "CLIENT", wantClient: new("clt_x"),
		},

		// ── explicit ANCHOR: must be backed by the domain setup ────────
		{
			name: "ANCHOR on registered anchor domain, client ignored", reqScope: new("ANCHOR"),
			isAnchorDom: true, reqClient: new("clt_x"), wantScope: "ANCHOR", wantClient: nil,
		},
		{
			name: "ANCHOR via ANCHOR mapping", reqScope: new("ANCHOR"),
			mapping:   &emaildomainmapping.EmailDomainMapping{ScopeType: emaildomainmapping.ScopeAnchor},
			wantScope: "ANCHOR", wantClient: nil,
		},
		{
			name: "ANCHOR on unmapped domain rejected", reqScope: new("ANCHOR"),
			wantErr: true,
		},
		{
			name: "ANCHOR on CLIENT-mapped domain rejected", reqScope: new("ANCHOR"),
			mapping: &emaildomainmapping.EmailDomainMapping{ScopeType: emaildomainmapping.ScopeClient},
			wantErr: true,
		},

		// ── explicit PARTNER: must be backed by a PARTNER mapping ──────
		{
			name: "PARTNER without mapping rejected", reqScope: new("PARTNER"),
			reqClient: new("clt_x"), wantErr: true,
		},
		{
			name: "PARTNER requires clientId", reqScope: new("PARTNER"),
			mapping: &emaildomainmapping.EmailDomainMapping{ScopeType: emaildomainmapping.ScopePartner},
			wantErr: true,
		},
		{
			name: "PARTNER rejects clientId not in mapping", reqScope: new("PARTNER"),
			mapping: &emaildomainmapping.EmailDomainMapping{
				ScopeType: emaildomainmapping.ScopePartner, GrantedClientIDs: []string{"clt_a"},
			},
			reqClient: new("clt_b"), wantErr: true,
		},
		{
			name: "PARTNER accepts granted clientId", reqScope: new("PARTNER"),
			mapping: &emaildomainmapping.EmailDomainMapping{
				ScopeType: emaildomainmapping.ScopePartner, GrantedClientIDs: []string{"clt_a", "clt_b"},
			},
			reqClient: new("clt_b"), wantScope: "PARTNER", wantClient: new("clt_b"),
		},
		{
			name: "PARTNER accepts primary clientId", reqScope: new("PARTNER"),
			mapping: &emaildomainmapping.EmailDomainMapping{
				ScopeType: emaildomainmapping.ScopePartner, PrimaryClientID: new("clt_primary"),
			},
			reqClient: new("clt_primary"), wantScope: "PARTNER", wantClient: new("clt_primary"),
		},

		// ── malformed scope ────────────────────────────────────────────
		{
			name: "unknown scope rejected", reqScope: new("SUPERUSER"),
			wantErr: true,
		},
		{
			name: "scope is case-insensitive and trimmed", reqScope: new("  client "),
			reqClient: new("clt_x"), wantScope: "CLIENT", wantClient: new("clt_x"),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			scope, client, err := deriveUserScope(c.reqScope, c.isAnchorDom, c.mapping, c.reqClient)
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
