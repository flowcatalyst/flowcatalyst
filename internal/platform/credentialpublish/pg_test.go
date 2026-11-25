//go:build integration

package credentialpublish

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/credentialpublish/credentialpublishtest"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/go-sdk/usecase"
)

func TestMain(m *testing.M) {
	key, err := encryption.GenerateKey()
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("FLOWCATALYST_APP_KEY", key)
	testpg.RunMain(m)
}

type env struct {
	pool *pgxpool.Pool
	enc  *encryption.Service
	repo *Repository
}

func newEnv(t *testing.T) env {
	t.Helper()
	pool := testpg.Pool(t)
	enc := encryption.MustFromEnv()
	require.NotNil(t, enc)
	// The worker claims whatever is due, so every test starts from empty
	// tables (nothing else writes them in this package's database).
	_, err := pool.Exec(context.Background(),
		`TRUNCATE iam_credential_publications, iam_credential_publication_items`)
	require.NoError(t, err)
	return env{pool: pool, enc: enc, repo: NewRepository(pool)}
}

// inTx runs fn in a transaction that commits when fn succeeds.
func (e env) inTx(t *testing.T, fn func(tx pgx.Tx) error) error {
	t.Helper()
	ctx := context.Background()
	tx, err := e.pool.Begin(ctx)
	require.NoError(t, err)
	if err := fn(tx); err != nil {
		_ = tx.Rollback(ctx)
		return err
	}
	return tx.Commit(ctx)
}

func (e env) queue(t *testing.T, owner Owner, p Pending) error {
	t.Helper()
	return e.inTx(t, func(tx pgx.Tx) error { return QueueInTx(context.Background(), tx, e.enc, owner, p) })
}

func saOwner() Owner { return Owner{Type: OwnerServiceAccount, ID: tsid.Generate(tsid.ServiceAccount)} }

func enable(base string) Pending {
	return Pending{Enable: &EnableRequest{BasePath: base, EnabledBy: "tester"}}
}

func str(s string) *string { return &s }

func (e env) items(t *testing.T, owner Owner) map[string]Item {
	t.Helper()
	_, items, err := e.repo.Status(context.Background(), owner)
	require.NoError(t, err)
	out := map[string]Item{}
	for _, it := range items {
		out[it.Credential] = it
	}
	return out
}

func (e env) pendingValue(t *testing.T, owner Owner, cred string) *string {
	t.Helper()
	var v *string
	require.NoError(t, e.pool.QueryRow(context.Background(),
		`SELECT pending_value FROM iam_credential_publication_items WHERE owner_type=$1 AND owner_id=$2 AND credential=$3`,
		string(owner.Type), owner.ID, cred).Scan(&v))
	return v
}

// seedLinked inserts a service account, its SERVICE principal and an OAuth
// client wired to that principal, returning the account and client row ids.
func (e env) seedLinked(t *testing.T) (sa, oac string) {
	t.Helper()
	ctx := context.Background()
	sa = tsid.Generate(tsid.ServiceAccount)
	principal := tsid.Generate(tsid.Principal)
	oac = tsid.Generate(tsid.OAuthClient)
	_, err := e.pool.Exec(ctx, `INSERT INTO iam_service_accounts (id, code, name) VALUES ($1, $2, 'cp')`, sa, "cp-"+sa)
	require.NoError(t, err)
	_, err = e.pool.Exec(ctx, `INSERT INTO iam_principals (id, type, name, service_account_id) VALUES ($1, 'SERVICE', 'cp', $2)`, principal, sa)
	require.NoError(t, err)
	_, err = e.pool.Exec(ctx,
		`INSERT INTO oauth_clients (id, client_id, client_name, client_type, service_account_principal_id)
		 VALUES ($1, $2, 'cp', 'CONFIDENTIAL', $3)`, oac, "cid-"+oac, principal)
	require.NoError(t, err)
	return sa, oac
}

