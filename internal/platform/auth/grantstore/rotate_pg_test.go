//go:build integration

package grantstore_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/grantstore"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// mintRoot inserts a fresh root refresh token with full lineage metadata and
// returns its raw form plus the entity.
func mintRoot(t *testing.T, repo *grantstore.RefreshTokenRepository, principalID string, clientID *string, rootFamily bool) (string, *grantstore.RefreshToken) {
	t.Helper()
	raw, entity, err := grantstore.GenerateTokenPair(principalID)
	require.NoError(t, err)
	entity.OAuthClientID = clientID
	entity.Scopes = []string{"platform:messaging:event:view"}
	entity.AccessibleClients = []string{"clt_rotate_test1"}
	if rootFamily {
		fam := entity.ID
		entity.TokenFamily = &fam
	}
	require.NoError(t, repo.Insert(context.Background(), entity))
	return raw, entity
}

// TestRotate_FullContract pins the shared rotation core end-to-end against
// real Postgres: lineage preservation, replaced-by linking, and the BCP
// §4.14.2 reuse detection that kills the whole family when a rotated-out
// token is replayed.
func TestRotate_FullContract(t *testing.T) {
	ctx := context.Background()
	repo := grantstore.NewRefreshTokenRepository(testpg.Pool(t))
	cid := "oac_rotate_contract"
	raw0, entity0 := mintRoot(t, repo, "prn_rotatefull001", &cid, true)
	family := *entity0.TokenFamily

	// Rotation 1: scopes/client/family preserved on the replacement.
	res, err := grantstore.Rotate(ctx, repo, raw0, nil)
	require.NoError(t, err)
	require.NotNil(t, res.Stored, "valid token must rotate")
	assert.Equal(t, entity0.ID, res.Stored.ID)
	require.NotNil(t, res.New)
	assert.Equal(t, entity0.Scopes, res.New.Scopes, "scopes preserved")
	require.NotNil(t, res.New.OAuthClientID)
	assert.Equal(t, cid, *res.New.OAuthClientID, "client binding preserved")
	require.NotNil(t, res.New.TokenFamily)
	assert.Equal(t, family, *res.New.TokenFamily, "rotation family preserved")
	assert.NotEmpty(t, res.NewRaw)

	// The rotated-out token is revoked and linked to its replacement.
	prior, err := repo.FindByHash(ctx, grantstore.HashToken(raw0))
	require.NoError(t, err)
	require.NotNil(t, prior)
	assert.True(t, prior.Revoked, "presented token must be revoked")
	require.NotNil(t, prior.ReplacedBy, "replaced-by link must be recorded")
	assert.Equal(t, res.New.TokenHash, *prior.ReplacedBy)

	// Rotation 2 chains within the same family.
	res2, err := grantstore.Rotate(ctx, repo, res.NewRaw, nil)
	require.NoError(t, err)
	require.NotNil(t, res2.Stored)
	require.NotNil(t, res2.New.TokenFamily)
	assert.Equal(t, family, *res2.New.TokenFamily)

	// REPLAY raw0 (rotated out in step 1): reuse detection must refuse the
	// rotation AND revoke the entire family — including res2's still-valid
	// replacement.
	replay, err := grantstore.Rotate(ctx, repo, raw0, nil)
	require.NoError(t, err)
	assert.Nil(t, replay.Stored, "replayed rotated-out token must not rotate")
	survivor, err := repo.FindValidByHash(ctx, grantstore.HashToken(res2.NewRaw))
	require.NoError(t, err)
	assert.Nil(t, survivor, "every token in the family must be revoked after reuse detection")
}

// TestRotate_AuthorizeFailureRotatesNothing pins the binding-check contract:
// when the authorize hook rejects (e.g. wrong OAuth client), the presented
// token must stay valid — a failed caller must not burn the legitimate
// holder's token.
func TestRotate_AuthorizeFailureRotatesNothing(t *testing.T) {
	ctx := context.Background()
	repo := grantstore.NewRefreshTokenRepository(testpg.Pool(t))
	cid := "oac_rotate_authz"
	raw, _ := mintRoot(t, repo, "prn_rotateauthz01", &cid, true)

	sentinel := errors.New("binding mismatch")
	res, err := grantstore.Rotate(ctx, repo, raw, func(*grantstore.RefreshToken) error { return sentinel })
	require.ErrorIs(t, err, sentinel)
	require.NotNil(t, res.Stored, "stored token is surfaced so the caller can shape its response")

	still, err := repo.FindValidByHash(ctx, grantstore.HashToken(raw))
	require.NoError(t, err)
	require.NotNil(t, still, "token must remain valid after an authorize failure")
	assert.False(t, still.Revoked)
}

// TestRotate_InvalidTokenIsNilStored pins the "invalid or expired" shape: no
// error, nil Stored.
func TestRotate_InvalidTokenIsNilStored(t *testing.T) {
	ctx := context.Background()
	repo := grantstore.NewRefreshTokenRepository(testpg.Pool(t))
	res, err := grantstore.Rotate(ctx, repo, "never-issued-token", nil)
	require.NoError(t, err)
	assert.Nil(t, res.Stored)
	assert.Empty(t, res.NewRaw)
}

// TestRotate_LegacyTokenRootsFreshFamily pins the legacy branch: a token
// minted before family tracking (TokenFamily nil) gets a fresh family rooted
// at its replacement, so reuse detection works from the first rotation on.
func TestRotate_LegacyTokenRootsFreshFamily(t *testing.T) {
	ctx := context.Background()
	repo := grantstore.NewRefreshTokenRepository(testpg.Pool(t))
	raw, _ := mintRoot(t, repo, "prn_rotatelegacy1", nil, false /* no family */)

	res, err := grantstore.Rotate(ctx, repo, raw, nil)
	require.NoError(t, err)
	require.NotNil(t, res.Stored)
	require.NotNil(t, res.New.TokenFamily, "legacy rotation must root a family")
	assert.Equal(t, res.New.ID, *res.New.TokenFamily, "family roots at the replacement's ID")
}

// TestRotate_ConcurrentPresentationsExactlyOneWins pins the atomic-claim
// guarantee: N simultaneous presentations of ONE refresh token produce exactly
// one replacement; the family never forks. Losers are treated as replays
// (reuse detection), so afterwards the whole family is revoked.
func TestRotate_ConcurrentPresentationsExactlyOneWins(t *testing.T) {
	ctx := context.Background()
	repo := grantstore.NewRefreshTokenRepository(testpg.Pool(t))
	cid := "oac_rotate_race"
	raw, entity := mintRoot(t, repo, "prn_rotaterace001", &cid, true)

	const n = 12
	var wins, losses atomic.Int32
	var newRaws sync.Map
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			res, err := grantstore.Rotate(ctx, repo, raw, nil)
			if err != nil {
				t.Errorf("rotate error: %v", err)
				return
			}
			if res.New != nil {
				wins.Add(1)
				newRaws.Store(res.NewRaw, true)
			} else {
				losses.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()

	assert.EqualValues(t, 1, wins.Load(), "exactly one presentation may rotate")
	assert.EqualValues(t, n-1, losses.Load())

	// No fork: at most the winner's descendant ever existed, and a losing
	// replay revoked the family (reuse detection), so nothing is valid now.
	newRaws.Range(func(k, _ any) bool {
		v, err := repo.FindValidByHash(ctx, grantstore.HashToken(k.(string)))
		require.NoError(t, err)
		if v != nil {
			// Winner finished after every loser ran: the descendant is the
			// single live token of the family.
			assert.Equal(t, *entity.TokenFamily, *v.TokenFamily)
		}
		return true
	})
}
