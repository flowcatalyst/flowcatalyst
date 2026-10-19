//go:build integration

package operations_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/fnfixture"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/artifact"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
)

// seedVersion inserts a version row directly (publish needs a real artifact
// and the describe engine; pruning only cares about the row). Digests default
// to a number-derived unique hex string.
func (f *testFunctionFixture) seedVersion(number int32, status function.VersionStatus, age time.Duration, digest string) function.Version {
	f.t.Helper()
	if digest == "" {
		digest = fmt.Sprintf("%064x", int64(number)+int64(len(f.fn.ID))*1_000_000+time.Now().UnixNano()%1_000_000_000_000)
	}
	v := function.Version{
		ID: tsid.Generate(tsid.FunctionVersion), FunctionID: f.fn.ID, Number: number, Digest: digest,
		SizeBytes: 1, ABI: 1, Runtime: "js", Describe: []byte(`{}`), Status: status,
		CreatedAt: time.Now().UTC().Add(-age),
	}
	require.NoError(f.t, f.repo.InsertVersion(context.Background(), &v))
	return v
}

func (f *testFunctionFixture) seedMany(from, to int32) {
	f.t.Helper()
	for n := from; n <= to; n++ {
		f.seedVersion(n, function.VersionReady, 48*time.Hour, "")
	}
}

func (f *testFunctionFixture) prune(keep int, dryRun bool) operations.PruneResult {
	f.t.Helper()
	res, err := usecaseop.RunTx(testpg.AnchorCtx(), f.uow, operations.PruneVersions(f.repo),
		operations.PruneCommand{FunctionID: f.fn.ID, Keep: keep, DryRun: dryRun}, testpg.TestEC())
	require.NoError(f.t, err)
	return res
}

func (f *testFunctionFixture) versionNumbers() []int32 {
	f.t.Helper()
	vs, err := f.repo.ListVersionsByFunction(context.Background(), f.fn.ID)
	require.NoError(f.t, err)
	out := make([]int32, 0, len(vs))
	for i := len(vs) - 1; i >= 0; i-- { // oldest first
		out = append(out, vs[i].Number)
	}
	return out
}

func prunedNumbers(res operations.PruneResult) []int32 {
	out := []int32{}
	for _, p := range res.Pruned {
		out = append(out, p.Number)
	}
	return out
}

func (f *testFunctionFixture) aliasDirect(name string, v function.Version) {
	f.t.Helper()
	require.NoError(f.t, f.repo.UpsertAlias(context.Background(),
		&function.Alias{FunctionID: f.fn.ID, Name: name, VersionID: v.ID, UpdatedAt: time.Now().UTC()}))
}

func TestPrune_OldestUntaggedFirst_TaggedNeverPrunedNorCounted(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_oprune000001", "opruneapp1", "prune-fn")
	f.seedMany(1, 27)
	v2, _ := f.repo.GetVersionByNumber(context.Background(), f.fn.ID, 2)
	v4, _ := f.repo.GetVersionByNumber(context.Background(), f.fn.ID, 4)
	f.aliasDirect("live", *v2)
	f.aliasDirect("canary", *v4)

	res := f.prune(20, false)
	assert.Equal(t, 25, res.Untagged, "27 versions minus 2 tagged")
	assert.Equal(t, []int32{1, 3, 5, 6, 7}, prunedNumbers(res))

	got := f.versionNumbers()
	assert.Len(t, got, 22)
	assert.Contains(t, got, int32(2), "live-tagged survives")
	assert.Contains(t, got, int32(4), "canary-tagged survives")
	assert.Equal(t, int32(8), got[2], "oldest untagged left is v8")

	// One pruned event per version, Go-only family.
	var n int
	for _, e := range f.functionEvents() {
		if e.Type == "platform:function:version:pruned" {
			n++
			assert.Equal(t, "platform:function", e.Source)
		}
	}
	assert.Equal(t, 5, n)

	// Idempotent: a second run finds nothing to do.
	again := f.prune(20, false)
	assert.Empty(t, again.Pruned)
	assert.Len(t, f.versionNumbers(), 22)
}

func TestPrune_DryRunChangesNothing(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_oprune000002", "opruneapp2", "dry-fn")
	f.seedMany(1, 25)
	before := len(f.functionEvents())

	res := f.prune(20, true)
	assert.True(t, res.DryRun)
	assert.Equal(t, []int32{1, 2, 3, 4, 5}, prunedNumbers(res))
	assert.Empty(t, res.ReleasedDigests)
	assert.Len(t, f.versionNumbers(), 25)
	assert.Len(t, f.functionEvents(), before, "no events on a dry run")
}

