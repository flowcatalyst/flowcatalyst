package pages

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/go-sdk/usecase"
)

const (
	permFieldWorkerView   = "platform:iam:field-worker:view"
	permFieldWorkerManage = "platform:iam:field-worker:manage"
	permFieldDeviceView   = "platform:iam:field-device:view"
	permFieldDeviceManage = "platform:iam:field-device:manage"
	permFieldWorkerImpers = "platform:iam:field-worker:impersonate"

	tempPin = "7391-TEMP"
)

var fieldDay = time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)

// fakeField is the field store and the field commands in one: it serves the
// fixtures, records every command and applies the real policy validation.
type fakeField struct {
	workers  []fieldidentity.Identity
	devices  []fieldidentity.Device
	policies map[string]fieldidentity.Policy
	apps     []application.Application
	signIns  []fieldidentity.LoginAttempt
	sessions map[string]int
	// badges are the workers' ACTIVE badges by worker id; principals name the
	// platform principals that appear as actors.
	badges     map[string]*fieldidentity.Badge
	principals map[string]string
	// users are the platform users a worker can be linked to (by email).
	users []fieldidentity.PrincipalRef

	// What the last commands carried (tokens and flags are not in calls).
	lastEnsure  fieldidentity.EnsureWorkerCommand
	lastUpdate  fieldidentity.UpdateWorkerCommand
	lastTokens  []string
	lastCodeCmd fieldidentity.CreateSignInCodeCommand

	calls []string
	// failWith, when set, is returned by the next command.
	failWith error
}

func (f *fakeField) Workers(_ context.Context, flt fieldidentity.IdentityFilter) ([]fieldidentity.Identity, int64, error) {
	var out []fieldidentity.Identity
	for _, w := range f.workers {
		if w.ClientID != flt.ClientID {
			continue
		}
		if flt.Status != "" && w.Status != flt.Status {
			continue
		}
		if flt.AppID != "" && !w.HasApp(flt.AppID) {
			continue
		}
		if flt.IsTest != nil && w.IsTest != *flt.IsTest {
			continue
		}
		if flt.AppIDs != nil && !slices.ContainsFunc(w.Apps, func(g fieldidentity.AppGrant) bool { return slices.Contains(flt.AppIDs, g.AppID) }) {
			continue
		}
		if q := strings.ToUpper(flt.Query); q != "" && !strings.HasPrefix(strings.ToUpper(w.Name), q) && !strings.HasPrefix(w.WorkerCode, q) {
			continue
		}
		out = append(out, w)
	}
	total := int64(len(out))
	lo := min(flt.Offset, len(out))
	hi := min(lo+flt.Limit, len(out))
	return out[lo:hi], total, nil
}

func (f *fakeField) Worker(_ context.Context, id string) (*fieldidentity.Identity, error) {
	for i := range f.workers {
		if f.workers[i].ID == id {
			w := f.workers[i]
			return &w, nil
		}
	}
	return nil, nil
}

func (f *fakeField) WorkerByCode(_ context.Context, clientID, code string) (*fieldidentity.Identity, error) {
	for i := range f.workers {
		if f.workers[i].ClientID == clientID && f.workers[i].WorkerCode == fieldidentity.NormalizeWorkerCode(code) {
			w := f.workers[i]
			return &w, nil
		}
	}
	return nil, nil
}

func (f *fakeField) SignIns(_ context.Context, identityID string, offset, limit int) ([]fieldidentity.LoginAttempt, int64, error) {
	var out []fieldidentity.LoginAttempt
	for _, a := range f.signIns {
		if a.IdentityID == identityID {
			out = append(out, a)
		}
	}
	total := int64(len(out))
	lo := min(offset, len(out))
	return out[lo:min(lo+limit, len(out))], total, nil
}

func (f *fakeField) Devices(_ context.Context, flt fieldidentity.DeviceFilter) ([]fieldidentity.Device, int64, error) {
	var out []fieldidentity.Device
	for _, d := range f.devices {
		if d.ClientID != flt.ClientID || (flt.Status != "" && d.Status != flt.Status) || (flt.Mode != "" && d.Mode != flt.Mode) {
			continue
		}
		if flt.AppIDs != nil && !slices.ContainsFunc(d.AppIDs, func(a string) bool { return slices.Contains(flt.AppIDs, a) }) {
			continue
		}
		out = append(out, d)
	}
	total := int64(len(out))
	lo := min(flt.Offset, len(out))
	return out[lo:min(lo+flt.Limit, len(out))], total, nil
}

func (f *fakeField) Device(_ context.Context, id string) (*fieldidentity.Device, error) {
	for i := range f.devices {
		if f.devices[i].ID == id {
			d := f.devices[i]
			return &d, nil
		}
	}
	return nil, nil
}

func (f *fakeField) ActiveSessions(context.Context, []string) (map[string]int, error) {
	return f.sessions, nil
}

func (f *fakeField) Policy(_ context.Context, clientID string) (fieldidentity.Policy, error) {
	if p, ok := f.policies[clientID]; ok {
		return p, nil
	}
	return fieldidentity.DefaultPolicy(clientID), nil
}

func (f *fakeField) Applications(context.Context) ([]application.Application, error) {
	return f.apps, nil
}

func (f *fakeField) ActiveBadge(_ context.Context, identityID string) (*fieldidentity.Badge, error) {
	return f.badges[identityID], nil
}

func (f *fakeField) PrincipalName(_ context.Context, id string) (string, bool, error) {
	name, ok := f.principals[id]
	return name, ok, nil
}

func (f *fakeField) PrincipalRefs(_ context.Context, ids []string) (map[string]fieldidentity.PrincipalRef, error) {
	out := map[string]fieldidentity.PrincipalRef{}
	for _, u := range f.users {
		if slices.Contains(ids, u.ID) {
			out[u.ID] = u
		}
	}
	return out, nil
}

func (f *fakeField) PrincipalByEmail(_ context.Context, email string) (*fieldidentity.PrincipalRef, error) {
	for _, u := range f.users {
		if strings.EqualFold(u.Email, email) {
			return &u, nil
		}
	}
	return nil, nil
}

func (f *fakeField) record(s string) error {
	f.calls = append(f.calls, s)
	err := f.failWith
	f.failWith = nil
	return err
}

