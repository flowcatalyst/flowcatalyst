package api

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// ── per-application overrides (docs/field-policy-application-overrides.md) ──

// FieldPolicyBoolMember is an overridable on/off member as an application sees
// it: the value in force for the application, where it comes from, whether the
// client locked it, and the application's own override (null: it follows the
// client).
type FieldPolicyBoolMember struct {
	Effective bool   `json:"effective"`
	Source    string `json:"source" enum:"DEFAULT,CLIENT,APPLICATION" doc:"DEFAULT: the client has no policy of its own; CLIENT: the client's policy; APPLICATION: this application's override"`
	Locked    bool   `json:"locked" doc:"The client does not let its applications override this member (set by platform administrators)"`
	Override  *bool  `json:"override" doc:"The application's own value; null when it follows the client"`
	Overruled bool   `json:"overruled" doc:"The application has an override that does not apply: the client locked the member, or the application is not active and enabled for the client"`
}

// FieldPolicyIntMember is an overridable number of seconds, as FieldPolicyBoolMember.
type FieldPolicyIntMember struct {
	Effective int    `json:"effective"`
	Source    string `json:"source" enum:"DEFAULT,CLIENT,APPLICATION"`
	Locked    bool   `json:"locked"`
	Override  *int   `json:"override" doc:"The application's own value; null when it follows the client"`
	Overruled bool   `json:"overruled"`
}

// FieldPolicyMembers are the overridable members of the field policy.
type FieldPolicyMembers struct {
	UnlockKeysEnabled     FieldPolicyBoolMember `json:"unlockKeysEnabled"`
	RequireEnrolledDevice FieldPolicyBoolMember `json:"requireEnrolledDevice"`
	BadgeOnlyLogin        FieldPolicyBoolMember `json:"badgeOnlyLogin"`
	SharedIdleSecs        FieldPolicyIntMember  `json:"sharedIdleSecs"`
	PersonalIdleSecs      FieldPolicyIntMember  `json:"personalIdleSecs"`
	ShiftMaxSecs          FieldPolicyIntMember  `json:"shiftMaxSecs"`
	UnenrolledIdleSecs    FieldPolicyIntMember  `json:"unenrolledIdleSecs"`
	PhotosEnabled         FieldPolicyBoolMember `json:"photosEnabled"`
}

// FieldPolicyApplicationResponse is one application's view of a client's field
// policy: each overridable member in its three states, and the whole
// effective policy of the application's sessions.
type FieldPolicyApplicationResponse struct {
	ClientID      string              `json:"clientId"`
	ApplicationID string              `json:"applicationId"`
	AppCode       string              `json:"appCode"`
	Members       FieldPolicyMembers  `json:"members"`
	Effective     FieldPolicyResponse `json:"effective"`
	UpdatedAt     *httpcompat.Time    `json:"updatedAt,omitempty" doc:"When the override last changed; absent when the application has none"`
	UpdatedBy     string              `json:"updatedBy,omitempty"`
}

// FieldPolicyOverrideValues are override values by member; a member absent is
// not touched (PATCH set) or not overridden (a read).
type FieldPolicyOverrideValues struct {
	UnlockKeysEnabled     *bool `json:"unlockKeysEnabled,omitempty"`
	RequireEnrolledDevice *bool `json:"requireEnrolledDevice,omitempty"`
	BadgeOnlyLogin        *bool `json:"badgeOnlyLogin,omitempty"`
	SharedIdleSecs        *int  `json:"sharedIdleSecs,omitempty" doc:"60 to 86400"`
	PersonalIdleSecs      *int  `json:"personalIdleSecs,omitempty" doc:"60 to 86400"`
	ShiftMaxSecs          *int  `json:"shiftMaxSecs,omitempty" doc:"3600 to 86400"`
	UnenrolledIdleSecs    *int  `json:"unenrolledIdleSecs,omitempty" doc:"60 to 86400"`
	PhotosEnabled         *bool `json:"photosEnabled,omitempty"`
}

