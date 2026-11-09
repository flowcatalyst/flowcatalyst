package pages

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/pushgw"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/sdksync"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/framework/go/ui/ds"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// Push → Applications (spec section 11, page 1): the applications that use the gateway, and per
// application the rules and ticket keys it synced (read-only), its limits with the operator's
// overrides (editable, audited) and the pause.

const pushControlsID = "push-app-controls"

// ── list ───────────────────────────────────────────────────────────────

type pushAppRow struct {
	Code, Name, Href string
	Rules, Keys      int
	Synced           string
	SyncedAt         *time.Time
	Limits           string
	Paused           bool
}

type pushAppsView struct {
	Rows  []pushAppRow
	Gates pushGates
	Error string
}

func (h *PushConsole) appsList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanViewPush(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	// "Open an application": the code typed in the form goes to its page
	if code := strings.TrimSpace(r.URL.Query().Get("open")); code != "" {
		app, err := h.Apps.FindByCode(ctx, code)
		switch {
		case err != nil:
			h.Shell.Fail(w, r, usecase.Internal("REPO", "find application failed", err))
			return
		case app != nil:
			http.Redirect(w, r, pushAppURL(app.Code), http.StatusSeeOther) //nolint:gosec // G710: the code is of a stored application, path-escaped
			return
		}
		h.renderApps(w, r, http.StatusNotFound, h.appsView(ctx, ac, "No application has the code "+code+"."))
		return
	}
	h.renderApps(w, r, http.StatusOK, h.appsView(ctx, ac, ""))
}

func (h *PushConsole) appsView(ctx context.Context, ac *auth.AuthContext, errText string) pushAppsView {
	v := pushAppsView{Gates: pushGatesFor(ac), Error: errText}
	apps, err := h.Config.ListConfigured(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "ui: push applications not readable", "err", err)
		v.Error = "The push applications could not be read."
		return v
	}
	for _, a := range apps {
		v.Rows = append(v.Rows, pushAppRow{
			Code: a.Code, Name: a.Name, Href: pushAppURL(a.Code), Rules: a.Rules, Keys: a.Keys,
			Synced: pushAge(a.SyncedAt, h.now()), SyncedAt: a.SyncedAt, Paused: a.Paused, Limits: pushLimitsSummary(a.Overrides),
		})
	}
	return v
}

func pushLimitsSummary(overrides int) string {
	switch overrides {
	case 0:
		return "Defaults"
	case 1:
		return "1 override"
	}
	return strconv.Itoa(overrides) + " overrides"
}

func (h *PushConsole) renderApps(w http.ResponseWriter, r *http.Request, status int, v pushAppsView) {
	h.Shell.RenderStatus(w, r, status, shell.Page{Title: "Push applications", Path: pushAppsPath}, pushAppsPage(v))
}

// ── detail ─────────────────────────────────────────────────────────────

type pushRuleRow struct {
	Pattern  string
	Requires []string
	Public   bool
	Synced   string
}

type pushKeyRow struct{ Kid, Alg, Synced string }

// pushLimitField is one limit as the form shows it: the override as typed ("" for the default),
// the default, what applies, and whether an override is set.
type pushLimitField struct {
	Key, Label, Unit, Help string
	Value                  string
	Default, Effective     int
	Override               bool
	Err                    string
}

type pushAppView struct {
	Code, Name string
	Paused     bool
	Fields     []pushLimitField
	FormErr    string
	UpdatedAt  *time.Time
	UpdatedBy  string
	Rules      []pushRuleRow
	Keys       []pushKeyRow
	// Rotation: more than one key is synced, so a rotation is under way.
	Rotation bool
	Gates    pushGates
	Now      time.Time
}

// limitSpecs are the editable limits in form order.
var limitSpecs = []struct {
	key, label, unit, help string
	def                    int
}{
	{"maxStreams", "Max streams", "streams", "Open streams that listen to the application's topics, across the gateway.", pushgw.DefaultMaxStreams},
	{"publishRate", "Publish rate", "pushes/s", "Pushes per second per gateway task; over it, ingest answers 429 and the application's outbox waits.", pushgw.DefaultPublishRate},
	{"publishBurst", "Publish burst", "pushes", "The most pushes admitted at once; at least the publish rate.", pushgw.DefaultPublishBurst},
	{"maxDataBytes", "Max data size", "bytes", "Largest push data. At most 65,536.", pushgw.DefaultMaxData},
}

