package pages

import (
	"cmp"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/a-h/templ"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
	subops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
)

// The create and edit dialogs (the SPA's SubscriptionCreateDrawer and the
// detail drawer's inline edit).

// subScopeChange asks for the event types of the new scope. Not a form
// request: Datastar validates a form it sends ({contentType: 'form'}), and the
// half-filled form's required fields would stop the request.
const subScopeChange = "@get('/ui/subscriptions/event-type-options?clientScoped=' + (el.checked ? 'true' : ''))"

const (
	subFormDialog = "sub-form"
	subFormSignal = "subForm"
)

var subFieldCodes = map[string]string{
	"CODE_REQUIRED":        "code",
	"INVALID_CODE_FORMAT":  "code",
	"CODE_EXISTS":          "code",
	"NAME_REQUIRED":        "name",
	"INVALID_ENDPOINT":     "endpoint",
	"EVENT_TYPES_REQUIRED": "eventTypes",
	"INVALID_QUEUE":        "queue",
}

var subModeOptions = []kit.Option{
	{Value: string(common.DispatchImmediate), Label: "Immediate"},
	{Value: string(common.DispatchNextOnError), Label: "Next on Error"},
	{Value: string(common.DispatchBlockOnError), Label: "Block on Error"},
}

// subQueueOptions are the values the use case accepts for queue (the
// dispatch priority); the SPA offers a free-text field.
var subQueueOptions = []kit.Option{
	{Value: "DEFAULT", Label: "Default"},
	{Value: "HIGH_PRIORITY", Label: "High priority"},
}

type subForm struct {
	Edit     bool
	SubID    string
	Subtitle string

	Code, Name, Description, Endpoint string
	ConnectionID, Queue, PoolID, Mode string
	MaxAge, Timeout, Delay            string
	ClientScoped                      bool
	Client                            string
	EventTypeIDs                      []string

	Errs formErrors
	Open bool

	Pools, Connections, EventTypes  []kit.Option
	CanPickPool, CanPickConnection  bool
	CanPickEventType, CanPickClient bool
	ClientOptions                   []kit.Option
}

func (f subForm) id(key string) string         { return fieldID(subFormDialog, key) }
func (f subForm) fieldError(key string) string { return f.Errs.Fields[key] }

func (f subForm) keys() []string {
	if f.Edit {
		return []string{"name", "description", "endpoint", "connectionId", "queue", "maxAgeSeconds", "timeoutSeconds", "delaySeconds", "mode"}
	}
	return []string{"code", "name", "description", "eventTypes", "endpoint", "connectionId", "queue", "dispatchPoolId", "mode", "maxAgeSeconds", "timeoutSeconds", "delaySeconds", "scope"}
}

// focusID is the control a field's error focuses (the scope group's client).
func (f subForm) focusID(key string) string {
	switch key {
	case "scope":
		return f.id("client")
	case "eventTypes":
		return f.id(key) + "-wrap"
	}
	return f.id(key)
}

func (f subForm) firstInvalid() string {
	for _, k := range f.keys() {
		if f.fieldError(k) != "" {
			return f.focusID(k)
		}
	}
	return ""
}

func (f subForm) fields() []templ.Component {
	out := make([]templ.Component, 0, len(f.keys()))
	for _, k := range f.keys() {
		out = append(out, subField(f, k))
	}
	return out
}

func (f subForm) dialog() kit.FormDialogProps {
	p := kit.FormDialogProps{
		ID: subFormDialog, Signal: subFormSignal, Width: kit.WidthWide, Action: subsPath + "/new", Open: f.Open, Error: f.Errs.Form,
		Title: "Create Subscription", Description: "Configure a new event subscription for webhook delivery", SubmitLabel: "Create Subscription",
	}
	if f.Edit {
		p.Action = subURL(f.SubID) + "/edit"
		p.Title, p.Description, p.SubmitLabel, p.RequireChange = "Edit Subscription", f.Subtitle, "Save", true
		p.Hidden = []kit.HiddenField{drawerField(subDrawerSig)}
	}
	return p
}

