//go:build integration

package webauthn

import (
	"context"
	"testing"
	"time"

	gowebauthn "github.com/go-webauthn/webauthn/webauthn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// Ceremony state is single-use: the first consume wins, a replay of the same
// state id finds nothing.
func TestCeremonyState_SingleUse(t *testing.T) {
	ctx := context.Background()
	repo := NewCeremonyRepository(testpg.Pool(t))
	sess := &gowebauthn.SessionData{Challenge: "abc", UserID: []byte("prn_a")}

	require.NoError(t, repo.StoreAuthentication(ctx, "state-single", nil, sess))
	first, err := repo.ConsumeAuthentication(ctx, "state-single")
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, "abc", first.Session.Challenge)

	again, err := repo.ConsumeAuthentication(ctx, "state-single")
	require.NoError(t, err)
	assert.Nil(t, again, "a consumed ceremony state cannot be replayed")

	require.NoError(t, repo.StoreRegistration(ctx, "state-reg", "prn_a", sess, nil))
	r1, err := repo.ConsumeRegistration(ctx, "state-reg")
	require.NoError(t, err)
	require.NotNil(t, r1)
	r2, err := repo.ConsumeRegistration(ctx, "state-reg")
	require.NoError(t, err)
	assert.Nil(t, r2)
}

// A registration state id cannot be redeemed as an authentication state or
// vice versa (the row id carries the ceremony type).
func TestCeremonyState_TypesAreIsolated(t *testing.T) {
	ctx := context.Background()
	repo := NewCeremonyRepository(testpg.Pool(t))
	sess := &gowebauthn.SessionData{Challenge: "xyz"}
	require.NoError(t, repo.StoreRegistration(ctx, "state-iso", "prn_a", sess, nil))
	got, err := repo.ConsumeAuthentication(ctx, "state-iso")
	require.NoError(t, err)
	assert.Nil(t, got)
	still, err := repo.ConsumeRegistration(ctx, "state-iso")
	require.NoError(t, err)
	assert.NotNil(t, still, "the wrong-type attempt must not burn the real state")
}

// An expired state is not returned, and PurgeExpired removes it.
func TestCeremonyState_Expired(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := NewCeremonyRepository(pool)
	require.NoError(t, repo.StoreAuthentication(ctx, "state-exp", nil, &gowebauthn.SessionData{Challenge: "e"}))
	_, err := pool.Exec(ctx, `UPDATE oauth_oidc_payloads SET expires_at = $2 WHERE id = $1`,
		makeID(authenticationType, "state-exp"), time.Now().Add(-time.Minute))
	require.NoError(t, err)

	got, err := repo.ConsumeAuthentication(ctx, "state-exp")
	require.NoError(t, err)
	assert.Nil(t, got, "an expired ceremony state is not redeemable")

	n, err := repo.PurgeExpired(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, n, int64(1))
}