func settingValue(s sdksync.PushSettings, key string) *int {
	switch key {
	case "maxStreams":
		return s.MaxStreams
	case "publishRate":
		return s.PublishRate
	case "publishBurst":
		return s.PublishBurst
	}
	return s.MaxDataBytes
}

func effectiveValue(e sdksync.PushSettingsResponse, key string) int {
	switch key {
	case "maxStreams":
		return e.Effective.MaxStreams
	case "publishRate":
		return e.Effective.PublishRate
	case "publishBurst":
		return e.Effective.PublishBurst
	}
	return e.Effective.MaxDataBytes
}

// limitFields builds the form fields from the stored settings; typed, when non-nil, replaces each
// field's value with what the operator typed (a failed save shows it again).
func limitFields(resp sdksync.PushSettingsResponse, typed map[string]string, errs map[string]string) []pushLimitField {
	out := make([]pushLimitField, 0, len(limitSpecs))
	for _, sp := range limitSpecs {
		f := pushLimitField{Key: sp.key, Label: sp.label, Unit: sp.unit, Help: sp.help, Default: sp.def, Effective: effectiveValue(resp, sp.key), Err: errs[sp.key]}
		if v := settingValue(resp.Settings, sp.key); v != nil {
			f.Value, f.Override = strconv.Itoa(*v), true
		}
		if t, ok := typed[sp.key]; ok {
			f.Value = t
		}
		out = append(out, f)
	}
	return out
}

func (h *PushConsole) loadApp(w http.ResponseWriter, r *http.Request) (*application.Application, bool) {
	app, err := h.Apps.FindByCode(r.Context(), chi.URLParam(r, "code"))
	if err != nil {
		h.Shell.Fail(w, r, usecase.Internal("REPO", "find application failed", err))
		return nil, false
	}
	if app == nil {
		h.Shell.Deny(w, r, http.StatusNotFound, "There is no such application.")
		return nil, false
	}
	return app, true
}

func (h *PushConsole) appView(ctx context.Context, ac *auth.AuthContext, app *application.Application, typed, errs map[string]string, formErr string) (pushAppView, error) {
	resp, err := h.Settings.Read(ctx, string(app.ID), app.Code)
	if err != nil {
		return pushAppView{}, err
	}
	v := pushAppView{
		Code: app.Code, Name: app.Name, Paused: resp.Settings.Paused, Fields: limitFields(resp, typed, errs), FormErr: formErr,
		UpdatedAt: resp.UpdatedAt, Gates: pushGatesFor(ac), Now: h.now(),
	}
	if resp.UpdatedBy != nil {
		v.UpdatedBy = *resp.UpdatedBy
	}
	rules, err := h.Config.RuleDetails(ctx, string(app.ID))
	if err != nil {
		return pushAppView{}, usecase.Internal("REPO", "read push rules failed", err)
	}
	for _, rl := range rules {
		v.Rules = append(v.Rules, pushRuleRow{Pattern: rl.Pattern, Requires: rl.Requires, Public: rl.Public, Synced: pushAge(&rl.SyncedAt, v.Now)})
	}
	keys, err := h.Config.KeyDetails(ctx, string(app.ID))
	if err != nil {
		return pushAppView{}, usecase.Internal("REPO", "read push ticket keys failed", err)
	}
	for _, k := range keys {
		v.Keys = append(v.Keys, pushKeyRow{Kid: k.Kid, Alg: cmp.Or(k.Alg, "-"), Synced: pushAge(&k.SyncedAt, v.Now)})
	}
	v.Rotation = len(v.Keys) > 1
	return v, nil
}

func (h *PushConsole) appDetail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanViewPush(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	app, ok := h.loadApp(w, r)
	if !ok {
		return
	}
	v, err := h.appView(ctx, ac, app, nil, nil, "")
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.renderApp(w, r, http.StatusOK, v)
}