func (f *fakeField) EnsureWorker(_ context.Context, cmd fieldidentity.EnsureWorkerCommand) (fieldidentity.Event, error) {
	f.lastEnsure = cmd
	return fieldidentity.Event{Type: fieldidentity.WorkerCreatedType, ID: "fwk_new"}, f.record("ensure " + cmd.ClientID + " " + cmd.WorkerCode + " " + strings.Join(cmd.AppCodes, ","))
}

func (f *fakeField) UpdateWorker(_ context.Context, cmd fieldidentity.UpdateWorkerCommand) error {
	f.lastUpdate = cmd
	return f.record("update " + cmd.ID)
}

func (f *fakeField) IssueBadge(_ context.Context, cmd fieldidentity.IssueBadgeCommand) (fieldidentity.IssuedBadge, error) {
	err := f.record("issue-badge " + cmd.ID)
	f.lastTokens = []string{cmd.Token}
	w, _ := f.Worker(context.Background(), cmd.ID)
	b := &fieldidentity.Badge{ID: "fwb_new", ClientID: cmd.ClientID, IdentityID: cmd.ID, Status: fieldidentity.BadgeActive, CreatedAt: fieldDay}
	return fieldidentity.IssuedBadge{Worker: w, Badge: b}, err
}

func (f *fakeField) IssueBadges(_ context.Context, cmd fieldidentity.IssueBadgesCommand) ([]fieldidentity.IssuedBadge, error) {
	err := f.record("issue-badges " + strings.Join(cmd.WorkerIDs, ","))
	f.lastTokens = cmd.Tokens
	var out []fieldidentity.IssuedBadge
	for _, id := range cmd.WorkerIDs {
		w, _ := f.Worker(context.Background(), id)
		out = append(out, fieldidentity.IssuedBadge{Worker: w, Badge: &fieldidentity.Badge{ID: "fwb_" + id, IdentityID: id, Status: fieldidentity.BadgeActive, CreatedAt: fieldDay}})
	}
	return out, err
}

func (f *fakeField) RevokeBadge(_ context.Context, ref fieldidentity.BadgeRef) error {
	return f.record("revoke-badge " + ref.ID + " " + ref.BadgeID)
}

// CreateSignInCode applies the operation's target checks (an active worker
// granted the application).
func (f *fakeField) CreateSignInCode(_ context.Context, cmd fieldidentity.CreateSignInCodeCommand) (fieldidentity.CreateSignInCodeResult, error) {
	f.lastCodeCmd = cmd
	err := f.record("sign-in-code " + cmd.ID + " " + cmd.AppCode)
	w, _ := f.Worker(context.Background(), cmd.ID)
	if err == nil && w != nil {
		app := ""
		for _, a := range f.apps {
			if a.Code == cmd.AppCode {
				app = string(a.ID)
			}
		}
		switch {
		case w.Status != fieldidentity.StatusActive:
			err = usecase.Conflict(fieldidentity.CodeWorkerUnavailable, "the worker is not active")
		case app == "" || !w.HasApp(app):
			err = usecase.Conflict(fieldidentity.CodeNoAppAccess, "the worker is not granted that application")
		}
	}
	return fieldidentity.CreateSignInCodeResult{CodeID: "fwi_1", WorkerID: cmd.ID, AppCode: cmd.AppCode, ExpiresAt: time.Now().UTC().Add(fieldidentity.SignInCodeTTL)}, err
}

func (f *fakeField) LinkWorker(_ context.Context, cmd fieldidentity.LinkWorkerCommand) error {
	return f.record("link " + cmd.ID + " " + cmd.PrincipalID)
}

func (f *fakeField) UnlinkWorker(_ context.Context, r fieldidentity.WorkerRef) error {
	return f.record("unlink " + r.ID)
}

func (f *fakeField) GrantWorkerApp(_ context.Context, cmd fieldidentity.WorkerAppCommand) error {
	return f.record("grant " + cmd.ID + " " + cmd.AppCode)
}

func (f *fakeField) RevokeWorkerApp(_ context.Context, cmd fieldidentity.WorkerAppCommand) error {
	return f.record("revoke " + cmd.ID + " " + cmd.AppCode)
}

func (f *fakeField) SuspendWorker(_ context.Context, r fieldidentity.WorkerRef) error {
	return f.record("suspend " + r.ID)
}

func (f *fakeField) ActivateWorker(_ context.Context, r fieldidentity.WorkerRef) error {
	return f.record("activate " + r.ID)
}

func (f *fakeField) UnlockWorker(_ context.Context, r fieldidentity.WorkerRef) error {
	return f.record("unlock " + r.ID)
}

func (f *fakeField) DeleteWorker(_ context.Context, r fieldidentity.WorkerRef) error {
	return f.record("delete " + r.ID)
}

func (f *fakeField) ResetWorkerPin(_ context.Context, cmd fieldidentity.ResetPinCommand) (fieldidentity.ResetPinResult, error) {
	err := f.record("reset-pin " + cmd.ID)
	return fieldidentity.ResetPinResult{WorkerID: cmd.ID, TemporaryPIN: tempPin, PinChangeRequired: true}, err
}

func (f *fakeField) CreateEnrolmentCode(_ context.Context, cmd fieldidentity.CreateEnrolmentCodeCommand) (fieldidentity.CreateEnrolmentCodeResult, error) {
	err := f.record("enrol " + cmd.ClientID + " " + cmd.Mode + " " + strings.Join(cmd.AppCodes, ",") + " " + cmd.AssignedWorkerID)
	if err == nil && cmd.Mode == "SHARED" && cmd.AssignedWorkerID != "" {
		err = usecase.Validation("SHARED_DEVICE_ASSIGNED", "a SHARED device cannot be assigned to a worker")
	}
	return fieldidentity.CreateEnrolmentCodeResult{CodeID: "fec_1", ExpiresAt: time.Now().UTC().Add(15 * time.Minute)}, err
}

func (f *fakeField) SuspendDevice(_ context.Context, r fieldidentity.DeviceRef) error {
	return f.record("suspend-device " + r.ID)
}

func (f *fakeField) ActivateDevice(_ context.Context, r fieldidentity.DeviceRef) error {
	return f.record("activate-device " + r.ID)
}

func (f *fakeField) RevokeDevice(_ context.Context, r fieldidentity.DeviceRef) error {
	return f.record("revoke-device " + r.ID)
}