func TestPrune_NewestAndYoungPublishedAreProtected(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_oprune000003", "opruneapp3", "prot-fn")
	f.seedVersion(1, function.VersionPublished, 10*time.Minute, "") // young PUBLISHED, oldest number
	f.seedVersion(2, function.VersionFailed, 48*time.Hour, "")
	f.seedVersion(3, function.VersionRetired, 48*time.Hour, "")
	f.seedVersion(4, function.VersionPublished, 3*time.Hour, "") // stuck PUBLISHED: prunable
	f.seedVersion(5, function.VersionReady, 48*time.Hour, "")    // newest

	res := f.prune(1, false)
	assert.Equal(t, []int32{2, 3, 4}, prunedNumbers(res))
	assert.Equal(t, []int32{1, 5}, f.versionNumbers(), "young PUBLISHED and the newest survive")
}

func TestPrune_PublishedPruneBumpsPoolRevision(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_oprune000004", "opruneapp4", "rev-fn")
	f.seedVersion(1, function.VersionPublished, 3*time.Hour, "")
	f.seedVersion(2, function.VersionReady, 48*time.Hour, "")
	before := f.poolRevision()
	f.prune(1, false)
	assert.Greater(t, f.poolRevision(), before)
}

func TestPrune_SharedArtifactKeptUntilUnreferenced(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	a := newTestFunctionFixture(t, "app_oprune000005", "opruneapp5", "share-a")
	b := newTestFunctionFixture(t, "app_oprune000006", "opruneapp6", "share-b")

	put := func(content string) string {
		d := putArtifactBytes(t, a.artifacts, []byte(content))
		return d
	}
	d1, dShared, d3 := put("one-"+t.Name()), put("shared-"+t.Name()), put("three-"+t.Name())
	// b uses a's store for the same bytes: publish the shared digest there too.
	b.artifacts = a.artifacts

	a.seedVersion(1, function.VersionReady, 48*time.Hour, d1)
	a.seedVersion(2, function.VersionReady, 48*time.Hour, dShared)
	a.seedVersion(3, function.VersionReady, 48*time.Hour, d3)
	b.seedVersion(1, function.VersionReady, 48*time.Hour, dShared)

	res := a.prune(1, false)
	assert.Equal(t, []int32{1, 2}, prunedNumbers(res))
	assert.ElementsMatch(t, []string{d1}, res.ReleasedDigests, "the shared digest is still referenced by b")

	deleted := operations.DeleteReleasedArtifacts(ctx, a.repo, a.artifacts, res.ReleasedDigests)
	assert.Equal(t, []string{d1}, deleted)
	ok, _ := a.artifacts.Exists(ctx, d1)
	assert.False(t, ok, "unreferenced artifact deleted")
	ok, _ = a.artifacts.Exists(ctx, dShared)
	assert.True(t, ok, "shared artifact kept")
	ok, _ = a.artifacts.Exists(ctx, d3)
	assert.True(t, ok, "kept version's artifact untouched")

	// Once b's reference goes, the shared digest is released on its prune.
	b.seedVersion(2, function.VersionReady, 48*time.Hour, "")
	resB := b.prune(1, false)
	assert.Equal(t, []string{dShared}, resB.ReleasedDigests)
}

// A reference that appears between the prune committing and the artifact
// delete (a concurrent publish of the same digest) saves the artifact.
func TestDeleteReleasedArtifacts_RechecksReferencesBeforeDeleting(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newTestFunctionFixture(t, "app_oprune000007", "opruneapp7", "recheck-fn")
	d := putArtifactBytes(t, f.artifacts, []byte("recheck-"+t.Name()))
	f.seedVersion(1, function.VersionReady, time.Hour, d)
	deleted := operations.DeleteReleasedArtifacts(ctx, f.repo, f.artifacts, []string{d})
	assert.Empty(t, deleted)
	ok, _ := f.artifacts.Exists(ctx, d)
	assert.True(t, ok)
}

// failingStore fails every Delete.
type failingStore struct{ artifact.Store }

func (failingStore) Delete(context.Context, string) error { return errors.New("store down") }

func TestPrune_ArtifactDeleteFailureIsTolerated(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newTestFunctionFixture(t, "app_oprune000008", "opruneapp8", "fail-fn")
	d1 := putArtifactBytes(t, f.artifacts, []byte("f1-"+t.Name()))
	f.seedVersion(1, function.VersionReady, 48*time.Hour, d1)
	f.seedVersion(2, function.VersionReady, 48*time.Hour, "")

	res := operations.PruneAfterPublish(testpg.AnchorCtx(), f.uow, f.repo, failingStore{f.artifacts}, f.fn.ID, 1, testpg.TestEC())
	assert.Equal(t, []int32{1}, prunedNumbers(res), "the row is gone even though the artifact delete failed")
	assert.Equal(t, []int32{2}, f.versionNumbers())
	ok, _ := f.artifacts.Exists(ctx, d1)
	assert.True(t, ok, "orphan left in the store, logged")
}

