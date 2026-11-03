package pages

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/framework/go/ui/ds"
	"github.com/flowcatalyst/framework/go/ui/kit"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// FieldDevices is the field devices page (/ui/field-devices): a client's
// enrolled devices, the enrolment dialog (a single-use code with its QR code)
// and the suspend, activate and revoke actions of the admin API. Reads make
// the API's check (CanReadFieldDevices); every change CanManageFieldDevices
// for the device's client.
type FieldDevices struct {
	Store   FieldStore
	Ops     FieldCommands
	Clients ClientStore
	// BaseURL is the platform's public base URL, embedded in the enrolment QR
	// payload (fcfield:1:<baseUrl>:<code>), as the API's.
	BaseURL string
	Shell   *shell.Shell
}

const (
	fieldDevicesPath  = "/ui/field-devices"
	fieldDevicesForm  = "field-devices-list"
	fieldDevicesLive  = "@get('/ui/field-devices', {contentType: 'form'})"
	fieldDevicesTable = "field-devices-region"
	fieldModeFacet    = "mode"

	fdDrawerID   = "fd-drawer"
	fdDrawerSig  = "selFieldDevice"
	fdDialogSlot = "fd-dialog-slot"

	fdSuspendID  = "fd-suspend"
	fdActivateID = "fd-activate"
	fdRevokeID   = "fd-revoke"

	fdFormDialog = "fd-form"
	fdFormSignal = "fdForm"
)

var fieldDeviceList = &listSpec{
	path:        fieldDevicesPath,
	sortKeys:    []string{"enrolled"}, // the API orders newest first
	statusOK:    func(s string) bool { return s == "ACTIVE" || s == "SUSPENDED" || s == "REVOKED" },
	facets:      []string{fieldClientParam, fieldModeFacet},
	rows:        []int{25, 50, 100},
	defaultRows: 25,
}

var (
	fieldDeviceStatusOptions = []kit.Option{{Value: "ACTIVE", Label: "Active"}, {Value: "SUSPENDED", Label: "Suspended"}, {Value: "REVOKED", Label: "Revoked"}}
	fieldDeviceModeOptions   = []kit.Option{{Value: "PERSONAL", Label: "Personal"}, {Value: "SHARED", Label: "Shared"}}
)

// Mount registers the page's routes (POSTs behind parseForm).
func (h *FieldDevices) Mount(r chi.Router) {
	r.Get("/field-devices", h.list)
	r.Get("/field-devices/new", h.newForm)
	r.Get("/field-devices/{id}", h.detail)
	post := r.With(parseForm)
	post.Post("/field-devices/new", h.create)
	post.Post("/field-devices/{id}/suspend", h.suspend)
	post.Post("/field-devices/{id}/activate", h.activate)
	post.Post("/field-devices/{id}/revoke", h.revoke)
}

// ── view ───────────────────────────────────────────────────────────────

type fieldDeviceRow struct {
	ID, Label, Platform, Model, JKT string
	Mode, ModeSeverity              string
	ModeTag                         kit.TagSeverity
	Attestation                     string
	AttestTag                       kit.TagSeverity
	Status                          string
	StatusTag                       kit.TagSeverity
	RawStatus                       fieldidentity.DeviceStatus
	Worker                          string
	Apps                            []string
	Sessions                        int
	LastSeen, Enrolled              time.Time
	DetailHref, Base                string
}

func (r fieldDeviceRow) name() string {
	if r.Label != "" {
		return r.Label
	}
	return r.ID
}

type fieldDevicesView struct {
	Query         listQuery
	Client        string
	ClientName    string
	ClientOptions []kit.Option
	Rows          []fieldDeviceRow
	Total         int
	Page, Pages   int
	CanManage     bool
	Dialog        templ.Component
	Drawer        *fieldDeviceDetail
}

func attestationTag(level string) kit.TagSeverity {
	switch level {
	case "STRONGBOX", "TEE":
		return kit.TagSuccess
	case "SOFTWARE":
		return kit.TagWarn
	}
	return kit.TagDanger
}