func (f *fakeField) PutPolicy(_ context.Context, cmd fieldidentity.PutPolicyCommand) error {
	if err := f.record("put-policy " + cmd.ClientID); err != nil {
		return err
	}
	p, err := cmd.Build() // the real range validation
	if err != nil {
		return err
	}
	p.IsDefault, p.UpdatedAt = false, fieldDay
	if f.policies == nil {
		f.policies = map[string]fieldidentity.Policy{}
	}
	f.policies[cmd.ClientID] = p
	return nil
}

func fieldWorker(id, client, code, name string, apps ...string) fieldidentity.Identity {
	hash := "x"
	w := fieldidentity.Identity{
		ID: id, ClientID: client, WorkerCode: code, Name: name, Status: fieldidentity.StatusActive, PINHash: &hash,
		CreatedAt: fieldDay, UpdatedAt: fieldDay,
	}
	for _, a := range apps {
		w.Apps = append(w.Apps, fieldidentity.AppGrant{AppID: a, Source: fieldidentity.SourceAdmin, GrantedAt: fieldDay})
	}
	return w
}

func fieldFixture() *fakeField {
	until := time.Now().UTC().Add(10 * time.Minute)
	ann := fieldWorker("fwk_ann", "clt_acme", "ANN", "Ann <Picker>", "app_wms")
	ann.LockedUntil = &until
	bob := fieldWorker("fwk_bob", "clt_acme", "BOB", "Bob", "app_wms", "app_tms")
	bob.HardLocked, bob.Status = true, fieldidentity.StatusDisabled
	gus := fieldWorker("fwk_gus", "clt_globex", "GUS", "Gus", "app_wms")
	seen := fieldDay.Add(time.Hour)
	worker := "fwk_ann"
	return &fakeField{
		workers: []fieldidentity.Identity{ann, bob, gus},
		devices: []fieldidentity.Device{
			{
				ID: "fdv_1", ClientID: "clt_acme", Mode: fieldidentity.ModePersonal, Label: "Ann's phone", Platform: "android", Model: "Pixel", JKT: "jkt1",
				AttestationLevel: "STRONGBOX", AssignedIdentityID: &worker, Status: fieldidentity.DeviceActive, EnrolledAt: fieldDay, LastSeenAt: &seen, AppIDs: []string{"app_wms"},
			},
			{
				ID: "fdv_2", ClientID: "clt_acme", Mode: fieldidentity.ModeShared, Label: "Dock scanner", JKT: "jkt2",
				AttestationLevel: "SOFTWARE", Status: fieldidentity.DeviceSuspended, EnrolledAt: fieldDay, AppIDs: []string{"app_wms", "app_tms"},
			},
			{
				ID: "fdv_3", ClientID: "clt_globex", Mode: fieldidentity.ModeShared, Label: "Globex tablet", JKT: "jkt3",
				AttestationLevel: "NONE", Status: fieldidentity.DeviceActive, EnrolledAt: fieldDay, AppIDs: []string{"app_wms"},
			},
		},
		apps: []application.Application{
			{ID: ids.ApplicationID("app_wms"), Code: "wms", Name: "Warehouse", Active: true},
			{ID: ids.ApplicationID("app_tms"), Code: "tms", Name: "Transport", Active: true},
			{ID: ids.ApplicationID("app_old"), Code: "old", Name: "Retired", Active: false},
		},
		sessions: map[string]int{"fdv_1": 3},
		signIns: []fieldidentity.LoginAttempt{
			{ID: "a1", IdentityID: "fwk_ann", DeviceID: "fdv_1", ApplicationID: "app_wms", Outcome: fieldidentity.OutcomeSuccess, AttemptedAt: fieldDay.Add(2 * time.Hour)},
			{ID: "a2", IdentityID: "fwk_ann", DeviceID: "fdv_1", ApplicationID: "app_wms", Outcome: fieldidentity.OutcomeFailure, FailureReason: "BAD_PIN", AttemptedAt: fieldDay.Add(time.Hour)},
		},
	}
}

// fieldCaller is a client-scoped user (every application) with the given
// permissions.
func fieldCaller(clients []string, perms ...string) *auth.AuthContext {
	return &auth.AuthContext{
		PrincipalID: "prn_fc", Email: "fc@acme.test", Scope: auth.ScopeClient, Clients: clients, AllApplications: true,
		Roles: []string{"r"}, Permissions: perms, PrincipalType: auth.PrincipalTypeUser,
	}
}

// fieldAnchor is the super-admin with every application.
func fieldAnchor() *auth.AuthContext {
	ac := anchorAdmin()
	ac.AllApplications = true
	return ac
}

// nativePost is a form post without Datastar: a refusal is its HTTP status
// (a Datastar action answers 200 with an error flash).
func nativePost(target string, kv ...string) req {
	q := postForm(target, kv...)
	q.datastar = false
	return q
}

func serveField(t *testing.T, f *fakeField, ac *auth.AuthContext, q req) *httptest.ResponseRecorder {
	t.Helper()
	sh := shellForTest()
	clients := portalClients()
	return serveMounted(t, ac, q,
		&FieldWorkers{Store: f, Ops: f, Clients: clients, Shell: sh},
		&FieldDevices{Store: f, Ops: f, Clients: clients, BaseURL: "https://fc.example/", Shell: sh},
		&FieldPolicy{Store: f, Ops: f, Clients: clients, Shell: sh})
}

func fieldBody(t *testing.T, f *fakeField, ac *auth.AuthContext, q req) string {
	t.Helper()
	return serveField(t, f, ac, q).Body.String()
}

// ── workers ────────────────────────────────────────────────────────────

