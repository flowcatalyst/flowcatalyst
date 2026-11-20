//go:build integration

package operations_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/payloadstore/payloadtest"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// The payload edit's read side (docs/payload-offload-design.md): the previous
// payload it records the size and digest of is the job's payload row, read
// from the payload store when it was offloaded. A store that cannot be read
// refuses the edit for now; a payload that is gone does not block the edit
// that repairs it.
func TestEditDispatchJobPayload_PreviousPayloadOfTheNewShape(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	store := payloadtest.NewCountingStore()
	svc := payloadtest.Service(t, store)
	repo := dispatchjob.NewRepository(pool).UsePayloads(svc)
	uow := testpg.NewUoW(t)
	previous := `{"pad":"` + strings.Repeat("e", 3*payloadtest.OffloadMinBytes) + `"}`

	// newShapeJob is a FAILED job whose payload is an offloaded payload row
	// (written here directly, to control the stored row).
	newShapeJob := func(t *testing.T) (string, string) {
		t.Helper()
		j := seedJob(t, repo, "opstest:edit:newshape", common.DispatchFailed, seedOpts{})
		var createdAt time.Time
		require.NoError(t, pool.QueryRow(ctx, `SELECT created_at FROM msg_dispatch_jobs WHERE id = $1`, j.ID).Scan(&createdAt))
		row := payloadtest.Store(t, svc, pool, "", createdAt, []byte(previous))
		require.True(t, row.Offloaded())
		_, err := pool.Exec(ctx, `UPDATE msg_dispatch_jobs SET payload = NULL, payload_id = $2 WHERE id = $1`, j.ID, row.ID)
		require.NoError(t, err)
		return j.ID, row.Ref.Key
	}
	sum := sha256.Sum256([]byte(previous))

	t.Run("offloaded: read for the audit", func(t *testing.T) {
		id, _ := newShapeJob(t)
		ev, err := runAuthorized(uow, operations.EditDispatchJobPayload(repo), operations.EditPayloadCommand{ID: id, Payload: `{"fixed":true}`})
		require.NoError(t, err)
		assert.Equal(t, len(previous), ev.PreviousSize)
		assert.Equal(t, hex.EncodeToString(sum[:]), ev.PreviousSHA256)
	})

	t.Run("store unavailable: refused, try again", func(t *testing.T) {
		id, _ := newShapeJob(t)
		store.FailReads(errors.New("timeout"))
		t.Cleanup(func() { store.FailReads(nil) })
		_, err := runAuthorized(uow, operations.EditDispatchJobPayload(repo), operations.EditPayloadCommand{ID: id, Payload: `{"fixed":true}`})
		testpg.RequireUsecaseError(t, err, usecase.KindInternal, "PAYLOAD_STORE_UNAVAILABLE")
	})

	t.Run("payload missing: the edit repairs it", func(t *testing.T) {
		id, key := newShapeJob(t)
		store.Delete(key)
		ev, err := runAuthorized(uow, operations.EditDispatchJobPayload(repo), operations.EditPayloadCommand{ID: id, Payload: `{"fixed":true}`})
		require.NoError(t, err)
		assert.Equal(t, len(previous), ev.PreviousSize, "the payload row's size")
		assert.Empty(t, ev.PreviousSHA256, "unknown: the payload is gone")
	})
}
