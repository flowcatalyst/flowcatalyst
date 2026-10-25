package pages

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
)

// A user whose scope was never set (NULL): the Type select offers "Not set
// (unchanged)", selected, and a name-only edit sends no association. Before,
// the select showed (and posted) its first option, so a rename made the user
// an ANCHOR.
func TestUserEditOfAnUnsetScopeChangesOnlyTheName(t *testing.T) {
	h := newUsrHarness()
	h.users.users[1].Scope = "" // prn_2, home client Acme
	body := usrBody(t, h.do(t, anchorAdmin(), req{target: "/ui/users/prn_2/edit", datastar: true}))
	mustContain(t, body, `name="scope"`, "Not set (unchanged)", `<option value="" selected>`, "usrScope: &#39;&#39;",
		`data-show="$usrScope === &#39;CLIENT&#39; || $usrScope === &#39;PARTNER&#39;"`)

	post := func(form url.Values) {
		h.users.calls = nil
		usrBody(t, h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/users/prn_2/edit", datastar: true, multipart: true, form: form}))
	}
	// What the dialog posts for a rename: scope "" (unchanged), the client
	// select's stored value.
	post(url.Values{"name": {"Robert"}, "scope": {""}, "clientId": {"clt_acme"}})
	if !slices.Equal(h.users.calls, []string{"update:prn_2"}) {
		t.Errorf("a rename of an unset-scope user wrote %v", h.users.calls)
	}
	// Choosing a type is an actual change and is sent.
	post(url.Values{"name": {"Bob <Builder>"}, "scope": {"CLIENT"}, "clientId": {"clt_acme"}})
	if !slices.Equal(h.users.calls, []string{"associate:prn_2"}) || h.users.assoc.ClientID != "clt_acme" {
		t.Errorf("choosing CLIENT: %v %+v", h.users.calls, h.users.assoc)
	}
}

// A stored scope is compared correctly: posting it back unchanged sends no
// association, and an ANCHOR user's hidden client select is not compared.
func TestUserEditSendsAnAssociationOnlyForAChange(t *testing.T) {
	h := newUsrHarness()
	post := func(id string, form url.Values) {
		h.users.calls = nil
		usrBody(t, h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/users/" + id + "/edit", datastar: true, multipart: true, form: form}))
	}
	post("prn_2", url.Values{"name": {"Robert"}, "scope": {"CLIENT"}, "clientId": {"clt_acme"}})
	if !slices.Equal(h.users.calls, []string{"update:prn_2"}) {
		t.Errorf("unchanged CLIENT: %v", h.users.calls)
	}
	post("prn_1", url.Values{"name": {"Ada Lovelace"}, "scope": {"ANCHOR"}, "clientId": {"clt_globex"}})
	if !slices.Equal(h.users.calls, []string{"update:prn_1"}) {
		t.Errorf("unchanged ANCHOR: %v", h.users.calls)
	}
	post("prn_2", url.Values{"name": {"Bob <Builder>"}, "scope": {"SUPERUSER"}, "clientId": {""}})
	if len(h.users.calls) != 0 {
		t.Errorf("an unknown type wrote %v", h.users.calls)
	}
}

// Owner ruling 2026-10-25: changing a user's type or client access needs
// user:manage-access as well as anchor tier. The page hides the controls
// without it, and a forged post is refused before anything is written.
func TestUserAccessChangesNeedManageAccess(t *testing.T) {
	without := usrAnchor(usrPermView, usrPermUpdate, usrPermDelete, usrPermAssign, usrPermRoles, usrPermClient)
	with := usrAnchor(usrPermView, usrPermUpdate, usrPermClient, usrPermManageAccess)

	t.Run("the controls are hidden without it", func(t *testing.T) {
		h := newUsrHarness()
		edit := usrBody(t, h.do(t, without, req{target: "/ui/users/prn_2/edit", datastar: true}))
		mustContain(t, edit, `name="name"`)
		mustNotContain(t, edit, `name="scope"`, `name="clientId"`)
		partner := usrBody(t, h.do(t, without, req{target: "/ui/users/prn_3", datastar: true}))
		mustContain(t, partner, "Granted Access", "Acme Corp")
		mustNotContain(t, partner, `/ui/users/prn_3/clients/new`, `/clients/clt_acme/revoke`)
		dialog := usrBody(t, h.do(t, without, req{target: "/ui/users/prn_3/clients/new", datastar: true}))
		mustContain(t, dialog, "permission required: platform:iam:user:manage-access")
		mustNotContain(t, dialog, "<dialog")
	})
	t.Run("and offered with it", func(t *testing.T) {
		h := newUsrHarness()
		edit := usrBody(t, h.do(t, with, req{target: "/ui/users/prn_2/edit", datastar: true}))
		mustContain(t, edit, `name="scope"`, `name="clientId"`)
		partner := usrBody(t, h.do(t, with, req{target: "/ui/users/prn_3", datastar: true}))
		mustContain(t, partner, `href="/ui/users/prn_3/clients/new"`, `data-action="/ui/users/prn_3/clients/clt_acme/revoke"`)
	})

	forged := []struct {
		name, target, call string
		form               url.Values
	}{
		{"promote to anchor", "/ui/users/prn_2/edit", "associate:prn_2", url.Values{"name": {"Bob <Builder>"}, "scope": {"ANCHOR"}}},
		{"to partner", "/ui/users/prn_2/edit", "associate:prn_2", url.Values{"name": {"Bob <Builder>"}, "scope": {"PARTNER"}, "clientId": {"clt_globex"}}},
		{"grant client access", "/ui/users/prn_3/clients", "grant:prn_3:clt_acme", url.Values{"clientId": {"clt_acme"}}},
		{"revoke client access", "/ui/users/prn_3/clients/clt_acme/revoke", "revoke:prn_3:clt_acme", url.Values{}},
	}
	for _, c := range forged {
		t.Run(c.name+" is refused without it", func(t *testing.T) {
			h := newUsrHarness()
			rec := h.do(t, without, req{method: http.MethodPost, target: c.target, form: c.form, datastar: true, multipart: true})
			if len(h.users.calls) != 0 {
				t.Fatalf("ran %v", h.users.calls)
			}
			if !strings.Contains(rec.Body.String(), "platform:iam:user:manage-access") {
				t.Errorf("no refusal in the answer: %s", rec.Body.String())
			}
		})
		t.Run(c.name+" goes through with it", func(t *testing.T) {
			h := newUsrHarness()
			h.do(t, with, req{method: http.MethodPost, target: c.target, form: c.form, datastar: true, multipart: true})
			if !slices.Contains(h.users.calls, c.call) {
				t.Errorf("calls = %v, want %s", h.users.calls, c.call)
			}
		})
		t.Run(c.name+" never passes for a client administrator", func(t *testing.T) {
			h := newUsrHarness()
			rec := h.do(t, usrClientAdmin(usrAllPerms...), req{method: http.MethodPost, target: c.target, form: c.form, datastar: true, multipart: true})
			if slices.Contains(h.users.calls, c.call) {
				t.Fatalf("a client administrator ran %v", h.users.calls)
			}
			if !strings.Contains(strings.ToLower(rec.Body.String()), "anchor scope required") {
				t.Errorf("no refusal in the answer: %s", rec.Body.String())
			}
		})
	}
}
