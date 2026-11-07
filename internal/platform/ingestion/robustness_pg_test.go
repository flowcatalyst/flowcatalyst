//go:build integration

package ingestion_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/platformsink"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// Runtime robustness (docs/ingestion-design.md section 7): the instance guard,
// the lease release, health-aware placement, the cap on a runtime returning
// after a gap, and unheld ingestions. A time base of its own, as fair share's:
// the runtime table is global and other tests' runtimes must not fall into
// these windows.
var tRobust0 = time.Date(2032, 3, 1, 12, 0, 0, 0, time.UTC)

func rday(n int) time.Time { return tRobust0.AddDate(0, 0, n) }

// pollFrom polls as the runtime from a process (instance).
func (e *env) pollFrom(runtime, instance string, now time.Time) (*ingestion.PollResult, error) {
	req := e.pollRequest(runtime, now, nil)
	req.InstanceID = &instance
	return e.repo.Poll(context.Background(), req)
}

func (e *env) release(t *testing.T, req ingestion.ReleaseRequest) int {
	t.Helper()
	n, err := e.repo.Release(context.Background(), req)
	require.NoError(t, err)
	return n
}

func (e *env) runtimeRow(t *testing.T, runtime string) (instance *string, lastSeen time.Time, left *time.Time) {
	t.Helper()
	require.NoError(t, e.pool.QueryRow(context.Background(),
		`SELECT instance_id, last_seen_at, left_at FROM ing_runtimes WHERE runtime_id = $1`, runtime).Scan(&instance, &lastSeen, &left))
	return instance, lastSeen, left
}

func (e *env) leaseEnd(t *testing.T, ingestionID string) time.Time {
	t.Helper()
	var end time.Time
	require.NoError(t, e.pool.QueryRow(context.Background(), `SELECT expires_at FROM ing_leases WHERE ingestion_id = $1`, ingestionID).Scan(&end))
	return end.UTC()
}

// seen records a runtime as last polling at, carrying the v2 baseline, as if
// it had polled then.
func (e *env) seen(t *testing.T, runtime string, at time.Time) {
	t.Helper()
	_, err := e.pool.Exec(context.Background(), `INSERT INTO ing_runtimes (runtime_id, contract_version, first_seen_at, last_seen_at) VALUES ($1, 2, $2, $2)
		ON CONFLICT (runtime_id) DO UPDATE SET last_seen_at = $2`, runtime, at)
	require.NoError(t, err)
}

// ── 1. The instance guard ──────────────────────────────────────────────────

// Two processes with one runtime id: while the first is live the second is
// refused, and the refusal changes nothing (not last_seen_at, not a lease).
// Once the first has gone quiet for a lease, the second takes over.
func TestInstanceGuard_SecondProcessIsRefusedWhileTheFirstIsLive(t *testing.T) {
	e := newEnv(t)
	t0 := rday(1)
	i := e.live(t, "guarded", e.sftp(t))
	rt := "rt-guard-" + unique()

	res, err := e.pollFrom(rt, "instance-one", t0)
	require.NoError(t, err)
	require.Equal(t, []string{i.ID}, given(res))

	_, err = e.pollFrom(rt, "instance-two", t0.Add(time.Minute))
	assert.Equal(t, "RUNTIME_ID_IN_USE", conflictCode(t, err))
	assert.ErrorContains(t, err, "runtime id "+rt+" is in use by another live process")
	instance, lastSeen, _ := e.runtimeRow(t, rt)
	assert.Equal(t, "instance-one", *instance, "the refused instance is not stored")
	assert.True(t, lastSeen.Equal(t0), "a refused poll does not count as the runtime being seen: %s", lastSeen)
	assert.True(t, e.leaseEnd(t, i.ID).Equal(t0.Add(lease)), "a refused poll renews nothing")

	// The first keeps polling as before.
	res, err = e.pollFrom(rt, "instance-one", t0.Add(time.Minute))
	require.NoError(t, err)
	assert.Equal(t, []string{i.ID}, given(res))

	// Silent for a lease: the second is accepted and runs it.
	res, err = e.pollFrom(rt, "instance-two", t0.Add(time.Minute+lease))
	require.NoError(t, err)
	assert.Equal(t, []string{i.ID}, given(res))
	instance, _, _ = e.runtimeRow(t, rt)
	assert.Equal(t, "instance-two", *instance)
}