// PatchFieldPolicyOverrideRequest sets and clears members of an application's
// override. A member in set gets the application's own value; a member in
// clear follows the client again; any other member is left as it is.
type PatchFieldPolicyOverrideRequest struct {
	Set   *FieldPolicyOverrideValues `json:"set,omitempty"`
	Clear []string                   `json:"clear,omitempty" enum:"unlockKeysEnabled,requireEnrolledDevice,badgeOnlyLogin,sharedIdleSecs,personalIdleSecs,shiftMaxSecs,unenrolledIdleSecs,photosEnabled" doc:"Members that follow the client again"`
}

// FieldPolicyOverrideSummary is one application's override in the client's list.
type FieldPolicyOverrideSummary struct {
	ApplicationID string                    `json:"applicationId"`
	AppCode       string                    `json:"appCode"`
	Overrides     FieldPolicyOverrideValues `json:"overrides"`
	Overruled     []string                  `json:"overruled" doc:"Members it overrides that do not apply: locked by the client, or the application is not active and enabled for the client"`
	UpdatedAt     httpcompat.Time           `json:"updatedAt"`
	UpdatedBy     string                    `json:"updatedBy,omitempty"`
}

// FieldPolicyOverrideListResponse is GET /api/field-policies/{clientId}/applications.
type FieldPolicyOverrideListResponse struct {
	Applications []FieldPolicyOverrideSummary `json:"applications"`
}

// PutFieldPolicyLocksRequest replaces the client's locks.
type PutFieldPolicyLocksRequest struct {
	LockedMembers []string `json:"lockedMembers" enum:"unlockKeysEnabled,requireEnrolledDevice,badgeOnlyLogin,sharedIdleSecs,personalIdleSecs,shiftMaxSecs,unenrolledIdleSecs,photosEnabled" doc:"The members applications may not override; an existing override of one is kept and does not apply"`
}

type policyAppInput struct {
	ClientID string `path:"clientId" doc:"A clt_ id or the client's identifier"`
	AppCode  string `path:"appCode" doc:"The application's code"`
}

type patchPolicyOverrideInput struct {
	ClientID string `path:"clientId" doc:"A clt_ id or the client's identifier"`
	AppCode  string `path:"appCode" doc:"The application's code"`
	Body     PatchFieldPolicyOverrideRequest
}

type putPolicyLocksInput struct {
	ClientID string `path:"clientId" doc:"A clt_ id or the client's identifier"`
	Body     PutFieldPolicyLocksRequest
}

func valuesOf(o *fieldidentity.PolicyOverride) FieldPolicyOverrideValues {
	var v FieldPolicyOverrideValues
	b := func(m fieldidentity.PolicyMember) *bool {
		if x, ok := o.Get(m); ok {
			if y, ok := x.(bool); ok {
				return &y
			}
		}
		return nil
	}
	n := func(m fieldidentity.PolicyMember) *int {
		if x, ok := o.Get(m); ok {
			if y, ok := x.(int); ok {
				return &y
			}
		}
		return nil
	}
	v.UnlockKeysEnabled, v.RequireEnrolledDevice = b(fieldidentity.MemberUnlockKeysEnabled), b(fieldidentity.MemberRequireEnrolledDevice)
	v.BadgeOnlyLogin, v.PhotosEnabled = b(fieldidentity.MemberBadgeOnlyLogin), b(fieldidentity.MemberPhotosEnabled)
	v.SharedIdleSecs, v.PersonalIdleSecs = n(fieldidentity.MemberSharedIdleSecs), n(fieldidentity.MemberPersonalIdleSecs)
	v.ShiftMaxSecs, v.UnenrolledIdleSecs = n(fieldidentity.MemberShiftMaxSecs), n(fieldidentity.MemberUnenrolledIdleSecs)
	return v
}