func deviceStatusTag(s fieldidentity.DeviceStatus) (string, kit.TagSeverity) {
	switch s {
	case fieldidentity.DeviceSuspended:
		return "Suspended", kit.TagWarn
	case fieldidentity.DeviceRevoked:
		return "Revoked", kit.TagDanger
	case fieldidentity.DeviceActive:
	}
	return "Active", kit.TagSuccess
}

func fieldDeviceRowOf(d *fieldidentity.Device, idx fieldApps, workers map[string]string, sessions map[string]int) fieldDeviceRow {
	base := fieldDevicesPath + "/" + url.PathEscape(d.ID)
	row := fieldDeviceRow{
		ID: d.ID, Label: d.Label, Platform: d.Platform, Model: d.Model, JKT: d.JKT,
		Mode: "Personal", ModeTag: kit.TagInfo, Attestation: d.AttestationLevel, AttestTag: attestationTag(d.AttestationLevel),
		RawStatus: d.Status, Sessions: sessions[d.ID], Enrolled: d.EnrolledAt, DetailHref: base, Base: base,
	}
	if d.Mode == fieldidentity.ModeShared {
		row.Mode, row.ModeTag = "Shared", kit.TagSecondary
	}
	row.Status, row.StatusTag = deviceStatusTag(d.Status)
	if d.LastSeenAt != nil {
		row.LastSeen = *d.LastSeenAt
	}
	if d.AssignedIdentityID != nil {
		row.Worker = *d.AssignedIdentityID
		if code, ok := workers[*d.AssignedIdentityID]; ok {
			row.Worker = code
		}
	}
	for _, id := range d.AppIDs {
		_, name := idx.ref(id)
		row.Apps = append(row.Apps, name)
	}
	return row
}

// workerCodes names the workers the devices are assigned to.
func (h *FieldDevices) workerCodes(ctx context.Context, devs []fieldidentity.Device) map[string]string {
	out := map[string]string{}
	for i := range devs {
		id := devs[i].AssignedIdentityID
		if id == nil {
			continue
		}
		if _, done := out[*id]; done {
			continue
		}
		if w, err := h.Store.Worker(ctx, *id); err == nil && w != nil {
			out[*id] = w.WorkerCode
		} else {
			out[*id] = *id
		}
	}
	return out
}

func (h *FieldDevices) view(ctx context.Context, ac *auth.AuthContext, q listQuery) (fieldDevicesView, error) {
	opts, err := portalClientOptions(ctx, h.Clients, ac)
	if err != nil {
		return fieldDevicesView{}, err
	}
	clientID, opts := portalSelectedClient(ac, opts, first(q.facet(fieldClientParam)))
	mode := first(q.facet(fieldModeFacet))
	if mode != "PERSONAL" && mode != "SHARED" {
		mode = ""
	}
	q = q.withFacet(fieldClientParam, nil).withFacet(fieldModeFacet, nil)
	v := fieldDevicesView{Query: q, Client: clientID, ClientOptions: opts, ClientName: portalClientName(opts, clientID), Page: 1, Pages: 1}
	if clientID == "" {
		return v, nil
	}
	if err := auth.CanReadFieldDevices(ac, clientID); err != nil {
		return fieldDevicesView{}, err
	}
	v.CanManage = auth.CanManageFieldDevices(ac, clientID) == nil
	idx, err := loadFieldApps(ctx, h.Store)
	if err != nil {
		return fieldDevicesView{}, err
	}
	q = q.withFacet(fieldClientParam, []string{clientID})
	if mode != "" {
		q = q.withFacet(fieldModeFacet, []string{mode})
	}
	filter := fieldidentity.DeviceFilter{
		ClientID: clientID, Status: fieldidentity.DeviceStatus(q.Status), Mode: fieldidentity.Mode(mode), AppIDs: fieldCallerApps(ac),
	}
	filter.Offset, filter.Limit = (q.Page-1)*q.Rows, q.Rows
	rows, total, err := h.Store.Devices(ctx, filter)
	if err != nil {
		return fieldDevicesView{}, usecase.Internal("REPO", "search_field_devices failed", err)
	}
	if page, _, _, _ := q.pageBounds(int(total)); page != q.Page {
		q.Page = page
		filter.Offset = (page - 1) * q.Rows
		if rows, total, err = h.Store.Devices(ctx, filter); err != nil {
			return fieldDevicesView{}, usecase.Internal("REPO", "search_field_devices failed", err)
		}
	}
	ids := make([]string, len(rows))
	for n := range rows {
		ids[n] = rows[n].ID
	}
	sessions, err := h.Store.ActiveSessions(ctx, ids)
	if err != nil {
		return fieldDevicesView{}, usecase.Internal("REPO", "count_field_sessions failed", err)
	}
	workers := h.workerCodes(ctx, rows)
	v.Query, v.Total, v.Page = q, int(total), q.Page
	_, v.Pages, _, _ = q.pageBounds(v.Total)
	for n := range rows {
		v.Rows = append(v.Rows, fieldDeviceRowOf(&rows[n], idx, workers, sessions))
	}
	return v, nil
}

