//go:build integration

package api

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/payloadstore/payloadtest"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// What a caller of the dispatch-job routes sees of a job of the new shape
// (docs/payload-offload-design.md, section 11): the routes that return one
// job's payload return it resolved, or payloadUnavailable; payloadLength is
// the payload row's size; the raw list never reads the store.
func TestDispatchJobRoutes_PayloadShapes(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	store := payloadtest.NewCountingStore()
	svc := payloadtest.Service(t, store)
	s := &State{Repo: dispatchjob.NewRepository(pool).UsePayloads(svc)}
	at := time.Now().UTC().Add(3 * time.Hour).Truncate(time.Microsecond)
	put := func(payload, payloadID *string) string {
		t.Helper()
		id := tsid.GenerateUntyped()
		_, err := pool.Exec(ctx,
			`INSERT INTO msg_dispatch_jobs (id, code, target_url, status, payload, payload_id, created_at, updated_at)
			 VALUES ($1, 'payloadapi:job:test:made', 'http://example.invalid/hook', 'PENDING', $2, $3, $4, $4)`,
			id, payload, payloadID, at)
		require.NoError(t, err)
		return id
	}
	legacyPayload := `{"b":1,"a":2}`
	legacy := put(&legacyPayload, nil)
	inlinePayload := `{"z": 1,"a":  [3, 1]}`
	inlineRow := payloadtest.Store(t, svc, pool, "", at, []byte(inlinePayload))
	inline := put(nil, &inlineRow.ID)
	bigPayload := `{"pad":"` + strings.Repeat("w", 3*payloadtest.OffloadMinBytes) + `"}`
	bigRow := payloadtest.Store(t, svc, pool, "", at, []byte(bigPayload))
	require.True(t, bigRow.Offloaded())
	offloaded := put(nil, &bigRow.ID)

	anchor := testpg.WithAuth(ctx, &auth.AuthContext{
		PrincipalID: "prn_djpayload", Scope: auth.ScopeAnchor,
		Permissions: []string{viewPerm, viewRawPerm}, AllApplications: true,
	})

	t.Run("raw list: sizes from the payload row, no store call", func(t *testing.T) {
		out, err := s.listDebugRaw(anchor, &rawListInput{Size: 1000})
		require.NoError(t, err)
		got := map[string]RawDispatchJobResponse{}
		for _, j := range out.Body {
			got[j.ID] = j
		}
		assert.Equal(t, len(legacyPayload), got[legacy].PayloadLength)
		assert.Equal(t, len(inlinePayload), got[inline].PayloadLength)
		assert.Equal(t, len(bigPayload), got[offloaded].PayloadLength)
		assert.Zero(t, store.Gets())
	})

	t.Run("detail routes return the payload resolved", func(t *testing.T) {
		for _, get := range []func(context.Context, *apicommon.IDInput) (*apicommon.Out[DispatchJobResponse], error){s.getByID, s.getRaw} {
			for id, want := range map[string]string{legacy: legacyPayload, inline: inlinePayload, offloaded: bigPayload} {
				out, err := get(anchor, &apicommon.IDInput{ID: id})
				require.NoError(t, err)
				require.NotNil(t, out.Body.Payload, id)
				assert.Equal(t, want, *out.Body.Payload, "byte for byte")
				assert.False(t, out.Body.PayloadUnavailable)
			}
		}
		assert.EqualValues(t, 2, store.Gets(), "one read per detail read of the offloaded job")
	})

	t.Run("store failing: the job without its payload", func(t *testing.T) {
		store.FailReads(errors.New("timeout"))
		t.Cleanup(func() { store.FailReads(nil) })
		out, err := s.getByID(anchor, &apicommon.IDInput{ID: offloaded})
		require.NoError(t, err, "not a 500")
		assert.Nil(t, out.Body.Payload)
		assert.True(t, out.Body.PayloadUnavailable)
		b, err := json.Marshal(out.Body)
		require.NoError(t, err)
		assert.Contains(t, string(b), `"payloadUnavailable":true`)
		assert.NotContains(t, string(b), `"payload":`)

		out, err = s.getByID(anchor, &apicommon.IDInput{ID: legacy})
		require.NoError(t, err)
		assert.False(t, out.Body.PayloadUnavailable)
		b, err = json.Marshal(out.Body)
		require.NoError(t, err)
		assert.NotContains(t, string(b), `payloadUnavailable`, "absent unless it says something")
	})
}
