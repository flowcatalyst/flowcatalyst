//go:build integration

package control

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
	"github.com/flowcatalyst/go-sdk/webhook"
)

// seedPlainServiceAccount inserts an account linked to NO application — the
// np `bevco-integration` shape.
func seedPlainServiceAccount(t *testing.T, code, secret string, active bool) *serviceaccount.ServiceAccount {
	t.Helper()
	pool := testpg.Pool(t)
	sa := seedServiceAccount(t, pool, serviceaccount.NewRepository(pool), "", code, secret)
	_, err := pool.Exec(context.Background(), `UPDATE iam_service_accounts SET application_id = NULL, active = $2 WHERE id = $1`, sa.ID, active)
	require.NoError(t, err)
	return sa
}

func secretOf(t *testing.T, s *State, pool string, fn *function.Function) string {
	t.Helper()
	doc, err := s.buildDesired(context.Background(), pool, wasmJS)
	require.NoError(t, err)
	for _, f := range doc.Functions {
		if f.ID == fn.ID {
			return f.WebhookSecret
		}
	}
	t.Fatalf("function %s not in the desired document", fn.Address)
	return ""
}

// TestDesired_WebhookSecret_FunctionAccountVsApplicationFallback: the secret a
// runner verifies with is the function's own service account's when set;
// otherwise the application's (unchanged); an inactive or missing own account
// yields an empty secret and never falls through to the application's.
func TestDesired_WebhookSecret_FunctionAccountVsApplicationFallback(t *testing.T) {
	t.Parallel()
	s := newTestState(t)
	pool := "pool-" + t.Name()
	appID := "app_" + shortID(t)
	appCode := "wsapp" + shortID(t)
	seedApplication(t, testpg.Pool(t), appID, appCode)
	seedServiceAccount(t, testpg.Pool(t), serviceaccount.NewRepository(testpg.Pool(t)), appID, "appsa"+shortID(t), "app-secret")
	own := seedPlainServiceAccount(t, "ownsa"+shortID(t), "own-secret", true)
	off := seedPlainServiceAccount(t, "offsa"+shortID(t), "off-secret", false)

	build := func(name string, sa *string) *function.Function {
		fn := seedFunction(t, testpg.Pool(t), s.Repo, appID, appCode, name, func(f *function.Function) {
			f.Pool = &pool
			f.ServiceAccountID = sa
		})
		v := seedVersion(t, s.Repo, fn.ID, 1, function.VersionReady, "wasm", describeJSON(t))
		seedAlias(t, s.Repo, fn.ID, "live", v.ID)
		return fn
	}
	fallback := build("fallback-fn", nil)
	ownFn := build("own-fn", &own.ID)
	offFn := build("off-fn", &off.ID)
	ghost := "sac_doesnotexist"
	ghostFn := build("ghost-fn", &ghost)

	assert.Equal(t, "app-secret", secretOf(t, s, pool, fallback), "unset: the application's account, unchanged")
	assert.Equal(t, "own-secret", secretOf(t, s, pool, ownFn), "set: the function's own account")
	assert.Equal(t, "", secretOf(t, s, pool, offFn), "inactive own account: empty, no fall-through to the application's")
	assert.Equal(t, "", secretOf(t, s, pool, ghostFn), "missing own account: empty")
}

// TestWebhookDelivery_VerifiesWithFunctionAccount is the np failure end to
// end: the application has NO linked service account, a delivery is signed by
// account X (exactly as the connection-signed dispatch path resolves it, by
// id), and the secret the runner receives in the desired document verifies it
// — where, before the function carried its own account, the secret was empty
// and every delivery was 401 INVALID_SIGNATURE.
func TestWebhookDelivery_VerifiesWithFunctionAccount(t *testing.T) {
	t.Parallel()
	s := newTestState(t)
	pool := "pool-" + t.Name()
	appID := "app_" + shortID(t)
	appCode := "npapp" + shortID(t)
	seedApplication(t, testpg.Pool(t), appID, appCode) // no linked service account
	x := seedPlainServiceAccount(t, "bevco-integ"+shortID(t), "whsec_account_x", true)

	fn := seedFunction(t, testpg.Pool(t), s.Repo, appID, appCode, "hook", func(f *function.Function) { f.Pool = &pool })
	v := seedVersion(t, s.Repo, fn.ID, 1, function.VersionReady, "wasm", describeJSON(t))
	seedAlias(t, s.Repo, fn.ID, "live", v.ID)

	// The platform signs with account X, resolved by id like the delivery path.
	creds, err := serviceaccount.NewCachedOutboundCredsByIDResolver(serviceaccount.NewRepository(testpg.Pool(t)), time.Minute)(context.Background(), x.ID)
	require.NoError(t, err)
	require.NotEmpty(t, creds.SigningSecret)
	body := []byte(`{"order":1}`)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := webhook.NewValidator(creds.SigningSecret).ComputeSignature(ts, body)

	// Before: no account on the function, none on the application: empty
	// secret, the runner cannot verify.
	before := secretOf(t, s, pool, fn)
	assert.Empty(t, before)
	assert.Error(t, webhook.NewValidator(before).Validate(sig, ts, body))

	// After: the function names account X.
	fn.ServiceAccountID = &x.ID
	withTx(t, testpg.Pool(t), func(tx *usecasepgx.DbTx) error {
		return s.Repo.Persist(context.Background(), fn, tx)
	})
	after := secretOf(t, s, pool, fn)
	require.Equal(t, "whsec_account_x", after)
	assert.NoError(t, webhook.NewValidator(after).Validate(sig, ts, body), "a delivery signed by X verifies on the runner")
	assert.Error(t, webhook.NewValidator(after).Validate(webhook.NewValidator("someone-else").ComputeSignature(ts, body), ts, body), "another account's signature does not")
}
