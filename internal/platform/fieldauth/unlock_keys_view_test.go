package fieldauth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
)

// unlockKeysFor over every case of docs/field-unlock-keys-spec.md section 2.3.
func TestUnlockKeysFor(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)
	on := fieldidentity.Policy{UnlockKeysEnabled: true, UnlockKeyAttestation: fieldidentity.UnlockAttestationNone}
	required := on
	required.UnlockKeyAttestation = fieldidentity.UnlockAttestationRequired
	personal := &fieldidentity.Device{ID: "fdv_p", Mode: fieldidentity.ModePersonal, Status: fieldidentity.DeviceActive}
	shared := &fieldidentity.Device{ID: "fdv_s", Mode: fieldidentity.ModeShared, Status: fieldidentity.DeviceActive}
	suspended := &fieldidentity.Device{ID: "fdv_p", Mode: fieldidentity.ModePersonal, Status: fieldidentity.DeviceSuspended}
	pinSession := func(dev string, ago time.Duration) *fieldidentity.Session {
		return &fieldidentity.Session{DeviceID: dev, AMR: []string{"pin"}, AuthTime: now.Add(-ago)}
	}
	secs := func(n int64) *int64 { return &n }
	cases := map[string]struct {
		pol  fieldidentity.Policy
		dev  *fieldidentity.Device
		sess *fieldidentity.Session
		want unlockKeysRef
	}{
		"off":               {fieldidentity.Policy{}, personal, pinSession("fdv_p", 0), unlockKeysRef{}},
		"on, fresh PIN":     {on, personal, pinSession("fdv_p", time.Minute), unlockKeysRef{Allowed: true, RegisterExpiresIn: secs(540)}},
		"REQUIRED":          {required, personal, pinSession("fdv_p", 0), unlockKeysRef{Allowed: true, AttestationRequired: true, RegisterExpiresIn: secs(600)}},
		"window passed":     {on, personal, pinSession("fdv_p", 10*time.Minute), unlockKeysRef{Allowed: true}},
		"unenrolled":        {required, nil, pinSession("", 0), unlockKeysRef{}},
		"shared":            {required, shared, pinSession("fdv_s", 0), unlockKeysRef{}},
		"suspended device":  {on, suspended, pinSession("fdv_p", 0), unlockKeysRef{}},
		"session elsewhere": {on, personal, pinSession("fdv_other", 0), unlockKeysRef{}},
		"act-as": {on, personal, &fieldidentity.Session{
			DeviceID: "fdv_p", AMR: []string{"pin", "imp"}, AuthTime: now,
			ActorType: fieldidentity.ActorFieldWorker, ActorID: "fwu_sup",
		}, unlockKeysRef{}},
		"unlock session": {
			on, personal, &fieldidentity.Session{DeviceID: "fdv_p", AMR: []string{"hwk", "uv"}, AuthTime: now},
			unlockKeysRef{Allowed: true},
		},
		"PIN change pending": {on, personal, &fieldidentity.Session{
			DeviceID: "fdv_p", AMR: []string{"pin"}, AuthTime: now,
			PINChangeRequired: true,
		}, unlockKeysRef{Allowed: true}},
	}
	for name, tc := range cases {
		assert.Equal(t, tc.want, unlockKeysFor(tc.pol, tc.dev, tc.sess, now), name)
	}
}
