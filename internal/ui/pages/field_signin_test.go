package pages

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/a-h/templ"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// The sign-in options of docs/field-worker-sign-in-options-spec.md section 7:
// badges, the test and supervisor flags, "Sign in as…", the history's method
// and actor, and the policy's four fields.

// badgeFixture is the field fixture with a supervisor (Sue), a test identity
// (Tess) and an ACTIVE badge on Ann.
func badgeFixture() *fakeField {
	f := fieldFixture()
	sue := fieldWorker("fwk_sue", "clt_acme", "SUE", "Sue Supervisor", "app_wms")
	sue.CanImpersonate = true
	tess := fieldWorker("fwk_tess", "clt_acme", "TESS", "Tess Test", "app_wms", "app_tms")
	tess.IsTest = true
	f.workers = append(f.workers, sue, tess)
	used := fieldDay.Add(3 * time.Hour)
	f.badges = map[string]*fieldidentity.Badge{
		"fwk_ann": {ID: "fwb_ann", ClientID: "clt_acme", IdentityID: "fwk_ann", Status: fieldidentity.BadgeActive, CreatedAt: fieldDay, LastUsedAt: &used},
	}
	f.principals = map[string]string{"prn_admin": "Pat Admin <pat@acme.test>"}
	return f
}

// fieldServiceAccount is a client-scoped service account (every application).
func fieldServiceAccount(clients []string, perms ...string) *auth.AuthContext {
	ac := fieldCaller(clients, perms...)
	ac.PrincipalType = "SERVICE"
	return ac
}

func TestFieldWorkerListFlagsAndTestFilter(t *testing.T) {
	f := badgeFixture()
	ac := fieldCaller([]string{"clt_acme"}, permFieldWorkerView, permFieldWorkerManage)
	body := fieldBody(t, f, ac, req{target: "/ui/field-workers?client=clt_acme"})
	mustContain(t, body, ">TEST<", ">SUPERVISOR<", "Tess Test", "Sue Supervisor", `name="test"`, "Test identities", "Real workers")
	// SUE is the only supervisor and TESS the only test identity in her row.
	if n := strings.Count(body, ">TEST<"); n != 1 {
		t.Errorf("TEST tag shown %d times, want 1", n)
	}
	if n := strings.Count(body, ">SUPERVISOR<"); n != 1 {
		t.Errorf("SUPERVISOR tag shown %d times, want 1", n)
	}
	// The isTest filter: only test identities, only real workers, junk dropped.
	body = fieldBody(t, f, ac, req{target: "/ui/field-workers?client=clt_acme&test=true"})
	mustContain(t, body, "Tess Test")
	mustNotContain(t, body, "Sue Supervisor", "Bob")
	body = fieldBody(t, f, ac, req{target: "/ui/field-workers?client=clt_acme&test=false"})
	mustContain(t, body, "Sue Supervisor", "Bob")
	mustNotContain(t, body, "Tess Test")
	body = fieldBody(t, f, ac, req{target: "/ui/field-workers?client=clt_acme&test=maybe"})
	mustContain(t, body, "Tess Test", "Sue Supervisor")
	// The drawer carries the same tags.
	drawer := fieldBody(t, f, ac, req{target: "/ui/field-workers/fwk_tess", datastar: true})
	mustContain(t, drawer, ">TEST<")
	mustNotContain(t, drawer, ">SUPERVISOR<")
}

func TestFieldWorkerBadgeSection(t *testing.T) {
	f := badgeFixture()
	manager := fieldCaller([]string{"clt_acme"}, permFieldWorkerView, permFieldWorkerManage)
	// Ann has a badge: its state, no token, and Reissue / Revoke.
	body := fieldBody(t, f, manager, req{target: "/ui/field-workers/fwk_ann", datastar: true})
	mustContain(t, body, "Badge", "Active", "Issued", "Last used", "Reissue badge", "Revoke badge",
		`data-action="/ui/field-workers/fwk_ann/badge"`, `data-action="/ui/field-workers/fwk_ann/badge/revoke"`)
	mustNotContain(t, body, "FWB1", "Issue badge<")
	// Bob has none: Issue.
	body = fieldBody(t, f, manager, req{target: "/ui/field-workers/fwk_bob", datastar: true})
	mustContain(t, body, "No active badge", "Issue badge")
	mustNotContain(t, body, "Reissue badge", "Revoke badge")
	// The confirmations: reissue says it cancels the old badge.
	page := fieldBody(t, f, manager, req{target: "/ui/field-workers?client=clt_acme"})
	mustContain(t, page, `<dialog id="fw-badge-issue"`, `<dialog id="fw-badge-reissue"`, `<dialog id="fw-badge-revoke"`,
		"Their current badge is cancelled at once and stops signing in")
	// A viewer sees the state but cannot act.
	viewer := fieldBody(t, f, fieldCaller([]string{"clt_acme"}, permFieldWorkerView), req{target: "/ui/field-workers/fwk_ann", datastar: true})
	mustContain(t, viewer, "Badge", "Active", "Last used")
	mustNotContain(t, viewer, "Reissue badge", "Revoke badge", "Issue badge")
}

