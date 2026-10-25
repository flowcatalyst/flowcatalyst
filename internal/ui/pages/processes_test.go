package pages

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/process"
	procops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/process/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

type fakeProcStore struct{ procs []process.Process }

func (f *fakeProcStore) FindWithFilters(context.Context, *string, *string, *string) ([]process.Process, error) {
	return append([]process.Process(nil), f.procs...), nil
}

func (f *fakeProcStore) FindByID(_ context.Context, id string) (*process.Process, error) {
	for i := range f.procs {
		if f.procs[i].ID == id {
			p := f.procs[i]
			return &p, nil
		}
	}
	return nil, nil
}

type fakeProcOps struct {
	created []procops.CreateCommand
	updated []procops.UpdateCommand
	actions []string
	err     error
	store   *fakeProcStore
}

func (o *fakeProcOps) Create(_ context.Context, cmd procops.CreateCommand) (string, error) {
	o.created = append(o.created, cmd)
	if o.err != nil {
		return "", o.err
	}
	o.store.procs = append(o.store.procs, proc("prc_9", cmd.Code, cmd.Name, process.StatusCurrent))
	return "prc_9", nil
}

func (o *fakeProcOps) Update(_ context.Context, cmd procops.UpdateCommand) error {
	o.updated = append(o.updated, cmd)
	return o.err
}

func (o *fakeProcOps) act(name, id string) error {
	o.actions = append(o.actions, name+":"+id)
	return o.err
}

func (o *fakeProcOps) Archive(_ context.Context, id string) error { return o.act("archive", id) }
func (o *fakeProcOps) Delete(_ context.Context, id string) error  { return o.act("delete", id) }

func proc(id, code, name string, status process.Status) process.Process {
	parts := strings.Split(code, ":")
	day := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	return process.Process{
		ID: id, Code: code, Name: name, Status: status, Source: process.SourceUI, Application: parts[0], Subdomain: parts[1], ProcessName: parts[2],
		DiagramType: "mermaid", Tags: []string{}, CreatedAt: day, UpdatedAt: day,
	}
}

type procHarness struct {
	store *fakeProcStore
	ops   *fakeProcOps
}

func newProcHarness() *procHarness {
	desc := "Ships <orders>"
	ship := proc("prc_1", "orders:fulfilment:shipment-flow", "Shipment Flow", process.StatusCurrent)
	ship.Description = &desc
	ship.Tags = []string{"fulfilment", "billing"}
	// A label carrying a Datastar attribute: the drawing's view is data-ignore
	// and fc-diagram.js strips Datastar attributes (see the kit's diagram.md).
	ship.Body = "graph TD\n  A[Start] --> B{<script>alert(1)</script>}\n  B --> C[\"<span data-init='alert(2)'>x</span>\"]"
	refund := proc("prc_2", "orders:billing:refunds", "Refunds", process.StatusArchived)
	onboard := proc("prc_3", "crm:accounts:onboarding", "Onboarding", process.StatusCurrent)
	onboard.DiagramType = "plantuml"
	onboard.Body = "@startuml\n@enduml"
	s := &fakeProcStore{procs: []process.Process{ship, refund, onboard}}
	return &procHarness{store: s, ops: &fakeProcOps{store: s}}
}

func (h *procHarness) rec(t *testing.T, ac *auth.AuthContext, q req) *httptest.ResponseRecorder {
	t.Helper()
	return serveMounted(t, ac, q, &Processes{Store: h.store, Ops: h.ops, Shell: shellForTest()})
}

func (h *procHarness) do(t *testing.T, ac *auth.AuthContext, q req) string {
	t.Helper()
	return h.rec(t, ac, q).Body.String()
}

const procViewPerm = "platform:messaging:process:view"