func TestFieldWorkersList(t *testing.T) {
	f := fieldFixture()
	body := fieldBody(t, f, fieldCaller([]string{"clt_acme"}, permFieldWorkerView, permFieldWorkerManage), req{target: "/ui/field-workers?client=clt_acme"})
	mustContain(t, body,
		`<title>Field Workers · FlowCatalyst</title>`,
		`href="/ui/field-workers" class="fc-nav-item" title="Field Workers" aria-current="page"`,
		`<h1 class="fc-page-title">Field Workers</h1>`,
		"ANN", "Ann &lt;Picker&gt;", "BOB",
		"Soft locked", "Hard locked", "Suspended", "Warehouse", "Transport",
		`until <time datetime="`,
		"Showing 1 to 2 of 2 field workers", `id="field-workers-pager"`,
		`<dialog id="fw-suspend"`, `<dialog id="fw-reset-pin"`, `<dialog id="fw-delete"`, "New Field Worker",
	)
	mustNotContain(t, body, "GUS", "Ann <Picker>")
	// Row menus follow the state: Ann is locked and active, Bob is suspended.
	mustContain(t, menuOfPortal(t, body, "fw-fwk_ann-actions"), "Suspend…", "Unlock…", "Reset PIN…", "Delete…", `data-action="/ui/field-workers/fwk_ann/reset-pin"`)
	mustNotContain(t, menuOfPortal(t, body, "fw-fwk_ann-actions"), "Activate…")
	mustContain(t, menuOfPortal(t, body, "fw-fwk_bob-actions"), "Activate…", "Unlock…")
	mustNotContain(t, menuOfPortal(t, body, "fw-fwk_bob-actions"), "Suspend…")

	// Filters: status, app, search prefix.
	body = fieldBody(t, f, fieldAnchor(), req{target: "/ui/field-workers?client=clt_acme&status=DISABLED"})
	mustContain(t, body, "BOB")
	mustNotContain(t, body, "ANN</code>")
	body = fieldBody(t, f, fieldAnchor(), req{target: "/ui/field-workers?client=clt_acme&app=tms"})
	mustContain(t, body, "BOB")
	mustNotContain(t, body, "ANN</code>")
	body = fieldBody(t, f, fieldAnchor(), req{target: "/ui/field-workers?client=clt_acme&q=an"})
	mustContain(t, body, "ANN")
	mustNotContain(t, body, "BOB</code>")
	body = fieldBody(t, f, fieldAnchor(), req{target: "/ui/field-workers?client=clt_acme&q=zzz"})
	mustContain(t, body, "No field workers match.")
	body = fieldBody(t, f, fieldAnchor(), req{target: "/ui/field-workers"})
	mustContain(t, body, "Select a client to view its field workers.")
	// Live answer patches the table and the URL.
	rec := serveField(t, f, fieldAnchor(), req{target: "/ui/field-workers?client=clt_acme&status=ACTIVE", datastar: true})
	mustContain(t, rec.Body.String(), `id="field-workers-region"`, "datastar-patch-elements", "status=ACTIVE")
}

func TestFieldWorkerDrawerAndSignIns(t *testing.T) {
	f := fieldFixture()
	ac := fieldCaller([]string{"clt_acme"}, permFieldWorkerManage)
	rec := serveField(t, f, ac, req{target: "/ui/field-workers/fwk_ann", datastar: true})
	body := rec.Body.String()
	// The application line shows the name and the code once each.
	if n := strings.Count(body, "<code class=\"text-xs\">wms</code>"); n != 1 {
		t.Errorf("application code shown %d times in the drawer", n)
	}
	mustNotContain(t, body, "wms · granted")
	mustContain(t, body, "granted ", "until ")
	mustContain(t, body, "Sign-in history", "BAD_PIN", "Success", "Failure", "Applications", "Grant", `id="fw-history"`)
	// The history pages.
	f.signIns = nil
	for n := range 12 {
		f.signIns = append(f.signIns, fieldidentity.LoginAttempt{ID: "s" + string(rune('a'+n)), IdentityID: "fwk_ann", Outcome: fieldidentity.OutcomeSuccess, AttemptedAt: fieldDay.Add(time.Duration(n) * time.Minute)})
	}
	body = fieldBody(t, f, ac, req{target: "/ui/field-workers/fwk_ann/sign-ins?page=1", datastar: true})
	mustContain(t, body, `id="fw-history"`, "Page 2 of 2", "12 attempts", "Newer")
	mustNotContain(t, body, "Older")
	// A native visit renders the list with the drawer open.
	body = fieldBody(t, f, ac, req{target: "/ui/field-workers/fwk_ann"})
	mustContain(t, body, `id="fw-drawer"`, "Sign-in history", `<h1 class="fc-page-title">Field Workers</h1>`)
}

func TestFieldWorkerPermissions(t *testing.T) {
	f := fieldFixture()
	if rec := serveField(t, f, fieldCaller([]string{"clt_acme"}, "platform:messaging:event:view"), req{target: "/ui/field-workers?client=clt_acme"}); rec.Code != http.StatusForbidden {
		t.Errorf("no field permission: status %d", rec.Code)
	}
	// A viewer sees the workers but no way to change them.
	viewer := fieldBody(t, f, fieldCaller([]string{"clt_acme"}, permFieldWorkerView), req{target: "/ui/field-workers?client=clt_acme"})
	mustContain(t, viewer, "ANN", "View details")
	mustNotContain(t, viewer, `<dialog id="fw-`, "New Field Worker", "Reset PIN…", "Suspend…", "Delete…", "Unlock…", `aria-label="Remove `)
	drawer := fieldBody(t, f, fieldCaller([]string{"clt_acme"}, permFieldWorkerView), req{target: "/ui/field-workers/fwk_ann", datastar: true})
	mustContain(t, drawer, "Sign-in history")
	mustNotContain(t, drawer, "Reset PIN", "Delete", "fc-danger-item", "Grant")
	// A manager sees them.
	manager := fieldBody(t, f, fieldCaller([]string{"clt_acme"}, permFieldWorkerManage), req{target: "/ui/field-workers?client=clt_acme"})
	mustContain(t, manager, `<dialog id="fw-delete"`, "Reset PIN…", "New Field Worker")
}