func submittedSubForm(r *http.Request, edit bool) subForm {
	return subForm{
		Edit:         edit,
		Code:         strings.TrimSpace(r.PostFormValue("code")),
		Name:         r.PostFormValue("name"),
		Description:  r.PostFormValue("description"),
		Endpoint:     strings.TrimSpace(r.PostFormValue("endpoint")),
		ConnectionID: strings.TrimSpace(r.PostFormValue("connectionId")),
		Queue:        r.PostFormValue("queue"),
		PoolID:       strings.TrimSpace(r.PostFormValue("dispatchPoolId")),
		Mode:         r.PostFormValue("mode"),
		MaxAge:       r.PostFormValue("maxAgeSeconds"),
		Timeout:      r.PostFormValue("timeoutSeconds"),
		Delay:        r.PostFormValue("delaySeconds"),
		ClientScoped: r.PostFormValue("clientScoped") != "",
		Client:       strings.TrimSpace(r.PostFormValue("client")),
		EventTypeIDs: facetValues(r.PostForm["eventTypes"]),
	}
}

// numbers reads the three timing fields (blank = not sent).
func (f *subForm) numbers() (maxAge, timeout, delay *int32) {
	read := func(key, raw string) *int32 {
		n, ok := optionalInt32(raw)
		if !ok {
			f.Errs.field(key, notWholeNumber)
		}
		return n
	}
	return read("maxAgeSeconds", f.MaxAge), read("timeoutSeconds", f.Timeout), read("delaySeconds", f.Delay)
}

// ── create ─────────────────────────────────────────────────────────────

func (h *Subscriptions) createForm(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanCreateSubscriptions(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	// The SPA's defaults.
	f := subForm{Open: true, Queue: "DEFAULT", Mode: string(common.DispatchNextOnError), MaxAge: "86400", Timeout: "30", Delay: "0"}
	h.showForm(w, r, ac, f)
}

func (h *Subscriptions) create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	f := submittedSubForm(r, false)
	if err := auth.CanCreateSubscriptions(ac); err != nil {
		f.Errs.apply(r, err, nil)
		h.formFailed(w, r, ac, f)
		return
	}
	maxAge, timeout, delay := f.numbers()
	cmd := subops.CreateCommand{
		Code: f.Code, Name: f.Name, Endpoint: f.Endpoint, Description: optionalText(f.Description),
		ConnectionID: optionalText(f.ConnectionID), DispatchPoolID: optionalText(f.PoolID),
		Mode: f.Mode, Queue: &f.Queue, MaxAgeSeconds: maxAge, TimeoutSeconds: timeout, DelaySeconds: delay,
	}
	bindings, msg, err := h.bindings(r, ac, f.EventTypeIDs, f.ClientScoped)
	switch {
	case err != nil:
		f.Errs.apply(r, err, nil)
	case msg != "":
		f.Errs.field("eventTypes", msg)
	}
	cmd.EventTypes = bindings
	// The SPA sends a client only for a client-scoped subscription.
	if f.ClientScoped {
		id, msg, err := resolveClient(ctx, h.Clients, ac, f.Client)
		switch {
		case err != nil:
			f.Errs.apply(r, err, nil)
		case msg != "":
			f.Errs.field("scope", msg)
		}
		cmd.ClientID = id
	}
	if f.Errs.any() {
		h.formFailed(w, r, ac, f)
		return
	}
	newID, err := h.Ops.Create(ctx, cmd)
	if err != nil {
		f.Errs.apply(r, err, subFieldCodes)
		h.formFailed(w, r, ac, f)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashSubCreated)
		http.Redirect(w, r, subsPath, http.StatusSeeOther)
		return
	}
	sse := ds.NewSSE(w, r)
	if err := sse.CloseDialog(subFormDialog); err != nil {
		slog.DebugContext(ctx, "ui: subscription form close not delivered", "err", err)
	}
	h.afterChange(sse, r, ac, newID, false)
	if s, err := h.loadSubscription(ctx, ac, newID); err != nil {
		h.Shell.FailSSE(sse, r, err)
	} else if err := openSubDrawer(sse, toSubDetail(s, subPermsFor(ac))); err != nil {
		slog.DebugContext(ctx, "ui: subscription drawer patch not delivered", "err", err)
	}
	if err := flashKey(sse, flashSubCreated); err != nil {
		slog.DebugContext(ctx, "ui: subscription flash not delivered", "err", err)
	}
}