// A poll without an instance is not checked and stores none; a goodbye frees
// the id at once for a successor.
func TestInstanceGuard_NoInstanceAndGoodbye(t *testing.T) {
	e := newEnv(t)
	t0 := rday(2)
	rt := "rt-guard-" + unique()
	_, err := e.pollFrom(rt, "instance-one", t0)
	require.NoError(t, err)

	_, err = e.repo.Poll(context.Background(), e.pollRequest(rt, t0.Add(time.Second), nil))
	require.NoError(t, err, "a poll naming no instance is not checked")
	instance, _, _ := e.runtimeRow(t, rt)
	assert.Nil(t, instance, "and stores none")
	_, err = e.pollFrom(rt, "instance-two", t0.Add(2*time.Second))
	require.NoError(t, err, "no instance stored: nothing to be refused by")

	// instance-two says goodbye; instance-three is accepted straight away.
	e.release(t, ingestion.ReleaseRequest{RuntimeID: rt, InstanceID: new("instance-two"), Goodbye: true, Now: t0.Add(3 * time.Second)})
	_, _, left := e.runtimeRow(t, rt)
	require.NotNil(t, left)
	_, err = e.pollFrom(rt, "instance-three", t0.Add(4*time.Second))
	require.NoError(t, err, "a runtime that said goodbye no longer holds its id")
	_, _, left = e.runtimeRow(t, rt)
	assert.Nil(t, left, "a poll clears left_at")
}

// ── 2. Release ─────────────────────────────────────────────────────────────

// A runtime that stopped a route hands its lease back: another runtime takes
// it at its next poll instead of after the lease expires. Ids it does not hold
// are ignored and not counted, and an expired lease keeps its earlier end.
func TestRelease_HandsTheLeaseOverAtOnce(t *testing.T) {
	e := newEnv(t)
	t0 := rday(3)
	ids := e.polledAll(t, 2)
	a, b := "rt-rel-a-"+unique(), "rt-rel-b-"+unique()
	require.Len(t, given(e.poll(t, a, t0)), 2)
	require.Empty(t, given(e.poll(t, b, t0.Add(time.Second))))
	kept := given(e.poll(t, a, t0.Add(2*time.Second))) // share 1: one kept, one released
	require.Len(t, kept, 1)
	surplus := ids[0]
	if surplus == kept[0] {
		surplus = ids[1]
	}
	assert.Empty(t, given(e.poll(t, b, t0.Add(3*time.Second))), "a released lease is not handed over before it expires")

	at := t0.Add(4 * time.Second)
	n := e.release(t, ingestion.ReleaseRequest{RuntimeID: b, IngestionIDs: []string{surplus}, Now: at})
	assert.Equal(t, 0, n, "b does not hold it: ignored")
	n = e.release(t, ingestion.ReleaseRequest{RuntimeID: a, IngestionIDs: []string{surplus, "ing_notheldbyany"}, Now: at})
	assert.Equal(t, 1, n)
	assert.True(t, e.leaseEnd(t, surplus).Equal(at), "the lease ends now")
	assert.Equal(t, 1, e.count(t, `SELECT count(*) FROM ing_leases WHERE ingestion_id = $1 AND released`, surplus))
	assert.True(t, e.leaseEnd(t, kept[0]).After(at), "the lease a keeps is untouched")

	assert.Equal(t, []string{surplus}, given(e.poll(t, b, at.Add(time.Second))), "b takes it at its next poll")

	// Released again after it expired: the end does not move later.
	later := at.Add(time.Hour)
	_ = e.release(t, ingestion.ReleaseRequest{RuntimeID: b, IngestionIDs: []string{surplus}, Now: later})
	assert.True(t, e.leaseEnd(t, surplus).Before(later))
}