// setOf is the PATCH's set as the use case's map.
func setOf(v FieldPolicyOverrideValues) map[fieldidentity.PolicyMember]any {
	out := map[fieldidentity.PolicyMember]any{}
	b := func(m fieldidentity.PolicyMember, x *bool) {
		if x != nil {
			out[m] = *x
		}
	}
	n := func(m fieldidentity.PolicyMember, x *int) {
		if x != nil {
			out[m] = *x
		}
	}
	b(fieldidentity.MemberUnlockKeysEnabled, v.UnlockKeysEnabled)
	b(fieldidentity.MemberRequireEnrolledDevice, v.RequireEnrolledDevice)
	b(fieldidentity.MemberBadgeOnlyLogin, v.BadgeOnlyLogin)
	n(fieldidentity.MemberSharedIdleSecs, v.SharedIdleSecs)
	n(fieldidentity.MemberPersonalIdleSecs, v.PersonalIdleSecs)
	n(fieldidentity.MemberShiftMaxSecs, v.ShiftMaxSecs)
	n(fieldidentity.MemberUnenrolledIdleSecs, v.UnenrolledIdleSecs)
	b(fieldidentity.MemberPhotosEnabled, v.PhotosEnabled)
	return out
}

func membersOf(p fieldidentity.Policy, o *fieldidentity.PolicyOverride, applies bool) FieldPolicyMembers {
	var out FieldPolicyMembers
	for _, st := range fieldidentity.MemberStates(p, o, applies) {
		if st.Member.Bool() {
			bm := FieldPolicyBoolMember{Source: string(st.Source), Locked: st.Locked, Overruled: st.Overruled}
			bm.Effective, _ = st.Effective.(bool)
			if v, ok := st.Override.(bool); ok {
				bm.Override = &v
			}
			switch st.Member {
			case fieldidentity.MemberUnlockKeysEnabled:
				out.UnlockKeysEnabled = bm
			case fieldidentity.MemberRequireEnrolledDevice:
				out.RequireEnrolledDevice = bm
			case fieldidentity.MemberBadgeOnlyLogin:
				out.BadgeOnlyLogin = bm
			case fieldidentity.MemberPhotosEnabled:
				out.PhotosEnabled = bm
			case fieldidentity.MemberSharedIdleSecs, fieldidentity.MemberPersonalIdleSecs,
				fieldidentity.MemberShiftMaxSecs, fieldidentity.MemberUnenrolledIdleSecs:
			}
			continue
		}
		im := FieldPolicyIntMember{Source: string(st.Source), Locked: st.Locked, Overruled: st.Overruled}
		im.Effective, _ = st.Effective.(int)
		if v, ok := st.Override.(int); ok {
			im.Override = &v
		}
		switch st.Member {
		case fieldidentity.MemberSharedIdleSecs:
			out.SharedIdleSecs = im
		case fieldidentity.MemberPersonalIdleSecs:
			out.PersonalIdleSecs = im
		case fieldidentity.MemberShiftMaxSecs:
			out.ShiftMaxSecs = im
		case fieldidentity.MemberUnenrolledIdleSecs:
			out.UnenrolledIdleSecs = im
		case fieldidentity.MemberUnlockKeysEnabled, fieldidentity.MemberRequireEnrolledDevice,
			fieldidentity.MemberBadgeOnlyLogin, fieldidentity.MemberPhotosEnabled:
		}
	}
	return out
}

// policyApp resolves the client (through the gate) and the application, and
// confines an application-confined caller to its own application.
func (s *State) policyApp(ctx context.Context, clientRef, appCode string, can func(*auth.AuthContext, string) error) (string, *application.Application, error) {
	clientID, err := s.overrideClientGate(ctx, clientRef, can)
	if err != nil {
		return "", nil, err
	}
	code := strings.TrimSpace(appCode)
	app, err := s.Ops.Apps.FindByCode(ctx, code)
	if err == nil && app == nil && strings.ToLower(code) != code {
		app, err = s.Ops.Apps.FindByCode(ctx, strings.ToLower(code))
	}
	if err != nil {
		return "", nil, usecase.Internal("REPO", "find_application failed", err)
	}
	if app == nil {
		return "", nil, httperror.NotFound("Application", appCode)
	}
	ac := auth.FromContext(ctx)
	if !ac.CanAccessApplication(string(app.ID)) {
		return "", nil, usecase.Authorization("APPLICATION_FORBIDDEN", "an application can see and change only its own override")
	}
	// An application's own account reaches its override only for clients it
	// is active and enabled for (its service account is anchor-scoped: the
	// client scope alone would let it reach every client).
	if ac.IsApplicationScoped() {
		serves, err := s.Ops.R.Policies.ApplicationServes(ctx, clientID, string(app.ID))
		if err != nil {
			return "", nil, usecase.Internal("REPO", "find_client_config failed", err)
		}
		if !serves {
			return "", nil, errNotServed()
		}
	}
	return clientID, app, nil
}

