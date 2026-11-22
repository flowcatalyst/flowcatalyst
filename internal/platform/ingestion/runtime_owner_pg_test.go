//go:build integration

package ingestion_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/platformsink"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// A runtime id belongs to the account that first registered it
// (docs/ingestion-design.md section 7, "The runtime id's owner"). A time base
// of its own, as the robustness tests': the runtime table is global.
var tOwner0 = time.Date(2033, 5, 1, 12, 0, 0, 0, time.UTC)

// otherAccount is a runtime account that is not runtimeAccount.
const otherAccount = "prn_ingrtother001"

func notOwnedCode(t *testing.T, err error) string {
	t.Helper()
	ue := usecase.AsError(err)
	require.NotNil(t, ue, "want a usecase error, got %v", err)
	assert.Equal(t, usecase.KindAuthorization, ue.Kind)
	return ue.Code
}

func (e *env) pollAsAccount(runtime, principal string, instance *string, now time.Time) (*ingestion.PollResult, error) {
	req := e.pollRequest(runtime, now, nil)
	req.Principal, req.InstanceID = principal, instance
	return e.repo.Poll(context.Background(), req)
}

func (e *env) runtimeOwner(t *testing.T, runtime string) *string {
	t.Helper()
	var owner *string
	require.NoError(t, e.pool.QueryRow(context.Background(),
		`SELECT principal_id FROM ing_runtimes WHERE runtime_id = $1`, runtime).Scan(&owner))
	return owner
}

// The first poll registers the id to its account. A poll or a release naming
// the id from another account is refused 403 RUNTIME_ID_NOT_OWNED and changes
// nothing (not last_seen_at, not a lease, not the kinds); the owner goes on as
// before. The owner check comes before the instance guard.
func TestRuntimeOwner_FirstPollRegistersTheID(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	t0 := tOwner0.AddDate(0, 0, 1)
	i := e.live(t, "owned", e.sftp(t))
	rt := "rt-owner-" + unique()

	res, err := e.pollAsAccount(rt, runtimeAccount, new("instance-one"), t0)
	require.NoError(t, err)
	require.Equal(t, []string{i.ID}, given(res))
	owner := e.runtimeOwner(t, rt)
	require.NotNil(t, owner)
	assert.Equal(t, runtimeAccount, *owner)

	// Another account, live owner or not, with or without an instance.
	for _, instance := range []*string{nil, new("instance-one"), new("instance-two")} {
		_, err = e.pollAsAccount(rt, otherAccount, instance, t0.Add(time.Minute))
		assert.Equal(t, ingestion.RuntimeIDNotOwned, notOwnedCode(t, err), "a poll from another account is refused before the instance guard")
		assert.ErrorContains(t, err, "runtime id "+rt+" is registered to another service account")
		assert.NotContains(t, err.Error(), runtimeAccount, "the refusal does not name the owner")
	}
	_, err = e.pollAsAccount(rt, otherAccount, nil, t0.Add(time.Hour))
	assert.Equal(t, ingestion.RuntimeIDNotOwned, notOwnedCode(t, err), "nor once the owner has gone quiet")
	instance, lastSeen, _ := e.runtimeRow(t, rt)
	assert.Equal(t, "instance-one", *instance)
	assert.True(t, lastSeen.Equal(t0), "a refused poll does not count as the runtime being seen: %s", lastSeen)
	assert.True(t, e.leaseEnd(t, i.ID).Equal(t0.Add(lease)), "a refused poll renews nothing")

	_, err = e.repo.Release(ctx, ingestion.ReleaseRequest{
		RuntimeID: rt, Principal: otherAccount, IngestionIDs: []string{i.ID}, Goodbye: true, Now: t0.Add(2 * time.Minute),
	})
	assert.Equal(t, ingestion.RuntimeIDNotOwned, notOwnedCode(t, err))
	assert.True(t, e.leaseEnd(t, i.ID).Equal(t0.Add(lease)), "a refused release releases nothing")
	_, _, left := e.runtimeRow(t, rt)
	assert.Nil(t, left, "and records no goodbye")

	assert.Equal(t, ingestion.RuntimeIDNotOwned, notOwnedCode(t, e.repo.CheckRuntimeOwner(ctx, rt, otherAccount)))
	require.NoError(t, e.repo.CheckRuntimeOwner(ctx, rt, runtimeAccount))
	require.NoError(t, e.repo.CheckRuntimeOwner(ctx, "rt-never-"+unique(), otherAccount), "an id no account holds is not refused")

	// The owner goes on as before, and its goodbye does not free the id.
	res, err = e.pollAsAccount(rt, runtimeAccount, new("instance-one"), t0.Add(3*time.Minute))
	require.NoError(t, err)
	assert.Equal(t, []string{i.ID}, given(res))
	assert.Equal(t, 1, e.release(t, ingestion.ReleaseRequest{RuntimeID: rt, IngestionIDs: []string{i.ID}, Goodbye: true, Now: t0.Add(4 * time.Minute)}))
	_, err = e.pollAsAccount(rt, otherAccount, nil, t0.Add(5*time.Minute))
	assert.Equal(t, ingestion.RuntimeIDNotOwned, notOwnedCode(t, err), "a goodbye does not give the id away")
}