// An imposter cannot release another process's leases: a release naming
// another instance than the stored one is refused and changes nothing.
func TestRelease_ImposterIsRefused(t *testing.T) {
	e := newEnv(t)
	t0 := rday(4)
	i := e.live(t, "rel-imposter", e.sftp(t))
	rt := "rt-rel-" + unique()
	_, err := e.pollFrom(rt, "instance-one", t0)
	require.NoError(t, err)

	_, err = e.repo.Release(context.Background(), ingestion.ReleaseRequest{
		RuntimeID: rt, InstanceID: new("instance-imposter"), IngestionIDs: []string{i.ID}, Goodbye: true, Now: t0.Add(time.Second),
	})
	assert.Equal(t, "RUNTIME_ID_IN_USE", conflictCode(t, err))
	assert.True(t, e.leaseEnd(t, i.ID).Equal(t0.Add(lease)), "the lease is untouched")
	_, _, left := e.runtimeRow(t, rt)
	assert.Nil(t, left, "no goodbye recorded")

	assert.Equal(t, 1, e.release(t, ingestion.ReleaseRequest{RuntimeID: rt, InstanceID: new("instance-one"), IngestionIDs: []string{i.ID}, Now: t0.Add(2 * time.Second)}))
}

// A runtime that says goodbye stops counting as live at once: the survivor's
// share grows at its next poll, not a lease later.
func TestRelease_GoodbyeLeavesTheFairShare(t *testing.T) {
	e := newEnv(t)
	t0 := rday(5)
	ids := e.polledAll(t, 2)
	a, b := "rt-bye-a-"+unique(), "rt-bye-b-"+unique()
	now := t0
	var held map[string][]string
	for range 5 { // converge: one each
		held = map[string][]string{a: given(e.poll(t, a, now)), b: given(e.poll(t, b, now.Add(time.Second)))}
		now = now.Add(time.Minute)
	}
	require.Len(t, held[a], 1)
	require.Len(t, held[b], 1)

	e.release(t, ingestion.ReleaseRequest{RuntimeID: a, IngestionIDs: held[a], Goodbye: true, Now: now})
	assert.ElementsMatch(t, ids, given(e.poll(t, b, now.Add(time.Second))), "a has left: b's share is both, and a's lease has ended")
}

// ── 3. Health-aware placement ──────────────────────────────────────────────

// failed records the runtime's route of the ingestion as status at, as a report
// would (the runtime holds the lease) or, for a runtime that no longer does,
// as it was left.
func (e *env) routeState(t *testing.T, ingestionID, runtime, status string, at time.Time) {
	t.Helper()
	_, err := e.pool.Exec(context.Background(), `INSERT INTO ing_route_states (ingestion_id, runtime_id, version, status, updated_at) VALUES ($1, $2, 1, $3, $4)
		ON CONFLICT (ingestion_id, runtime_id) DO UPDATE SET status = $3, updated_at = $4`, ingestionID, runtime, status, at)
	require.NoError(t, err)
}

// An ingestion FAILED on runtime A moves to B: A neither renews nor lists it
// while B (live, carrying the kind) has not failed it, and B takes it when the
// lease expires.
func TestPlacement_FailedMovesToAHealthyRuntime(t *testing.T) {
	e := newEnv(t)
	t0 := rday(6)
	i := e.live(t, "sick", e.sftp(t))
	a, b := "rt-sick-a-"+unique(), "rt-sick-b-"+unique()
	require.Equal(t, []string{i.ID}, given(e.poll(t, a, t0)))
	require.Empty(t, given(e.poll(t, b, t0.Add(time.Second))))
	sum := e.report(t, ingestion.Report{RuntimeID: a, Now: t0.Add(10 * time.Second), Routes: []ingestion.RouteReport{
		{IngestionID: i.ID, Version: 1, Status: "FAILED", Error: new("login refused")},
	}})
	require.Equal(t, 1, sum.Accepted)

	assert.Empty(t, given(e.poll(t, a, t0.Add(time.Minute))), "a stops running what it fails while b has not")
	assert.Equal(t, 1, e.count(t, `SELECT count(*) FROM ing_leases WHERE ingestion_id = $1 AND runtime_id = $2 AND released`, i.ID, a),
		"its lease is released like any surplus")
	assert.Empty(t, given(e.poll(t, b, t0.Add(time.Minute+time.Second))), "never handed over before the lease ends")
	assert.Empty(t, given(e.poll(t, a, t0.Add(2*time.Minute))))
	assert.Equal(t, []string{i.ID}, given(e.poll(t, b, t0.Add(lease+time.Second))), "b takes it once the lease has run out")
}