// A badge is shown once: in the answer to the issue, never again.
func TestFieldWorkerBadgeIsShownOnce(t *testing.T) {
	f := badgeFixture()
	ac := fieldCaller([]string{"clt_acme"}, permFieldWorkerView, permFieldWorkerManage)
	rec := serveField(t, f, ac, postForm("/ui/field-workers/fwk_ann/badge", "drawer", "fwk_ann"))
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !slices.Equal(f.calls, []string{"issue-badge fwk_ann"}) || len(f.lastTokens) != 1 {
		t.Fatalf("issue: status %d, calls %v", rec.Code, f.calls)
	}
	token := f.lastTokens[0]
	if !fieldidentity.WellFormedToken(fieldidentity.BadgeTokenPrefix, token) {
		t.Fatalf("token %q is not a badge token", token)
	}
	// The card: name (escaped), worker code, client, the token as small print and
	// as the QR of the bare token.
	mustContain(t, body, `id="fw-badge-reveal"`, `id="fw-print-sheet"`, `class="fw-card"`, "Ann &lt;Picker&gt;", "<code>ANN</code>", "Acme Corp",
		"These badges are shown once", "Their previous badge has been cancelled", ">Print<", "window.print()", "d.remove()", "@page", "A4")
	if n := strings.Count(body, token); n != 1 {
		t.Errorf("the token appears %d times in the answer, want once (as text)", n)
	}
	qr, err := qrSVGLabeled(token, "Badge QR code for ANN")
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, body, renderToString(t, qr))
	mustContain(t, body, `aria-label="Badge QR code for ANN"`)
	// Uncached, and in no header or cookie.
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control %q, want no-store", rec.Header().Get("Cache-Control"))
	}
	for k, vs := range rec.Header() {
		for _, v := range vs {
			if strings.Contains(v, token) {
				t.Errorf("header %s carries the token", k)
			}
		}
	}
	// The flash names the worker; nothing fetched afterwards has the token.
	mustContain(t, body, "ANN has a new badge")
	for _, q := range []req{
		{target: "/ui/field-workers?client=clt_acme"},
		{target: "/ui/field-workers?client=clt_acme", datastar: true},
		{target: "/ui/field-workers/fwk_ann"},
		{target: "/ui/field-workers/fwk_ann", datastar: true},
		{target: "/ui/field-workers/fwk_ann/sign-ins", datastar: true},
	} {
		got := serveField(t, f, ac, q)
		if strings.Contains(got.Body.String(), token) {
			t.Errorf("GET %s re-serves the badge token", q.target)
		}
		for _, c := range got.Result().Cookies() {
			if strings.Contains(c.Value, token) {
				t.Errorf("GET %s sets a cookie with the token", q.target)
			}
		}
	}
	// A native post gets the page with the dialog: no redirect that could carry it.
	f.calls = nil
	native := serveField(t, f, ac, req{method: http.MethodPost, target: "/ui/field-workers/fwk_bob/badge", form: map[string][]string{}})
	if native.Code != http.StatusOK || native.Header().Get("Location") != "" || native.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("native issue: status %d Location %q Cache-Control %q", native.Code, native.Header().Get("Location"), native.Header().Get("Cache-Control"))
	}
	mustContain(t, native.Body.String(), `id="fw-badge-reveal"`, `<h1 class="fc-page-title">Field Workers</h1>`, f.lastTokens[0])
	// Bob had no badge: nothing was cancelled.
	mustNotContain(t, native.Body.String(), "previous badge has been cancelled")
	// A refused issue shows no token.
	f.failWith = usecase.Internal("BOOM", "boom", nil)
	failed := serveField(t, f, ac, postForm("/ui/field-workers/fwk_ann/badge"))
	mustNotContain(t, failed.Body.String(), f.lastTokens[0], "fw-badge-reveal")
}

func TestFieldWorkerBadgeRevoke(t *testing.T) {
	f := badgeFixture()
	ac := fieldCaller([]string{"clt_acme"}, permFieldWorkerManage)
	rec := serveField(t, f, ac, postForm("/ui/field-workers/fwk_ann/badge/revoke", "drawer", "fwk_ann"))
	if rec.Code != http.StatusOK || !slices.Equal(f.calls, []string{"revoke-badge fwk_ann fwb_ann"}) {
		t.Errorf("revoke: status %d, calls %v", rec.Code, f.calls)
	}
	mustContain(t, rec.Body.String(), "ANN&#39;s badge no longer signs in")
	// No active badge: nothing to revoke.
	f.calls = nil
	if rec := serveField(t, f, ac, nativePost("/ui/field-workers/fwk_bob/badge/revoke")); rec.Code != http.StatusNotFound || len(f.calls) != 0 {
		t.Errorf("revoke without a badge: status %d, calls %v", rec.Code, f.calls)
	}
	// Actions are POST only.
	for _, target := range []string{"/ui/field-workers/fwk_ann/badge", "/ui/field-workers/fwk_ann/badge/revoke", "/ui/field-workers/badges/print"} {
		if rec := serveField(t, f, ac, req{target: target}); rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("GET %s: status %d, want 405", target, rec.Code)
		}
	}
}