func (h *FieldDevices) renderPage(w http.ResponseWriter, r *http.Request, status int, v fieldDevicesView) {
	h.Shell.RenderStatus(w, r, status, shell.Page{Title: "Field Devices", Path: fieldDevicesPath}, fieldDevicesPage(v))
}

func (v fieldDevicesView) patchTable(sse *ds.SSE) error {
	return errors.Join(sse.Patch(fieldDevicesRegion(v)), sse.Patch(kit.Paginator(v.pager())))
}

func (v fieldDevicesView) url() string { return v.Query.URL() }

func (v fieldDevicesView) mode() string { return first(v.Query.facet(fieldModeFacet)) }

func (v fieldDevicesView) filterCount() int {
	n := 0
	if v.Query.Status != "" {
		n++
	}
	if v.mode() != "" {
		n++
	}
	return n
}

func (v fieldDevicesView) clearURL() string {
	return listQuery{spec: fieldDeviceList, Sort: v.Query.Sort, Dir: v.Query.Dir, Page: 1, Rows: v.Query.Rows}.
		withFacet(fieldClientParam, []string{v.Client}).URL()
}

func (v fieldDevicesView) confirm(id, title, message, label string, tone kit.ConfirmTone) kit.ConfirmDialogProps {
	return kit.ConfirmDialogProps{
		ID: id, Action: fieldDevicesPath, Title: title, Message: message, ConfirmLabel: label, Tone: tone,
		Hidden:    []kit.HiddenField{drawerField(fdDrawerSig)},
		FormAttrs: templ.Attributes{"data-on:submit": kit.SubmitConfirm},
	}
}

func (v fieldDevicesView) drawerSignals() string {
	if v.Drawer == nil {
		return "{" + fdDrawerSig + ": '', fieldCopied: ''}"
	}
	return "{" + fdDrawerSig + ": '" + kit.ID(v.Drawer.ID) + "', fieldCopied: ''}"
}

func deviceActions(base string, status fieldidentity.DeviceStatus) []fieldAction {
	var out []fieldAction
	switch status {
	case fieldidentity.DeviceActive:
		out = append(out, fieldAction{"ban", "Suspend…", "Block sign-in on this device and end its sessions. It can be activated again.", fdSuspendID, base + "/suspend", false})
	case fieldidentity.DeviceSuspended:
		out = append(out, fieldAction{"circle-check", "Activate…", "Let this device sign workers in again.", fdActivateID, base + "/activate", false})
	case fieldidentity.DeviceRevoked:
	}
	if status != fieldidentity.DeviceRevoked {
		out = append(out, fieldAction{"trash-2", "Revoke…", "Permanently remove this device and end its sessions. It must be enrolled again with a new code. Cannot be undone.", fdRevokeID, base + "/revoke", true})
	}
	return out
}

func (v fieldDevicesView) rowActions(row fieldDeviceRow) kit.RowActionsProps {
	p := kit.RowActionsProps{ID: "fd-" + row.ID + "-actions", RowLabel: row.name()}
	p.Menu = append(p.Menu, kit.RowAction{Icon: "eye", Label: "View details", Href: row.DetailHref, Attrs: templ.Attributes{"data-on:click__prevent": liveLink}})
	if !v.CanManage {
		return p
	}
	for _, a := range deviceActions(row.Base, row.RawStatus) {
		p.Menu = append(p.Menu, kit.RowAction{Icon: a.Icon, Label: a.Label, Destructive: a.Destructive, Attrs: confirmTrigger(a.Dialog, a.Action)})
	}
	return p
}