// A healthy runtime already at its share still takes an ingestion the other
// avoids (it is never left unheld), and then sheds one of its own, which the
// avoiding runtime takes: each ends with one again.
func TestPlacement_AFullHealthyRuntimeRescuesAndRebalances(t *testing.T) {
	e := newEnv(t)
	t0 := rday(20)
	// y first, so x has the higher id: b's renewals, in id order, would reach
	// its own y before the rescued x unless the rescued one goes first.
	y := e.live(t, "rescue-y", e.sftp(t))
	x := e.live(t, "rescue-x", e.sftp(t))
	require.Less(t, y.ID, x.ID)
	a, b := "rt-rescue-a-"+unique(), "rt-rescue-b-"+unique()
	e.seen(t, a, t0.Add(-time.Second)) // two live runtimes: a share of one each
	require.Equal(t, []string{y.ID}, given(e.poll(t, b, t0)))
	require.Equal(t, []string{x.ID}, given(e.poll(t, a, t0.Add(time.Second))))
	e.routeState(t, x.ID, a, "FAILED", t0.Add(10*time.Second))

	assert.Empty(t, given(e.poll(t, a, t0.Add(time.Minute))), "a avoids x")
	assert.Equal(t, []string{y.ID}, given(e.poll(t, b, t0.Add(time.Minute+time.Second))), "x's lease has not ended")
	assert.Empty(t, given(e.poll(t, a, t0.Add(2*time.Minute))))
	assert.ElementsMatch(t, []string{x.ID, y.ID}, given(e.poll(t, b, t0.Add(lease+time.Second))),
		"b is at its share yet takes x, which a avoids")
	assert.Equal(t, []string{x.ID}, given(e.poll(t, b, t0.Add(lease+time.Minute))),
		"b keeps the rescued x and sheds y to get back to its share")
	assert.Empty(t, given(e.poll(t, a, t0.Add(lease+time.Minute+time.Second))), "y's lease has not ended")
	assert.Equal(t, []string{x.ID}, given(e.poll(t, b, t0.Add(2*lease+30*time.Second))), "b keeps polling (a live peer)")
	assert.Equal(t, []string{y.ID}, given(e.poll(t, a, t0.Add(2*lease+time.Minute))), "a takes y: one each again")
}

// An ingestion every live carrier has failed recently stays where it is: a
// definition broken everywhere stays put and visible.
func TestPlacement_FailedEverywhereStays(t *testing.T) {
	e := newEnv(t)
	t0 := rday(7)
	i := e.live(t, "broken", e.sftp(t))
	a, b := "rt-broken-a-"+unique(), "rt-broken-b-"+unique()
	require.Equal(t, []string{i.ID}, given(e.poll(t, a, t0)))
	require.Empty(t, given(e.poll(t, b, t0.Add(time.Second))))
	e.routeState(t, i.ID, b, "FAILING", t0.Add(5*time.Second)) // b failed it when it held it
	e.routeState(t, i.ID, a, "FAILED", t0.Add(10*time.Second))

	assert.Equal(t, []string{i.ID}, given(e.poll(t, a, t0.Add(time.Minute))), "every live carrier fails it: it stays on a")
}