// errNotServed is what an application-confined caller is answered for a client
// its application does not serve, and for one that does not exist: the same,
// so the override routes do not tell an application which clients exist.
func errNotServed() error {
	return usecase.Authorization("APPLICATION_FORBIDDEN", "an application can see and change its override only for a client it is active and enabled for")
}

// overrideClientGate is clientGate for the override routes: an
// application-confined caller is answered for an unknown client as for a
// client its application does not serve.
func (s *State) overrideClientGate(ctx context.Context, clientRef string, can func(*auth.AuthContext, string) error) (string, error) {
	clientID, err := s.clientGate(ctx, clientRef, nil, can)
	if err != nil && auth.FromContext(ctx).IsApplicationScoped() {
		if ue := usecase.AsError(err); ue != nil && ue.Kind == usecase.KindNotFound {
			return "", errNotServed()
		}
	}
	return clientID, err
}

func (s *State) policyAppResponse(ctx context.Context, clientID string, app *application.Application) (*apicommon.Out[FieldPolicyApplicationResponse], error) {
	p, err := s.Ops.R.Policies.Effective(ctx, clientID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_field_policy failed", err)
	}
	o, err := s.Ops.R.Policies.Override(ctx, clientID, string(app.ID))
	if err != nil {
		return nil, usecase.Internal("REPO", "find_field_policy_override failed", err)
	}
	applies, err := s.Ops.R.Policies.ApplicationServes(ctx, clientID, string(app.ID))
	if err != nil {
		return nil, usecase.Internal("REPO", "find_client_config failed", err)
	}
	eff := p
	if applies {
		eff = p.ForApplication(o)
	}
	out := FieldPolicyApplicationResponse{
		ClientID: clientID, ApplicationID: string(app.ID), AppCode: app.Code,
		Members: membersOf(p, o, applies), Effective: policyResponse(eff),
	}
	if !o.Empty() {
		t := jsontime.New(o.UpdatedAt)
		out.UpdatedAt, out.UpdatedBy = &t, o.UpdatedBy
	}
	return &apicommon.Out[FieldPolicyApplicationResponse]{Body: out}, nil
}

func (s *State) getPolicyApplication(ctx context.Context, in *policyAppInput) (*apicommon.Out[FieldPolicyApplicationResponse], error) {
	clientID, app, err := s.policyApp(ctx, in.ClientID, in.AppCode, auth.CanReadFieldPolicyOverrides)
	if err != nil {
		return nil, err
	}
	return s.policyAppResponse(ctx, clientID, app)
}

func (s *State) patchPolicyApplication(ctx context.Context, in *patchPolicyOverrideInput) (*apicommon.Out[FieldPolicyApplicationResponse], error) {
	clientID, app, err := s.policyApp(ctx, in.ClientID, in.AppCode, auth.CanManageFieldPolicyOverrides)
	if err != nil {
		return nil, err
	}
	cmd := fieldidentity.PatchPolicyOverrideCommand{ClientID: clientID, ApplicationID: string(app.ID)}
	if in.Body.Set != nil {
		cmd.Set = setOf(*in.Body.Set)
	}
	for _, m := range in.Body.Clear {
		cmd.Clear = append(cmd.Clear, fieldidentity.PolicyMember(m))
	}
	if _, err := usecaseop.RunTx(ctx, s.UoW, s.Ops.PatchPolicyOverride(), cmd, auth.NewExecutionContext(ctx)); err != nil {
		return nil, fail(err)
	}
	return s.policyAppResponse(ctx, clientID, app)
}