func TestFieldWorkerForgedPostsAreRefused(t *testing.T) {
	f := fieldFixture()
	viewer := fieldCaller([]string{"clt_acme"}, permFieldWorkerView)
	// A Datastar post is refused with an error flash and runs nothing.
	mustContain(t, fieldBody(t, f, viewer, postForm("/ui/field-workers/fwk_ann/reset-pin")), "fc-flash-error", "platform:iam:field-worker:manage")
	for _, target := range []string{
		"/ui/field-workers/fwk_ann/suspend", "/ui/field-workers/fwk_bob/activate", "/ui/field-workers/fwk_ann/unlock",
		"/ui/field-workers/fwk_ann/reset-pin", "/ui/field-workers/fwk_ann/delete", "/ui/field-workers/fwk_ann/apps/wms/revoke",
	} {
		rec := serveField(t, f, viewer, nativePost(target))
		if rec.Code != http.StatusForbidden {
			t.Errorf("viewer POST %s: status %d, want 403", target, rec.Code)
		}
		mustNotContain(t, rec.Body.String(), tempPin)
	}
	for _, q := range []req{
		nativePost("/ui/field-workers/new", "client", "clt_acme", "workerCode", "NEW", "name", "New"),
		nativePost("/ui/field-workers/fwk_ann/grant", "app", "tms"),
	} {
		if rec := serveField(t, f, viewer, q); rec.Code != http.StatusForbidden {
			t.Errorf("viewer POST %s: status %d, want 403", q.target, rec.Code)
		}
	}
	if len(f.calls) != 0 {
		t.Errorf("a refused post ran %v", f.calls)
	}
}

func TestFieldWorkersAreClientBoxed(t *testing.T) {
	f := fieldFixture()
	acme := fieldCaller([]string{"clt_acme"}, permFieldWorkerView, permFieldWorkerManage)
	// The other client's list is refused; its worker is not found, not forbidden.
	if rec := serveField(t, f, acme, req{target: "/ui/field-workers?client=clt_globex"}); rec.Code != http.StatusForbidden {
		t.Errorf("another client's list: status %d", rec.Code)
	}
	if rec := serveField(t, f, acme, req{target: "/ui/field-workers/fwk_gus"}); rec.Code != http.StatusNotFound {
		t.Errorf("another client's worker drawer: status %d", rec.Code)
	}
	if rec := serveField(t, f, acme, req{target: "/ui/field-workers/fwk_gus/sign-ins"}); rec.Code != http.StatusNotFound {
		t.Errorf("another client's sign-ins: status %d", rec.Code)
	}
	for _, target := range []string{
		"/ui/field-workers/fwk_gus/suspend", "/ui/field-workers/fwk_gus/reset-pin", "/ui/field-workers/fwk_gus/delete",
		"/ui/field-workers/fwk_gus/unlock", "/ui/field-workers/fwk_gus/activate", "/ui/field-workers/fwk_gus/apps/wms/revoke",
	} {
		rec := serveField(t, f, acme, nativePost(target))
		if rec.Code != http.StatusNotFound {
			t.Errorf("POST %s on another client's worker: status %d, want 404", target, rec.Code)
		}
		mustNotContain(t, rec.Body.String(), tempPin)
	}
	if rec := serveField(t, f, acme, nativePost("/ui/field-workers/new", "client", "clt_globex", "workerCode", "X", "name", "X")); rec.Code != http.StatusForbidden {
		t.Errorf("create in another client: status %d", rec.Code)
	}
	if rec := serveField(t, f, acme, nativePost("/ui/field-workers/fwk_gus/grant", "app", "tms")); rec.Code != http.StatusNotFound {
		t.Errorf("grant on another client's worker: status %d", rec.Code)
	}
	if len(f.calls) != 0 {
		t.Errorf("a boxed post ran %v", f.calls)
	}
	// A client user lands on their own client's workers without choosing, and
	// never sees the other client's.
	own := fieldBody(t, f, acme, req{target: "/ui/field-workers"})
	mustContain(t, own, "ANN")
	mustNotContain(t, own, "GUS")
}

func TestFieldWorkerActionsRunTheOperation(t *testing.T) {
	f := fieldFixture()
	ac := fieldCaller([]string{"clt_acme"}, permFieldWorkerManage)
	for target, want := range map[string]string{
		"/ui/field-workers/fwk_ann/suspend":         "suspend fwk_ann",
		"/ui/field-workers/fwk_bob/activate":        "activate fwk_bob",
		"/ui/field-workers/fwk_bob/unlock":          "unlock fwk_bob",
		"/ui/field-workers/fwk_ann/delete":          "delete fwk_ann",
		"/ui/field-workers/fwk_bob/apps/tms/revoke": "revoke fwk_bob tms",
	} {
		f.calls = nil
		rec := serveField(t, f, ac, postForm(target, "drawer", "fwk_ann"))
		if rec.Code != http.StatusOK {
			t.Errorf("POST %s: status %d", target, rec.Code)
		}
		if !slices.Equal(f.calls, []string{want}) {
			t.Errorf("POST %s ran %v, want %q", target, f.calls, want)
		}
		mustContain(t, rec.Body.String(), `id="field-workers-region"`, "datastar-patch")
	}
	// A native post redirects with a flash key and carries nothing else.
	r := req{method: http.MethodPost, target: "/ui/field-workers/fwk_ann/suspend", form: map[string][]string{}}
	rec := serveField(t, f, ac, r)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/ui/field-workers?client=clt_acme" {
		t.Errorf("native post: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	// An action is POST only: a GET changes nothing.
	f.calls = nil
	for _, target := range []string{"/ui/field-workers/fwk_ann/suspend", "/ui/field-workers/fwk_ann/reset-pin", "/ui/field-workers/fwk_ann/delete"} {
		if rec := serveField(t, f, ac, req{target: target}); rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("GET %s: status %d, want 405", target, rec.Code)
		}
	}
	if len(f.calls) != 0 {
		t.Errorf("a GET ran %v", f.calls)
	}
}

func TestFieldWorkerCreateAndGrant(t *testing.T) {
	f := fieldFixture()
	ac := fieldCaller([]string{"clt_acme"}, permFieldWorkerManage)
	form := fieldBody(t, f, ac, req{target: "/ui/field-workers/new?client=clt_acme", datastar: true})
	mustContain(t, form, `id="fw-form"`, "Worker code", `name="apps" value="wms"`, `name="apps" value="tms"`, `name="client" value="clt_acme"`)
	mustNotContain(t, form, `value="old"`) // inactive applications are not offered
	rec := serveField(t, f, ac, postForm("/ui/field-workers/new", "client", "clt_acme", "workerCode", "zed", "name", "Zed", "apps", "wms", "apps", "tms"))
	if !slices.Equal(f.calls, []string{"ensure clt_acme zed wms,tms"}) {
		t.Errorf("create ran %v", f.calls)
	}
	mustContain(t, rec.Body.String(), "ZED created", "datastar-patch")
	// A refusal lands on the field.
	f.calls = nil
	f.failWith = usecase.Validation("WORKER_CODE_INVALID", "workerCode is at most 64 characters")
	body := fieldBody(t, f, ac, postForm("/ui/field-workers/new", "client", "clt_acme", "workerCode", "zed", "name", "Zed"))
	mustContain(t, body, "WorkerCode is at most 64 characters", `id="fw-form-worker-code-err"`)
	// Grant.
	f.calls = nil
	body = fieldBody(t, f, ac, req{target: "/ui/field-workers/fwk_ann/grant", datastar: true})
	mustContain(t, body, `id="fw-grant-form"`, `value="tms"`)
	mustNotContain(t, body, `value="wms"`) // already granted
	fieldBody(t, f, ac, postForm("/ui/field-workers/fwk_ann/grant", "app", "tms", "drawer", "fwk_ann"))
	if !slices.Equal(f.calls, []string{"grant fwk_ann tms"}) {
		t.Errorf("grant ran %v", f.calls)
	}
}