func (e env) seedStandalone(t *testing.T) string {
	t.Helper()
	oac := tsid.Generate(tsid.OAuthClient)
	_, err := e.pool.Exec(context.Background(),
		`INSERT INTO oauth_clients (id, client_id, client_name, client_type) VALUES ($1, $2, 'cp', 'CONFIDENTIAL')`, oac, "cid-"+oac)
	require.NoError(t, err)
	return oac
}

func TestQueueEnableThenChanges(t *testing.T) {
	e := newEnv(t)
	o := saOwner()
	require.NoError(t, e.queue(t, o, Pending{
		Enable: &EnableRequest{BasePath: "/fc/service-accounts/x/", EnabledBy: "me"},
		Changes: []Change{
			{Credential: CredClientID},
			{Credential: CredClientSecret, Plaintext: str("s3cret")},
			{Credential: CredAuthToken},
		},
	}))
	pub, items, err := e.repo.Status(context.Background(), o)
	require.NoError(t, err)
	require.NotNil(t, pub)
	assert.Equal(t, "aws-ps", pub.Store)
	assert.Equal(t, "/fc/service-accounts/x/", pub.BasePath)
	assert.Equal(t, "me", pub.EnabledBy)
	require.Len(t, items, 3)
	assert.Equal(t, "auth-token", items[0].Credential, "ordered by credential")
	got := e.items(t, o)
	assert.Equal(t, "/fc/service-accounts/x/client-id", got[CredClientID].Path)
	assert.Equal(t, StatePendingPut, got[CredClientID].State)
	assert.NotNil(t, got[CredClientID].NextAttemptAt)

	// The client secret is held encrypted; the others carry no value.
	pv := e.pendingValue(t, o, CredClientSecret)
	require.NotNil(t, pv)
	assert.NotContains(t, *pv, "s3cret")
	dec, err := e.enc.Decrypt(*pv)
	require.NoError(t, err)
	assert.Equal(t, "s3cret", dec)
	assert.Nil(t, e.pendingValue(t, o, CredClientID))
}

func TestQueueRolledBackQueuesNothing(t *testing.T) {
	e := newEnv(t)
	o := saOwner()
	boom := errors.New("boom")
	err := e.inTx(t, func(tx pgx.Tx) error {
		require.NoError(t, QueueInTx(context.Background(), tx, e.enc, o, Pending{
			Enable:  &EnableRequest{BasePath: "/fc/a/"},
			Changes: []Change{{Credential: CredClientID}},
		}))
		return boom
	})
	require.ErrorIs(t, err, boom)
	pub, items, err := e.repo.Status(context.Background(), o)
	require.NoError(t, err)
	assert.Nil(t, pub)
	assert.Empty(t, items)
}

func TestQueueWithoutPublicationIgnoresChanges(t *testing.T) {
	e := newEnv(t)
	o := saOwner()
	require.NoError(t, e.queue(t, o, Pending{Changes: []Change{{Credential: CredAuthToken}}}))
	_, items, err := e.repo.Status(context.Background(), o)
	require.NoError(t, err)
	assert.Empty(t, items)
}

func TestQueueAlreadyEnabled(t *testing.T) {
	e := newEnv(t)
	o := saOwner()
	require.NoError(t, e.queue(t, o, enable("/fc/a/")))
	err := e.queue(t, o, enable("/fc/b/"))
	testpg.RequireUsecaseError(t, err, usecase.KindConflict, CodeAlreadyEnabled)
	pub, _, _ := e.repo.Status(context.Background(), o)
	assert.Equal(t, "/fc/a/", pub.BasePath, "the first path stays")
}