func TestProcessesList(t *testing.T) {
	h := newProcHarness()
	body := h.do(t, anchorAdmin(), req{target: "/ui/processes"})
	mustContain(t, body, `<title>Processes · FlowCatalyst</title>`, "Create Process", "Ships &lt;orders&gt;", ">fulfilment<", ">billing<",
		">ARCHIVED<", ">CURRENT<", "Showing 1 to 3 of 3 processes", `<dialog id="proc-delete"`, `<dialog id="proc-archive"`,
		`<span class="fc-code-seg app">orders</span>`, `<option value="crm">crm</option>`, `<option value="accounts">accounts</option>`)
	// Archived rows keep Edit, disabled with the reason.
	mustContain(t, body, procArchivedNoEdit)

	// Live: the application narrows the list and the subdomain options.
	live := h.do(t, anchorAdmin(), req{target: "/ui/processes?app=orders&status=CURRENT", datastar: true})
	mustContain(t, live, "Shipment Flow", `id="procs-subdomain-filter"`, `<option value="fulfilment">`, "replaceState", "/ui/processes?app=orders\\u0026status=CURRENT")
	mustNotContain(t, live, "Onboarding", "Refunds", `<option value="accounts">`)

	// A subdomain that is not one of the application's is dropped; so is an unknown application.
	pruned := h.do(t, anchorAdmin(), req{target: "/ui/processes?app=crm&subdomain=billing", datastar: true})
	mustContain(t, pruned, "Onboarding", `&#34;/ui/processes?app=crm&#34;)`)
	unknown := h.do(t, anchorAdmin(), req{target: "/ui/processes?app=nope"})
	mustContain(t, unknown, "Showing 1 to 3 of 3 processes")

	// The quick search covers code and name, not the description.
	mustContain(t, h.do(t, anchorAdmin(), req{target: "/ui/processes?q=refund"}), "Showing 1 to 1 of 1 processes")
	mustContain(t, h.do(t, anchorAdmin(), req{target: "/ui/processes?q=ships"}), "No processes found")

	h.store.procs = nil
	empty := h.do(t, anchorAdmin(), req{target: "/ui/processes"})
	mustContain(t, empty, "No processes yet", "Create your first process")
}

func TestProcessesPermissions(t *testing.T) {
	h := newProcHarness()
	if rec := h.rec(t, connUser("platform:messaging:event:view"), req{target: "/ui/processes"}); rec.Code != http.StatusForbidden {
		t.Errorf("no view: %d", rec.Code)
	}
	if rec := h.rec(t, connUser("platform:messaging:event:view"), req{target: "/ui/processes/prc_1"}); rec.Code != http.StatusForbidden {
		t.Errorf("no view, drawer: %d", rec.Code)
	}
	viewer := h.do(t, connUser(procViewPerm), req{target: "/ui/processes"})
	mustNotContain(t, viewer, "Create Process", `aria-label="Edit `, `<dialog id="proc-delete"`, `<dialog id="proc-archive"`)
	mustNotContain(t, h.do(t, connUser(procViewPerm), req{target: "/ui/processes/prc_1", datastar: true}), "/ui/processes/prc_1/archive", "/ui/processes/prc_1/edit")

	// A delete-only user may neither create, edit nor archive (the API's checks, 1f83887).
	deleter := connUser(procViewPerm, "platform:messaging:process:delete")
	h.do(t, deleter, req{method: http.MethodPost, target: "/ui/processes/prc_1/archive", datastar: true, form: url.Values{}})
	h.do(t, deleter, req{method: http.MethodPost, target: "/ui/processes/new", datastar: true, form: url.Values{"code": {"a:b:c"}, "name": {"X"}}})
	h.do(t, deleter, req{method: http.MethodPost, target: "/ui/processes/prc_1/edit", datastar: true, form: url.Values{"name": {"X"}}})
	if rec := h.rec(t, deleter, req{target: "/ui/processes/new"}); rec.Code != http.StatusForbidden {
		t.Errorf("delete-only user opened the create dialog: %d", rec.Code)
	}
	updater := connUser(procViewPerm, "platform:messaging:process:update")
	h.do(t, updater, req{method: http.MethodPost, target: "/ui/processes/prc_2/delete", datastar: true, form: url.Values{}})
	if len(h.ops.actions)+len(h.ops.created)+len(h.ops.updated) != 0 {
		t.Fatalf("an operation ran without its permission: %v %v %v", h.ops.actions, h.ops.created, h.ops.updated)
	}
	// Each one with its permission.
	h.do(t, updater, req{method: http.MethodPost, target: "/ui/processes/prc_1/archive", datastar: true, form: url.Values{}})
	h.do(t, deleter, req{method: http.MethodPost, target: "/ui/processes/prc_2/delete", datastar: true, form: url.Values{}})
	if strings.Join(h.ops.actions, ",") != "archive:prc_1,delete:prc_2" {
		t.Errorf("actions = %v", h.ops.actions)
	}
}

