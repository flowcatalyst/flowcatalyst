package fieldidentity

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPolicyForApplication(t *testing.T) {
	t.Parallel()
	p := DefaultPolicy("clt_a")
	p.IsDefault = false
	o := &PolicyOverride{ApplicationID: "app_a", Values: map[PolicyMember]any{
		MemberUnlockKeysEnabled: true, MemberShiftMaxSecs: 3600, MemberPhotosEnabled: false,
	}}
	eff := p.ForApplication(o)
	assert.True(t, eff.UnlockKeysEnabled)
	assert.Equal(t, 3600, eff.ShiftMaxSecs)
	assert.False(t, eff.PhotosEnabled)
	assert.Equal(t, p.PINMinLength, eff.PINMinLength, "nothing else moves")
	assert.Equal(t, p, p.ForApplication(nil))

	p.LockedMembers = []PolicyMember{MemberShiftMaxSecs}
	eff = p.ForApplication(o)
	assert.Equal(t, p.ShiftMaxSecs, eff.ShiftMaxSecs, "a locked member keeps the client's value")
	st := MemberStates(p, o, true)
	for _, s := range st {
		switch s.Member {
		case MemberShiftMaxSecs:
			assert.Equal(t, MemberState{Member: s.Member, Effective: p.ShiftMaxSecs, Source: SourceClient, Locked: true, Override: 3600, Overruled: true}, s)
		case MemberUnlockKeysEnabled:
			assert.Equal(t, MemberState{Member: s.Member, Effective: true, Source: SourceApplication, Override: true}, s)
		case MemberRequireEnrolledDevice:
			assert.Equal(t, MemberState{Member: s.Member, Effective: false, Source: SourceClient}, s)
		case MemberBadgeOnlyLogin, MemberSharedIdleSecs, MemberPersonalIdleSecs, MemberUnenrolledIdleSecs, MemberPhotosEnabled:
		}
	}
	assert.Len(t, st, len(OverridableMembers()))
}

func TestUnlockKeysOnSomewhere(t *testing.T) {
	t.Parallel()
	off := DefaultPolicy("clt_a")
	on := off
	on.UnlockKeysEnabled = true
	appOn := []PolicyOverride{{ApplicationID: "app_a", Values: map[PolicyMember]any{MemberUnlockKeysEnabled: true}}}
	appOff := []PolicyOverride{{ApplicationID: "app_a", Values: map[PolicyMember]any{MemberUnlockKeysEnabled: false}}}
	assert.True(t, UnlockKeysOnSomewhere(on, nil))
	assert.True(t, UnlockKeysOnSomewhere(on, appOff), "the client's default still holds for the others")
	assert.False(t, UnlockKeysOnSomewhere(off, nil))
	assert.False(t, UnlockKeysOnSomewhere(off, appOff))
	assert.True(t, UnlockKeysOnSomewhere(off, appOn))
	locked := off
	locked.LockedMembers = []PolicyMember{MemberUnlockKeysEnabled}
	assert.False(t, UnlockKeysOnSomewhere(locked, appOn), "a lock overrules the override")
}

func TestCheckOverrideValue(t *testing.T) {
	t.Parallel()
	assert.NoError(t, CheckOverrideValue(MemberUnlockKeysEnabled, true))
	assert.Error(t, CheckOverrideValue(MemberUnlockKeysEnabled, 1))
	assert.NoError(t, CheckOverrideValue(MemberShiftMaxSecs, 3600))
	assert.Error(t, CheckOverrideValue(MemberShiftMaxSecs, 3599))
	assert.NoError(t, CheckOverrideValue(MemberPersonalIdleSecs, 86400))
	assert.Error(t, CheckOverrideValue(MemberPersonalIdleSecs, 86401))
	assert.Error(t, CheckOverrideValue(PolicyMember("pinMinLength"), 6))
	for _, m := range OverridableMembers() {
		assert.True(t, m.Valid(), m)
	}
}

func TestMemberStatesOfAnApplicationThatDoesNotServe(t *testing.T) {
	t.Parallel()
	p := DefaultPolicy("clt_a")
	o := &PolicyOverride{Values: map[PolicyMember]any{MemberUnlockKeysEnabled: true}}
	for _, s := range MemberStates(p, o, false) {
		if s.Member == MemberUnlockKeysEnabled {
			assert.Equal(t, MemberState{Member: s.Member, Effective: false, Source: SourceDefault, Override: true, Overruled: true}, s)
		}
	}
}
