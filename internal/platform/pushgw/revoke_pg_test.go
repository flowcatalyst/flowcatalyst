//go:build integration

package pushgw

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
)

const authExpired = "event: auth-expired\ndata: {}\n\n"

// fieldStream opens a field worker's stream on node `on`, subscribed through `via`.
func fieldStream(t *testing.T, w *world, via, on *node, f *fieldWorker) *stream {
	t.Helper()
	topic := "alpha:worker:" + f.worker + ":jobs"
	res := via.subscribe(subscribeBody(topic), f.header(t, "POST", pushPath+"/subscriptions"))
	require.Equal(t, 200, res.status, string(res.body))
	s := on.listen(res.json()["streamToken"].(string), "")
	s.greeting()
	return s
}

// Revoking a field login family ends its streams on every gateway task, whichever process revoked:
// the trigger on field_sessions notifies, each task ends the family's streams through the push log.
func TestFieldRevocationEndsStreamsAcrossTasks(t *testing.T) {
	w := newWorld(t, "alpha")
	w.rule("alpha", "alpha:worker:{principal}:jobs", false)
	a, b := w.start(), w.start()

	for name, revoke := range map[string]string{
		"logout":      `UPDATE field_sessions SET revoked_at = NOW(), revoked_reason = 'LOGOUT' WHERE family_id = $1`,
		"admin":       `UPDATE field_sessions SET revoked_at = NOW(), revoked_reason = 'ADMIN' WHERE family_id = $1`,
		"row deleted": `DELETE FROM field_sessions WHERE family_id = $1`,
	} {
		t.Run(name, func(t *testing.T) {
			f := w.fieldWorker("alpha", "clt_acme")
			s := fieldStream(t, w, a, b, f) // subscribed on one task, held on the other
			_, err := w.pool.Exec(t.Context(), revoke, f.family)
			require.NoError(t, err)
			frames := s.framesUntilClosed(5 * time.Second)
			require.NotEmpty(t, frames, "the stream ends with auth-expired")
			assert.Equal(t, authExpired, frames[len(frames)-1])
			assert.True(t, s.ended(time.Second))
		})
	}

	t.Run("a refresh rotation keeps the family's streams", func(t *testing.T) {
		f := w.fieldWorker("alpha", "clt_acme")
		other := w.fieldWorker("alpha", "clt_acme") // revoked meanwhile: proves the channel works
		s, so := fieldStream(t, w, a, b, f), fieldStream(t, w, a, b, other)
		_, err := w.pool.Exec(t.Context(),
			`UPDATE field_sessions SET revoked_at = NOW(), rotated_at = NOW(), revoked_reason = 'ROTATED' WHERE family_id = $1`, f.family)
		require.NoError(t, err)
		_, err = w.pool.Exec(t.Context(),
			`UPDATE field_sessions SET revoked_at = NOW(), revoked_reason = 'LOGOUT' WHERE family_id = $1`, other.family)
		require.NoError(t, err)
		require.True(t, so.ended(5*time.Second))
		assert.False(t, s.ended(300*time.Millisecond), "a rotation is not an end")
		publishTo(t, a, "clt_acme", "alpha:worker:"+f.worker+":jobs", "job.assigned", map[string]any{"n": 1})
		assert.Contains(t, s.nextEvent(), "event: job.assigned")
	})
}

// A platform sign-out (NotifyPrincipalEnd from any process) ends that principal's streams on every
// task and no other principal's.
func TestSignOutEndsThePrincipalsStreams(t *testing.T) {
	w := newWorld(t, "alpha")
	w.rule("alpha", "alpha:feed:{id}", false, "p:use")
	a, b := w.start(), w.start()
	open := func() (*stream, string) {
		tok, pid := w.bearer(who{scope: principal.ScopeClient, clients: []string{"clt_acme"}, perms: []string{"p:use"}})
		res := a.subscribe(subscribeBody("alpha:feed:1"), bearerHeader(tok))
		require.Equal(t, 200, res.status, string(res.body))
		s := b.listen(res.json()["streamToken"].(string), "")
		s.greeting()
		return s, pid
	}
	out, outID := open()
	stays, _ := open()

	require.NoError(t, NotifyPrincipalEnd(t.Context(), w.pool, outID))

	frames := out.framesUntilClosed(5 * time.Second)
	require.NotEmpty(t, frames)
	assert.Equal(t, authExpired, frames[len(frames)-1])
	assert.False(t, stays.ended(300*time.Millisecond))
	publishTo(t, a, "clt_acme", "alpha:feed:1", "x.y", map[string]any{})
	assert.True(t, strings.HasPrefix(stays.nextEvent(), "id: "))
}

// A sign-out with no principal (no valid cookie) announces nothing.
func TestNotifyPrincipalEndIgnoresNobody(t *testing.T) {
	w := newWorld(t)
	require.NoError(t, NotifyPrincipalEnd(t.Context(), w.pool, ""))
}