func TestProcessDrawer(t *testing.T) {
	h := newProcHarness()
	cur := h.do(t, anchorAdmin(), req{target: "/ui/processes/prc_1", datastar: true})
	mustContain(t, cur, `"selProc":"prc_1"`, "Ships &lt;orders&gt;", `href="/ui/processes/prc_1/edit"`, `data-action="/ui/processes/prc_1/archive"`,
		`<fc-diagram class="block" data-bundle="/ui/assets/mermaid.`, `class="fc-code-block mt-2"`,
		"B{&lt;script&gt;alert(1)&lt;/script&gt;}", `role="img" aria-label="Diagram of `, ">fulfilment<",
		"&lt;span data-init=&#39;alert(2)&#39;&gt;x&lt;/span&gt;", ` hidden data-ignore></div>`)
	mustNotContain(t, cur, "<script>alert", "<span data-init", `/ui/processes/prc_1/delete`, "current UI")

	arch := h.do(t, anchorAdmin(), req{target: "/ui/processes/prc_2", datastar: true})
	mustContain(t, arch, `data-action="/ui/processes/prc_2/delete"`, "No diagram body yet.", ">ARCHIVED<")
	mustNotContain(t, arch, "/ui/processes/prc_2/edit", "/ui/processes/prc_2/archive")

	other := h.do(t, anchorAdmin(), req{target: "/ui/processes/prc_3", datastar: true})
	mustContain(t, other, "diagram type is plantuml", "@startuml", `aria-label="Diagram source (plantuml)"`)
	mustNotContain(t, other, "<fc-diagram")

	// Full page: the list with the drawer open.
	page := h.do(t, anchorAdmin(), req{target: "/ui/processes/prc_1"})
	mustContain(t, page, `{selProc: &#39;prc_1&#39;}`, "Shipment Flow", "Showing 1 to 3 of 3 processes", `src="/ui/assets/fc-diagram.`)
	if rec := h.rec(t, anchorAdmin(), req{target: "/ui/processes/prc_404", datastar: true}); !strings.Contains(rec.Body.String(), "fc-flash-error") {
		t.Errorf("missing process: %s", rec.Body.String())
	}
}