func TestFieldBadgesAreRefusedWithoutManage(t *testing.T) {
	f := badgeFixture()
	viewer := fieldCaller([]string{"clt_acme"}, permFieldWorkerView)
	for _, q := range []req{
		nativePost("/ui/field-workers/fwk_ann/badge"), nativePost("/ui/field-workers/fwk_ann/badge/revoke"),
		nativePost("/ui/field-workers/badges/print", "ids", "fwk_ann"),
	} {
		rec := serveField(t, f, viewer, q)
		if rec.Code != http.StatusForbidden {
			t.Errorf("viewer POST %s: status %d, want 403", q.target, rec.Code)
		}
		mustNotContain(t, rec.Body.String(), "FWB1")
	}
	// Another client's worker is not found, and a mixed bulk issues nothing.
	acme := fieldCaller([]string{"clt_acme"}, permFieldWorkerView, permFieldWorkerManage)
	for _, q := range []req{
		nativePost("/ui/field-workers/fwk_gus/badge"), nativePost("/ui/field-workers/fwk_gus/badge/revoke"),
		nativePost("/ui/field-workers/badges/print", "ids", "fwk_ann", "ids", "fwk_gus"),
	} {
		if rec := serveField(t, f, acme, q); rec.Code != http.StatusNotFound {
			t.Errorf("POST %s on another client's worker: status %d, want 404", q.target, rec.Code)
		}
	}
	if len(f.calls) != 0 {
		t.Errorf("a refused post ran %v", f.calls)
	}
}

func TestFieldBadgePrintSheet(t *testing.T) {
	f := badgeFixture()
	ac := fieldCaller([]string{"clt_acme"}, permFieldWorkerView, permFieldWorkerManage)
	// The list: a box per row, select-all, and a confirmation that says the
	// existing badges are replaced.
	page := fieldBody(t, f, ac, req{target: "/ui/field-workers?client=clt_acme"})
	mustContain(t, page, `name="ids" value="fwk_ann" form="fw-print-badges-form"`, `name="ids" value="fwk_bob" form="fw-print-badges-form"`,
		`id="fw-select-all"`, "Print badges", `<dialog id="fw-print-badges"`, `id="fw-print-badges-form"`,
		"A worker who already has a badge loses it", "cancelled at once", "Printing issues new badges and cancels the workers' existing ones.",
		`data-action="/ui/field-workers/badges/print"`)
	// A viewer gets no boxes.
	viewer := fieldBody(t, f, fieldCaller([]string{"clt_acme"}, permFieldWorkerView), req{target: "/ui/field-workers?client=clt_acme"})
	mustNotContain(t, viewer, `name="ids"`, "fw-select-all", "Print badges")

	// Print two: one operation, one card each, every token once.
	rec := serveField(t, f, ac, postForm("/ui/field-workers/badges/print", "ids", "fwk_ann", "ids", "fwk_sue"))
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !slices.Equal(f.calls, []string{"issue-badges fwk_ann,fwk_sue"}) || len(f.lastTokens) != 2 {
		t.Fatalf("print: status %d, calls %v", rec.Code, f.calls)
	}
	mustContain(t, body, `id="fw-badge-reveal"`, "Badges to print", "2 new badges", "Print the sheet", "cancelled", `id="fw-print-sheet"`,
		"<code>ANN</code>", "<code>SUE</code>", "Acme Corp", "Sue Supervisor", "These badges are shown once")
	if n := strings.Count(body, `class="fw-card"`); n != 2 {
		t.Errorf("%d cards, want 2", n)
	}
	for _, tok := range f.lastTokens {
		if n := strings.Count(body, tok); n != 1 {
			t.Errorf("token %q appears %d times, want once", tok, n)
		}
	}
	// Print rules: only the sheet prints, on A4, in a grid.
	mustContain(t, body, "@media print", "@page { size: A4", "grid-template-columns: repeat(2, 1fr)", "break-inside: avoid", "fw-printing")
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control %q", rec.Header().Get("Cache-Control"))
	}
	// Nothing chosen, or too many, is refused and issues nothing.
	f.calls = nil
	if rec := serveField(t, f, ac, nativePost("/ui/field-workers/badges/print")); rec.Code != http.StatusBadRequest {
		t.Errorf("no workers ticked: status %d, want 400", rec.Code)
	}
	var many []string
	for n := range 201 {
		many = append(many, "ids", "fwk_"+strings.Repeat("a", 1+n%7)+string(rune('a'+n%26))+string(rune('a'+(n/26)%26)))
	}
	if rec := serveField(t, f, ac, nativePost("/ui/field-workers/badges/print", many...)); rec.Code == http.StatusOK {
		t.Errorf("201 workers: status %d, want a refusal", rec.Code)
	}
	if len(f.calls) != 0 {
		t.Errorf("a refused print ran %v", f.calls)
	}
}

