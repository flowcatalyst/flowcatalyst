package pages

import (
	"net/http"
	"slices"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/go-sdk/usecase"
)

func invalidLink() error {
	return usecase.BusinessRule(fieldidentity.CodeInvalidLink, "the platform user cannot access the worker's client")
}

// The "Linked platform user" section of the worker drawer
// (docs/field-native-session-spec.md section 5): the state, link / change /
// unlink for a manager, and the API's permission and client box.

// linkFixture is the field fixture with Ann linked to a platform user, Bob
// linked to a user that no longer exists, and one more user to link.
func linkFixture() *fakeField {
	f := fieldFixture()
	for n := range f.workers {
		switch f.workers[n].ID {
		case "fwk_ann":
			f.workers[n].LinkedPrincipalID = "prn_pat"
		case "fwk_bob":
			f.workers[n].LinkedPrincipalID = "prn_gone"
		}
	}
	f.users = []fieldidentity.PrincipalRef{
		{ID: "prn_pat", Name: "Pat Admin", Email: "pat@acme.test"},
		{ID: "prn_lee", Name: "Lee Dispatcher", Email: "lee@acme.test"},
	}
	return f
}

func TestFieldWorkerLinkSection(t *testing.T) {
	f := linkFixture()
	manager := fieldCaller([]string{"clt_acme"}, permFieldWorkerView, permFieldWorkerManage)
	// Ann is linked: who, and Change / Unlink.
	body := fieldBody(t, f, manager, req{target: "/ui/field-workers/fwk_ann", datastar: true})
	mustContain(t, body, "Linked platform user", "Pat Admin", "pat@acme.test", "Change",
		`href="/ui/field-workers/fwk_ann/link"`, `data-action="/ui/field-workers/fwk_ann/unlink"`)
	mustNotContain(t, body, "Not linked")
	// Bob's user is gone: the id is shown as unresolvable.
	body = fieldBody(t, f, manager, req{target: "/ui/field-workers/fwk_bob", datastar: true})
	mustContain(t, body, "No longer exists", "prn_gone")
	// Nobody is linked on a worker without a link: Link.
	f.workers[0].LinkedPrincipalID = ""
	body = fieldBody(t, f, manager, req{target: "/ui/field-workers/fwk_ann", datastar: true})
	mustContain(t, body, "Not linked", "Link platform user")
	mustNotContain(t, body, "Unlink")
	// The confirmation is on the page.
	page := fieldBody(t, f, manager, req{target: "/ui/field-workers?client=clt_acme"})
	mustContain(t, page, `<dialog id="fw-unlink"`, "Remove the link between this worker and their platform user")
	// A viewer sees the state but cannot act.
	viewer := fieldBody(t, linkFixture(), fieldCaller([]string{"clt_acme"}, permFieldWorkerView), req{target: "/ui/field-workers/fwk_ann", datastar: true})
	mustContain(t, viewer, "Linked platform user", "Pat Admin")
	mustNotContain(t, viewer, "Unlink", "Change", "/link")
}

func TestFieldWorkerLinkAndUnlink(t *testing.T) {
	f := linkFixture()
	ac := fieldCaller([]string{"clt_acme"}, permFieldWorkerManage)
	// The form opens as a dialog.
	rec := serveField(t, f, ac, req{target: "/ui/field-workers/fwk_ann/link", datastar: true})
	if rec.Code != http.StatusOK {
		t.Fatalf("link form: status %d", rec.Code)
	}
	mustContain(t, rec.Body.String(), `id="fw-link-form"`, "Platform user&#39;s email")
	// Linking resolves the email and runs the operation.
	rec = serveField(t, f, ac, postForm("/ui/field-workers/fwk_bob/link", "drawer", "fwk_bob", "email", " LEE@acme.test "))
	if rec.Code != http.StatusOK || !slices.Equal(f.calls, []string{"link fwk_bob prn_lee"}) {
		t.Errorf("link: status %d, calls %v", rec.Code, f.calls)
	}
	mustContain(t, rec.Body.String(), "BOB is linked to lee@acme.test")
	// An address no user has is refused 422 on the field, nothing runs.
	f.calls = nil
	rec = serveField(t, f, ac, nativePost("/ui/field-workers/fwk_bob/link", "email", "nobody@acme.test"))
	if rec.Code != http.StatusUnprocessableEntity || len(f.calls) != 0 {
		t.Errorf("unknown user: status %d, calls %v", rec.Code, f.calls)
	}
	mustContain(t, rec.Body.String(), "No platform user has that email address")
	rec = serveField(t, f, ac, nativePost("/ui/field-workers/fwk_bob/link", "email", " "))
	if rec.Code != http.StatusBadRequest || len(f.calls) != 0 {
		t.Errorf("blank email: status %d, calls %v", rec.Code, f.calls)
	}
	// The operation's refusal shows on the field (422 INVALID_LINK).
	f.failWith = invalidLink()
	rec = serveField(t, f, ac, nativePost("/ui/field-workers/fwk_bob/link", "email", "pat@acme.test"))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("INVALID_LINK: status %d, want 422", rec.Code)
	}
	mustContain(t, rec.Body.String(), "cannot access the worker")
	// Unlink.
	f.calls = nil
	rec = serveField(t, f, ac, postForm("/ui/field-workers/fwk_ann/unlink", "drawer", "fwk_ann"))
	if rec.Code != http.StatusOK || !slices.Equal(f.calls, []string{"unlink fwk_ann"}) {
		t.Errorf("unlink: status %d, calls %v", rec.Code, f.calls)
	}
	mustContain(t, rec.Body.String(), "ANN is no longer linked to a platform user")
}

func TestFieldWorkerLinkIsRefusedWithoutManage(t *testing.T) {
	f := linkFixture()
	viewer := fieldCaller([]string{"clt_acme"}, permFieldWorkerView)
	for _, q := range []req{
		{target: "/ui/field-workers/fwk_ann/link"},
		nativePost("/ui/field-workers/fwk_ann/link", "email", "lee@acme.test"),
		nativePost("/ui/field-workers/fwk_ann/unlink"),
	} {
		if rec := serveField(t, f, viewer, q); rec.Code != http.StatusForbidden {
			t.Errorf("viewer %s %s: status %d, want 403", q.method, q.target, rec.Code)
		}
	}
	// Another client's worker is not found.
	acme := fieldCaller([]string{"clt_acme"}, permFieldWorkerView, permFieldWorkerManage)
	for _, q := range []req{
		{target: "/ui/field-workers/fwk_gus/link"},
		nativePost("/ui/field-workers/fwk_gus/link", "email", "lee@acme.test"),
		nativePost("/ui/field-workers/fwk_gus/unlink"),
	} {
		if rec := serveField(t, f, acme, q); rec.Code != http.StatusNotFound {
			t.Errorf("%s %s on another client's worker: status %d, want 404", q.method, q.target, rec.Code)
		}
	}
	if len(f.calls) != 0 {
		t.Errorf("a refused request ran %v", f.calls)
	}
}
