//go:build integration

package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func beginIDs(t *testing.T, s *State, email string) []string {
	t.Helper()
	out, err := s.authenticateBegin(context.Background(), &authenticateBeginInput{Body: AuthenticateBeginRequest{Email: email}})
	require.NoError(t, err)
	raw, err := json.Marshal(out.Body.Options)
	require.NoError(t, err)
	var o struct {
		PublicKey struct {
			AllowCredentials []struct {
				ID string `json:"id"`
			} `json:"allowCredentials"`
		} `json:"publicKey"`
	}
	require.NoError(t, json.Unmarshal(raw, &o))
	var ids []string
	for _, c := range o.PublicKey.AllowCredentials {
		ids = append(ids, c.ID)
	}
	return ids
}

// L4 end to end: two authenticate/begin calls for the same unknown email list
// the same credential id; another email lists another; an account that really
// has a passkey still lists its own credential's id, stable across calls.
func TestAuthenticateBeginDecoyIsStableAgainstPostgres(t *testing.T) {
	r := newRig(t, "pgl4")
	r.s.Principals = principal.NewRepository(testpg.Pool(t))

	u1, u2 := beginIDs(t, r.s, "ghost1@pgl4.test"), beginIDs(t, r.s, "ghost1@pgl4.test")
	require.Len(t, u1, 1)
	assert.Equal(t, u1, u2, "an unknown email's decoy id changed between calls")
	assert.NotEqual(t, u1, beginIDs(t, r.s, "ghost2@pgl4.test"))

	credID, _ := r.register(r.alice)
	real1, real2 := beginIDs(t, r.s, "alice@pgl4.test"), beginIDs(t, r.s, "alice@pgl4.test")
	require.Len(t, real1, 1)
	assert.Equal(t, real1, real2)
	stored, err := r.s.Creds.FindByCredentialID(context.Background(), mustB64(t, real1[0]))
	require.NoError(t, err)
	require.NotNil(t, stored, "the real account's listed id must be its registered credential (%s)", credID)
	assert.NotEqual(t, u1, real1)
}

func mustB64(t *testing.T, s string) []byte {
	t.Helper()
	b, err := base64.RawURLEncoding.DecodeString(s)
	require.NoError(t, err)
	return b
}