func TestFieldWorkerCreateFlagsAndPermission(t *testing.T) {
	f := badgeFixture()
	manager := fieldCaller([]string{"clt_acme"}, permFieldWorkerView, permFieldWorkerManage)
	withImp := fieldCaller([]string{"clt_acme"}, permFieldWorkerView, permFieldWorkerManage, permFieldWorkerImpers)
	// The switches: test for every manager, supervisor only with the permission.
	form := fieldBody(t, f, manager, req{target: "/ui/field-workers/new?client=clt_acme", datastar: true})
	mustContain(t, form, `name="isTest"`, "Test identity")
	mustNotContain(t, form, `name="canImpersonate"`, "May act as other workers")
	form = fieldBody(t, f, withImp, req{target: "/ui/field-workers/new?client=clt_acme", datastar: true})
	mustContain(t, form, `name="isTest"`, `name="canImpersonate"`, "May act as other workers")

	// Test identity: manage is enough.
	serveField(t, f, manager, postForm("/ui/field-workers/new", "client", "clt_acme", "workerCode", "qa1", "name", "QA", "isTest", "true"))
	if !f.lastEnsure.IsTest || f.lastEnsure.CanImpersonate || !slices.Equal(f.calls, []string{"ensure clt_acme qa1 "}) {
		t.Errorf("create test identity: %+v, calls %v", f.lastEnsure, f.calls)
	}
	// The supervisor flag without the impersonate permission is refused (403),
	// whether the form offered it or not, and nothing runs.
	f.calls = nil
	for _, q := range []req{
		nativePost("/ui/field-workers/new", "client", "clt_acme", "workerCode", "sup", "name", "Sup", "canImpersonate", "true"),
		postForm("/ui/field-workers/new", "client", "clt_acme", "workerCode", "sup", "name", "Sup", "canImpersonate", "true"),
	} {
		rec := serveField(t, f, manager, q)
		if !q.datastar && rec.Code != http.StatusForbidden {
			t.Errorf("supervisor without permission: status %d, want 403", rec.Code)
		}
		if q.datastar {
			mustContain(t, rec.Body.String(), "platform:iam:field-worker:impersonate")
		}
	}
	if len(f.calls) != 0 {
		t.Errorf("a refused create ran %v", f.calls)
	}
	// With the permission it is created as a supervisor.
	serveField(t, f, withImp, postForm("/ui/field-workers/new", "client", "clt_acme", "workerCode", "sup", "name", "Sup", "canImpersonate", "true"))
	if !f.lastEnsure.CanImpersonate || f.lastEnsure.IsTest {
		t.Errorf("create supervisor: %+v", f.lastEnsure)
	}
}