// A row written before migration 125 has no account: nothing is refused on
// it, and the next poll binds it to its caller.
func TestRuntimeOwner_ARowWithoutAnAccountIsBoundByTheNextPoll(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	t0 := tOwner0.AddDate(0, 0, 2)
	rt := "rt-owner-old-" + unique()
	e.seen(t, rt, t0)
	require.Nil(t, e.runtimeOwner(t, rt))
	require.NoError(t, e.repo.CheckRuntimeOwner(ctx, rt, otherAccount), "no account recorded: not refused")
	require.NoError(t, e.repo.CheckRuntimeOwner(ctx, rt, runtimeAccount))

	_, err := e.pollAsAccount(rt, runtimeAccount, nil, t0.Add(time.Minute))
	require.NoError(t, err)
	owner := e.runtimeOwner(t, rt)
	require.NotNil(t, owner)
	assert.Equal(t, runtimeAccount, *owner)
	_, err = e.pollAsAccount(rt, otherAccount, nil, t0.Add(2*time.Minute))
	assert.Equal(t, ingestion.RuntimeIDNotOwned, notOwnedCode(t, err))
}

// A poll or a release that names no calling account is a bug in the caller,
// answered as an error, never as a registration.
func TestRuntimeOwner_ACallWithoutAnAccountIsAnError(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	rt := "rt-owner-none-" + unique()
	_, err := e.pollAsAccount(rt, "", nil, tOwner0.AddDate(0, 0, 3))
	require.Error(t, err)
	assert.Nil(t, usecase.AsError(err), "not a refusal the caller is answered with")
	_, err = e.repo.Release(ctx, ingestion.ReleaseRequest{RuntimeID: rt, Goodbye: true})
	require.Error(t, err)
	require.Error(t, e.repo.CheckRuntimeOwner(ctx, rt, ""))
	assert.Equal(t, 0, e.count(t, `SELECT count(*) FROM ing_runtimes WHERE runtime_id = $1`, rt))
}

// What frees an id: retention removes a runtime unseen for 7 days, and with
// its row the account it belonged to; another account may then register it.
// On a database of its own, so the prune touches no other test's rows.
func TestRuntimeOwner_RetentionFreesTheID(t *testing.T) {
	ctx := context.Background()
	pool := testpg.ScratchDB(t, "ing_runtime_owner_"+unique())
	e := newEnvOn(t, pool, usecasepgx.New(pool, platformsink.New()))
	t0 := tOwner0.AddDate(0, 0, 4)
	rt := "rt-owner-pruned-" + unique()
	_, err := e.pollAsAccount(rt, runtimeAccount, nil, t0)
	require.NoError(t, err)

	_, err = e.repo.Prune(ctx, t0.Add(6*24*time.Hour))
	require.NoError(t, err)
	_, err = e.pollAsAccount(rt, otherAccount, nil, t0.Add(6*24*time.Hour))
	assert.Equal(t, ingestion.RuntimeIDNotOwned, notOwnedCode(t, err), "seen within 7 days: still the owner's")

	_, err = e.repo.Prune(ctx, t0.Add(8*24*time.Hour))
	require.NoError(t, err)
	_, err = e.pollAsAccount(rt, otherAccount, nil, t0.Add(8*24*time.Hour))
	require.NoError(t, err, "pruned: the id is free")
	owner := e.runtimeOwner(t, rt)
	require.NotNil(t, owner)
	assert.Equal(t, otherAccount, *owner)
}