func (h *FieldDevices) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	v, err := h.view(ctx, auth.FromContext(ctx), parseListQuery(r.URL.Query(), fieldDeviceList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if ds.IsDatastar(r) {
		sse := ds.NewSSE(w, r)
		if err := errors.Join(v.patchTable(sse), sse.Patch(kit.TableToolbarState(v.toolbar().State())), sse.ReplaceURL(v.url())); err != nil {
			slog.DebugContext(ctx, "ui: field devices patch not delivered", "err", err)
		}
		return
	}
	h.renderPage(w, r, http.StatusOK, v)
}

// ── loading ────────────────────────────────────────────────────────────

func fieldDeviceID(r *http.Request) (string, error) {
	id := chi.URLParam(r, "id")
	if !kit.ValidID(id) {
		return "", httperror.NotFound("FieldDevice", id)
	}
	return id, nil
}

// loadDevice loads a device for a by-id request with the API's checks (see
// FieldWorkers.loadWorker).
func (h *FieldDevices) loadDevice(ctx context.Context, ac *auth.AuthContext, id string, write bool) (*fieldidentity.Device, error) {
	gate := auth.CanReadFieldDevices
	if write {
		gate = auth.CanManageFieldDevices
	}
	d, err := h.Store.Device(ctx, id)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_field_device failed", err)
	}
	clientID := ""
	if d != nil {
		clientID = d.ClientID
	}
	if err := fieldGate(gate, ac, clientID, d != nil, "FieldDevice", id); err != nil {
		return nil, err
	}
	if ac.IsApplicationScoped() {
		reachable := false
		for _, a := range d.AppIDs {
			reachable = reachable || ac.CanAccessApplication(a)
		}
		if !reachable {
			return nil, httperror.NotFound("FieldDevice", id)
		}
	}
	return d, nil
}

// ── drawer ─────────────────────────────────────────────────────────────

type fieldDeviceDetail struct {
	fieldDeviceRow
	ClientID  string
	CanManage bool
}

func (d fieldDeviceDetail) drawerProps() kit.DrawerProps {
	sub := strings.TrimSpace(d.Platform + " " + d.Model)
	return kit.DrawerProps{ID: fdDrawerID, Signal: fdDrawerSig, Label: "Field device", Title: d.name(), Subtitle: sub, Extras: fieldWorkerStatusTag(d.Status, d.StatusTag)}
}

func (d fieldDeviceDetail) actions() []fieldAction {
	if !d.CanManage {
		return nil
	}
	return deviceActions(d.Base, d.RawStatus)
}

func (h *FieldDevices) detailFor(ctx context.Context, ac *auth.AuthContext, d *fieldidentity.Device) (fieldDeviceDetail, error) {
	idx, err := loadFieldApps(ctx, h.Store)
	if err != nil {
		return fieldDeviceDetail{}, err
	}
	sessions, err := h.Store.ActiveSessions(ctx, []string{d.ID})
	if err != nil {
		return fieldDeviceDetail{}, usecase.Internal("REPO", "count_field_sessions failed", err)
	}
	row := fieldDeviceRowOf(d, idx, h.workerCodes(ctx, []fieldidentity.Device{*d}), sessions)
	return fieldDeviceDetail{fieldDeviceRow: row, ClientID: d.ClientID, CanManage: auth.CanManageFieldDevices(ac, d.ClientID) == nil}, nil
}