func TestFieldWorkerEdit(t *testing.T) {
	f := badgeFixture()
	manager := fieldCaller([]string{"clt_acme"}, permFieldWorkerView, permFieldWorkerManage)
	withImp := fieldCaller([]string{"clt_acme"}, permFieldWorkerView, permFieldWorkerManage, permFieldWorkerImpers)
	// The form carries the current values; the supervisor switch only with the
	// impersonate permission (and its marker with it).
	form := fieldBody(t, f, manager, req{target: "/ui/field-workers/fwk_sue/edit", datastar: true})
	mustContain(t, form, `id="fw-edit-form"`, `value="Sue Supervisor"`, `name="isTest"`, `name="flags"`)
	mustNotContain(t, form, `name="canImpersonate"`, `name="impersonateField"`)
	form = fieldBody(t, f, withImp, req{target: "/ui/field-workers/fwk_sue/edit", datastar: true})
	mustContain(t, form, `name="canImpersonate"`, `name="impersonateField"`, `checked`)

	// A manager renames and marks a test identity; the supervisor flag is left.
	rec := serveField(t, f, manager, postForm("/ui/field-workers/fwk_sue/edit", "name", "Sue S", "email", "sue@acme.test", "flags", "1", "isTest", "true", "drawer", "fwk_sue"))
	if rec.Code != http.StatusOK || !slices.Equal(f.calls, []string{"update fwk_sue"}) {
		t.Fatalf("edit: status %d, calls %v", rec.Code, f.calls)
	}
	u := f.lastUpdate
	if u.Name != "Sue S" || u.Email == nil || *u.Email != "sue@acme.test" || u.IsTest == nil || !*u.IsTest || u.CanImpersonate != nil {
		t.Errorf("update command %+v", u)
	}
	mustContain(t, rec.Body.String(), "SUE updated")
	// Unticking the test switch (nothing posted, marker present) clears it.
	serveField(t, f, manager, postForm("/ui/field-workers/fwk_tess/edit", "name", "Tess Test", "flags", "1"))
	if f.lastUpdate.IsTest == nil || *f.lastUpdate.IsTest {
		t.Errorf("unticked test switch: %+v", f.lastUpdate)
	}
	// Without the marker the flag is left alone.
	serveField(t, f, manager, postForm("/ui/field-workers/fwk_tess/edit", "name", "Tess Test"))
	if f.lastUpdate.IsTest != nil {
		t.Errorf("no marker, but the test flag was sent: %+v", f.lastUpdate)
	}

	// Setting or clearing the supervisor flag needs the impersonate permission:
	// a forged switch is refused with a 403 and nothing runs.
	f.calls = nil
	for _, q := range []req{
		nativePost("/ui/field-workers/fwk_ann/edit", "name", "Ann", "canImpersonate", "true"),
		nativePost("/ui/field-workers/fwk_sue/edit", "name", "Sue", "impersonateField", "1"), // clearing it
	} {
		if rec := serveField(t, f, manager, q); rec.Code != http.StatusForbidden {
			t.Errorf("supervisor flag without permission (%s): status %d, want 403", q.target, rec.Code)
		}
	}
	if len(f.calls) != 0 {
		t.Errorf("a refused edit ran %v", f.calls)
	}
	// Leaving the flag as it is passes without the permission (the form of a
	// worker who is a supervisor, posted by someone who may not change it).
	serveField(t, f, manager, nativePost("/ui/field-workers/fwk_sue/edit", "name", "Sue", "canImpersonate", "true"))
	if len(f.calls) != 1 || f.lastUpdate.CanImpersonate != nil {
		t.Errorf("unchanged supervisor flag: calls %v, %+v", f.calls, f.lastUpdate)
	}
	// With the permission it is set and cleared.
	f.calls = nil
	serveField(t, f, withImp, postForm("/ui/field-workers/fwk_ann/edit", "name", "Ann", "impersonateField", "1", "canImpersonate", "true"))
	if f.lastUpdate.CanImpersonate == nil || !*f.lastUpdate.CanImpersonate {
		t.Errorf("set supervisor: %+v", f.lastUpdate)
	}
	serveField(t, f, withImp, postForm("/ui/field-workers/fwk_sue/edit", "name", "Sue", "impersonateField", "1"))
	if f.lastUpdate.CanImpersonate == nil || *f.lastUpdate.CanImpersonate {
		t.Errorf("clear supervisor: %+v", f.lastUpdate)
	}
	// A refusal lands on the field.
	f.failWith = usecase.Validation("EMAIL_INVALID", "email is not valid")
	mustContain(t, fieldBody(t, f, manager, postForm("/ui/field-workers/fwk_ann/edit", "name", "Ann", "email", "nope")), "Email is not valid", `id="fw-edit-form-email-err"`)
	// A viewer and another client's worker.
	f.calls = nil
	if rec := serveField(t, f, fieldCaller([]string{"clt_acme"}, permFieldWorkerView), nativePost("/ui/field-workers/fwk_ann/edit", "name", "X")); rec.Code != http.StatusForbidden {
		t.Errorf("viewer edit: status %d", rec.Code)
	}
	if rec := serveField(t, f, manager, nativePost("/ui/field-workers/fwk_gus/edit", "name", "X")); rec.Code != http.StatusNotFound {
		t.Errorf("another client's worker: status %d", rec.Code)
	}
	if len(f.calls) != 0 {
		t.Errorf("a refused edit ran %v", f.calls)
	}
}

func TestFieldWorkerSignInAsVisibility(t *testing.T) {
	f := badgeFixture()
	drawer := func(ac *auth.AuthContext, id string) string {
		return fieldBody(t, f, ac, req{target: "/ui/field-workers/" + id, datastar: true})
	}
	imp := fieldCaller([]string{"clt_acme"}, permFieldWorkerView, permFieldWorkerManage, permFieldWorkerImpers)
	manager := fieldCaller([]string{"clt_acme"}, permFieldWorkerView, permFieldWorkerManage)
	// The impersonate permission: real workers and test identities.
	mustContain(t, drawer(imp, "fwk_ann"), "Sign in as", `href="/ui/field-workers/fwk_ann/sign-in-as"`)
	mustContain(t, drawer(imp, "fwk_tess"), `href="/ui/field-workers/fwk_tess/sign-in-as"`)
	// Manage alone: only a test identity.
	mustNotContain(t, drawer(manager, "fwk_ann"), "Sign in as", "sign-in-as")
	mustContain(t, drawer(manager, "fwk_tess"), `href="/ui/field-workers/fwk_tess/sign-in-as"`)
	// A service account: a test identity only, even with the impersonate permission.
	sa := fieldServiceAccount([]string{"clt_acme"}, permFieldWorkerView, permFieldWorkerManage, permFieldWorkerImpers)
	mustNotContain(t, drawer(sa, "fwk_ann"), "sign-in-as")
	mustContain(t, drawer(sa, "fwk_tess"), `href="/ui/field-workers/fwk_tess/sign-in-as"`)
	// A viewer never; a suspended worker never.
	viewer := fieldCaller([]string{"clt_acme"}, permFieldWorkerView)
	mustNotContain(t, drawer(viewer, "fwk_ann"), "sign-in-as")
	mustNotContain(t, drawer(imp, "fwk_bob"), "sign-in-as")
}