func TestQueueRegenerateRequeues(t *testing.T) {
	e := newEnv(t)
	o := saOwner()
	require.NoError(t, e.queue(t, o, enable("/fc/a/")))
	require.NoError(t, e.queue(t, o, Pending{Changes: []Change{{Credential: CredAuthToken}}}))
	_, err := e.pool.Exec(context.Background(),
		`UPDATE iam_credential_publication_items SET state='PUBLISHED', attempts=3, last_error='x', next_attempt_at=NULL WHERE owner_id=$1`, o.ID)
	require.NoError(t, err)
	require.NoError(t, e.queue(t, o, Pending{Changes: []Change{{Credential: CredAuthToken}}}))
	it := e.items(t, o)[CredAuthToken]
	assert.Equal(t, StatePendingPut, it.State)
	assert.Zero(t, it.Attempts)
	assert.Empty(t, it.LastError)
	assert.NotNil(t, it.NextAttemptAt)
}

func TestQueueUnavailableThenRotationPublishes(t *testing.T) {
	e := newEnv(t)
	o := saOwner()
	require.NoError(t, e.queue(t, o, Pending{
		Enable:  &EnableRequest{BasePath: "/fc/a/"},
		Changes: []Change{{Credential: CredClientSecret, Unavailable: true}},
	}))
	it := e.items(t, o)[CredClientSecret]
	assert.Equal(t, StateNotPublishable, it.State)
	assert.Nil(t, it.NextAttemptAt, "nothing for the worker to do")

	require.NoError(t, e.queue(t, o, Pending{Changes: []Change{{Credential: CredClientSecret, Plaintext: str("new")}}}))
	assert.Equal(t, StatePendingPut, e.items(t, o)[CredClientSecret].State)
}

func TestQueueValueOnlyForClientSecret(t *testing.T) {
	e := newEnv(t)
	o := saOwner()
	require.NoError(t, e.queue(t, o, enable("/fc/a/")))
	err := e.queue(t, o, Pending{Changes: []Change{{Credential: CredAuthToken, Plaintext: str("tok")}}})
	require.Error(t, err)
	err = e.queue(t, o, Pending{Changes: []Change{{Credential: "nonsense"}}})
	require.Error(t, err)
}

func TestQueueRemove(t *testing.T) {
	e := newEnv(t)
	o := saOwner()
	require.NoError(t, e.queue(t, o, Pending{
		Enable: &EnableRequest{BasePath: "/fc/a/"},
		Changes: []Change{
			{Credential: CredAuthToken},
			{Credential: CredClientSecret, Unavailable: true},
		},
	}))
	require.NoError(t, e.queue(t, o, Pending{Changes: []Change{
		{Credential: CredAuthToken, Remove: true},     // may exist at the store
		{Credential: CredClientSecret, Remove: true},  // never published
		{Credential: CredSigningSecret, Remove: true}, // never queued
	}}))
	got := e.items(t, o)
	assert.Equal(t, StatePendingDelete, got[CredAuthToken].State)
	assert.NotContains(t, got, CredClientSecret)
	assert.NotContains(t, got, CredSigningSecret)
}

func TestDisableQueuesDeletesAndNotEnabled(t *testing.T) {
	e := newEnv(t)
	o := saOwner()
	ctx := context.Background()
	err := e.inTx(t, func(tx pgx.Tx) error { return DisableInTx(ctx, tx, o) })
	testpg.RequireUsecaseError(t, err, usecase.KindBusinessRule, CodeNotEnabled)

	require.NoError(t, e.queue(t, o, Pending{
		Enable: &EnableRequest{BasePath: "/fc/a/"},
		Changes: []Change{
			{Credential: CredClientID}, {Credential: CredClientSecret, Plaintext: str("s")},
			{Credential: CredSigningSecret, Unavailable: true},
		},
	}))
	require.NoError(t, e.inTx(t, func(tx pgx.Tx) error { return DisableInTx(ctx, tx, o) }))
	pub, items, err := e.repo.Status(ctx, o)
	require.NoError(t, err)
	assert.Nil(t, pub, "off, but the items remain so the page can say removing")
	require.Len(t, items, 2)
	for _, it := range items {
		assert.Equal(t, StatePendingDelete, it.State)
	}
	assert.Nil(t, e.pendingValue(t, o, CredClientSecret), "the waiting secret is dropped")

	// Turning it on again while the deletes are pending is refused.
	err = e.queue(t, o, enable("/fc/a/"))
	testpg.RequireUsecaseError(t, err, usecase.KindBusinessRule, CodeRemovalPending)
}