// After the cooldown the runtime is eligible again, and a STARTED state never
// excludes.
func TestPlacement_CooldownAndStarted(t *testing.T) {
	e := newEnv(t)
	t0 := rday(8)
	i := e.live(t, "cooldown", e.sftp(t))
	a, b := "rt-cool-a-"+unique(), "rt-cool-b-"+unique()
	cool := 2 * time.Minute
	pollA := func(at time.Time) []string {
		req := e.pollRequest(a, at, nil)
		req.UnhealthyCooldown = cool
		res, err := e.repo.Poll(context.Background(), req)
		require.NoError(t, err)
		return given(res)
	}
	require.Equal(t, []string{i.ID}, pollA(t0))
	require.Empty(t, given(e.poll(t, b, t0.Add(time.Second))))

	e.routeState(t, i.ID, a, "STARTED", t0.Add(10*time.Second))
	assert.Equal(t, []string{i.ID}, pollA(t0.Add(30*time.Second)), "a STARTED route is not avoided")

	e.routeState(t, i.ID, a, "FAILING", t0.Add(40*time.Second))
	assert.Empty(t, pollA(t0.Add(time.Minute)), "FAILING within the cooldown: avoided")
	require.Empty(t, given(e.poll(t, b, t0.Add(time.Minute+time.Second))))
	// The FAILING report is older than the cooldown; a's lease (released at
	// t0+1m) has not ended, so b could not take it and a takes it back.
	assert.Equal(t, []string{i.ID}, pollA(t0.Add(40*time.Second+cool+time.Second)), "after the cooldown a is eligible again")
}

// ── 4. The cap on a runtime returning after a gap ─────────────────────────

// After a platform outage longer than a lease every runtime returns at once.
// The first to poll counts only itself as live, so without the cap it would
// claim everything; it claims its share over the runtimes seen recently.
func TestReturnCap_AfterAnOutageTheFirstPollerTakesItsShare(t *testing.T) {
	e := newEnv(t)
	t0 := rday(9)
	ids := e.polledAll(t, 9)
	runtimes := []string{"rt-out-a-" + unique(), "rt-out-b-" + unique(), "rt-out-c-" + unique()}
	now := t0
	held := map[string][]string{}
	for range 6 { // polling every minute: three each
		for n, rt := range runtimes {
			held[rt] = given(e.poll(t, rt, now.Add(time.Duration(n)*time.Second)))
		}
		now = now.Add(time.Minute)
	}
	for _, rt := range runtimes {
		require.Len(t, held[rt], 3, "converged")
	}

	// The platform is unreachable for five minutes; every lease runs out.
	now = now.Add(5 * time.Minute)
	first := given(e.poll(t, runtimes[0], now))
	assert.Len(t, first, 3, "the first back claims a third, not all nine")
	all := first
	for n, rt := range runtimes[1:] {
		got := given(e.poll(t, rt, now.Add(time.Duration(n+1)*time.Second)))
		assert.Len(t, got, 3)
		all = append(all, got...)
	}
	assert.ElementsMatch(t, ids, all, "every ingestion held, none twice")
}

// A survivor that kept polling is never capped: when two of three runtimes
// die it takes their four ingestions at once when their leases expire, not
// its share over the runtimes it saw recently.
func TestReturnCap_ASurvivorIsNeverCapped(t *testing.T) {
	e := newEnv(t)
	t0 := rday(13)
	ids := e.polledAll(t, 6)
	p, q, r := "rt-surv-p-"+unique(), "rt-surv-q-"+unique(), "rt-surv-r-"+unique()
	now := t0
	held := map[string][]string{}
	for range 5 { // every minute: two each
		for n, rt := range []string{p, q, r} {
			held[rt] = given(e.poll(t, rt, now.Add(time.Duration(n)*time.Second)))
		}
		now = now.Add(time.Minute)
	}
	for _, rt := range []string{p, q, r} {
		require.Len(t, held[rt], 2, "converged")
	}
	// q and r go silent after +4m; their leases end by +7m02s and they are
	// not live at +8m. p polls every minute.
	for range 3 {
		assert.Len(t, given(e.poll(t, p, now)), 2)
		now = now.Add(time.Minute)
	}
	assert.ElementsMatch(t, ids, given(e.poll(t, p, now)), "all six at once: two renewed, four claimed")
}