func TestFieldWorkerSignInAsRefusals(t *testing.T) {
	f := badgeFixture()
	manager := fieldCaller([]string{"clt_acme"}, permFieldWorkerView, permFieldWorkerManage)
	sa := fieldServiceAccount([]string{"clt_acme"}, permFieldWorkerView, permFieldWorkerManage, permFieldWorkerImpers)
	imp := fieldCaller([]string{"clt_acme"}, permFieldWorkerView, permFieldWorkerManage, permFieldWorkerImpers)
	for name, c := range map[string]struct {
		ac     *auth.AuthContext
		target string
		want   int
	}{
		"manager, real worker":   {manager, "fwk_ann", http.StatusForbidden},
		"viewer, test identity":  {fieldCaller([]string{"clt_acme"}, permFieldWorkerView), "fwk_tess", http.StatusForbidden},
		"service account, real":  {sa, "fwk_ann", http.StatusForbidden},
		"other client's worker":  {imp, "fwk_gus", http.StatusNotFound},
		"other client, manager":  {manager, "fwk_gus", http.StatusForbidden}, // the permission is checked before the client, as the API's
		"unknown worker":         {imp, "fwk_nobody", http.StatusNotFound},
		"suspended worker (409)": {imp, "fwk_bob", http.StatusConflict},
	} {
		if rec := serveField(t, f, c.ac, req{target: "/ui/field-workers/" + c.target + "/sign-in-as"}); rec.Code != c.want {
			t.Errorf("GET sign-in-as, %s: status %d, want %d", name, rec.Code, c.want)
		}
		if name == "suspended worker (409)" {
			continue
		}
		rec := serveField(t, f, c.ac, nativePost("/ui/field-workers/"+c.target+"/sign-in-as", "app", "wms"))
		if rec.Code != c.want {
			t.Errorf("POST sign-in-as, %s: status %d, want %d", name, rec.Code, c.want)
		}
		mustNotContain(t, rec.Body.String(), "FWS1")
	}
	if len(f.calls) != 0 {
		t.Errorf("a refused post ran %v", f.calls)
	}
}

// The sign-in code is shown once: in the answer to the post, never again.
func TestFieldWorkerSignInCodeIsShownOnce(t *testing.T) {
	f := badgeFixture()
	imp := fieldCaller([]string{"clt_acme"}, permFieldWorkerView, permFieldWorkerManage, permFieldWorkerImpers)
	form := fieldBody(t, f, imp, req{target: "/ui/field-workers/fwk_tess/sign-in-as", datastar: true})
	mustContain(t, form, `id="fw-sign-in-form"`, "Sign in as TESS", `value="wms"`, `value="tms"`, "audited", "ten minutes")
	mustNotContain(t, form, `value="old"`)

	rec := serveField(t, f, imp, postForm("/ui/field-workers/fwk_tess/sign-in-as", "app", "tms", "drawer", "fwk_tess"))
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !slices.Equal(f.calls, []string{"sign-in-code fwk_tess tms"}) {
		t.Fatalf("sign-in-as: status %d, calls %v", rec.Code, f.calls)
	}
	code := f.lastCodeCmd.Code
	if !fieldidentity.WellFormedToken(fieldidentity.SignInCodePrefix, code) {
		t.Fatalf("code %q is not a sign-in code", code)
	}
	mustContain(t, body, `id="fw-sign-in-code"`, `id="fw-code-value"`, "<strong>TESS</strong>", "<strong>tms</strong>", "data-expires=", "Expires in",
		"works once", "audited under your name", "will not be shown again", "Copy sign-in code", "d.remove()", `aria-label="Sign-in code QR code for TESS"`)
	if n := strings.Count(body, code); n != 1 {
		t.Errorf("the code appears %d times, want once", n)
	}
	qr, err := qrSVGLabeled(code, "Sign-in code QR code for TESS")
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, body, renderToString(t, qr))
	// The expiry is the operation's: ten minutes from now.
	exp := between(body, `data-expires="`, `"`)
	at, err := time.Parse(time.RFC3339, exp)
	if err != nil || time.Until(at) < 9*time.Minute || time.Until(at) > 11*time.Minute {
		t.Errorf("expiry %q (%v)", exp, err)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control %q", rec.Header().Get("Cache-Control"))
	}
	mustContain(t, body, "Sign-in code for TESS in tms")
	for _, q := range []req{
		{target: "/ui/field-workers?client=clt_acme"},
		{target: "/ui/field-workers/fwk_tess"},
		{target: "/ui/field-workers/fwk_tess", datastar: true},
		{target: "/ui/field-workers/fwk_tess/sign-in-as", datastar: true},
	} {
		if strings.Contains(fieldBody(t, f, imp, q), code) {
			t.Errorf("GET %s re-serves the code", q.target)
		}
	}
	// A native post gets the page with the dialog, uncached, no redirect.
	native := serveField(t, f, imp, req{method: http.MethodPost, target: "/ui/field-workers/fwk_ann/sign-in-as", form: map[string][]string{"app": {"wms"}}})
	if native.Code != http.StatusOK || native.Header().Get("Location") != "" || native.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("native sign-in-as: status %d Location %q", native.Code, native.Header().Get("Location"))
	}
	mustContain(t, native.Body.String(), `id="fw-sign-in-code"`, `<h1 class="fc-page-title">Field Workers</h1>`)
	// A service account may make a code for a test identity.
	sa := fieldServiceAccount([]string{"clt_acme"}, permFieldWorkerView, permFieldWorkerManage)
	f.calls = nil
	if rec := serveField(t, f, sa, postForm("/ui/field-workers/fwk_tess/sign-in-as", "app", "wms")); rec.Code != http.StatusOK || len(f.calls) != 1 {
		t.Errorf("service account, test identity: status %d, calls %v", rec.Code, f.calls)
	}
	// An application the worker is not granted lands on the field.
	f.calls = nil
	body = fieldBody(t, f, imp, postForm("/ui/field-workers/fwk_ann/sign-in-as", "app", "tms"))
	mustContain(t, body, "The worker is not granted that application", `id="fw-sign-in-form-app-err"`)
	mustNotContain(t, body, "FWS1")
}