// bindings turns the chosen event type ids into the SPA's bindings: id,
// code and the CURRENT spec version, from the event types the form offered.
func (h *Subscriptions) bindings(r *http.Request, ac *auth.AuthContext, chosen []string, clientScoped bool) ([]subscription.EventTypeBinding, string, error) {
	if len(chosen) == 0 {
		return nil, "", nil
	}
	ets, err := h.eventTypeChoices(r, ac, clientScoped)
	if err != nil {
		return nil, "", err
	}
	out := make([]subscription.EventTypeBinding, 0, len(chosen))
	for _, id := range chosen {
		i := slices.IndexFunc(ets, func(et eventtype.EventType) bool { return et.ID == id })
		if i < 0 {
			return nil, "Choose event types from the list.", nil
		}
		etID, version := ets[i].ID, currentVersion(ets[i])
		out = append(out, subscription.EventTypeBinding{EventTypeID: &etID, EventTypeCode: ets[i].Code, SpecVersion: &version})
	}
	return out, "", nil
}

func currentVersion(et eventtype.EventType) string {
	for _, sv := range et.SpecVersions {
		if sv.Status == eventtype.SpecCurrent {
			return sv.Version
		}
	}
	return ""
}

// eventTypeChoices are the SPA's options: CURRENT event types with a
// CURRENT spec version whose client scoping matches the subscription's.
func (h *Subscriptions) eventTypeChoices(r *http.Request, ac *auth.AuthContext, clientScoped bool) ([]eventtype.EventType, error) {
	if h.EventTypes == nil || auth.CanReadEventTypes(ac) != nil {
		return nil, nil
	}
	all, err := visibleEventTypes(r.Context(), h.EventTypes, h.Apps, ac)
	if err != nil {
		return nil, err
	}
	out := all[:0:0]
	for _, et := range all {
		if et.Status == eventtype.StatusCurrent && currentVersion(et) != "" && et.ClientScoped == clientScoped {
			out = append(out, et)
		}
	}
	slices.SortStableFunc(out, func(a, b eventtype.EventType) int { return cmp.Compare(a.Name, b.Name) })
	return out, nil
}

// eventTypeOptions answers the Client Scoped switch: the event type list
// for the new scope, nothing chosen (the SPA clears the choice).
func (h *Subscriptions) eventTypeOptions(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanCreateSubscriptions(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f := subForm{ClientScoped: r.URL.Query().Get("clientScoped") != ""}
	if err := h.withEventTypes(r, ac, &f); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	sse := ds.NewSSE(w, r)
	p := f.eventTypesProps()
	if err := errors.Join(sse.Patch(kit.MultiSelectOptions(p)), sse.Patch(kit.MultiSelectSummary(p)), sse.Patch(subEventTypesHelp(f))); err != nil {
		slog.DebugContext(r.Context(), "ui: event type options patch not delivered", "err", err)
	}
}

// ── edit ───────────────────────────────────────────────────────────────

func (h *Subscriptions) editForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := firstError(auth.CanReadSubscriptions(ac), auth.CanUpdateSubscriptions(ac)); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := subID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	s, err := h.loadSubscription(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f := subForm{
		Edit: true, SubID: s.ID, Subtitle: s.Name + " (" + s.Code + ")", Open: true,
		Name: s.Name, Description: deref(s.Description), Endpoint: s.Endpoint, ConnectionID: deref(s.ConnectionID),
		Queue: deref(s.Queue), Mode: string(s.Mode),
		MaxAge: strconv.Itoa(int(s.MaxAgeSeconds)), Timeout: strconv.Itoa(int(s.TimeoutSeconds)), Delay: strconv.Itoa(int(s.DelaySeconds)),
	}
	h.showForm(w, r, ac, f)
}

func (h *Subscriptions) update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	f := submittedSubForm(r, true)
	if err := firstError(auth.CanReadSubscriptions(ac), auth.CanUpdateSubscriptions(ac)); err != nil {
		f.Errs.apply(r, err, nil)
		h.formFailed(w, r, ac, f)
		return
	}
	id, err := subID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	s, err := h.loadSubscription(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f.SubID, f.Subtitle = s.ID, s.Name+" ("+s.Code+")"
	maxAge, timeout, delay := f.numbers()
	if f.Errs.any() {
		h.formFailed(w, r, ac, f)
		return
	}
	// The SPA's PUT body: name, endpoint, queue and mode as edited;
	// description and the numbers only when set (`|| undefined`). The
	// connection is sent only when one is chosen (a blank one would store
	// an empty id; the use case cannot clear it).
	cmd := subops.UpdateCommand{
		ID: id, Name: &f.Name, Description: optionalText(f.Description), Endpoint: &f.Endpoint,
		ConnectionID: optionalText(f.ConnectionID), Queue: &f.Queue, Mode: &f.Mode,
		MaxAgeSeconds: positive(maxAge), TimeoutSeconds: positive(timeout), DelaySeconds: positive(delay),
	}
	if err := h.Ops.Update(ctx, cmd); err != nil {
		f.Errs.apply(r, err, subFieldCodes)
		h.formFailed(w, r, ac, f)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashSubUpdated)
		http.Redirect(w, r, subsPath, http.StatusSeeOther)
		return
	}
	sse := ds.NewSSE(w, r)
	if err := sse.CloseDialog(subFormDialog); err != nil {
		slog.DebugContext(ctx, "ui: subscription form close not delivered", "err", err)
	}
	h.afterChange(sse, r, ac, id, false)
	if err := flashKey(sse, flashSubUpdated); err != nil {
		slog.DebugContext(ctx, "ui: subscription flash not delivered", "err", err)
	}
}

