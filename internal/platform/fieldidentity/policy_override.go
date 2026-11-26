package fieldidentity

import (
	"slices"
	"strconv"
	"time"

	"github.com/flowcatalyst/go-sdk/usecase"
)

// Per-application overrides of a client's field policy
// (docs/field-policy-application-overrides.md). The effective policy of a
// session is the platform default, overlaid by the client's policy, overlaid
// by the override of the session's application, except for a member the
// client has locked.

// PolicyMember is an overridable member of the field policy, by its wire name.
type PolicyMember string

const (
	MemberUnlockKeysEnabled     PolicyMember = "unlockKeysEnabled"
	MemberRequireEnrolledDevice PolicyMember = "requireEnrolledDevice"
	MemberBadgeOnlyLogin        PolicyMember = "badgeOnlyLogin"
	MemberSharedIdleSecs        PolicyMember = "sharedIdleSecs"
	MemberPersonalIdleSecs      PolicyMember = "personalIdleSecs"
	MemberShiftMaxSecs          PolicyMember = "shiftMaxSecs"
	MemberUnenrolledIdleSecs    PolicyMember = "unenrolledIdleSecs"
	MemberPhotosEnabled         PolicyMember = "photosEnabled"
)

// OverridableMembers are the members an application may override, in the
// order they are shown (section 2 of the document).
func OverridableMembers() []PolicyMember {
	return []PolicyMember{
		MemberUnlockKeysEnabled, MemberRequireEnrolledDevice, MemberBadgeOnlyLogin,
		MemberSharedIdleSecs, MemberPersonalIdleSecs, MemberShiftMaxSecs, MemberUnenrolledIdleSecs,
		MemberPhotosEnabled,
	}
}

// Valid reports whether m is an overridable member.
func (m PolicyMember) Valid() bool { return slices.Contains(OverridableMembers(), m) }

// Bool reports whether the member holds a boolean (else a number of seconds).
func (m PolicyMember) Bool() bool {
	switch m {
	case MemberUnlockKeysEnabled, MemberRequireEnrolledDevice, MemberBadgeOnlyLogin, MemberPhotosEnabled:
		return true
	case MemberSharedIdleSecs, MemberPersonalIdleSecs, MemberShiftMaxSecs, MemberUnenrolledIdleSecs:
		return false
	}
	return false
}

// bounds are the member's range, the client-wide policy's (PutPolicyCommand.Build).
func (m PolicyMember) bounds() (lo, hi int) {
	switch m {
	case MemberShiftMaxSecs:
		return 3600, 86400
	case MemberSharedIdleSecs, MemberPersonalIdleSecs, MemberUnenrolledIdleSecs:
		return 60, 86400
	case MemberUnlockKeysEnabled, MemberRequireEnrolledDevice, MemberBadgeOnlyLogin, MemberPhotosEnabled:
	}
	return 0, 0
}

// value is the member's value in p (a bool or an int).
func (m PolicyMember) value(p Policy) any {
	switch m {
	case MemberUnlockKeysEnabled:
		return p.UnlockKeysEnabled
	case MemberRequireEnrolledDevice:
		return p.RequireEnrolledDevice
	case MemberBadgeOnlyLogin:
		return p.BadgeOnlyLogin
	case MemberSharedIdleSecs:
		return p.SharedIdleSecs
	case MemberPersonalIdleSecs:
		return p.PersonalIdleSecs
	case MemberShiftMaxSecs:
		return p.ShiftMaxSecs
	case MemberUnenrolledIdleSecs:
		return p.UnenrolledIdleSecs
	case MemberPhotosEnabled:
		return p.PhotosEnabled
	}
	return nil
}

// apply sets the member's value v (a bool or an int) on p.
func (m PolicyMember) apply(p *Policy, v any) {
	b, _ := v.(bool)
	n, _ := v.(int)
	switch m {
	case MemberUnlockKeysEnabled:
		p.UnlockKeysEnabled = b
	case MemberRequireEnrolledDevice:
		p.RequireEnrolledDevice = b
	case MemberBadgeOnlyLogin:
		p.BadgeOnlyLogin = b
	case MemberSharedIdleSecs:
		p.SharedIdleSecs = n
	case MemberPersonalIdleSecs:
		p.PersonalIdleSecs = n
	case MemberShiftMaxSecs:
		p.ShiftMaxSecs = n
	case MemberUnenrolledIdleSecs:
		p.UnenrolledIdleSecs = n
	case MemberPhotosEnabled:
		p.PhotosEnabled = b
	}
}

// PolicyOverride is one application's override for a client: the members it
// sets, by member (a bool or an int); a member absent follows the client.
type PolicyOverride struct {
	ClientID      string
	ApplicationID string
	Values        map[PolicyMember]any
	UpdatedAt     time.Time
	UpdatedBy     string
}

