package fieldidentity

import (
	"context"
	"slices"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
)

func dbqServesParams(clientID, applicationID string) dbq.FieldApplicationServesClientParams {
	return dbq.FieldApplicationServesClientParams{ApplicationID: ids.ApplicationID(applicationID), ClientID: ids.ClientID(clientID)}
}

// When an application's override applies
// (docs/field-policy-application-overrides.md section 1): the application is
// active and enabled for the client and, for a session, the worker holds it.
// Otherwise the client-wide policy decides. A path decides this before any
// credential is checked, so an application's override never changes what can
// be done to, or learnt about, a worker who does not hold the application.

// ApplicationServes reports whether the application is active and enabled for
// the client: the condition for its override to apply, for an
// application-confined caller to see or change its override there, and for
// the override to count where unlock keys are on.
func (r *PolicyRepo) ApplicationServes(ctx context.Context, clientID, applicationID string) (bool, error) {
	if applicationID == "" {
		return false, nil
	}
	return r.q.FieldApplicationServesClient(ctx, dbqServesParams(clientID, applicationID))
}

// EffectivePolicy is the policy of a session of the application for the
// worker: the client's policy, overlaid by the application's override when the
// application is active and enabled for the client and the worker holds it (a
// nil worker skips the last condition: the provisional policy of a path that
// has not identified the worker yet, which it decides again once it has).
func (o *Ops) EffectivePolicy(ctx context.Context, clientID, applicationID string, worker *Identity) (Policy, error) {
	p, err := o.R.Policies.Effective(ctx, clientID)
	if err != nil || applicationID == "" {
		return p, err
	}
	if worker != nil && !worker.HasApp(applicationID) {
		return p, nil
	}
	serves, err := o.R.Policies.ApplicationServes(ctx, clientID, applicationID)
	if err != nil || !serves {
		return p, err
	}
	ov, err := o.R.Policies.Override(ctx, clientID, applicationID)
	if err != nil {
		return Policy{}, err
	}
	return p.ForApplication(ov), nil
}

// applicableOverrides are the client's overrides of applications that are
// active and enabled for it: an override of an inactive, deleted or disabled
// application counts nowhere (it keeps no key alive).
func (o *Ops) applicableOverrides(ctx context.Context, clientID string, overrides []PolicyOverride) ([]PolicyOverride, error) {
	if len(overrides) == 0 {
		return overrides, nil
	}
	serving, err := o.R.Policies.q.FieldApplicationsServingClient(ctx, ids.ClientID(clientID))
	if err != nil {
		return nil, repoErr("find_serving_applications", err)
	}
	out := make([]PolicyOverride, 0, len(overrides))
	for _, x := range overrides {
		if slices.Contains(serving, x.ApplicationID) {
			out = append(out, x)
		}
	}
	return out, nil
}

// ApplicableOverrides are the client's overrides that count: of applications
// active and enabled for it.
func (o *Ops) ApplicableOverrides(ctx context.Context, clientID string) ([]PolicyOverride, error) {
	all, err := o.R.Policies.Overrides(ctx, clientID)
	if err != nil {
		return nil, err
	}
	return o.applicableOverrides(ctx, clientID, all)
}

// LockingRevokesKeys reports whether setting the client's locks to locked
// would leave no application with unlock keys on where some has them on now:
// the change would revoke every key of the client (the console asks first).
func (o *Ops) LockingRevokesKeys(ctx context.Context, clientID string, locked []PolicyMember) (bool, error) {
	p, err := o.R.Policies.Effective(ctx, clientID)
	if err != nil {
		return false, err
	}
	ov, err := o.ApplicableOverrides(ctx, clientID)
	if err != nil {
		return false, err
	}
	next := p
	next.LockedMembers = locked
	return UnlockKeysOnSomewhere(p, ov) && !UnlockKeysOnSomewhere(next, ov), nil
}