func TestProcessCreateAndEdit(t *testing.T) {
	h := newProcHarness()
	dlg := h.do(t, anchorAdmin(), req{target: "/ui/processes/new", datastar: true})
	// The slot is replaced, not morphed, so a second dialog opens as a modal too.
	mustContain(t, dlg, "data: mode replace\n", `data: elements <div id="proc-dialog-slot"><dialog id="proc-form"`)
	mustContain(t, dlg, "New process", `name="code"`, "Format: application:subdomain:process-name", "mermaid.js.org", "Comma-separated.")

	// The SPA's own check: code and name are required.
	blank := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/processes/new", datastar: true, form: url.Values{"code": {" "}, "name": {""}}})
	mustContain(t, blank, `id="proc-form-code-err"`, `id="proc-form-name-err"`, "Code is required")
	if len(h.ops.created) != 0 {
		t.Fatal("created without code and name")
	}

	body := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/processes/new", datastar: true, multipart: true, form: url.Values{
		"code": {" crm:accounts:churn "}, "name": {" Churn "}, "description": {"  "}, "tags": {"crm, , retention ,"}, "body": {"graph LR\n  A --> B\n"},
	}})
	c := h.ops.created[0]
	if c.Code != "crm:accounts:churn" || c.Name != "Churn" || c.Description != nil || strings.Join(c.Tags, "|") != "crm|retention" || c.Body != "graph LR\n  A --> B\n" {
		t.Errorf("create = %+v", c)
	}
	mustContain(t, body, `"selProc":"prc_9"`, "Process created", "proc-form")

	h.ops.err = usecase.Validation("INVALID_CODE_FORMAT", "Process code must follow format: application:subdomain:process-name")
	bad := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/processes/new", datastar: true, form: url.Values{"code": {"nope"}, "name": {"N"}}})
	mustContain(t, bad, `id="proc-form-code-err"`, "Process code must follow format")
	h.ops.err = usecase.Conflict("CODE_EXISTS", "Process with code 'crm:accounts:churn' already exists")
	mustContain(t, h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/processes/new", datastar: true, form: url.Values{"code": {"crm:accounts:churn"}, "name": {"N"}}}),
		`id="proc-form-code-err"`, "already exists")
	h.ops.err = nil

	edit := h.do(t, anchorAdmin(), req{target: "/ui/processes/prc_1/edit", datastar: true})
	mustContain(t, edit, "Edit process", "Shipment Flow (orders:fulfilment:shipment-flow)", `value="fulfilment, billing"`, "A[Start]")
	mustNotContain(t, edit, `name="code"`)
	// An archived process has no edit form, not even by a stale link.
	archived := h.do(t, anchorAdmin(), req{target: "/ui/processes/prc_2/edit", datastar: true})
	mustContain(t, archived, "fc-flash-error", process.CannotEditReason)
	mustNotContain(t, archived, "Edit process")
	h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/processes/prc_1/edit", datastar: true, form: url.Values{
		"name": {" Shipments "}, "description": {""}, "tags": {""}, "body": {"graph TD\n"}, "drawer": {"prc_1"},
	}})
	u := h.ops.updated[0]
	if *u.Name != "Shipments" || u.Description == nil || *u.Description != "" || u.Tags == nil || len(u.Tags) != 0 || *u.Body != "graph TD\n" {
		t.Errorf("update = %+v", u)
	}
	h.ops.err = usecase.Validation("NAME_REQUIRED", "name cannot be empty")
	mustContain(t, h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/processes/prc_1/edit", datastar: true, form: url.Values{"name": {" "}}}),
		`id="proc-form-name-err"`, "Name cannot be empty")
}

func TestProcessActions(t *testing.T) {
	h := newProcHarness()
	arch := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/processes/prc_1/archive", datastar: true, form: url.Values{"drawer": {"prc_1"}}})
	mustContain(t, arch, "Process archived", `id="procs-table"`, "data: selector #proc-drawer\n")
	del := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/processes/prc_2/delete", datastar: true, form: url.Values{"drawer": {"prc_2"}}})
	mustContain(t, del, "Process deleted", `"selProc":""`)

	h.ops.err = usecase.Internal("REPO", "boom", nil)
	mustContain(t, h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/processes/prc_2/delete", datastar: true, form: url.Values{}}), "fc-flash-error", "Something went wrong.")
	h.ops.err = nil

	native := h.rec(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/processes/prc_1/archive", form: url.Values{}})
	if native.Code != http.StatusSeeOther || native.Header().Get("Location") != "/ui/processes" || !strings.Contains(native.Header().Get("Set-Cookie"), "process-archived") {
		t.Errorf("native archive: %d %v", native.Code, native.Header())
	}
}