// Get is the override's value of m, if it sets one.
func (o *PolicyOverride) Get(m PolicyMember) (any, bool) {
	if o == nil {
		return nil, false
	}
	v, ok := o.Values[m]
	return v, ok
}

// Empty reports whether the override sets nothing.
func (o *PolicyOverride) Empty() bool { return o == nil || len(o.Values) == 0 }

// CheckOverrideValue refuses a value of the wrong kind or out of the member's
// range (422 POLICY_INVALID naming the member).
func CheckOverrideValue(m PolicyMember, v any) error {
	bad := func(msg string) error {
		return usecase.Validation("POLICY_INVALID", string(m)+" "+msg).WithDetails(map[string]any{"field": string(m)})
	}
	if !m.Valid() {
		return usecase.Validation("POLICY_INVALID", string(m)+" cannot be overridden by an application").
			WithDetails(map[string]any{"field": string(m)})
	}
	if m.Bool() {
		if _, ok := v.(bool); !ok {
			return bad("must be true or false")
		}
		return nil
	}
	n, ok := v.(int)
	lo, hi := m.bounds()
	if !ok || n < lo || n > hi {
		return bad("must be between " + strconv.Itoa(lo) + " and " + strconv.Itoa(hi))
	}
	return nil
}

// ForApplication is the effective policy for an application: p overlaid by
// the override's members the client has not locked.
func (p Policy) ForApplication(o *PolicyOverride) Policy {
	out := p
	if o.Empty() {
		return out
	}
	for _, m := range OverridableMembers() {
		if v, ok := o.Get(m); ok && !p.Locked(m) {
			m.apply(&out, v)
		}
	}
	return out
}

// Locked reports whether the client has locked m.
func (p Policy) Locked(m PolicyMember) bool { return slices.Contains(p.LockedMembers, m) }

// MemberSource is where a member's effective value comes from.
type MemberSource string

const (
	SourceDefault     MemberSource = "DEFAULT"
	SourceClient      MemberSource = "CLIENT"
	SourceApplication MemberSource = "APPLICATION"
)

// MemberState is one overridable member as an application sees it (D3).
type MemberState struct {
	Member    PolicyMember
	Effective any
	Source    MemberSource
	Locked    bool
	// Override is the application's own value, nil when it follows the client.
	Override any
	// Overruled is true when the application has an override that does not
	// apply: the client locked the member, or the application is not active
	// and enabled for the client.
	Overruled bool
}

// MemberStates are the overridable members of the client's policy p under
// the application's override o; applies is false when the application is not
// active and enabled for the client (its override then applies nowhere).
func MemberStates(p Policy, o *PolicyOverride, applies bool) []MemberState {
	eff := p
	if applies {
		eff = p.ForApplication(o)
	}
	out := make([]MemberState, 0, len(OverridableMembers()))
	for _, m := range OverridableMembers() {
		st := MemberState{Member: m, Effective: m.value(eff), Locked: p.Locked(m), Source: SourceClient}
		if p.IsDefault {
			st.Source = SourceDefault
		}
		if v, ok := o.Get(m); ok {
			st.Override = v
			if st.Locked || !applies {
				st.Overruled = true
			} else {
				st.Source = SourceApplication
			}
		}
		out = append(out, st)
	}
	return out
}

// UnlockKeysOnSomewhere reports whether any application of the client has
// unlock keys on under the client's policy p and its applications' overrides:
// the client's own switch (the default of every application that follows
// it), or an override that sets it on and is not locked. While it is true the
// client's keys are kept.
func UnlockKeysOnSomewhere(p Policy, overrides []PolicyOverride) bool {
	if p.UnlockKeysEnabled {
		return true
	}
	if p.Locked(MemberUnlockKeysEnabled) {
		return false
	}
	for n := range overrides {
		if v, ok := overrides[n].Get(MemberUnlockKeysEnabled); ok && v == true {
			return true
		}
	}
	return false
}

// unlockKeyApplications splits the client's applications with an override of
// unlockKeysEnabled that applies: those it sets on and those it sets off.
func unlockKeyApplications(p Policy, overrides []PolicyOverride) (on, off []string) {
	on, off = []string{}, []string{}
	if p.Locked(MemberUnlockKeysEnabled) {
		return on, off
	}
	for n := range overrides {
		v, ok := overrides[n].Get(MemberUnlockKeysEnabled)
		switch {
		case !ok:
		case v == true:
			on = append(on, overrides[n].ApplicationID)
		default:
			off = append(off, overrides[n].ApplicationID)
		}
	}
	return on, off
}
