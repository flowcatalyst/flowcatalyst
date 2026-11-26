package fieldidentity

import (
	"context"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// Error codes of the overrides.
const (
	// CodeApplicationNotEnabled: an override of an application that is not
	// active and enabled for the client (409).
	CodeApplicationNotEnabled = "APPLICATION_NOT_ENABLED"
	// CodePolicyLooseningForbidden: an application's own account tried to
	// loosen a member it may only tighten (403; details.field names it).
	CodePolicyLooseningForbidden = "POLICY_LOOSENING_FORBIDDEN"
	// CodePolicyMemberLocked: the client does not let its applications
	// override the member (409; details.field names it).
	CodePolicyMemberLocked = "POLICY_MEMBER_LOCKED"
)

// PatchPolicyOverrideCommand changes one application's override for a client:
// Set gives members their own value, Clear lets them follow the client again,
// ClearAll clears every member (the DELETE).
type PatchPolicyOverrideCommand struct {
	ClientID      string               `json:"clientId"`
	ApplicationID string               `json:"applicationId"`
	Set           map[PolicyMember]any `json:"set,omitempty"`
	Clear         []PolicyMember       `json:"clear,omitempty"`
	ClearAll      bool                 `json:"clearAll,omitempty"`
}

// PolicyOverrideResult is the override after the change, with the client's
// policy it applies to.
type PolicyOverrideResult struct {
	Policy   Policy
	Override *PolicyOverride
}

func validateOverridePatch(_ context.Context, cmd PatchPolicyOverrideCommand) error {
	if strings.TrimSpace(cmd.ClientID) == "" || strings.TrimSpace(cmd.ApplicationID) == "" {
		return usecase.Validation("POLICY_INVALID", "clientId and the application are required")
	}
	for m, v := range cmd.Set {
		if err := CheckOverrideValue(m, v); err != nil {
			return err
		}
		if slices.Contains(cmd.Clear, m) {
			return usecase.Validation("POLICY_INVALID", string(m)+" is both set and cleared").
				WithDetails(map[string]any{"field": string(m)})
		}
	}
	for _, m := range cmd.Clear {
		if !m.Valid() {
			return usecase.Validation("POLICY_INVALID", string(m)+" cannot be overridden by an application").
				WithDetails(map[string]any{"field": string(m)})
		}
	}
	return nil
}

// authorizeOverride: the client scope, and an application-confined caller
// only for its own application (an application's service account changes its
// own override with the permissions it holds for field work; client and
// platform administrators change any).
func authorizeOverride(ctx context.Context, clientID, applicationID string) error {
	ac := auth.FromContext(ctx)
	if err := auth.CheckScopeAccess(ac, &clientID); err != nil {
		return err
	}
	if !ac.CanAccessApplication(applicationID) {
		return usecase.Authorization("APPLICATION_FORBIDDEN", "an application can change only its own override")
	}
	return nil
}

// PatchPolicyOverride sets or clears members of an application's override of
// the client's field policy. A member the client has locked cannot be set
// (409 POLICY_MEMBER_LOCKED) but can be cleared. Turning unlock keys off for
// the application ends its key-signed sessions; the keys are revoked only when
// no application of the client has unlock keys on any more (section 4).
func (o *Ops) PatchPolicyOverride() usecaseop.TxOperation[PatchPolicyOverrideCommand, PolicyOverrideResult] {
	return usecaseop.TxOperation[PatchPolicyOverrideCommand, PolicyOverrideResult]{
		Name:     "PatchFieldPolicyOverride",
		Validate: validateOverridePatch,
		Authorize: func(ctx context.Context, cmd PatchPolicyOverrideCommand) error {
			return authorizeOverride(ctx, cmd.ClientID, cmd.ApplicationID)
		},
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd PatchPolicyOverrideCommand, ec usecase.ExecutionContext) (PolicyOverrideResult, error) {
			var zero PolicyOverrideResult
			if err := requireClient(ctx, o.Clients, cmd.ClientID); err != nil {
				return zero, err
			}
			app, err := o.Apps.FindByID(ctx, cmd.ApplicationID)
			if err != nil {
				return zero, repoErr("find_application", err)
			}
			if app == nil {
				return zero, httperror.NotFound("Application", cmd.ApplicationID)
			}
			ac := auth.FromContext(ctx)
			// The client's policy lock first: disabling the application for the
			// client takes it too, so "the application serves the client" below
			// cannot change before this override is saved.
			if err := o.lockPolicy(ctx, s, cmd.ClientID); err != nil {
				return zero, err
			}
			serves, err := o.R.Policies.ApplicationServes(ctx, cmd.ClientID, cmd.ApplicationID)
			if err != nil {
				return zero, repoErr("find_client_config", err)
			}
			if !serves {
				if ac.IsApplicationScoped() {
					return zero, usecase.Authorization("APPLICATION_FORBIDDEN",
						"an application can change its override only for a client it is active and enabled for")
				}
				return zero, usecase.Conflict(CodeApplicationNotEnabled, "the application is not active and enabled for this client")
			}
			p, err := o.R.Policies.Effective(ctx, cmd.ClientID)
			if err != nil {
				return zero, repoErr("find_field_policy", err)
			}
			before, err := o.overridesInTx(ctx, s, cmd.ClientID)
			if err != nil {
				return zero, err
			}
			cur := PolicyOverride{ClientID: cmd.ClientID, ApplicationID: cmd.ApplicationID, Values: map[PolicyMember]any{}}
			if i := slices.IndexFunc(before, func(x PolicyOverride) bool { return x.ApplicationID == cmd.ApplicationID }); i >= 0 {
				maps.Copy(cur.Values, before[i].Values)
			}
			// R1, judged now, against the stored override and the client's policy
			// as they are under the lock.
			if ac.IsApplicationScoped() {
				if err := applicationMayOnlyTighten(p, cur, cmd); err != nil {
					return zero, err
				}
			}
			next := PolicyOverride{
				ClientID: cur.ClientID, ApplicationID: cur.ApplicationID, Values: map[PolicyMember]any{},
				UpdatedAt: time.Now().UTC().Truncate(time.Microsecond), UpdatedBy: ec.PrincipalID,
			}
			if !cmd.ClearAll {
				for m, v := range cur.Values {
					if !slices.Contains(cmd.Clear, m) {
						next.Values[m] = v
					}
				}
			}
			var set []string
			for _, m := range OverridableMembers() {
				v, ok := cmd.Set[m]
				if !ok {
					continue
				}
				if p.Locked(m) {
					return zero, usecase.Conflict(CodePolicyMemberLocked,
						"the client does not let its applications override "+string(m)).
						WithDetails(map[string]any{"field": string(m)})
				}
				next.Values[m] = v
				set = append(set, string(m))
			}
			var cleared []string
			for _, m := range OverridableMembers() {
				if _, had := cur.Values[m]; had {
					if _, has := next.Values[m]; !has {
						cleared = append(cleared, string(m))
					}
				}
			}
			after := replaceOverride(before, next)
			unlockChanged := unlockKeyState(p, before) != unlockKeyState(p, after)
			if unlockChanged {
				if err := o.lockKeyWorkers(ctx, s, cmd.ClientID); err != nil {
					return zero, err
				}
			}
			if err := s.WithTx(ctx, func(tx pgx.Tx) error {
				return o.R.Policies.SaveOverride(ctx, tx, next)
			}); err != nil {
				return zero, repoErr("save_field_policy_override", err)
			}
			values := map[string]any{}
			for m, v := range next.Values {
				values[string(m)] = v
			}
			ev := NewEvent(ec, PolicyOverriddenType, "field-policy", cmd.ClientID, map[string]any{
				"clientId": cmd.ClientID, "applicationId": cmd.ApplicationID, "appCode": app.Code,
				"overrides": values, "set": nonNil(set), "cleared": nonNil(cleared),
			})
			if _, err := usecase.Into(usecasepgx.EmitEventScoped(ctx, s, ev, cmd)); err != nil {
				return zero, err
			}
			if unlockChanged {
				if err := o.settleUnlockKeys(ctx, s, ec, p, after, ec.PrincipalID, cmd); err != nil {
					return zero, err
				}
			}
			res := PolicyOverrideResult{Policy: p}
			if !next.Empty() {
				res.Override = &next
			}
			return res, nil
		},
	}
}