func TestPrune_NumbersNeverReused_AndPublishPrunesAutomatically(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newTestFunctionFixture(t, "app_oprune000009", "opruneapp9", "num-fn")
	f.seedMany(1, 30)
	f.prune(5, false)
	assert.Equal(t, []int32{26, 27, 28, 29, 30}, f.versionNumbers())

	digest := putArtifactBytes(t, f.artifacts, fnfixture.Wasm)
	pub := f.publish(digest, "")
	assert.Equal(t, int32(31), pub.Version.Number, "numbering continues past pruned versions")

	// The post-publish hook with keep=5: 6 untagged now → v26 goes.
	res := operations.PruneAfterPublish(testpg.AnchorCtx(), f.uow, f.repo, f.artifacts, f.fn.ID, 5, testpg.TestEC())
	assert.Equal(t, []int32{26}, prunedNumbers(res))
	assert.Equal(t, []int32{27, 28, 29, 30, 31}, f.versionNumbers())

	// The just-published artifact is the newest version's: kept.
	ok, _ := f.artifacts.Exists(ctx, digest)
	assert.True(t, ok)
}

func TestPrune_ConcurrentCallsPruneEachVersionOnce(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_oprune000010", "opruneapp10", "conc-fn")
	f.seedMany(1, 40)

	var wg sync.WaitGroup
	var mu sync.Mutex
	total := 0
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := usecaseop.RunTx(testpg.AnchorCtx(), f.uow, operations.PruneVersions(f.repo),
				operations.PruneCommand{FunctionID: f.fn.ID, Keep: 20}, testpg.TestEC())
			assert.NoError(t, err)
			mu.Lock()
			total += len(res.Pruned)
			mu.Unlock()
		}()
	}
	wg.Wait()
	assert.Equal(t, 20, total, "20 excess versions, each pruned by exactly one caller")
	assert.Len(t, f.versionNumbers(), 20)
}

// A promote racing a prune of the same version either wins (alias set, version
// kept) or loses cleanly (VERSION_NOT_FOUND); an alias never dangles.
func TestPrune_RacingPromoteNeverLeavesDanglingAlias(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	for i := range 6 {
		f := newTestFunctionFixture(t, fmt.Sprintf("app_oprunerace%03d", i), fmt.Sprintf("opruneracea%d", i), "race-fn")
		f.seedMany(1, 3)
		// Make v1 the READY target; keep=1 makes it prunable.
		var wg sync.WaitGroup
		var putErr error
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, putErr = f.putAlias("canary", 1)
		}()
		go func() {
			defer wg.Done()
			_, err := usecaseop.RunTx(testpg.AnchorCtx(), f.uow, operations.PruneVersions(f.repo),
				operations.PruneCommand{FunctionID: f.fn.ID, Keep: 1}, testpg.TestEC())
			assert.NoError(t, err)
		}()
		wg.Wait()

		aliases, err := f.repo.ListAliases(ctx, f.fn.ID)
		require.NoError(t, err)
		if putErr == nil {
			require.Len(t, aliases, 1)
			v, err := f.repo.GetVersionByID(ctx, aliases[0].VersionID)
			require.NoError(t, err)
			require.NotNil(t, v, "alias points at a live version")
		} else {
			ue, ok := errors.AsType[*usecase.Error](putErr)
			require.True(t, ok, "%v", putErr)
			assert.Equal(t, "VERSION_NOT_FOUND", ue.Code)
			assert.Empty(t, aliases)
		}
	}
}

func TestPrune_Validation(t *testing.T) {
	t.Parallel()
	f := newTestFunctionFixture(t, "app_oprune000011", "opruneapp11", "val-fn")
	_, err := usecaseop.RunTx(testpg.AnchorCtx(), f.uow, operations.PruneVersions(f.repo),
		operations.PruneCommand{FunctionID: f.fn.ID, Keep: 0}, testpg.TestEC())
	ue, ok := errors.AsType[*usecase.Error](err)
	require.True(t, ok)
	assert.Equal(t, "KEEP_INVALID", ue.Code)

	_, err = usecaseop.RunTx(testpg.AnchorCtx(), f.uow, operations.PruneVersions(f.repo),
		operations.PruneCommand{FunctionID: "fng_missing000001", Keep: 3}, testpg.TestEC())
	require.Error(t, err)
}