func (h *PushConsole) renderApp(w http.ResponseWriter, r *http.Request, status int, v pushAppView) {
	h.Shell.RenderStatus(w, r, status, shell.Page{Title: v.Name + " - Push", Path: pushAppsPath}, pushAppPage(v))
}

// ── edit ───────────────────────────────────────────────────────────────

// applied answers a successful edit: the controls again and a flash after a Datastar post, a
// redirect carrying the flash key after a native one.
func (h *PushConsole) applied(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, app *application.Application, flash string) {
	ctx := r.Context()
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flash)
		http.Redirect(w, r, pushAppURL(app.Code), http.StatusSeeOther) //nolint:gosec // G710: the code is of a stored application, path-escaped
		return
	}
	v, err := h.appView(ctx, ac, app, nil, nil, "")
	sse := ds.NewSSE(w, r)
	if err != nil {
		h.Shell.FailSSE(sse, r, err)
		return
	}
	if err := errors.Join(sse.Patch(pushAppControls(v)), flashKey(sse, flash)); err != nil {
		slog.DebugContext(ctx, "ui: push controls patch not delivered", "err", err)
	}
}

// parseLimit reads a limit field: blank is no override; anything else a whole number.
func parseLimit(raw string) (*int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return nil, false
	}
	return &n, true
}

// limitErrorField maps a settings validation message to the field it is about: the messages
// start with the field's name.
func limitErrorField(msg string) string {
	for _, sp := range limitSpecs {
		if strings.HasPrefix(msg, sp.key) {
			return sp.key
		}
	}
	return ""
}

func (h *PushConsole) saveLimits(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanManagePush(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	app, ok := h.loadApp(w, r)
	if !ok {
		return
	}
	cur, err := h.Settings.Read(ctx, string(app.ID), app.Code)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	next := sdksync.PushSettings{Paused: cur.Settings.Paused} // the pause has its own control
	typed, errs := map[string]string{}, map[string]string{}
	for _, sp := range limitSpecs {
		raw := r.PostFormValue(sp.key)
		typed[sp.key] = strings.TrimSpace(raw)
		n, ok := parseLimit(raw)
		if !ok {
			errs[sp.key] = notWholeNumber
			continue
		}
		switch sp.key {
		case "maxStreams":
			next.MaxStreams = n
		case "publishRate":
			next.PublishRate = n
		case "publishBurst":
			next.PublishBurst = n
		default:
			next.MaxDataBytes = n
		}
	}
	var formErr string
	if len(errs) == 0 {
		_, err = h.Settings.Update(ctx, ac, string(app.ID), app.Code, next)
		if err == nil {
			h.applied(w, r, ac, app, flashPushLimitsSaved)
			return
		}
		if _, isUC := errors.AsType[*usecase.Error](err); !isUC {
			h.Shell.Fail(w, r, err)
			return
		}
		status, msg := shell.Classify(r, err)
		if status >= http.StatusInternalServerError || status == http.StatusForbidden {
			h.Shell.Fail(w, r, err)
			return
		}
		if f := limitErrorField(msg); f != "" {
			errs[f] = sentence(msg)
		} else {
			formErr = sentence(msg)
		}
	}
	v, err := h.appView(ctx, ac, app, typed, errs, formErr)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if ds.IsDatastar(r) {
		sse := ds.NewSSE(w, r)
		_ = sse.Patch(pushAppControls(v))
		return
	}
	h.renderApp(w, r, http.StatusUnprocessableEntity, v)
}

func (h *PushConsole) setPaused(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanManagePush(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	app, ok := h.loadApp(w, r)
	if !ok {
		return
	}
	cur, err := h.Settings.Read(ctx, string(app.ID), app.Code)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	next := cur.Settings
	next.Paused = r.PostFormValue("paused") == "true"
	if _, err := h.Settings.Update(ctx, ac, string(app.ID), app.Code, next); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	flash := flashPushResumed
	if next.Paused {
		flash = flashPushPaused
	}
	h.applied(w, r, ac, app, flash)
}