// replaceOverride is overrides with next in place of the application's own
// (removed when next sets nothing).
func replaceOverride(overrides []PolicyOverride, next PolicyOverride) []PolicyOverride {
	out := make([]PolicyOverride, 0, len(overrides)+1)
	for _, x := range overrides {
		if x.ApplicationID != next.ApplicationID {
			out = append(out, x)
		}
	}
	if !next.Empty() {
		out = append(out, next)
	}
	return out
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// applicationMayOnlyTighten is rule R1 for an application-confined caller
// (docs/field-policy-application-overrides.md section 5, awaiting the owner's
// confirmation): requireEnrolledDevice and badgeOnlyLogin decide whether a PIN
// is checked and from where, and what follows from that check (the failure
// counter, lockout, the hard lock's revocation of keys) is the worker's across
// applications; photosEnabled loosened shows photos the client chose to hide.
// So an application may only tighten them for itself:
//
//   - setting: requireEnrolledDevice true, badgeOnlyLogin false, photosEnabled
//     false; the other value is refused whatever is in force;
//   - clearing (by clear or by the DELETE): refused when the stored override is
//     stricter than the client's value p, so that clearing would loosen what is
//     in force for the application (an administrator's tightening it cannot
//     undo); allowed when it does not loosen (an override equal to the
//     client's, or one the client's lock already overrules).
//
// The whole call is refused, naming every member it may not change, so nothing
// half-happens; loosening them is for a client or platform administrator.
func applicationMayOnlyTighten(p Policy, cur PolicyOverride, cmd PatchPolicyOverrideCommand) error {
	var refused []string
	for _, m := range OverridableMembers() {
		var looseValue bool // the value that loosens the member
		switch m {
		case MemberRequireEnrolledDevice:
			looseValue = false
		case MemberBadgeOnlyLogin, MemberPhotosEnabled:
			looseValue = true
		case MemberUnlockKeysEnabled, MemberSharedIdleSecs, MemberPersonalIdleSecs, MemberShiftMaxSecs,
			MemberUnenrolledIdleSecs:
			continue // an application sets these either way, within the ranges
		}
		if v, ok := cmd.Set[m]; ok && v == looseValue {
			refused = append(refused, string(m))
			continue
		}
		clearing := cmd.ClearAll || slices.Contains(cmd.Clear, m)
		if ov, had := cur.Values[m]; clearing && had && !p.Locked(m) && ov != looseValue && m.value(p) == looseValue {
			refused = append(refused, string(m))
		}
	}
	if len(refused) == 0 {
		return nil
	}
	return usecase.Authorization(CodePolicyLooseningForbidden,
		"an application can only tighten "+strings.Join(refused, ", ")+
			" for itself; a client or platform administrator can loosen or clear it").
		WithDetails(map[string]any{"field": refused[0], "fields": refused})
}