// The temporary PIN is shown once: in the reset response, never in anything
// the page can fetch again.
func TestFieldWorkerPinIsShownOnce(t *testing.T) {
	f := fieldFixture()
	ac := fieldCaller([]string{"clt_acme"}, permFieldWorkerView, permFieldWorkerManage)

	// Datastar reset: the dialog carries the PIN and the warning, uncached.
	rec := serveField(t, f, ac, postForm("/ui/field-workers/fwk_ann/reset-pin", "drawer", "fwk_ann"))
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !slices.Equal(f.calls, []string{"reset-pin fwk_ann"}) {
		t.Fatalf("reset: status %d, calls %v", rec.Code, f.calls)
	}
	mustContain(t, body, tempPin, "will not be shown again", `id="fw-pin-reveal"`, `id="fw-pin-value"`, "Copy temporary PIN", "d.remove()")
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control %q, want no-store", rec.Header().Get("Cache-Control"))
	}
	if strings.Count(body, tempPin) != 1 {
		t.Errorf("the PIN appears %d times in the reset answer, want once", strings.Count(body, tempPin))
	}
	// The flash and the refreshed drawer in the same answer do not repeat it,
	// and nothing is in a header or a cookie.
	for k, vs := range rec.Header() {
		for _, v := range vs {
			if strings.Contains(v, tempPin) {
				t.Errorf("header %s carries the PIN", k)
			}
		}
	}
	mustContain(t, body, "PIN reset for ANN")

	// Nothing the page fetches afterwards has it.
	for _, q := range []req{
		{target: "/ui/field-workers?client=clt_acme"},
		{target: "/ui/field-workers?client=clt_acme", datastar: true},
		{target: "/ui/field-workers/fwk_ann"},
		{target: "/ui/field-workers/fwk_ann", datastar: true},
		{target: "/ui/field-workers/fwk_ann/sign-ins", datastar: true},
		{target: "/ui/field-workers/fwk_ann/grant", datastar: true},
	} {
		got := serveField(t, f, ac, q)
		if strings.Contains(got.Body.String(), tempPin) {
			t.Errorf("GET %s re-serves the PIN", q.target)
		}
		for _, c := range got.Result().Cookies() {
			if strings.Contains(c.Value, tempPin) {
				t.Errorf("GET %s sets a cookie with the PIN", q.target)
			}
		}
	}

	// A native post (no JavaScript) gets the page with the dialog, uncached,
	// and no redirect that could carry it.
	native := serveField(t, f, ac, req{method: http.MethodPost, target: "/ui/field-workers/fwk_ann/reset-pin", form: map[string][]string{}})
	if native.Code != http.StatusOK || native.Header().Get("Location") != "" || native.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("native reset: status %d Location %q Cache-Control %q", native.Code, native.Header().Get("Location"), native.Header().Get("Cache-Control"))
	}
	mustContain(t, native.Body.String(), tempPin, "will not be shown again", `<h1 class="fc-page-title">Field Workers</h1>`)
	if strings.Contains(native.Header().Get("Set-Cookie"), tempPin) {
		t.Error("a cookie carries the PIN")
	}
	// A refused reset shows no PIN.
	f.failWith = usecase.Internal("PIN_GENERATE", "boom", nil)
	failed := serveField(t, f, ac, postForm("/ui/field-workers/fwk_ann/reset-pin"))
	mustNotContain(t, failed.Body.String(), tempPin)
}

// ── devices ────────────────────────────────────────────────────────────

func TestFieldDevicesList(t *testing.T) {
	f := fieldFixture()
	body := fieldBody(t, f, fieldCaller([]string{"clt_acme"}, permFieldDeviceView, permFieldDeviceManage), req{target: "/ui/field-devices?client=clt_acme"})
	mustContain(t, body,
		`<title>Field Devices · FlowCatalyst</title>`,
		`href="/ui/field-devices" class="fc-nav-item" title="Field Devices" aria-current="page"`,
		"Ann&#39;s phone", "Dock scanner", "Personal", "Shared", "STRONGBOX", "SOFTWARE", "Active", "Suspended",
		"<code class=\"text-xs\">ANN</code>", "Warehouse, Transport",
		`<dialog id="fd-revoke"`, "Enrol Device", "Showing 1 to 2 of 2 field devices",
	)
	mustNotContain(t, body, "Globex tablet")
	mustContain(t, menuOfPortal(t, body, "fd-fdv_1-actions"), "Suspend…", "Revoke…")
	mustNotContain(t, menuOfPortal(t, body, "fd-fdv_1-actions"), "Activate…")
	mustContain(t, menuOfPortal(t, body, "fd-fdv_2-actions"), "Activate…", "Revoke…")
	// The strong confirmation for the terminal action.
	mustContain(t, body, "Revoke field device permanently", "PERMANENTLY", "cannot be undone", "Revoke permanently")
	// Filters.
	body = fieldBody(t, f, fieldAnchor(), req{target: "/ui/field-devices?client=clt_acme&mode=SHARED"})
	mustContain(t, body, "Dock scanner")
	mustNotContain(t, body, "Ann&#39;s phone")
	body = fieldBody(t, f, fieldAnchor(), req{target: "/ui/field-devices?client=clt_acme&status=ACTIVE"})
	mustContain(t, body, "Ann&#39;s phone")
	mustNotContain(t, body, "Dock scanner")
	// Drawer: details and the active session count.
	drawer := fieldBody(t, f, fieldCaller([]string{"clt_acme"}, permFieldDeviceManage), req{target: "/ui/field-devices/fdv_1", datastar: true})
	mustContain(t, drawer, "Active sessions", "jkt1", "Suspend", "Revoke")
}