func TestOwnerDeleted(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	o := saOwner()
	// Never published: a no-op.
	require.NoError(t, e.inTx(t, func(tx pgx.Tx) error { return OwnerDeletedInTx(ctx, tx, o) }))

	require.NoError(t, e.queue(t, o, Pending{Enable: &EnableRequest{BasePath: "/fc/a/"}, Changes: []Change{{Credential: CredClientID}}}))
	require.NoError(t, e.inTx(t, func(tx pgx.Tx) error { return OwnerDeletedInTx(ctx, tx, o) }))
	pub, items, err := e.repo.Status(ctx, o)
	require.NoError(t, err)
	assert.Nil(t, pub)
	require.Len(t, items, 1)
	assert.Equal(t, StatePendingDelete, items[0].State)
}

func TestLinkedOAuthClientPublishesWithItsServiceAccount(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	sa, oac := e.seedLinked(t)
	saO := Owner{Type: OwnerServiceAccount, ID: sa}
	ocO := Owner{Type: OwnerOAuthClient, ID: oac}

	linked, err := e.repo.LinkedServiceAccount(ctx, oac)
	require.NoError(t, err)
	assert.Equal(t, sa, linked)
	standalone := e.seedStandalone(t)
	linked, err = e.repo.LinkedServiceAccount(ctx, standalone)
	require.NoError(t, err)
	assert.Empty(t, linked)
	linked, err = e.repo.LinkedServiceAccount(ctx, "oac_missing")
	require.NoError(t, err)
	assert.Empty(t, linked)

	require.NoError(t, e.queue(t, saO, enable("/fc/service-accounts/linked/")))

	// A rotation of the client's secret lands on the account's row.
	require.NoError(t, e.queue(t, ocO, Pending{Changes: []Change{{Credential: CredClientSecret, Plaintext: str("rot")}}}))
	assert.Equal(t, StatePendingPut, e.items(t, saO)[CredClientSecret].State)
	assert.Equal(t, "/fc/service-accounts/linked/client-secret", e.items(t, saO)[CredClientSecret].Path)
	assert.Empty(t, e.items(t, ocO), "nothing under the client's own row")

	// Deleting the client does not remove the account's published copies.
	require.NoError(t, e.inTx(t, func(tx pgx.Tx) error { return OwnerDeletedInTx(ctx, tx, ocO) }))
	pub, _, err := e.repo.Status(ctx, saO)
	require.NoError(t, err)
	require.NotNil(t, pub)
	assert.Equal(t, StatePendingPut, e.items(t, saO)[CredClientSecret].State)
}

func TestStandaloneOAuthClientHasItsOwnPublication(t *testing.T) {
	e := newEnv(t)
	oac := e.seedStandalone(t)
	ocO := Owner{Type: OwnerOAuthClient, ID: oac}
	require.NoError(t, e.queue(t, ocO, Pending{
		Enable:  &EnableRequest{BasePath: "/fc/oauth-clients/c/"},
		Changes: []Change{{Credential: CredClientID}, {Credential: CredClientSecret, Plaintext: str("s")}},
	}))
	assert.Len(t, e.items(t, ocO), 2)
}

// ---- worker ----

type fakeValues struct {
	mu      sync.Mutex
	values  map[string]string
	present map[string]bool
	err     error
}

func newValues() *fakeValues {
	return &fakeValues{values: map[string]string{}, present: map[string]bool{}}
}