// A brand-new fleet: the first runtime has no recent peers (one seen eleven
// minutes ago is not recent) and takes everything.
func TestReturnCap_ANewFleetTakesEverything(t *testing.T) {
	e := newEnv(t)
	t0 := rday(10)
	ids := e.polledAll(t, 4)
	e.seen(t, "rt-gone-"+unique(), t0.Add(-11*time.Minute))
	assert.ElementsMatch(t, ids, given(e.poll(t, "rt-new-"+unique(), t0)))
}

// The cap is on new claims only: a runtime returning after a goodbye renews
// what it still holds up to its share, whatever the cap.
func TestReturnCap_RenewalsAreNotCapped(t *testing.T) {
	e := newEnv(t)
	t0 := rday(11)
	e.polledAll(t, 6)
	a := "rt-renew-" + unique()
	require.Len(t, given(e.poll(t, a, t0)), 6, "alone")
	// Two runtimes seen four minutes before: recent, not live.
	e.seen(t, "rt-recent-1-"+unique(), t0.Add(-4*time.Minute))
	e.seen(t, "rt-recent-2-"+unique(), t0.Add(-4*time.Minute))
	// a says goodbye without releasing (its routes did not stop cleanly) and
	// comes back: it returns after a gap, its cap is 2, its share 6.
	e.release(t, ingestion.ReleaseRequest{RuntimeID: a, Goodbye: true, Now: t0.Add(time.Second)})
	assert.Len(t, given(e.poll(t, a, t0.Add(2*time.Second))), 6, "its six leases are renewed, not capped")
}

// ── 6. Unheld ──────────────────────────────────────────────────────────────

// A live polled ingestion with no unexpired lease for longer than a lease and
// two minutes is unheld, since the end of its last lease (or its updated_at
// when never leased). A shared one, a paused one and a held one never are.
func TestUnheld(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	t0 := rday(12)
	leased := e.live(t, "unheld-leased", e.sftp(t))
	never := e.live(t, "unheld-never", e.custom(t, "nobody-"+unique()))
	shared := e.live(t, "unheld-shared", sqsDef(t, "Shared", "q-unheld"))
	paused := e.live(t, "unheld-paused", e.sftp(t))
	paused = e.mustFind(t, paused.ID)
	paused.Pause()
	e.mustSave(t, paused)
	only := []string{leased.ID, never.ID, shared.ID, paused.ID}

	unheld := func(at time.Time) map[string]time.Time {
		t.Helper()
		got, err := e.repo.Unheld(ctx, only, at, lease)
		require.NoError(t, err)
		return got
	}
	// Leased at t0 (until t0+3m); the other polled one is of a kind nobody carries.
	require.Contains(t, given(e.poll(t, "rt-unheld-"+unique(), t0)), leased.ID)
	end := t0.Add(lease)
	threshold := ingestion.UnheldAfter(lease)
	assert.Equal(t, 5*time.Minute, threshold)

	got := unheld(t0.Add(time.Minute))
	assert.NotContains(t, got, leased.ID, "held")
	assert.NotContains(t, got, shared.ID, "a shared source has no lease")
	assert.NotContains(t, got, paused.ID, "a paused one runs nowhere by design")
	require.Contains(t, got, never.ID, "never leased: unheld since its updated_at")
	assert.True(t, got[never.ID].Equal(e.mustFind(t, never.ID).UpdatedAt.UTC()))

	assert.NotContains(t, unheld(end.Add(threshold-time.Second)), leased.ID, "not yet: within a lease and two minutes of the lease's end")
	got = unheld(end.Add(threshold + time.Second))
	require.Contains(t, got, leased.ID)
	assert.True(t, got[leased.ID].Equal(end), "since the end of its last lease: %s", got[leased.ID])
	assert.Equal(t, "not running: no runtime has held it since "+end.Format(time.RFC3339), ingestion.UnheldNotice(got[leased.ID]))

	all, err := e.repo.Unheld(ctx, nil, end.Add(threshold+time.Second), lease)
	require.NoError(t, err)
	assert.Contains(t, all, leased.ID, "nil ids: every live polled ingestion")
	none, err := e.repo.Unheld(ctx, []string{}, end.Add(threshold+time.Second), lease)
	require.NoError(t, err)
	assert.Empty(t, none)
}