func TestFieldSignInHistoryShowsMethodAndActor(t *testing.T) {
	f := badgeFixture()
	f.signIns = []fieldidentity.LoginAttempt{
		{ID: "h1", IdentityID: "fwk_ann", ApplicationID: "app_wms", AuthMethod: fieldidentity.MethodBadgePIN, Outcome: fieldidentity.OutcomeSuccess, AttemptedAt: fieldDay.Add(5 * time.Hour)},
		{ID: "h2", IdentityID: "fwk_ann", ApplicationID: "app_wms", AuthMethod: fieldidentity.MethodActAs, ActorID: "fwk_sue", Outcome: fieldidentity.OutcomeSuccess, AttemptedAt: fieldDay.Add(4 * time.Hour)},
		{ID: "h3", IdentityID: "fwk_ann", ApplicationID: "app_wms", AuthMethod: fieldidentity.MethodSignInCode, ActorID: "prn_admin", Outcome: fieldidentity.OutcomeSuccess, AttemptedAt: fieldDay.Add(3 * time.Hour)},
		{ID: "h4", IdentityID: "fwk_ann", ApplicationID: "app_wms", AuthMethod: fieldidentity.MethodSignInCode, ActorID: "prn_gone", Outcome: fieldidentity.OutcomeSuccess, AttemptedAt: fieldDay.Add(2 * time.Hour)},
		{ID: "h5", IdentityID: "fwk_ann", ApplicationID: "app_wms", AuthMethod: fieldidentity.MethodBadge, Outcome: fieldidentity.OutcomeFailure, FailureReason: "PIN_REQUIRED", AttemptedAt: fieldDay.Add(time.Hour)},
		{ID: "h6", IdentityID: "fwk_ann", ApplicationID: "app_wms", AuthMethod: fieldidentity.MethodPIN, Outcome: fieldidentity.OutcomeSuccess, AttemptedAt: fieldDay},
	}
	ac := fieldCaller([]string{"clt_acme"}, permFieldWorkerView)
	body := fieldBody(t, f, ac, req{target: "/ui/field-workers/fwk_ann/sign-ins", datastar: true})
	mustContain(t, body, "Method", "Actor",
		`<code class="text-xs">BADGE_PIN</code>`, `<code class="text-xs">ACT_AS</code>`, `<code class="text-xs">SIGN_IN_CODE</code>`, `<code class="text-xs">BADGE</code>`, `<code class="text-xs">PIN</code>`,
		"Sue Supervisor (SUE)",            // a worker actor: name and code
		"Pat Admin &lt;pat@acme.test&gt;", // a principal actor, escaped
		"prn_gone",                        // an actor that cannot be resolved: the id
		"PIN_REQUIRED")
}

// ── policy ─────────────────────────────────────────────────────────────

func TestFieldPolicyShowsTheSignInOptions(t *testing.T) {
	f := fieldFixture()
	manager := fieldCaller([]string{"clt_acme"}, permFieldDeviceView, permFieldDeviceManage)
	body := fieldBody(t, f, manager, req{target: "/ui/field-policy?client=clt_acme"})
	mustContain(t, body, "Sign-in options", `name="requireEnrolledDevice"`, `name="badgeOnlyLogin"`, `name="unenrolledIdleSecs"`, `value="900"`,
		`name="impersonationMaxSecs"`, `value="14400"`, `min="300"`, `max="43200"`, "Require an enrolled device", "Badge-only sign-in", "photocopied badge")
	mustNotContain(t, between(body, `id="field-policy-requireEnrolledDevice"`, `>`), "checked")
	// A viewer sees them disabled.
	viewer := fieldBody(t, f, fieldCaller([]string{"clt_acme"}, permFieldDeviceView), req{target: "/ui/field-policy?client=clt_acme"})
	mustContain(t, viewer, `name="badgeOnlyLogin"`, "You can view this policy but not change it.")
}

