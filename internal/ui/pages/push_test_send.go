package pages

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/pushgw"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/framework/go/push"
	"github.com/flowcatalyst/framework/go/ui/ds"
	"github.com/flowcatalyst/framework/go/ui/kit"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// Push → Test send (spec section 11, page 4): an anchor with the manage permission publishes
// {"test":true} of type fw-test.ping to a chosen topic for a chosen client through the gateway's
// publish path, to see a stream receive it. Audited against the topic's application.

const (
	pushTestForm   = "push-test-form"
	pushTestResult = "push-test-result"
)

type pushTestView struct {
	Gates     pushGates
	Available bool
	Topic     string
	Client    string
	Clients   []kit.Option
	TopicErr  string
	ClientErr string
	FormErr   string
	// Sent is the topic of the push that just went out ("" before one did).
	Sent string
}

func (h *PushConsole) testView(r *http.Request, ac *auth.AuthContext) pushTestView {
	ctx := r.Context()
	v := pushTestView{Gates: pushGatesFor(ac), Available: h.Publisher != nil && h.Publisher.Available(), Client: "_platform"}
	v.Clients = append(v.Clients, kit.Option{Value: "_platform", Label: "Platform (reaches every client)"})
	if h.Clients != nil {
		if cs, err := h.Clients.FindAll(ctx); err == nil {
			opts := make([]kit.Option, 0, len(cs))
			for _, c := range cs {
				if c.Status == client.StatusActive {
					opts = append(opts, kit.Option{Value: string(c.ID), Label: c.Name})
				}
			}
			sortOptions(opts)
			v.Clients = append(v.Clients, opts...)
		}
	}
	return v
}

func (h *PushConsole) renderTest(w http.ResponseWriter, r *http.Request, status int, v pushTestView) {
	h.Shell.RenderStatus(w, r, status, shell.Page{Title: "Push test send", Path: pushTestPath}, pushTestPage(v))
}

func (h *PushConsole) testPage(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanManagePush(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	v := h.testView(r, ac)
	v.Topic = strings.TrimSpace(r.URL.Query().Get("topic"))
	h.renderTest(w, r, http.StatusOK, v)
}

func (h *PushConsole) testSend(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanManagePush(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	v := h.testView(r, ac)
	v.Topic = strings.TrimSpace(r.PostFormValue("topic"))
	if c := strings.TrimSpace(r.PostFormValue("client")); c != "" {
		v.Client = c
	}
	h.checkAndSend(r, ac, &v)
	if v.Sent != "" {
		if !ds.IsDatastar(r) {
			h.Shell.SetFlash(w, flashPushTestSent)
			http.Redirect(w, r, pushTestPath+"?topic="+url.QueryEscape(v.Sent), http.StatusSeeOther)
			return
		}
		sse := ds.NewSSE(w, r)
		if err := errors.Join(sse.Patch(pushTestBody(v)), flashKey(sse, flashPushTestSent)); err != nil {
			slog.DebugContext(ctx, "ui: push test patch not delivered", "err", err)
		}
		return
	}
	if ds.IsDatastar(r) {
		_ = ds.NewSSE(w, r).Patch(pushTestBody(v))
		return
	}
	h.renderTest(w, r, http.StatusUnprocessableEntity, v)
}

// checkAndSend validates the form and publishes; it sets the view's errors or Sent.
func (h *PushConsole) checkAndSend(r *http.Request, ac *auth.AuthContext, v *pushTestView) {
	ctx := r.Context()
	if !v.Available {
		v.FormErr = "The push gateway does not run in this process. Send the test from a process that runs it (FC_PUSH_ENABLED)."
		return
	}
	if v.Topic == "" {
		v.TopicErr = "Enter a topic."
		return
	}
	if err := push.CheckTopic(v.Topic); err != nil {
		if ipe, ok := errors.AsType[*push.InvalidPushError](err); ok {
			v.TopicErr = sentence(ipe.Message)
		} else {
			v.TopicErr = "The topic is not valid."
		}
		return
	}
	appCode, _, _ := strings.Cut(v.Topic, ":")
	app, err := h.Apps.FindByCode(ctx, appCode)
	if err != nil {
		v.FormErr = "The application could not be read."
		slog.ErrorContext(ctx, "ui: push test: find application", "err", err)
		return
	}
	if app == nil {
		v.TopicErr = "No application has the code " + appCode + "."
		return
	}
	clientID := v.Client
	if clientID != "_platform" {
		c, err := h.Clients.FindByID(ctx, clientID)
		if err != nil || c == nil {
			v.ClientErr = "Choose one of the clients."
			return
		}
	}
	if err := h.Publisher.PublishTest(ctx, v.Topic, clientID); err != nil {
		if errors.Is(err, pushgw.ErrNoGateway) {
			v.FormErr = "The push gateway does not run in this process."
			return
		}
		_, msg := shell.Classify(r, usecase.Internal("PUSH", "test push failed", err))
		v.FormErr = msg
		return
	}
	h.audit(ctx, ac, app, "PUSH_TEST_SENT", map[string]any{"topic": v.Topic, "type": pushgw.TestType, "client": clientID})
	v.Sent = v.Topic
}