func (f *fakeValues) set(o Owner, cred, v string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.values[o.ID+"/"+cred] = v
	f.present[o.ID+"/"+cred] = true
}

func (f *fakeValues) remove(o Owner, cred string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.present, o.ID+"/"+cred)
}

func (f *fakeValues) Value(_ context.Context, o Owner, cred string) (string, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.values[o.ID+"/"+cred], f.present[o.ID+"/"+cred], f.err
}

func (e env) worker(store Store, vals ValueSource) *Worker {
	return &Worker{Repo: e.repo, Store: store, Values: vals, Enc: e.enc, IsLeader: func() bool { return true }}
}

func TestWorkerPutsAndClearsThePendingValue(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	o := saOwner()
	fake := credentialpublishtest.New()
	vals := newValues()
	vals.set(o, CredAuthToken, "tok-1")
	w := e.worker(NewParameterStore(fake, ""), vals)

	require.NoError(t, e.queue(t, o, Pending{
		Enable: &EnableRequest{BasePath: "/fc/service-accounts/w/"},
		Changes: []Change{
			{Credential: CredAuthToken},
			{Credential: CredClientSecret, Plaintext: str("sec-1")},
		},
	}))
	require.NotNil(t, e.pendingValue(t, o, CredClientSecret))

	n, err := w.RunOnce(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	p, ok := fake.Get("/fc/service-accounts/w/auth-token")
	require.True(t, ok)
	assert.Equal(t, "tok-1", p.Value)
	assert.Equal(t, "SERVICE_ACCOUNT/"+o.ID, p.Tags[TagKey])
	p, _ = fake.Get("/fc/service-accounts/w/client-secret")
	assert.Equal(t, "sec-1", p.Value)

	got := e.items(t, o)
	for _, c := range []string{CredAuthToken, CredClientSecret} {
		assert.Equal(t, StatePublished, got[c].State, c)
		assert.NotNil(t, got[c].PublishedAt)
		assert.Zero(t, got[c].Attempts)
		assert.Empty(t, got[c].LastError)
	}
	assert.Nil(t, e.pendingValue(t, o, CredClientSecret), "cleared once written")

	n, err = w.RunOnce(ctx)
	require.NoError(t, err)
	assert.Zero(t, n, "published items are not due")
}

func TestWorkerSecondRegeneratePublishesTheLatest(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	o := saOwner()
	fake := credentialpublishtest.New()
	vals := newValues()
	w := e.worker(NewParameterStore(fake, ""), vals)
	require.NoError(t, e.queue(t, o, enable("/fc/a/")))

	vals.set(o, CredSigningSecret, "v1")
	require.NoError(t, e.queue(t, o, Pending{Changes: []Change{{Credential: CredSigningSecret}}}))
	_, err := w.RunOnce(ctx)
	require.NoError(t, err)

	vals.set(o, CredSigningSecret, "v2")
	require.NoError(t, e.queue(t, o, Pending{Changes: []Change{{Credential: CredSigningSecret}}}))
	vals.set(o, CredSigningSecret, "v3") // two quick regenerates: only the latest is read
	require.NoError(t, e.queue(t, o, Pending{Changes: []Change{{Credential: CredSigningSecret}}}))
	_, err = w.RunOnce(ctx)
	require.NoError(t, err)

	p, _ := fake.Get("/fc/a/signing-secret")
	assert.Equal(t, "v3", p.Value)
	assert.Equal(t, StatePublished, e.items(t, o)[CredSigningSecret].State)
}

func TestWorkerCredentialGoneBecomesADelete(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	o := saOwner()
	fake := credentialpublishtest.New()
	vals := newValues()
	w := e.worker(NewParameterStore(fake, ""), vals)
	require.NoError(t, e.queue(t, o, enable("/fc/a/")))

	vals.set(o, CredAuthToken, "t")
	require.NoError(t, e.queue(t, o, Pending{Changes: []Change{{Credential: CredAuthToken}}}))
	_, err := w.RunOnce(ctx)
	require.NoError(t, err)
	require.Contains(t, fake.Names(), "/fc/a/auth-token")

	// Re-queued, but the token is gone by the time the worker reads it.
	vals.remove(o, CredAuthToken)
	require.NoError(t, e.queue(t, o, Pending{Changes: []Change{{Credential: CredAuthToken}}}))
	n, err := w.RunOnce(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, n, "the put turns into a delete that is due at once, and the same pass takes it")
	assert.Empty(t, fake.Names())
	assert.Empty(t, e.items(t, o), "the row goes with the parameter")
}

func TestWorkerDisableDeletesParametersThenRows(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	o := saOwner()
	fake := credentialpublishtest.New()
	vals := newValues()
	vals.set(o, CredAuthToken, "t")
	w := e.worker(NewParameterStore(fake, ""), vals)
	require.NoError(t, e.queue(t, o, Pending{Enable: &EnableRequest{BasePath: "/fc/a/"}, Changes: []Change{{Credential: CredAuthToken}}}))
	_, err := w.RunOnce(ctx)
	require.NoError(t, err)
	require.NoError(t, e.inTx(t, func(tx pgx.Tx) error { return DisableInTx(ctx, tx, o) }))

	n, err := w.RunOnce(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Empty(t, fake.Names())
	assert.Empty(t, e.items(t, o))
}

func TestWorkerFailureRecordsAttemptsErrorAndBackoff(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	o := saOwner()
	fake := credentialpublishtest.New()
	vals := newValues()
	vals.set(o, CredAuthToken, "SECRET-VALUE")
	now := time.Now().UTC()
	w := e.worker(NewParameterStore(fake, ""), vals)
	w.Now = func() time.Time { return now }
	require.NoError(t, e.queue(t, o, Pending{Enable: &EnableRequest{BasePath: "/fc/a/"}, Changes: []Change{{Credential: CredAuthToken}}}))
	fake.FailPath("/fc/a/auth-token", credentialpublishtest.Err("AccessDeniedException", "not allowed"))
	now = time.Now().UTC() // after the queueing, which is due at the database's now()

	n, err := w.RunOnce(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	it := e.items(t, o)[CredAuthToken]
	assert.Equal(t, StatePendingPut, it.State)
	assert.Equal(t, 1, it.Attempts)
	assert.Equal(t, "AccessDeniedException: not allowed", it.LastError)
	assert.NotContains(t, it.LastError, "SECRET-VALUE")
	require.NotNil(t, it.NextAttemptAt)
	assert.WithinDuration(t, now.Add(30*time.Second), *it.NextAttemptAt, time.Second)

	// Not due yet.
	n, err = w.RunOnce(ctx)
	require.NoError(t, err)
	assert.Zero(t, n)

	// Due, fails again: doubles.
	now = now.Add(31 * time.Second)
	n, err = w.RunOnce(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	it = e.items(t, o)[CredAuthToken]
	assert.Equal(t, 2, it.Attempts)
	assert.WithinDuration(t, now.Add(time.Minute), *it.NextAttemptAt, time.Second)

	// AWS recovers; Retry now makes it due regardless of the backoff.
	fake.ClearFailure("/fc/a/auth-token")
	require.NoError(t, e.repo.RetryNow(ctx, o))
	w.Now = time.Now
	_, err = w.RunOnce(ctx)
	require.NoError(t, err)
	it = e.items(t, o)[CredAuthToken]
	assert.Equal(t, StatePublished, it.State)
	assert.Zero(t, it.Attempts)
	assert.Empty(t, it.LastError)
}

func TestWorkerNeverOverwritesAForeignParameter(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	o := saOwner()
	fake := credentialpublishtest.New()
	fake.Seed("/fc/a/auth-token", "theirs", nil)
	vals := newValues()
	vals.set(o, CredAuthToken, "ours")
	w := e.worker(NewParameterStore(fake, ""), vals)
	require.NoError(t, e.queue(t, o, Pending{Enable: &EnableRequest{BasePath: "/fc/a/"}, Changes: []Change{{Credential: CredAuthToken}}}))

	_, err := w.RunOnce(ctx)
	require.NoError(t, err)
	it := e.items(t, o)[CredAuthToken]
	assert.Equal(t, StatePendingPut, it.State)
	assert.Equal(t, 1, it.Attempts)
	assert.Equal(t, "a parameter FlowCatalyst does not manage already exists at /fc/a/auth-token", it.LastError)
	p, _ := fake.Get("/fc/a/auth-token")
	assert.Equal(t, "theirs", p.Value)

	// Deleting: the foreign one stays, the row goes.
	require.NoError(t, e.inTx(t, func(tx pgx.Tx) error { return DisableInTx(ctx, tx, o) }))
	_, err = w.RunOnce(ctx)
	require.NoError(t, err)
	p, ok := fake.Get("/fc/a/auth-token")
	require.True(t, ok)
	assert.Equal(t, "theirs", p.Value)
	assert.Empty(t, e.items(t, o))
}

func TestWorkerUndecryptablePendingValueFailsAndNeverPutsEmpty(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	o := saOwner()
	fake := credentialpublishtest.New()
	w := e.worker(NewParameterStore(fake, ""), newValues())
	require.NoError(t, e.queue(t, o, Pending{
		Enable: &EnableRequest{BasePath: "/fc/a/"}, Changes: []Change{{Credential: CredClientSecret, Plaintext: str("x")}}}))
	_, err := e.pool.Exec(ctx,
		`UPDATE iam_credential_publication_items SET pending_value = 'not-a-ciphertext' WHERE owner_id = $1`, o.ID)
	require.NoError(t, err)

	_, err = w.RunOnce(ctx)
	require.NoError(t, err)
	it := e.items(t, o)[CredClientSecret]
	assert.Equal(t, "the stored value cannot be decrypted", it.LastError)
	assert.Equal(t, 1, it.Attempts)
	assert.Zero(t, fake.Puts)
}

func TestWorkerValueSourceErrorIsAFailure(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	o := saOwner()
	vals := newValues()
	vals.err = errors.New("service account credential cannot be decrypted")
	w := e.worker(NewParameterStore(credentialpublishtest.New(), ""), vals)
	require.NoError(t, e.queue(t, o, Pending{Enable: &EnableRequest{BasePath: "/fc/a/"}, Changes: []Change{{Credential: CredAuthToken}}}))
	_, err := w.RunOnce(ctx)
	require.NoError(t, err)
	it := e.items(t, o)[CredAuthToken]
	assert.Equal(t, StatePendingPut, it.State)
	assert.Contains(t, it.LastError, "cannot be decrypted")
}

// requeueingStore re-queues the item while the "AWS call" is in flight, as an
// operation committing a regenerate would.
type requeueingStore struct {
	Store
	onPut func()
}

func (s requeueingStore) Put(ctx context.Context, path, value, tag string) error {
	err := s.Store.Put(ctx, path, value, tag)
	if s.onPut != nil {
		s.onPut()
	}
	return err
}

func TestWorkerStaleOutcomeDoesNotOverwriteANewerRequeue(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	o := saOwner()
	fake := credentialpublishtest.New()
	vals := newValues()
	vals.set(o, CredAuthToken, "old")
	require.NoError(t, e.queue(t, o, Pending{Enable: &EnableRequest{BasePath: "/fc/a/"}, Changes: []Change{{Credential: CredAuthToken}}}))

	store := requeueingStore{Store: NewParameterStore(fake, "")}
	store.onPut = func() {
		vals.set(o, CredAuthToken, "new")
		require.NoError(t, e.queue(t, o, Pending{Changes: []Change{{Credential: CredAuthToken}}}))
	}
	w := e.worker(store, vals)
	_, err := w.RunOnce(ctx)
	require.NoError(t, err)

	// The old put's outcome must not mark the newer request as published.
	it := e.items(t, o)[CredAuthToken]
	assert.Equal(t, StatePendingPut, it.State)
	assert.Nil(t, it.PublishedAt)

	// The next pass publishes the latest value.
	store.onPut = nil
	w.Store = store
	_, err = w.RunOnce(ctx)
	require.NoError(t, err)
	p, _ := fake.Get("/fc/a/auth-token")
	assert.Equal(t, "new", p.Value)
	assert.Equal(t, StatePublished, e.items(t, o)[CredAuthToken].State)
}

func TestWorkerStaleFailureDoesNotOverwriteANewerRequeue(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	o := saOwner()
	fake := credentialpublishtest.New()
	vals := newValues()
	vals.set(o, CredAuthToken, "v")
	require.NoError(t, e.queue(t, o, Pending{Enable: &EnableRequest{BasePath: "/fc/a/"}, Changes: []Change{{Credential: CredAuthToken}}}))
	fake.FailPath("/fc/a/auth-token", credentialpublishtest.Err("ThrottlingException", "slow down"))
	requeued := false
	store := requeueingStore{Store: NewParameterStore(fake, ""), onPut: func() {
		if requeued {
			return
		}
		requeued = true
		require.NoError(t, e.queue(t, o, Pending{Changes: []Change{{Credential: CredAuthToken}}}))
	}}
	w := e.worker(store, vals)
	w.Now = func() time.Time { return time.Now().Add(-time.Hour) } // nothing is due again in this pass
	_, err := w.RunOnce(ctx)
	require.NoError(t, err)
	it := e.items(t, o)[CredAuthToken]
	assert.Zero(t, it.Attempts, "the newer request starts clean")
	assert.Empty(t, it.LastError)
}

func TestWorkerLeaseKeepsAnItemFromAConcurrentPass(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	o := saOwner()
	require.NoError(t, e.queue(t, o, Pending{Enable: &EnableRequest{BasePath: "/fc/a/"}, Changes: []Change{{Credential: CredAuthToken}}}))
	now := time.Now()
	c, err := e.repo.claim(ctx, now, lease)
	require.NoError(t, err)
	require.NotNil(t, c)
	c2, err := e.repo.claim(ctx, now, lease)
	require.NoError(t, err)
	assert.Nil(t, c2, "claimed, so not due for another pass")
	c3, err := e.repo.claim(ctx, now.Add(lease+time.Second), lease)
	require.NoError(t, err)
	assert.NotNil(t, c3, "the lease lapses if the holder dies")
}

func TestWorkerRunStopsWithContextAndOnlyWorksAsLeader(t *testing.T) {
	e := newEnv(t)
	o := saOwner()
	fake := credentialpublishtest.New()
	vals := newValues()
	vals.set(o, CredAuthToken, "t")
	require.NoError(t, e.queue(t, o, Pending{Enable: &EnableRequest{BasePath: "/fc/lead/" + o.ID + "/"}, Changes: []Change{{Credential: CredAuthToken}}}))
	w := e.worker(NewParameterStore(fake, ""), vals)
	w.Interval = 20 * time.Millisecond
	leader := false
	var mu sync.Mutex
	w.IsLeader = func() bool { mu.Lock(); defer mu.Unlock(); return leader }

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { w.Run(ctx); close(done) }()
	time.Sleep(150 * time.Millisecond)
	assert.Zero(t, fake.Puts, "not the leader")
	mu.Lock()
	leader = true
	mu.Unlock()
	require.Eventually(t, func() bool { return e.items(t, o)[CredAuthToken].State == StatePublished }, 5*time.Second, 20*time.Millisecond)
	cancel()
	<-done
}