func (s *State) deletePolicyApplication(ctx context.Context, in *policyAppInput) (*apicommon.Out[FieldPolicyApplicationResponse], error) {
	clientID, app, err := s.policyApp(ctx, in.ClientID, in.AppCode, auth.CanManageFieldPolicyOverrides)
	if err != nil {
		return nil, err
	}
	cmd := fieldidentity.PatchPolicyOverrideCommand{ClientID: clientID, ApplicationID: string(app.ID), ClearAll: true}
	if _, err := usecaseop.RunTx(ctx, s.UoW, s.Ops.PatchPolicyOverride(), cmd, auth.NewExecutionContext(ctx)); err != nil {
		return nil, fail(err)
	}
	return s.policyAppResponse(ctx, clientID, app)
}

func (s *State) listPolicyApplications(ctx context.Context, in *policyInput) (*apicommon.Out[FieldPolicyOverrideListResponse], error) {
	clientID, err := s.clientGate(ctx, in.ClientID, nil, auth.CanReadFieldPolicyOverrides)
	if err != nil {
		// An application-confined caller gets the same empty list for a client
		// that does not exist as for one it does not serve.
		if ue := usecase.AsError(err); ue != nil && ue.Kind == usecase.KindNotFound && auth.FromContext(ctx).IsApplicationScoped() {
			return &apicommon.Out[FieldPolicyOverrideListResponse]{Body: FieldPolicyOverrideListResponse{Applications: []FieldPolicyOverrideSummary{}}}, nil
		}
		return nil, err
	}
	p, err := s.Ops.R.Policies.Effective(ctx, clientID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_field_policy failed", err)
	}
	rows, err := s.Ops.R.Policies.Overrides(ctx, clientID)
	if err != nil {
		return nil, usecase.Internal("REPO", "list_field_policy_overrides failed", err)
	}
	ac := auth.FromContext(ctx)
	out := make([]FieldPolicyOverrideSummary, 0, len(rows))
	for n := range rows {
		o := &rows[n]
		if !ac.CanAccessApplication(o.ApplicationID) {
			continue // an application sees only its own
		}
		serves, err := s.Ops.R.Policies.ApplicationServes(ctx, clientID, o.ApplicationID)
		if err != nil {
			return nil, usecase.Internal("REPO", "find_client_config failed", err)
		}
		if !serves && ac.IsApplicationScoped() {
			continue // and only for clients it serves
		}
		code := o.ApplicationID
		if a, err := s.Ops.Apps.FindByID(ctx, o.ApplicationID); err == nil && a != nil {
			code = a.Code
		}
		sum := FieldPolicyOverrideSummary{
			ApplicationID: o.ApplicationID, AppCode: code, Overrides: valuesOf(o), Overruled: []string{},
			UpdatedAt: jsontime.New(o.UpdatedAt), UpdatedBy: o.UpdatedBy,
		}
		for _, m := range fieldidentity.OverridableMembers() {
			if _, set := o.Get(m); set && (p.Locked(m) || !serves) {
				sum.Overruled = append(sum.Overruled, string(m))
			}
		}
		out = append(out, sum)
	}
	return &apicommon.Out[FieldPolicyOverrideListResponse]{Body: FieldPolicyOverrideListResponse{Applications: out}}, nil
}

func (s *State) putPolicyLocks(ctx context.Context, in *putPolicyLocksInput) (*apicommon.Out[FieldPolicyResponse], error) {
	clientID, err := s.clientGate(ctx, in.ClientID, nil, auth.CanLockFieldPolicy)
	if err != nil {
		return nil, err
	}
	cmd := fieldidentity.SetPolicyLocksCommand{ClientID: clientID}
	for _, m := range in.Body.LockedMembers {
		cmd.LockedMembers = append(cmd.LockedMembers, fieldidentity.PolicyMember(m))
	}
	p, err := usecaseop.RunTx(ctx, s.UoW, s.Ops.SetPolicyLocks(), cmd, auth.NewExecutionContext(ctx))
	if err != nil {
		return nil, fail(err)
	}
	// The lock permission reads what it set (a platform administrator need not
	// hold the field-device permissions).
	return &apicommon.Out[FieldPolicyResponse]{Body: policyResponse(p)}, nil
}
