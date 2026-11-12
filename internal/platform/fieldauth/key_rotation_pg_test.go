//go:build integration

package fieldauth

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity/pin"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// After FLOWCATALYST_APP_KEY is rotated (the old key moved to
// FLOWCATALYST_APP_KEY_PREVIOUS) every stored PIN keeps working, and each is
// re-hashed under the new key's pepper on the worker's next sign-in, so the
// old key can later be dropped without breaking anyone who has signed in since.
func TestPINsSurviveAnAppKeyRotationAndMigrateOnSignIn(t *testing.T) {
	e := newEnv(t, envOpts{limits: roomyLimits()})
	oldHasher := e.s.Ops.Hasher
	withID := e.worker("kr-1", goodPIN, false, e.codeA)
	noID := e.worker("kr-2", goodPIN, false, e.codeA)
	assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_identities WHERE id = $1 AND pin_pepper_id = $2`, withID, oldHasher.PepperID()),
		"a new PIN hash records its pepper id")
	// kr-2's hash predates pepper ids.
	_, err := testpg.Pool(t).Exec(context.Background(), `UPDATE field_identities SET pin_pepper_id = NULL WHERE id = $1`, noID)
	require.NoError(t, err)
	d := e.enrolled(fieldidentity.ModeShared, e.codeA)

	rotated, err := pin.NewHasher("field-auth-rotated-app-key", "field-auth-test-app-key")
	require.NoError(t, err)
	e.s.Ops.Hasher = rotated

	for _, w := range []struct{ code, id string }{{"kr-1", withID}, {"kr-2", noID}} {
		before := e.pinHash(w.id)
		assert.Equal(t, http.StatusUnauthorized, d.login(e.codeA, w.code, "000111").Code, w.code+": a wrong PIN is still wrong")
		assert.Equal(t, before, e.pinHash(w.id), w.code+": a failed sign-in re-hashes nothing")
		mustLogin(t, d, e.codeA, w.code, goodPIN)
		assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_identities WHERE id = $1 AND pin_pepper_id = $2 AND pin_hash <> $3`,
			w.id, rotated.PepperID(), before), w.code+": re-hashed under the current pepper")
	}

	// The old key can now go: both PINs verify under the new key alone.
	e.s.Ops.Hasher, err = pin.NewHasher("field-auth-rotated-app-key")
	require.NoError(t, err)
	mustLogin(t, d, e.codeA, "kr-1", goodPIN)
	mustLogin(t, d, e.codeA, "kr-2", goodPIN)
}

// A PIN whose key was dropped before the worker used it cannot be checked. It
// is not a wrong guess: the answer is 403 PIN_RESET_REQUIRED on every path that
// verifies a PIN, and nothing counts against the worker.
func TestPINUnderADroppedKeyIsPINResetRequiredAndNotCounted(t *testing.T) {
	retired := func(t *testing.T, r resp) {
		t.Helper()
		assert.Equal(t, http.StatusForbidden, r.Code, r.Body.String())
		assert.Equal(t, "PIN_RESET_REQUIRED", r.str("error"))
	}
	noFailures := func(e *env, id string) {
		assert.Equal(t, 1, e.q(`SELECT COUNT(*) FROM field_identities WHERE id = $1 AND failed_since_success = 0 AND NOT hard_locked AND locked_until IS NULL`, id))
	}
	other := func(e *env) {
		var err error
		e.s.Ops.Hasher, err = pin.NewHasher("field-auth-other-app-key")
		require.NoError(t, err)
	}

	t.Run("login", func(t *testing.T) {
		e := newEnv(t, envOpts{limits: roomyLimits()})
		id := e.worker("kr-3", goodPIN, false, e.codeA)
		d := e.enrolled(fieldidentity.ModeShared, e.codeA)
		other(e)
		retired(t, d.login(e.codeA, "kr-3", goodPIN))
		noFailures(e, id)
	})
	t.Run("change PIN", func(t *testing.T) {
		e := newEnv(t, envOpts{limits: roomyLimits()})
		id := e.worker("kr-4", goodPIN, false, e.codeA)
		d := e.enrolled(fieldidentity.ModeShared, e.codeA)
		in := mustLogin(t, d, e.codeA, "kr-4", goodPIN)
		other(e)
		retired(t, d.callBearer("/field/auth/pin", map[string]any{"currentPin": goodPIN, "newPin": "739204"}, in.access))
		noFailures(e, id)
	})
	t.Run("act as", func(t *testing.T) {
		e := newEnv(t, envOpts{limits: roomyLimits()})
		supID := e.supervisor("kr-sup", goodPIN, e.codeA)
		e.worker("kr-5", goodPIN, false, e.codeA)
		d := e.enrolled(fieldidentity.ModePersonal, e.codeA)
		sup := mustLogin(t, d, e.codeA, "kr-sup", goodPIN)
		other(e)
		retired(t, d.callBearer("/field/auth/act-as", map[string]any{"workerCode": "kr-5", "pin": goodPIN}, sup.access))
		noFailures(e, supID)
	})
}

func (e *env) pinHash(id string) string {
	e.t.Helper()
	var h string
	require.NoError(e.t, testpg.Pool(e.t).QueryRow(context.Background(),
		`SELECT pin_hash FROM field_identities WHERE id = $1`, id).Scan(&h))
	return h
}