// Saving the page must keep every field of the policy, the sign-in options
// included: the save builds the whole PUT body from the form, and a field the
// page forgot to carry would reset to its default on every save.
func TestFieldPolicySavePreservesTheSignInOptions(t *testing.T) {
	f := fieldFixture()
	manager := fieldCaller([]string{"clt_acme"}, permFieldDeviceView, permFieldDeviceManage)
	f.policies = map[string]fieldidentity.Policy{"clt_acme": {
		ClientID: "clt_acme", PINMinLength: 8, SoftLockThreshold: 4, SoftLockSecs: 600, HardLockThreshold: 9, SharedIdleSecs: 300,
		PersonalIdleSecs: 3600, ShiftMaxSecs: 28800, RequireAttestation: true, PINReauthDays: 3,
		RequireEnrolledDevice: true, UnenrolledIdleSecs: 1200, BadgeOnlyLogin: true, ImpersonationMaxSecs: 7200, UpdatedAt: fieldDay,
	}}
	want := f.policies["clt_acme"]

	// The page shows them: both switches on, the numbers.
	body := fieldBody(t, f, manager, req{target: "/ui/field-policy?client=clt_acme"})
	mustContain(t, body, `value="1200"`, `value="7200"`)
	for _, name := range []string{"requireEnrolledDevice", "badgeOnlyLogin", "requireAttestation"} {
		if !strings.Contains(between(body, `name="`+name+`"`, `>`), "checked") {
			t.Errorf("switch %s is not shown on", name)
		}
	}

	// Post the form as the page renders it (a switch that is on posts "1", the
	// numbers post their value): nothing changes.
	rec := serveField(t, f, manager, postForm("/ui/field-policy", "client", "clt_acme",
		"pinMinLength", "8", "softLockThreshold", "4", "softLockSecs", "600", "hardLockThreshold", "9", "sharedIdleSecs", "300",
		"personalIdleSecs", "3600", "shiftMaxSecs", "28800", "pinReauthDays", "3", "unenrolledIdleSecs", "1200", "impersonationMaxSecs", "7200",
		"requireAttestation", "1", "requireEnrolledDevice", "1", "badgeOnlyLogin", "1"))
	mustContain(t, rec.Body.String(), "Field policy saved for Acme Corp")
	got := f.policies["clt_acme"]
	if !got.RequireEnrolledDevice || !got.BadgeOnlyLogin {
		t.Errorf("saving the page lost a sign-in switch: requireEnrolledDevice=%v badgeOnlyLogin=%v", got.RequireEnrolledDevice, got.BadgeOnlyLogin)
	}
	got.UpdatedAt, got.IsDefault = want.UpdatedAt, false
	if got != want {
		t.Errorf("saved policy %+v, want %+v", got, want)
	}
	// And the page read back shows them still.
	body = fieldBody(t, f, manager, req{target: "/ui/field-policy?client=clt_acme"})
	for _, name := range []string{"requireEnrolledDevice", "badgeOnlyLogin"} {
		if !strings.Contains(between(body, `name="`+name+`"`, `>`), "checked") {
			t.Errorf("switch %s is off after a save", name)
		}
	}

	// Turning them off, and a refused number, are the form's call.
	serveField(t, f, manager, postForm("/ui/field-policy", "client", "clt_acme", "unenrolledIdleSecs", "1800", "impersonationMaxSecs", "3600"))
	got = f.policies["clt_acme"]
	if got.RequireEnrolledDevice || got.BadgeOnlyLogin || got.UnenrolledIdleSecs != 1800 || got.ImpersonationMaxSecs != 3600 {
		t.Errorf("after switching off: %+v", got)
	}
	rec = serveField(t, f, manager, postForm("/ui/field-policy", "client", "clt_acme", "impersonationMaxSecs", "100", "requireEnrolledDevice", "1"))
	mustContain(t, rec.Body.String(), "Longest session signed in as another worker must be between 300 and 43200", `value="100"`)
	if f.policies["clt_acme"].RequireEnrolledDevice {
		t.Error("a refused save was applied")
	}
	rec = serveField(t, f, manager, postForm("/ui/field-policy", "client", "clt_acme", "unenrolledIdleSecs", "5"))
	mustContain(t, rec.Body.String(), "Idle timeout when the device is not enrolled must be between 60 and 86400")
}

func TestFieldDevicesPageSaysEnrolmentIsOptional(t *testing.T) {
	f := fieldFixture()
	body := fieldBody(t, f, fieldCaller([]string{"clt_acme"}, permFieldDeviceView), req{target: "/ui/field-devices?client=clt_acme"})
	mustContain(t, body, "Enrolment is optional", "from any device with a worker code and PIN")
	mustNotContain(t, body, "workers sign in on it with their PIN")
}

func renderToString(t *testing.T, c templ.Component) string {
	t.Helper()
	var b strings.Builder
	if err := c.Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	return b.String()
}
