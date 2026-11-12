package middleware

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/go-sdk/usecase"
)

func TestRequireRecentSignIn(t *testing.T) {
	window := 15 * time.Minute
	cookieCtx := func(age, win time.Duration) context.Context {
		ctx := WithSessionCookieAuth(context.Background())
		return WithSessionInfo(ctx, time.Now().Add(-age), win)
	}

	t.Run("a recent cookie session passes", func(t *testing.T) {
		require.NoError(t, RequireRecentSignIn(cookieCtx(time.Minute, window)))
	})
	t.Run("an older cookie session is refused 403 REAUTH_REQUIRED", func(t *testing.T) {
		err := RequireRecentSignIn(cookieCtx(20*time.Minute, window))
		require.Error(t, err)
		ue := usecase.AsError(err)
		require.NotNil(t, ue)
		require.Equal(t, "REAUTH_REQUIRED", ue.Code)
		require.Equal(t, "Sign in again to change your sign-in methods", ue.Message)
		require.Equal(t, 403, ue.HTTPStatus())
	})
	t.Run("a zero window disables the check", func(t *testing.T) {
		require.NoError(t, RequireRecentSignIn(cookieCtx(100*time.Hour, 0)))
	})
	t.Run("a bearer, an injected context and a bare context are not affected", func(t *testing.T) {
		require.NoError(t, RequireRecentSignIn(WithBearerAuth(context.Background())))
		require.NoError(t, RequireRecentSignIn(context.Background()))
		// session info without the cookie marker (nothing built it from the cookie)
		require.NoError(t, RequireRecentSignIn(WithSessionInfo(context.Background(), time.Now().Add(-100*time.Hour), window)))
	})
}
