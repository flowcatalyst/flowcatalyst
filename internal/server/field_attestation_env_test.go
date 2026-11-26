package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The attestation key status list decides which unlock keys are revoked: the
// platform refuses to start with an address that is not https while the list
// is on (FC_FIELD_ATTESTATION_STATUS_URL), and checks nothing while it is off.
func TestCheckStatusURL(t *testing.T) {
	t.Parallel()
	for raw, ok := range map[string]bool{
		"https://android.googleapis.com/attestation/status": true,
		"https://mirror.example.com/status":                 true,
		"http://mirror.example.com/status":                  false,
		"http://127.0.0.1:9000/status":                      false,
		"mirror.example.com/status":                         false,
	} {
		err := checkStatusURL(true, raw)
		assert.Equal(t, ok, err == nil, raw)
		if err != nil {
			assert.Contains(t, err.Error(), "FC_FIELD_ATTESTATION_STATUS_URL", raw)
		}
	}
	assert.NoError(t, checkStatusURL(false, "http://anything"), "the list is off")
}
