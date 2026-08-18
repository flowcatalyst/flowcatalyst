package api

import (
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
)

func strptr(s string) *string { return &s }

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
			reqClient: strptr("clt_x"), wantScope: "CLIENT", wantClient: strptr("clt_x"),
		},
		{
			name: "no scope, anchor domain NOT promoted → client", isAnchorDom: true,
			reqClient: strptr("clt_x"), wantScope: "CLIENT", wantClient: strptr("clt_x"),
		},
		{
			name:      "no scope, ANCHOR mapping NOT promoted → client",
			mapping:   &emaildomainmapping.EmailDomainMapping{ScopeType: emaildomainmapping.ScopeAnchor},
			reqClient: strptr("clt_x"), wantScope: "CLIENT", wantClient: strptr("clt_x"),
		},
		{
			name: "no scope, PARTNER mapping NOT promoted → client",
			mapping: &emaildomainmapping.EmailDomainMapping{
				ScopeType: emaildomainmapping.ScopePartner, GrantedClientIDs: []string{"clt_x"},
			},
			reqClient: strptr("clt_x"), wantScope: "CLIENT", wantClient: strptr("clt_x"),
		},
		{
			name:      "no scope, CLIENT mapping falls back to primary when no clientId",
			mapping:   &emaildomainmapping.EmailDomainMapping{ScopeType: emaildomainmapping.ScopeClient, PrimaryClientID: strptr("clt_primary")},
			wantScope: "CLIENT", wantClient: strptr("clt_primary"),
		},
		{
			name:      "no scope, no clientId → client with nil client (op rejects downstream)",
			wantScope: "CLIENT", wantClient: nil,
		},

		// ── explicit CLIENT ────────────────────────────────────────────
		{
			name: "explicit CLIENT uses request clientId over mapping primary", reqScope: strptr("CLIENT"),
			mapping:   &emaildomainmapping.EmailDomainMapping{ScopeType: emaildomainmapping.ScopeClient, PrimaryClientID: strptr("clt_primary")},
			reqClient: strptr("clt_req"), wantScope: "CLIENT", wantClient: strptr("clt_req"),
		},
		{
			name: "explicit CLIENT on anchor domain is a downgrade, allowed", reqScope: strptr("CLIENT"),
			isAnchorDom: true, reqClient: strptr("clt_x"), wantScope: "CLIENT", wantClient: strptr("clt_x"),
		},

		// ── explicit ANCHOR: must be backed by the domain setup ────────
		{
			name: "ANCHOR on registered anchor domain, client ignored", reqScope: strptr("ANCHOR"),
			isAnchorDom: true, reqClient: strptr("clt_x"), wantScope: "ANCHOR", wantClient: nil,
		},
		{
			name: "ANCHOR via ANCHOR mapping", reqScope: strptr("ANCHOR"),
			mapping:   &emaildomainmapping.EmailDomainMapping{ScopeType: emaildomainmapping.ScopeAnchor},
			wantScope: "ANCHOR", wantClient: nil,
		},
		{
			name: "ANCHOR on unmapped domain rejected", reqScope: strptr("ANCHOR"),
			wantErr: true,
		},
		{
			name: "ANCHOR on CLIENT-mapped domain rejected", reqScope: strptr("ANCHOR"),
			mapping: &emaildomainmapping.EmailDomainMapping{ScopeType: emaildomainmapping.ScopeClient},
			wantErr: true,
		},

		// ── explicit PARTNER: must be backed by a PARTNER mapping ──────
		{
			name: "PARTNER without mapping rejected", reqScope: strptr("PARTNER"),
			reqClient: strptr("clt_x"), wantErr: true,
		},
		{
			name: "PARTNER requires clientId", reqScope: strptr("PARTNER"),
			mapping: &emaildomainmapping.EmailDomainMapping{ScopeType: emaildomainmapping.ScopePartner},
			wantErr: true,
		},
		{
			name: "PARTNER rejects clientId not in mapping", reqScope: strptr("PARTNER"),
			mapping: &emaildomainmapping.EmailDomainMapping{
				ScopeType: emaildomainmapping.ScopePartner, GrantedClientIDs: []string{"clt_a"},
			},
			reqClient: strptr("clt_b"), wantErr: true,
		},
		{
			name: "PARTNER accepts granted clientId", reqScope: strptr("PARTNER"),
			mapping: &emaildomainmapping.EmailDomainMapping{
				ScopeType: emaildomainmapping.ScopePartner, GrantedClientIDs: []string{"clt_a", "clt_b"},
			},
			reqClient: strptr("clt_b"), wantScope: "PARTNER", wantClient: strptr("clt_b"),
		},
		{
			name: "PARTNER accepts primary clientId", reqScope: strptr("PARTNER"),
			mapping: &emaildomainmapping.EmailDomainMapping{
				ScopeType: emaildomainmapping.ScopePartner, PrimaryClientID: strptr("clt_primary"),
			},
			reqClient: strptr("clt_primary"), wantScope: "PARTNER", wantClient: strptr("clt_primary"),
		},

		// ── malformed scope ────────────────────────────────────────────
		{
			name: "unknown scope rejected", reqScope: strptr("SUPERUSER"),
			wantErr: true,
		},
		{
			name: "scope is case-insensitive and trimmed", reqScope: strptr("  client "),
			reqClient: strptr("clt_x"), wantScope: "CLIENT", wantClient: strptr("clt_x"),
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