// ── rendering ──────────────────────────────────────────────────────────

// withChoices fills the selects with what the SPA loads for them, each only
// when the user may read it (the SPA's call would fail otherwise).
func (h *Subscriptions) withChoices(r *http.Request, ac *auth.AuthContext, f *subForm) {
	ctx := r.Context()
	if f.CanPickConnection = h.Connections != nil && auth.CanReadConnections(ac) == nil; f.CanPickConnection {
		active := string(connection.StatusActive)
		conns, err := h.Connections.FindWithFilters(ctx, &active, nil)
		if err != nil {
			slog.WarnContext(ctx, "ui: connections unavailable", "err", err)
			f.CanPickConnection = false
		}
		for _, c := range auth.FilterClientScoped(ac, conns, func(c *connection.Connection) *string { return ids.StringPtr(c.ClientID) }) {
			f.Connections = append(f.Connections, kit.Option{Value: c.ID, Label: c.Name + " (" + c.Code + ")"})
		}
		if f.ConnectionID != "" && !slices.ContainsFunc(f.Connections, func(o kit.Option) bool { return o.Value == f.ConnectionID }) {
			f.Connections = append(f.Connections, kit.Option{Value: f.ConnectionID, Label: f.ConnectionID})
		}
	}
	if f.Edit {
		return
	}
	if f.CanPickPool = h.Pools != nil && auth.CanReadDispatchPools(ac) == nil; f.CanPickPool {
		active := string(dispatchpool.StatusActive)
		pools, err := h.Pools.FindWithFilters(ctx, &active, nil)
		if err != nil {
			slog.WarnContext(ctx, "ui: dispatch pools unavailable", "err", err)
			f.CanPickPool = false
		}
		for _, p := range auth.FilterClientScoped(ac, pools, func(p *dispatchpool.DispatchPool) *string { return ids.StringPtr(p.ClientID) }) {
			f.Pools = append(f.Pools, kit.Option{Value: p.ID, Label: poolChoiceLabel(p)})
		}
	}
	if err := h.withEventTypes(r, ac, f); err != nil {
		slog.WarnContext(ctx, "ui: event types unavailable", "err", err)
	}
	f.CanPickClient = canPickClient(h.Clients, ac)
	if opts, err := clientSuggestions(ctx, h.Clients, ac, f.Client); err != nil {
		slog.WarnContext(ctx, "ui: client suggestions unavailable", "err", err)
	} else {
		f.ClientOptions = opts
	}
}

func (h *Subscriptions) withEventTypes(r *http.Request, ac *auth.AuthContext, f *subForm) error {
	f.CanPickEventType = h.EventTypes != nil && auth.CanReadEventTypes(ac) == nil
	ets, err := h.eventTypeChoices(r, ac, f.ClientScoped)
	if err != nil {
		f.CanPickEventType = false
		return err
	}
	f.EventTypes = f.EventTypes[:0]
	for _, et := range ets {
		f.EventTypes = append(f.EventTypes, kit.Option{Value: et.ID, Label: et.Name + " — " + et.Code + " (v" + currentVersion(et) + ")"})
	}
	return nil
}

// poolChoiceLabel is the SPA's pool option: name, then code, rate and
// concurrency.
func poolChoiceLabel(p dispatchpool.DispatchPool) string {
	rate := "unlimited"
	if p.RateLimit != nil {
		rate = strconv.Itoa(int(*p.RateLimit)) + "/min"
	}
	return p.Name + " — " + p.Code + " (" + rate + ", " + strconv.Itoa(int(p.Concurrency)) + " concurrent)"
}