func (h *FieldDevices) detail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := fieldDeviceID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	dev, err := h.loadDevice(ctx, ac, id, false)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	d, err := h.detailFor(ctx, ac, dev)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if ds.IsDatastar(r) {
		sse := ds.NewSSE(w, r)
		if err := errors.Join(
			sse.Patch(fieldDeviceDrawerContent(d), ds.Inner("#"+fdDrawerID)),
			sse.PatchSignals(map[string]any{fdDrawerSig: d.ID}),
			sse.Focus(fdDrawerID+"-title"),
		); err != nil {
			slog.DebugContext(ctx, "ui: field device drawer patch not delivered", "err", err)
		}
		return
	}
	v, err := h.view(ctx, ac, parseListQuery(nil, fieldDeviceList).withFacet(fieldClientParam, []string{dev.ClientID}))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	v.Drawer = &d
	h.renderPage(w, r, http.StatusOK, v)
}

// ── actions ────────────────────────────────────────────────────────────

func (h *FieldDevices) suspend(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, flashFieldDeviceSuspended, func(ctx context.Context, d *fieldidentity.Device) error {
		return h.Ops.SuspendDevice(ctx, fieldidentity.DeviceRef{ID: d.ID, ClientID: d.ClientID})
	})
}

func (h *FieldDevices) activate(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, flashFieldDeviceActivated, func(ctx context.Context, d *fieldidentity.Device) error {
		return h.Ops.ActivateDevice(ctx, fieldidentity.DeviceRef{ID: d.ID, ClientID: d.ClientID})
	})
}

func (h *FieldDevices) revoke(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, flashFieldDeviceRevoked, func(ctx context.Context, d *fieldidentity.Device) error {
		return h.Ops.RevokeDevice(ctx, fieldidentity.DeviceRef{ID: d.ID, ClientID: d.ClientID})
	})
}

func (h *FieldDevices) runAction(w http.ResponseWriter, r *http.Request, flash string, run func(ctx context.Context, d *fieldidentity.Device) error) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := fieldDeviceID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	dev, err := h.loadDevice(ctx, ac, id, true)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if err := run(ctx, dev); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flash)
		http.Redirect(w, r, fieldDevicesPath+"?"+fieldClientParam+"="+url.QueryEscape(dev.ClientID), http.StatusSeeOther) //nolint:gosec // G710: the target is a page constant; the client id is query-escaped and was checked by the manage check
		return
	}
	sse := ds.NewSSE(w, r)
	q := refererList(r, fieldDeviceList).withFacet(fieldClientParam, []string{dev.ClientID})
	if v, err := h.view(ctx, ac, q); err != nil {
		h.Shell.FailSSE(sse, r, err)
	} else if err := v.patchTable(sse); err != nil {
		slog.DebugContext(ctx, "ui: field devices patch not delivered", "err", err)
	}
	if r.PostFormValue("drawer") == id {
		cur, err := h.loadDevice(ctx, ac, id, false)
		var d fieldDeviceDetail
		if err == nil {
			d, err = h.detailFor(ctx, ac, cur)
		}
		if err == nil {
			err = sse.Patch(fieldDeviceDrawerContent(d), ds.Inner("#"+fdDrawerID))
		}
		if err != nil {
			slog.DebugContext(ctx, "ui: field device drawer patch not delivered", "err", err)
		}
	}
	if err := portalFlash(sse, flash, dev.Label+" "+dev.ID); err != nil {
		slog.DebugContext(ctx, "ui: field device flash not delivered", "err", err)
	}
}

// ── enrol ──────────────────────────────────────────────────────────────

var fieldDeviceFieldCodes = map[string]string{
	"MODE_INVALID": "mode", "APP_CODES_REQUIRED": "apps", "UNKNOWN_APPLICATION": "apps", "APPLICATION_FORBIDDEN": "apps",
	"SHARED_DEVICE_ASSIGNED": "worker",
}

type fieldDeviceForm struct {
	Client, ClientName string
	Mode, Worker       string
	Label              string
	Apps               []string
	AppChoices         []kit.Option
	Errs               formErrors
	Open               bool
}

func (f fieldDeviceForm) id(key string) string { return fieldID(fdFormDialog, key) }

func (f fieldDeviceForm) dialog() kit.FormDialogProps {
	return kit.FormDialogProps{
		ID: fdFormDialog, Signal: fdFormSignal, Action: fieldDevicesPath + "/new", Open: f.Open, Error: f.Errs.Form,
		Title: "Enrol device", Description: "Create a single-use enrolment code for a device of " + f.ClientName + ". The code is shown once, with a QR code for the field app, and expires shortly.",
		SubmitLabel: "Create code", Hidden: []kit.HiddenField{{Name: fieldClientParam, Value: f.Client}},
	}
}