func TestFieldDevicePermissionsAndBoxing(t *testing.T) {
	f := fieldFixture()
	viewer := fieldCaller([]string{"clt_acme"}, permFieldDeviceView)
	body := fieldBody(t, f, viewer, req{target: "/ui/field-devices?client=clt_acme"})
	mustContain(t, body, "Dock scanner", "View details")
	mustNotContain(t, body, `<dialog id="fd-`, "Enrol Device", "Revoke…", "Suspend…", "Activate…")
	drawer := fieldBody(t, f, viewer, req{target: "/ui/field-devices/fdv_1", datastar: true})
	mustNotContain(t, drawer, "fc-danger-item", "Revoke")
	for _, q := range []req{
		nativePost("/ui/field-devices/fdv_1/suspend"), nativePost("/ui/field-devices/fdv_2/activate"), nativePost("/ui/field-devices/fdv_1/revoke"),
		nativePost("/ui/field-devices/new", "client", "clt_acme", "mode", "SHARED", "apps", "wms"),
	} {
		if rec := serveField(t, f, viewer, q); rec.Code != http.StatusForbidden {
			t.Errorf("viewer POST %s: status %d, want 403", q.target, rec.Code)
		}
	}
	manager := fieldCaller([]string{"clt_acme"}, permFieldDeviceManage)
	if rec := serveField(t, f, manager, req{target: "/ui/field-devices?client=clt_globex"}); rec.Code != http.StatusForbidden {
		t.Errorf("another client's list: status %d", rec.Code)
	}
	if rec := serveField(t, f, manager, req{target: "/ui/field-devices/fdv_3"}); rec.Code != http.StatusNotFound {
		t.Errorf("another client's device: status %d", rec.Code)
	}
	for _, target := range []string{"/ui/field-devices/fdv_3/suspend", "/ui/field-devices/fdv_3/activate", "/ui/field-devices/fdv_3/revoke"} {
		if rec := serveField(t, f, manager, nativePost(target)); rec.Code != http.StatusNotFound {
			t.Errorf("POST %s on another client's device: status %d, want 404", target, rec.Code)
		}
	}
	if rec := serveField(t, f, manager, nativePost("/ui/field-devices/new", "client", "clt_globex", "mode", "SHARED", "apps", "wms")); rec.Code != http.StatusForbidden {
		t.Errorf("enrol in another client: status %d", rec.Code)
	}
	if len(f.calls) != 0 {
		t.Errorf("a refused post ran %v", f.calls)
	}
	// Actions run for the manager of the right client.
	serveField(t, f, manager, nativePost("/ui/field-devices/fdv_1/revoke", "drawer", "fdv_1"))
	serveField(t, f, manager, nativePost("/ui/field-devices/fdv_1/suspend"))
	serveField(t, f, manager, nativePost("/ui/field-devices/fdv_2/activate"))
	if !slices.Equal(f.calls, []string{"revoke-device fdv_1", "suspend-device fdv_1", "activate-device fdv_2"}) {
		t.Errorf("device actions ran %v", f.calls)
	}
	if rec := serveField(t, f, manager, req{target: "/ui/field-devices/fdv_1/revoke"}); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET revoke: status %d, want 405", rec.Code)
	}
}

func TestFieldDeviceEnrolShowsTheCodeOnce(t *testing.T) {
	f := fieldFixture()
	ac := fieldCaller([]string{"clt_acme"}, permFieldDeviceView, permFieldDeviceManage, permFieldWorkerView)
	form := fieldBody(t, f, ac, req{target: "/ui/field-devices/new?client=clt_acme", datastar: true})
	mustContain(t, form, `<div class="flex items-center gap-2"><span`, `<label class="`)
	mustContain(t, form, `id="fd-form"`, `value="PERSONAL"`, `value="SHARED"`, `name="apps" value="wms"`, "Assigned worker")
	rec := serveField(t, f, ac, postForm("/ui/field-devices/new", "client", "clt_acme", "mode", "PERSONAL", "apps", "wms", "worker", "ann", "label", "Dock"))
	body := rec.Body.String()
	if len(f.calls) != 1 || !strings.HasPrefix(f.calls[0], "enrol clt_acme PERSONAL wms fwk_ann") {
		t.Fatalf("enrol ran %v", f.calls)
	}
	mustContain(t, body, `id="fd-code-reveal"`, `id="fd-code-value"`, "will not be shown again", "data-expires=", "Expires in", "d.remove()", `aria-label="Device enrolment QR code"`, "<svg")
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control %q", rec.Header().Get("Cache-Control"))
	}
	code := strings.TrimSpace(between(body, `id="fd-code-value" class="fc-mono-chip text-2xl">`, `</code>`))
	if len(code) < 8 {
		t.Fatalf("no code in the answer: %q", code)
	}
	// The QR encodes the payload, not the code in clear: the code is in the
	// answer once, as text.
	if strings.Count(body, code) < 1 {
		t.Error("the code is missing")
	}
	// Nothing fetched afterwards has the code.
	for _, q := range []req{
		{target: "/ui/field-devices?client=clt_acme"},
		{target: "/ui/field-devices?client=clt_acme", datastar: true},
		{target: "/ui/field-devices/fdv_1"},
		{target: "/ui/field-devices/new?client=clt_acme", datastar: true},
	} {
		if strings.Contains(fieldBody(t, f, ac, q), code) {
			t.Errorf("GET %s re-serves the enrolment code", q.target)
		}
	}
	// A native post shows the page with the dialog, uncached, no redirect.
	native := serveField(t, f, ac, req{method: http.MethodPost, target: "/ui/field-devices/new", form: map[string][]string{"client": {"clt_acme"}, "mode": {"SHARED"}, "apps": {"wms"}}})
	if native.Code != http.StatusOK || native.Header().Get("Location") != "" || native.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("native enrol: status %d Location %q Cache-Control %q", native.Code, native.Header().Get("Location"), native.Header().Get("Cache-Control"))
	}
	mustContain(t, native.Body.String(), `id="fd-code-reveal"`, "<svg")
	// Refusals land on fields.
	body = fieldBody(t, f, ac, postForm("/ui/field-devices/new", "client", "clt_acme", "mode", "SHARED", "apps", "wms", "worker", "ann"))
	mustContain(t, body, "A SHARED device cannot be assigned to a worker", `id="fd-form-worker-err"`)
	body = fieldBody(t, f, ac, postForm("/ui/field-devices/new", "client", "clt_acme", "mode", "PERSONAL", "apps", "wms", "worker", "nobody"))
	mustContain(t, body, "No field worker with this code in Acme Corp.")
}