// The gauge counts the unheld ingestions as Unheld does, by the database's clock.
func TestUnheldGauge(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	i := e.live(t, "gauge", e.sftp(t))
	_, err := e.pool.Exec(ctx, `UPDATE ing_ingestions SET updated_at = now() - interval '1 hour' WHERE id = $1`, i.ID)
	require.NoError(t, err)

	require.NoError(t, ingestion.CountUnheld(ctx, e.repo, lease))
	want, err := e.repo.Unheld(ctx, nil, time.Time{}, lease)
	require.NoError(t, err)
	require.Contains(t, want, i.ID)
	families, err := ingestion.MetricsRegistry.Gather()
	require.NoError(t, err)
	var value float64 = -1
	for _, f := range families {
		if f.GetName() == "fc_ingestion_unheld" {
			value = f.GetMetric()[0].GetGauge().GetValue()
		}
	}
	assert.Equal(t, float64(len(want)), value, fmt.Sprintf("fc_ingestion_unheld counts the %d unheld ingestions", len(want)))
}

// The minute recount also logs one structured WARN line when something is
// unheld, for a log metric filter (the gauge is not scraped everywhere): count
// a top-level number, the ids capped, and nothing at all when nothing is unheld.
// On a database of its own, so no other test's ingestions count.
func TestCountUnheld_LogsOneLine(t *testing.T) {
	ctx := context.Background()
	pool := testpg.ScratchDB(t, "ing_unheld_log_"+unique())
	e := newEnvOn(t, pool, usecasepgx.New(pool, platformsink.New()))

	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })
	lines := func() []map[string]any {
		var out []map[string]any
		for line := range bytes.SplitSeq(buf.Bytes(), []byte("\n")) {
			var rec map[string]any
			if json.Unmarshal(line, &rec) == nil && rec["msg"] == "ingestions unheld" {
				out = append(out, rec)
			}
		}
		return out
	}

	require.NoError(t, ingestion.CountUnheld(ctx, e.repo, lease))
	assert.Empty(t, lines(), "nothing unheld: nothing logged")

	i := e.live(t, "unheld-log", e.sftp(t))
	_, err := pool.Exec(ctx, `UPDATE ing_ingestions SET updated_at = now() - interval '1 hour' WHERE id = $1`, i.ID)
	require.NoError(t, err)
	buf.Reset()
	require.NoError(t, ingestion.CountUnheld(ctx, e.repo, lease))
	got := lines()
	require.Len(t, got, 1)
	assert.Equal(t, "WARN", got[0]["level"])
	assert.EqualValues(t, 1, got[0]["count"], "a top-level JSON number")
	assert.Equal(t, []any{i.ID}, got[0]["ids"])
	assert.EqualValues(t, 0, got[0]["more"])

	// Past the cap: the first UnheldLogIDs ids, the rest counted.
	for n := range ingestion.UnheldLogIDs + 2 {
		extra := e.live(t, fmt.Sprintf("unheld-log-%d", n), e.sftp(t))
		_, err := pool.Exec(ctx, `UPDATE ing_ingestions SET updated_at = now() - interval '1 hour' WHERE id = $1`, extra.ID)
		require.NoError(t, err)
	}
	buf.Reset()
	require.NoError(t, ingestion.CountUnheld(ctx, e.repo, lease))
	got = lines()
	require.Len(t, got, 1)
	assert.EqualValues(t, ingestion.UnheldLogIDs+3, got[0]["count"])
	assert.Len(t, got[0]["ids"], ingestion.UnheldLogIDs)
	assert.EqualValues(t, 3, got[0]["more"])
}