func (f fieldDeviceForm) keys() []string { return []string{"mode", "apps", "worker", "label"} }

func (f fieldDeviceForm) fields() []templ.Component {
	out := make([]templ.Component, 0, 4)
	for _, k := range f.keys() {
		out = append(out, fieldDeviceFormField(f, k))
	}
	return out
}

func (f fieldDeviceForm) firstInvalid() string {
	for _, k := range f.keys() {
		if f.Errs.Fields[k] != "" {
			return f.id(k)
		}
	}
	return ""
}

func (f fieldDeviceForm) describedBy(key string) string {
	if f.Errs.Fields[key] != "" {
		return f.id(key) + "-err"
	}
	return ""
}

func (f fieldDeviceForm) fieldProps(key, label, help string, required bool) kit.FieldProps {
	return kit.FieldProps{
		Label: label, For: f.id(key), Description: help, Required: required, Error: f.Errs.Fields[key],
		ErrorID: f.id(key) + "-err", Attrs: templ.Attributes{"id": f.id(key) + "-field"},
	}
}

func (h *FieldDevices) manageClient(r *http.Request, ac *auth.AuthContext) (string, error) {
	clientID := strings.TrimSpace(r.PostFormValue(fieldClientParam))
	if clientID == "" {
		clientID = strings.TrimSpace(r.URL.Query().Get(fieldClientParam))
	}
	if clientID == "" || !kit.ValidID(clientID) {
		return "", usecase.Validation("CLIENT_ID_REQUIRED", "clientId is required")
	}
	if err := auth.CanManageFieldDevices(ac, clientID); err != nil {
		return "", err
	}
	return clientID, nil
}

func (h *FieldDevices) clientLabel(ctx context.Context, ac *auth.AuthContext, clientID string) string {
	opts, err := portalClientOptions(ctx, h.Clients, ac)
	if err != nil {
		return clientID
	}
	return portalClientName(opts, clientID)
}

func (h *FieldDevices) choices(ctx context.Context, ac *auth.AuthContext) []kit.Option {
	idx, err := loadFieldApps(ctx, h.Store)
	if err != nil {
		return nil
	}
	return fieldAppChoices(idx.reachable(ac, true))
}

func (h *FieldDevices) newForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	clientID, err := h.manageClient(r, ac)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f := fieldDeviceForm{Client: clientID, ClientName: h.clientLabel(ctx, ac, clientID), Mode: "PERSONAL", AppChoices: h.choices(ctx, ac), Open: true}
	if ds.IsDatastar(r) {
		if err := patchDialogSlot(ds.NewSSE(w, r), fdDialogSlot, fieldDeviceFormDialog(f)); err != nil {
			slog.DebugContext(ctx, "ui: field device form patch not delivered", "err", err)
		}
		return
	}
	v, err := h.view(ctx, ac, parseListQuery(nil, fieldDeviceList).withFacet(fieldClientParam, []string{clientID}))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	v.Dialog = fieldDeviceFormDialog(f)
	h.renderPage(w, r, http.StatusOK, v)
}

func (h *FieldDevices) formFailed(w http.ResponseWriter, r *http.Request, f fieldDeviceForm) {
	if ds.IsDatastar(r) {
		patchFormFailure(ds.NewSSE(w, r), r, fdFormDialog, f.fields(), f.Errs.Form, f.firstInvalid())
		return
	}
	f.Open = true
	v, err := h.view(r.Context(), auth.FromContext(r.Context()), parseListQuery(nil, fieldDeviceList).withFacet(fieldClientParam, []string{f.Client}))
	if err != nil {
		v = fieldDevicesView{}
	}
	v.Dialog = fieldDeviceFormDialog(f)
	h.renderPage(w, r, cmpOr(f.Errs.Status, http.StatusUnprocessableEntity), v)
}

// fieldCodeReveal is what the dialog after creating a code shows, once.
type fieldCodeReveal struct {
	Code, Payload string
	ExpiresAt     time.Time
}

