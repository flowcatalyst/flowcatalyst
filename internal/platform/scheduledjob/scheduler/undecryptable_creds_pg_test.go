//go:build integration

package scheduler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// A service account whose stored credential was encrypted under another key
// must not put ciphertext on the wire: the firing goes out with no
// Authorization header and no signature headers.
func TestDispatcherTick_UndecryptableCredentialIsNotSent(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	jobs := scheduledjob.NewRepository(pool)
	instances := scheduledjob.NewInstanceRepository(pool)
	uow := testpg.NewUoW(t)
	ec := testpg.TestEC()

	curKey, err := encryption.GenerateKey()
	require.NoError(t, err)
	t.Setenv("FLOWCATALYST_APP_KEY", curKey)
	repo := serviceaccount.NewRepository(pool)

	otherKey, err := encryption.GenerateKey()
	require.NoError(t, err)
	other, err := encryption.New(otherKey)
	require.NoError(t, err)
	tokCT, err := other.Encrypt("tok-secret")
	require.NoError(t, err)
	secCT, err := other.Encrypt("sig-secret")
	require.NoError(t, err)
	tokCT, secCT = "encrypted:"+tokCT, "encrypted:"+secCT

	const saID = "sa_sjundecrypt01"
	_, err = pool.Exec(ctx,
		`INSERT INTO iam_service_accounts (id, code, name, active, wh_auth_type, wh_auth_token_ref, wh_signing_secret_ref)
		 VALUES ($1, 'sj-undecrypt', 'SJ Undecrypt', true, 'BEARER_TOKEN', $2, $3)`, saID, tokCT, secCT)
	require.NoError(t, err)
	resolve := serviceaccount.NewCachedOutboundCredsByIDResolver(repo, time.Minute)

	got := make(chan http.Header, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.Header.Clone()
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	targetURL := server.URL

	appID := "app_sjundecrypt01"
	created, err := usecaseop.Run(testpg.OperatorCtx(), uow, operations.CreateScheduledJob(jobs), operations.CreateCommand{
		Code: "sjdsp-undecrypt", Name: "SJ Undecrypt", Crons: []string{"0 0 * * * *"},
		TargetURL: &targetURL, ApplicationID: &appID,
	}, ec)
	require.NoError(t, err)
	fired, err := usecaseop.Run(testpg.AnchorCtx(), uow, operations.FireNow(jobs, instances),
		operations.FireNowCommand{ID: created.ScheduledJobID}, ec)
	require.NoError(t, err)

	d := &dispatcher{
		cfg:       Config{DispatchInterval: time.Second, DispatchBatchSize: 32, HTTPTimeout: time.Second},
		jobs:      jobs,
		instances: instances,
		http:      &http.Client{Timeout: time.Second},
		isLeader:  func() bool { return true },
		creds: func(ctx context.Context, _ string) (serviceaccount.OutboundCreds, error) {
			return resolve(ctx, saID)
		},
	}
	require.NoError(t, d.tick(ctx))

	h := <-got
	assert.Empty(t, h.Get("Authorization"), "no bearer: the ciphertext must not be sent as one")
	assert.Empty(t, h.Get(signatureHeader))
	assert.Empty(t, h.Get(timestampHeader))
	for name, vals := range h {
		for _, v := range vals {
			assert.False(t, strings.Contains(v, tokCT) || strings.Contains(v, secCT) || strings.Contains(v, "encrypted:"),
				"header %s carries stored ciphertext", name)
		}
	}
	inst, err := instances.FindByID(ctx, fired.InstanceID)
	require.NoError(t, err)
	assert.Equal(t, scheduledjob.InstanceStatusDelivered, inst.Status)
}
