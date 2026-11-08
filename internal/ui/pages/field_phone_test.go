package pages

import (
	"testing"

	"github.com/flowcatalyst/go-sdk/usecase"
)

func TestFieldWorkerPhoneCreateForm(t *testing.T) {
	f := fieldFixture()
	ac := fieldCaller([]string{"clt_acme"}, permFieldWorkerManage)
	form := fieldBody(t, f, ac, req{target: "/ui/field-workers/new?client=clt_acme", datastar: true})
	mustContain(t, form, `name="phone"`, `type="tel"`, "Phone")

	serveField(t, f, ac, postForm("/ui/field-workers/new", "client", "clt_acme", "workerCode", "zed", "name", "Zed", "phone", " +27 82 123 4567 "))
	if f.lastEnsure.Phone != "+27 82 123 4567" {
		t.Errorf("create carried phone %q", f.lastEnsure.Phone)
	}

	// A refusal lands on the phone field.
	f.failWith = usecase.Validation("PHONE_INVALID", "phone is not a valid phone number")
	body := fieldBody(t, f, ac, postForm("/ui/field-workers/new", "client", "clt_acme", "workerCode", "zed", "name", "Zed", "phone", "12"))
	mustContain(t, body, `id="fw-form-phone-err"`, `value="12"`)
}

func TestFieldWorkerPhoneEditAndDetail(t *testing.T) {
	f := fieldFixture()
	for i := range f.workers {
		if f.workers[i].ID == "fwk_ann" {
			f.workers[i].Phone = "(021) 555 0100"
		}
	}
	ac := fieldCaller([]string{"clt_acme"}, permFieldWorkerManage)

	mustContain(t, fieldBody(t, f, ac, req{target: "/ui/field-workers/fwk_ann", datastar: true}), "Phone", "(021) 555 0100")

	form := fieldBody(t, f, ac, req{target: "/ui/field-workers/fwk_ann/edit", datastar: true})
	mustContain(t, form, `name="phone"`, `type="tel"`, `value="(021) 555 0100"`)

	serveField(t, f, ac, postForm("/ui/field-workers/fwk_ann/edit", "name", "Ann", "phone", " 082-123-4567 ", "drawer", "fwk_ann"))
	if f.lastUpdate.Phone == nil || *f.lastUpdate.Phone != "082-123-4567" {
		t.Errorf("edit carried phone %v", f.lastUpdate.Phone)
	}
	// Blank clears it: posted as an empty (non-nil) value.
	serveField(t, f, ac, postForm("/ui/field-workers/fwk_ann/edit", "name", "Ann", "phone", "", "drawer", "fwk_ann"))
	if f.lastUpdate.Phone == nil || *f.lastUpdate.Phone != "" {
		t.Errorf("blank edit carried phone %v", f.lastUpdate.Phone)
	}

	f.failWith = usecase.Validation("PHONE_INVALID", "phone is not a valid phone number")
	body := fieldBody(t, f, ac, postForm("/ui/field-workers/fwk_ann/edit", "name", "Ann", "phone", "12", "drawer", "fwk_ann"))
	mustContain(t, body, `id="fw-edit-form-phone-err"`)
}
