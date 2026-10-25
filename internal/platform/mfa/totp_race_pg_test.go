//go:build integration

package mfa

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// TestVerifyTOTP_ConcurrentReplayExactlyOnePasses pins the atomic step claim:
// N simultaneous submissions of one valid code yield exactly one success.
func TestVerifyTOTP_ConcurrentReplayExactlyOnePasses(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	const pid = "prn_totprace00001" // 17 chars
	_, err := pool.Exec(ctx,
		`INSERT INTO iam_principals (id, type, scope, name, active, email)
		 VALUES ($1, 'USER', 'ANCHOR', 'Race User', TRUE, 'race@example.com')`, pid)
	require.NoError(t, err)

	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	enc, err := encryption.New(key)
	require.NoError(t, err)
	svc := NewService(NewRepository(pool), enc, nil, DefaultConfig())

	enr, err := svc.BeginTOTPEnrollment(ctx, pid, "race@example.com")
	require.NoError(t, err)
	// Enroll with the PREVIOUS step so the current step's code is still unused.
	prev, err := totp.GenerateCode(enr.Secret, time.Now().Add(-30*time.Second))
	require.NoError(t, err)
	ok, err := svc.ConfirmTOTPEnrollment(ctx, pid, prev)
	require.NoError(t, err)
	require.True(t, ok)

	code, err := totp.GenerateCode(enr.Secret, time.Now())
	require.NoError(t, err)

	const n = 12
	var passed atomic.Int32
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			<-start
			if ok, err := svc.VerifyTOTP(ctx, pid, code); err == nil && ok {
				passed.Add(1)
			}
		})
	}
	close(start)
	wg.Wait()
	require.EqualValues(t, 1, passed.Load(), "exactly one concurrent verification may pass")

	ok, err = svc.VerifyTOTP(ctx, pid, code)
	require.NoError(t, err)
	require.False(t, ok, "sequential replay still rejected")
}