func between(s, a, b string) string {
	_, rest, ok := strings.Cut(s, a)
	if !ok {
		return ""
	}
	out, _, _ := strings.Cut(rest, b)
	return out
}

// ── policy ─────────────────────────────────────────────────────────────

func TestFieldPolicy(t *testing.T) {
	f := fieldFixture()
	manager := fieldCaller([]string{"clt_acme"}, permFieldDeviceView, permFieldDeviceManage)
	body := fieldBody(t, f, manager, req{target: "/ui/field-policy?client=clt_acme"})
	mustContain(t, body,
		`<title>Field Policy · FlowCatalyst</title>`,
		`href="/ui/field-policy" class="fc-nav-item" title="Field Policy" aria-current="page"`,
		"Default", "The platform defaults apply to Acme Corp",
		`name="pinMinLength"`, `value="6"`, `min="4"`, `max="12"`, `name="softLockSecs"`, `value="900"`, `min="30"`, `max="86400"`,
		`name="shiftMaxSecs"`, `min="3600"`, `name="pinReauthDays"`, `max="90"`, `name="requireAttestation"`, "Save policy",
	)
	// Refused values keep what was typed and name the field; nothing is saved.
	rec := serveField(t, f, manager, postForm("/ui/field-policy", "client", "clt_acme", "pinMinLength", "3", "softLockThreshold", "5", "softLockSecs", "900"))
	mustContain(t, rec.Body.String(), "Minimum PIN length must be between 4 and 12", `value="3"`)
	mustNotContain(t, rec.Body.String(), "pinMinLength must", "PinMinLength")
	if _, saved := f.policies["clt_acme"]; saved {
		t.Error("an invalid policy was saved")
	}
	rec = serveField(t, f, manager, postForm("/ui/field-policy", "client", "clt_acme", "softLockSecs", "abc"))
	mustContain(t, rec.Body.String(), "Enter a whole number.")
	// A good save marks the policy custom.
	rec = serveField(t, f, manager, postForm("/ui/field-policy", "client", "clt_acme", "pinMinLength", "8", "softLockSecs", "600", "requireAttestation", "1"))
	mustContain(t, rec.Body.String(), "Custom", "Field policy saved for Acme Corp", `value="8"`, `value="600"`)
	if p := f.policies["clt_acme"]; p.PINMinLength != 8 || p.SoftLockSecs != 600 || !p.RequireAttestation || p.SoftLockThreshold != 5 {
		t.Errorf("saved policy %+v", p)
	}
	body = fieldBody(t, f, manager, req{target: "/ui/field-policy?client=clt_acme"})
	mustContain(t, body, "Custom", `value="8"`, "last updated <time")
	// A viewer sees the values, no save; a forged post is refused.
	viewer := fieldCaller([]string{"clt_acme"}, permFieldDeviceView)
	body = fieldBody(t, f, viewer, req{target: "/ui/field-policy?client=clt_acme"})
	mustContain(t, body, `name="pinMinLength"`, "You can view this policy but not change it.")
	mustNotContain(t, body, "Save policy")
	f.calls = nil
	if rec := serveField(t, f, viewer, nativePost("/ui/field-policy", "client", "clt_acme", "pinMinLength", "9")); rec.Code != http.StatusForbidden {
		t.Errorf("viewer save: status %d", rec.Code)
	}
	// Client boxing.
	if rec := serveField(t, f, manager, req{target: "/ui/field-policy?client=clt_globex"}); rec.Code != http.StatusForbidden {
		t.Errorf("another client's policy: status %d", rec.Code)
	}
	if rec := serveField(t, f, manager, nativePost("/ui/field-policy", "client", "clt_globex", "pinMinLength", "9")); rec.Code != http.StatusForbidden {
		t.Errorf("save for another client: status %d", rec.Code)
	}
	if len(f.calls) != 0 {
		t.Errorf("a refused save ran %v", f.calls)
	}
}

// An application-confined service account sees only the workers and devices
// of its own applications and cannot change the client-wide policy.
func TestFieldApplicationConfinedCaller(t *testing.T) {
	f := fieldFixture()
	sa := fieldCaller([]string{"clt_acme"}, permFieldWorkerView, permFieldWorkerManage, permFieldDeviceView, permFieldDeviceManage)
	sa.AllApplications, sa.Applications = false, []string{"app_tms"}
	body := fieldBody(t, f, sa, req{target: "/ui/field-workers?client=clt_acme"})
	mustContain(t, body, "BOB")
	mustNotContain(t, body, "ANN</code>")
	// Ann holds only the warehouse app: she is not found.
	if rec := serveField(t, f, sa, req{target: "/ui/field-workers/fwk_ann"}); rec.Code != http.StatusNotFound {
		t.Errorf("worker outside the caller's applications: status %d", rec.Code)
	}
	if rec := serveField(t, f, sa, nativePost("/ui/field-workers/fwk_ann/suspend")); rec.Code != http.StatusNotFound {
		t.Errorf("suspend outside the caller's applications: status %d", rec.Code)
	}
	body = fieldBody(t, f, sa, req{target: "/ui/field-devices?client=clt_acme"})
	mustContain(t, body, "Dock scanner")
	mustNotContain(t, body, "Ann&#39;s phone")
	body = fieldBody(t, f, sa, req{target: "/ui/field-policy?client=clt_acme"})
	mustContain(t, body, "You can view this policy but not change it.")
	if rec := serveField(t, f, sa, nativePost("/ui/field-policy", "client", "clt_acme", "pinMinLength", "9")); rec.Code != http.StatusForbidden {
		t.Errorf("confined policy save: status %d", rec.Code)
	}
	if len(f.calls) != 0 {
		t.Errorf("a refused post ran %v", f.calls)
	}
}