// create mirrors POST /api/field-devices/enrolment-codes. The code is in this
// response only: never in a URL, a redirect, a flash, a signal or a log; the
// response is no-store and "Done" removes the dialog.
func (h *FieldDevices) create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	f := fieldDeviceForm{
		Client: strings.TrimSpace(r.PostFormValue(fieldClientParam)), Mode: strings.TrimSpace(r.PostFormValue("mode")),
		Worker: strings.TrimSpace(r.PostFormValue("worker")), Label: strings.TrimSpace(r.PostFormValue("label")),
		Apps: formSet(r.PostForm["apps"]),
	}
	clientID, err := h.manageClient(r, ac)
	if err != nil {
		f.Errs.apply(r, err, nil)
		h.formFailed(w, r, f)
		return
	}
	f.Client, f.ClientName, f.AppChoices = clientID, h.clientLabel(ctx, ac, clientID), h.choices(ctx, ac)
	cmd := fieldidentity.CreateEnrolmentCodeCommand{ClientID: clientID, Mode: f.Mode, AppCodes: f.Apps, Label: f.Label}
	if f.Worker != "" {
		wk, err := h.Store.WorkerByCode(ctx, clientID, f.Worker)
		if err != nil {
			f.Errs.apply(r, usecase.Internal("REPO", "find_field_worker failed", err), nil)
			h.formFailed(w, r, f)
			return
		}
		if wk == nil {
			f.Errs.field("worker", "No field worker with this code in "+f.ClientName+".")
			h.formFailed(w, r, f)
			return
		}
		cmd.AssignedWorkerID = wk.ID
	}
	code, err := fieldidentity.NewEnrolmentCodeValue()
	if err != nil {
		f.Errs.apply(r, usecase.Internal("CODE_GENERATE", "could not generate an enrolment code", err), nil)
		h.formFailed(w, r, f)
		return
	}
	cmd.Code = code
	w = noStore{w}
	res, err := h.Ops.CreateEnrolmentCode(ctx, cmd)
	if err != nil {
		f.Errs.apply(r, err, fieldDeviceFieldCodes)
		h.formFailed(w, r, f)
		return
	}
	reveal := fieldCodeReveal{Code: code, Payload: "fcfield:1:" + strings.TrimRight(h.BaseURL, "/") + ":" + code, ExpiresAt: res.ExpiresAt}
	if !ds.IsDatastar(r) {
		v, err := h.view(ctx, ac, parseListQuery(nil, fieldDeviceList).withFacet(fieldClientParam, []string{clientID}))
		if err != nil {
			h.Shell.Fail(w, r, err)
			return
		}
		v.Dialog = fieldCodeDialog(reveal)
		h.renderPage(w, r, http.StatusOK, v)
		return
	}
	sse := ds.NewSSE(w, r)
	if err := patchDialogSlot(sse, fdDialogSlot, fieldCodeDialog(reveal)); err != nil {
		slog.DebugContext(ctx, "ui: field enrolment code not delivered", "err", err)
	}
	// The flash never carries the code.
	if err := portalFlash(sse, flashFieldCodeCreated, "Enrolment code created; it is shown once"); err != nil {
		slog.DebugContext(ctx, "ui: field device flash not delivered", "err", err)
	}
}

// fieldQR is the code's QR payload drawn as an inline SVG.
func fieldQR(payload string) templ.Component {
	c, err := qrSVGLabeled(payload, "Device enrolment QR code")
	if err != nil {
		return templ.Raw(`<p class="text-sm text-muted-foreground">The QR code could not be drawn; type the code in the app.</p>`)
	}
	return c
}

func fieldExpiryCountdown() string {
	return "const end = Date.parse(el.dataset.expires); const tick = () => { const s = Math.max(0, Math.round((end - Date.now()) / 1000)); el.textContent = s > 0 ? Math.floor(s / 60) + 'm ' + String(s % 60).padStart(2, '0') + 's' : 'expired'; }; tick(); const timer = setInterval(() => { if (!el.isConnected) { clearInterval(timer); return } tick() }, 1000)"
}
