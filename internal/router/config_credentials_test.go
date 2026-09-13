package router

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

// stubTokens stands in for the client-credentials manager.
type stubTokens struct {
	token        atomic.Value // string
	mints        atomic.Int32
	invalidated  atomic.Int32
	failWithText atomic.Value // string; non-empty = minting fails
}

func newStubTokens(tok string) *stubTokens {
	s := &stubTokens{}
	s.token.Store(tok)
	s.failWithText.Store("")
	return s
}

func (s *stubTokens) Token(context.Context) (string, error) {
	if msg, _ := s.failWithText.Load().(string); msg != "" {
		return "", assertError(msg)
	}
	s.mints.Add(1)
	tok, _ := s.token.Load().(string)
	return tok, nil
}

func (s *stubTokens) Invalidate() {
	s.invalidated.Add(1)
	s.token.Store("re-minted")
}

type assertError string

func (e assertError) Error() string { return string(e) }

// configServer records the Authorization header it was sent.
func configServer(t *testing.T, queueName string, status *atomic.Int32) (*httptest.Server, *atomic.Value) {
	t.Helper()
	seen := &atomic.Value{}
	seen.Store("")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.Store(r.Header.Get("Authorization"))
		if code := int(status.Load()); code != 0 && code != http.StatusOK {
			w.WriteHeader(code)
			return
		}
		_ = json.NewEncoder(w).Encode(common.RouterConfig{
			Queues: []common.QueueConfig{fakeQueueCfg(queueName)},
		})
	}))
	t.Cleanup(srv.Close)
	return srv, seen
}

// The credential belongs to ONE platform. A deployed FLOWCATALYST_CONFIG_URL
// lists third-party config services beside the platform's own document, so
// which URL may receive the token cannot be positional — it is decided by
// origin, and every other origin is fetched exactly as before.
func TestConfigSource_BearerGoesOnlyToThePlatformOrigin(t *testing.T) {
	var ok atomic.Int32
	platform, platformSaw := configServer(t, "q-platform", &ok)
	thirdParty, thirdPartySaw := configServer(t, "q-third-party", &ok)

	cs := NewConfigSource(platform.URL + "," + thirdParty.URL)
	cs.MaxAttempts = 1
	tokens := newStubTokens("tok-1")
	cs.SetCredentials(tokens, platform.URL)

	_, err := cs.Fetch(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "Bearer tok-1", platformSaw.Load(), "the platform's own document must carry the credential")
	assert.Equal(t, "", thirdPartySaw.Load(), "another origin must never receive the credential")
}

// A path on the platform URL must not change the decision, and a different
// port must.
func TestConfigSource_OriginMatchIgnoresPathAndRespectsPort(t *testing.T) {
	var ok atomic.Int32
	platform, saw := configServer(t, "q-origin", &ok)

	cs := NewConfigSource(platform.URL + "/api/dispatch/router-config")
	cs.MaxAttempts = 1
	cs.SetCredentials(newStubTokens("tok-path"), platform.URL+"/")

	_, err := cs.Fetch(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "Bearer tok-path", saw.Load(), "the same origin with a different path still gets the token")

	// A different port is a different platform.
	other, otherSaw := configServer(t, "q-other-port", &ok)
	cs2 := NewConfigSource(other.URL)
	cs2.MaxAttempts = 1
	cs2.SetCredentials(newStubTokens("tok-nope"), platform.URL)
	_, err = cs2.Fetch(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "", otherSaw.Load(), "a different port must not receive the credential")
}

// A 401 is the one failure re-minting can fix: without invalidating, every
// attempt until the token expires fails identically.
func TestConfigSource_UnauthorizedInvalidatesTheCachedToken(t *testing.T) {
	var status atomic.Int32
	status.Store(http.StatusUnauthorized)
	platform, saw := configServer(t, "q-401", &status)

	cs := NewConfigSource(platform.URL)
	cs.MaxAttempts = 1
	tokens := newStubTokens("stale")
	cs.SetCredentials(tokens, platform.URL)

	_, err := cs.Fetch(context.Background())
	require.Error(t, err)
	require.Equal(t, int32(1), tokens.invalidated.Load(), "a 401 must drop the cached token")

	// The next attempt mints again and succeeds.
	status.Store(http.StatusOK)
	_, err = cs.Fetch(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "Bearer re-minted", saw.Load())
}

// Any other failing status is an ordinary attempt failure — nothing about the
// token, so the cached one is kept.
func TestConfigSource_OtherFailuresDoNotInvalidateTheToken(t *testing.T) {
	var status atomic.Int32
	status.Store(http.StatusInternalServerError)
	platform, _ := configServer(t, "q-500", &status)

	cs := NewConfigSource(platform.URL)
	cs.MaxAttempts = 1
	tokens := newStubTokens("good")
	cs.SetCredentials(tokens, platform.URL)

	_, err := cs.Fetch(context.Background())
	require.Error(t, err)
	assert.Equal(t, int32(0), tokens.invalidated.Load(), "a 500 says nothing about the token")
}

// A minting failure is an attempt failure like any transport failure: the
// fetch fails and the retry loop keeps trying, rather than panicking or
// fetching unauthenticated.
func TestConfigSource_MintFailureIsAnAttemptFailure(t *testing.T) {
	var ok atomic.Int32
	platform, saw := configServer(t, "q-mint-fail", &ok)

	cs := NewConfigSource(platform.URL)
	cs.MaxAttempts = 1
	tokens := newStubTokens("unused")
	tokens.failWithText.Store("token endpoint returned 503")
	cs.SetCredentials(tokens, platform.URL)

	_, err := cs.Fetch(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "all 1 source(s) failed")
	assert.Equal(t, "", saw.Load(), "the document must not be fetched unauthenticated when minting fails")
}

// With no credentials configured the source behaves exactly as it always did.
func TestConfigSource_NoCredentialsFetchesUnauthenticated(t *testing.T) {
	var ok atomic.Int32
	platform, saw := configServer(t, "q-anon", &ok)

	cs := NewConfigSource(platform.URL)
	cs.MaxAttempts = 1

	_, err := cs.Fetch(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "", saw.Load())
}