func (h *Subscriptions) showForm(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, f subForm) {
	h.withChoices(r, ac, &f)
	if ds.IsDatastar(r) {
		if err := ds.NewSSE(w, r).Patch(subFormDialogC(f), ds.Inner("#"+subDialogSlot)); err != nil {
			slog.DebugContext(r.Context(), "ui: subscription form patch not delivered", "err", err)
		}
		return
	}
	view, err := h.listView(r.Context(), ac, parseListQuery(nil, subList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view.Dialog = subFormDialogC(f)
	h.renderPage(w, r, http.StatusOK, view)
}

func (h *Subscriptions) formFailed(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, f subForm) {
	h.withChoices(r, ac, &f)
	if ds.IsDatastar(r) {
		patchFormFailure(ds.NewSSE(w, r), r, subFormDialog, f.fields(), f.Errs.Form, f.firstInvalid())
		return
	}
	view, err := h.listView(r.Context(), ac, parseListQuery(nil, subList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f.Open = true
	view.Dialog = subFormDialogC(f)
	h.renderPage(w, r, cmpOr(f.Errs.Status, http.StatusUnprocessableEntity), view)
}

func (f subForm) fieldProps(key, label, help string, required bool) kit.FieldProps {
	return kit.FieldProps{Label: label, For: f.focusID(key), Required: required, Description: help, Error: f.fieldError(key), Attrs: templ.Attributes{"id": f.id(key) + "-field"}}
}

func (f subForm) describedBy(key string) string {
	if f.fieldError(key) != "" {
		return f.focusID(key) + "-err"
	}
	return ""
}

func (f subForm) inputProps(key, value string, p kit.InputProps) kit.InputProps {
	p.ID, p.Name, p.Value = f.id(key), key, value
	p.Invalid = f.fieldError(key) != ""
	p.DescribedBy = f.describedBy(key)
	return p
}

func (f subForm) selectProps(key, selected, placeholder string, opts []kit.Option, enabled, required bool) kit.SelectProps {
	return kit.SelectProps{
		ID: f.id(key), Name: key, Options: opts, Selected: selected, Placeholder: placeholder,
		Required: required, Disabled: !enabled, Invalid: f.fieldError(key) != "", DescribedBy: f.describedBy(key),
	}
}

// eventTypesProps is the event type multi-select (with search, as the SPA's
// `filter`).
func (f subForm) eventTypesProps() kit.MultiSelectProps {
	return kit.MultiSelectProps{
		ID: f.id("eventTypes"), Name: "eventTypes", Label: "Event Types", Options: f.EventTypes, Selected: f.EventTypeIDs,
		Placeholder: "Select event types", ManyLabel: "{0} event types selected", Search: true, Signal: "subEtQ",
		EmptyText: "No event types with a current schema for this scope", DescribedBy: f.id("eventTypes") + "-help",
	}
}

func (f subForm) eventTypesHelp() string {
	if !f.CanPickEventType {
		return "Choosing event types needs permission to view event types."
	}
	scope := "non-client-scoped"
	if f.ClientScoped {
		scope = "client-scoped"
	}
	return "Select which event types this subscription will receive. Only " + scope + " event types are shown."
}

func (f subForm) poolHelp() string {
	if !f.CanPickPool {
		return "Choosing a dispatch pool needs permission to view dispatch pools."
	}
	return "Rate-limiting pool for this subscription's dispatch jobs"
}

func (f subForm) connectionHelp() string {
	if !f.CanPickConnection {
		return "Choosing a connection needs permission to view connections."
	}
	return "The connection used for delivering events"
}

func (f subForm) clientHelp() string {
	if !f.CanPickClient {
		return "Choosing a client needs permission to view clients; the subscription receives events for all clients."
	}
	return "Leave empty to receive events for all clients, or select a specific client."
}

func (f subForm) scopeSignals() string {
	if f.ClientScoped {
		return "{subClientScoped: 'true'}"
	}
	return "{subClientScoped: ''}"
}

func subNumberAttrs(minimum string) templ.Attributes {
	return templ.Attributes{"inputmode": "numeric", "min": minimum, "step": "1"}
}

func subNameAttrs(edit bool) templ.Attributes {
	a := templ.Attributes{"maxlength": "255", "autocomplete": "off"}
	if edit {
		a["autofocus"] = true
	}
	return a
}

// queuePlaceholder: create requires a queue (the SPA's form does); an edit
// may clear it (stored as unset, which publishes as DEFAULT).
func (f subForm) queuePlaceholder() string {
	if f.Edit {
		return "Not set (Default)"
	}
	return ""
}
