package sdksync

import (
	"context"
	"errors"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/pushgw"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// The operator's controls over an application's push gateway use (docs/push-gateway-spec.md
// section 10): limits and a pause. They are the operator's call, not the application's, so they
// are never part of the application's sync. Anchor only, audited; the gateway tasks reload within
// a second (NOTIFY fw_push_config).

type pushSettingsInput struct {
	AppCode string `path:"appCode" doc:"Application code"`
}

// PushSettings are an application's push gateway limits. A limit left out (null) takes the
// gateway default.
type PushSettings struct {
	MaxStreams   *int `json:"maxStreams,omitempty" doc:"Open streams that listen to the application's topics (default 20,000)"`
	PublishRate  *int `json:"publishRate,omitempty" doc:"Pushes per second per gateway task (default 500)"`
	PublishBurst *int `json:"publishBurst,omitempty" doc:"Burst of the publish rate (default 2,000; at least the rate)"`
	MaxDataBytes *int `json:"maxDataBytes,omitempty" doc:"Largest push data, in bytes (default 16,384; at most 65,536)"`
	Paused       bool `json:"paused" doc:"Ingest answers 429 for the application until unpaused; open streams stay connected"`
}

// PushSettingsResponse is the stored overrides and the effective values.
type PushSettingsResponse struct {
	ApplicationCode string       `json:"applicationCode"`
	Settings        PushSettings `json:"settings"`
	Effective       struct {
		MaxStreams   int `json:"maxStreams"`
		PublishRate  int `json:"publishRate"`
		PublishBurst int `json:"publishBurst"`
		MaxDataBytes int `json:"maxDataBytes"`
	} `json:"effective"`
	UpdatedAt *time.Time `json:"updatedAt,omitempty"`
	UpdatedBy *string    `json:"updatedBy,omitempty"`
}

type pushSettingsOutput struct{ Body PushSettingsResponse }

type putPushSettingsInput struct {
	AppCode string `path:"appCode" doc:"Application code"`
	Body    PushSettings
}

func (s *State) pushSettingsFor(ctx context.Context, appCode string) (string, string, *pushSettingsOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanManagePush(ac); err != nil {
		return "", "", nil, err
	}
	if s.Push == nil {
		return "", "", nil, usecase.Internal("PUSH", "push settings are not wired", nil)
	}
	app, err := s.resolveApp(ctx, appCode)
	if err != nil {
		return "", "", nil, err
	}
	stored, err := s.Push.Settings(ctx, string(app.ID))
	if err != nil {
		return "", "", nil, usecase.Internal("PUSH", "reading the push settings failed", err)
	}
	return string(app.ID), app.Code, &pushSettingsOutput{Body: settingsResponse(app.Code, stored)}, nil
}

// NewPushSettingsResponse is the answer for stored overrides: the overrides and the effective
// values (tests of the console build their fakes with it).
func NewPushSettingsResponse(code string, st pushgw.AppSettings) PushSettingsResponse {
	return settingsResponse(code, st)
}

func settingsResponse(code string, st pushgw.AppSettings) PushSettingsResponse {
	var r PushSettingsResponse
	r.ApplicationCode = code
	r.Settings = PushSettings{
		MaxStreams: st.MaxStreams, PublishRate: st.PublishRate, PublishBurst: st.PublishBurst,
		MaxDataBytes: st.MaxDataBytes, Paused: st.Paused,
	}
	r.Effective.MaxStreams = orDefault(st.MaxStreams, pushgw.DefaultMaxStreams)
	r.Effective.PublishRate = orDefault(st.PublishRate, pushgw.DefaultPublishRate)
	r.Effective.PublishBurst = orDefault(st.PublishBurst, max(pushgw.DefaultPublishBurst, r.Effective.PublishRate))
	r.Effective.MaxDataBytes = orDefault(st.MaxDataBytes, pushgw.DefaultMaxData)
	r.UpdatedAt, r.UpdatedBy = st.UpdatedAt, st.UpdatedBy
	return r
}

func orDefault(v *int, def int) int {
	if v != nil {
		return *v
	}
	return def
}

func (s *State) getPushSettings(ctx context.Context, in *pushSettingsInput) (*pushSettingsOutput, error) {
	_, _, out, err := s.pushSettingsFor(ctx, in.AppCode)
	return out, err
}

// PushSettingsOps is the operator's edit of an application's push settings, shared by the API
// handler (PUT /api/applications/{appCode}/push/settings) and the console's Push pages, so the
// rules (anchor scope and platform:admin:push:manage, validation, the audit record) live once.
type PushSettingsOps struct {
	Push  *pushgw.ConfigRepo
	Audit *audit.Repository
}

// Read returns the application's overrides and effective values. It checks no permission: the
// caller has (the API needs manage, the console view).
func (o PushSettingsOps) Read(ctx context.Context, appID, appCode string) (PushSettingsResponse, error) {
	stored, err := o.Push.Settings(ctx, appID)
	if err != nil {
		return PushSettingsResponse{}, usecase.Internal("PUSH", "reading the push settings failed", err)
	}
	return settingsResponse(appCode, stored), nil
}

// Update replaces the application's overrides with next (what it leaves out returns to the
// default) and audits it. The caller must be an anchor with platform:admin:push:manage.
func (o PushSettingsOps) Update(ctx context.Context, ac *auth.AuthContext, appID, appCode string, next PushSettings) (PushSettingsResponse, error) {
	if err := auth.CanManagePush(ac); err != nil {
		return PushSettingsResponse{}, err
	}
	st := pushgw.AppSettings{
		MaxStreams: next.MaxStreams, PublishRate: next.PublishRate, PublishBurst: next.PublishBurst,
		MaxDataBytes: next.MaxDataBytes, Paused: next.Paused,
	}
	if err := pushgw.ValidateSettings(st); err != nil {
		if ve, ok := errors.AsType[*pushgw.ValidationError](err); ok {
			return PushSettingsResponse{}, usecase.Validation(ve.Code, ve.Message)
		}
		return PushSettingsResponse{}, err
	}
	if err := o.Push.SetSettings(ctx, appID, st, ac.PrincipalID); err != nil {
		return PushSettingsResponse{}, usecase.Internal("PUSH", "writing the push settings failed", err)
	}
	AuditPushChange(ctx, o.Audit, ac, appID, "PUSH_SETTINGS_CHANGED", map[string]any{"settings": next})
	return o.Read(ctx, appID, appCode)
}

func (s *State) putPushSettings(ctx context.Context, in *putPushSettingsInput) (*pushSettingsOutput, error) {
	appID, appCode, _, err := s.pushSettingsFor(ctx, in.AppCode)
	if err != nil {
		return nil, err
	}
	out, err := (PushSettingsOps{Push: s.Push, Audit: s.Audit}).Update(ctx, auth.FromContext(ctx), appID, appCode, in.Body)
	if err != nil {
		return nil, err
	}
	return &pushSettingsOutput{Body: out}, nil
}
