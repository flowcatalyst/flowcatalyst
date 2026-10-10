package pages

import (
	"net/http"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
)

// A client-scoped user may choose one of its own active clients in a form
// (docs/platform-bug-fixes.md, dispatch pools/connections item): the client
// field needed CanReadClients (anchor-only), so such a user could create no
// pool, connection or subscription (a platform-wide one needs anchor scope).
func TestClientScopedUserPicksItsOwnClient(t *testing.T) {
	h := newHarness()
	h.clients = fakeClients{clients: []client.Client{
		{ID: "clt_acme", Name: "Acme Corp", Identifier: "acme", Status: client.StatusActive},
		{ID: "clt_globex", Name: "Globex <Inc>", Identifier: "globex", Status: client.StatusActive},
		{ID: "clt_initech", Name: "Initech", Identifier: "initech", Status: client.StatusSuspended},
	}}
	acmeWriter := &auth.AuthContext{
		PrincipalID: "prn_acme", Scope: auth.ScopeClient, Clients: []string{"clt_acme", "clt_initech"}, Roles: []string{"r"},
		Permissions:   []string{"platform:messaging:dispatch-pool:view", "platform:messaging:dispatch-pool:create"},
		PrincipalType: auth.PrincipalTypeUser,
	}

	dialog := h.do(t, acmeWriter, req{target: "/ui/dispatch-pools/new", datastar: true}).Body.String()
	mustContain(t, dialog, `<option value="acme">Acme Corp</option>`, "/ui/clients/options?list=pool-form-client-options")
	mustNotContain(t, dialog, "Globex", "Initech", "Choosing a client needs permission to view clients.")

	options := h.do(t, acmeWriter, req{target: "/ui/clients/options?list=pool-form-client-options&q=", datastar: true}).Body.String()
	mustContain(t, options, `<option value="acme">Acme Corp</option>`)
	mustNotContain(t, options, "globex", "initech")

	h.do(t, acmeWriter, req{method: http.MethodPost, target: "/ui/dispatch-pools/new", datastar: true, form: createForm("client", "acme")})
	if len(h.ops.created) != 1 || deref(h.ops.created[0].ClientID) != "clt_acme" {
		t.Fatalf("created %+v", h.ops.created)
	}
	refused := h.do(t, acmeWriter, req{method: http.MethodPost, target: "/ui/dispatch-pools/new", datastar: true, form: createForm("code", "other", "client", "globex")}).Body.String()
	mustContain(t, refused, noSuchClient)
	if len(h.ops.created) != 1 {
		t.Fatal("a pool was created for another tenant's client")
	}
}
