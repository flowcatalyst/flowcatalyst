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

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/event"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/payloadstore/payloadtest"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// What a caller of the event routes sees of an event of the new shape
// (docs/payload-offload-design.md, section 11): the data as the producer sent
// it, dataSize, dataOffloaded where a list left offloaded data out, and
// dataUnavailable where a detail read could not read the store. A legacy row
// answers as before, with dataSize added.
func TestEventRoutes_PayloadShapes(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	store := payloadtest.NewCountingStore()
	svc := payloadtest.Service(t, store)
	s := &State{Repo: event.NewRepository(pool).UsePayloads(svc)}
	typ := "payloadapi:" + strings.ToLower(tsid.GenerateUntyped()) + ":event:made"
	at := time.Now().UTC().Add(3 * time.Hour).Truncate(time.Microsecond)

	put := func(legacy, payloadID *string) string {
		t.Helper()
		id := tsid.GenerateUntyped()
		_, err := pool.Exec(ctx,
			`INSERT INTO msg_events (id, type, source, time, data, deduplication_id, created_at, payload_id)
			 VALUES ($1, $2, 'test://payload', $3, $4::jsonb, $1, $3, $5)`, id, typ, at, legacy, payloadID)
		require.NoError(t, err)
		_, err = pool.Exec(ctx,
			`INSERT INTO msg_events_read (id, type, source, time, data, deduplication_id, created_at, payload_id)
			 VALUES ($1, $2, 'test://payload', $3, $4, $1, $3, $5)`, id, typ, at, legacy, payloadID)
		require.NoError(t, err)
		return id
	}
	legacyData := `{"b":1,"a":2}`
	legacy := put(&legacyData, nil)
	inlineData := `{"z": 1,"a":  [3, 1]}`
	inlineRow := payloadtest.Store(t, svc, pool, "", at, []byte(inlineData))
	inline := put(nil, &inlineRow.ID)
	bigData := `{"z":1,"pad":"` + strings.Repeat("y", 3*payloadtest.OffloadMinBytes) + `","a":2}`
	bigRow := payloadtest.Store(t, svc, pool, "", at, []byte(bigData))
	require.True(t, bigRow.Offloaded())
	offloaded := put(nil, &bigRow.ID)

	anchor := testpg.WithAuth(ctx, &auth.AuthContext{
		PrincipalID: "prn_payloadviewer", Scope: auth.ScopeAnchor,
		Permissions: []string{"platform:messaging:event:view", "platform:messaging:event:view-raw"}, AllApplications: true,
	})

	t.Run("list", func(t *testing.T) {
		out, err := s.list(anchor, &listInput{Types: typ, Size: 100})
		require.NoError(t, err)
		got := map[string]EventRead{}
		for _, e := range out.Body {
			got[e.ID] = e
		}
		require.Len(t, got, 3)
		assert.Equal(t, len(legacyData), got[legacy].DataSize)
		assert.False(t, got[legacy].DataOffloaded)
		assert.Equal(t, len(inlineData), got[inline].DataSize)
		assert.False(t, got[inline].DataOffloaded)
		assert.Equal(t, len(bigData), got[offloaded].DataSize)
		assert.True(t, got[offloaded].DataOffloaded)

		raw, err := s.listDebugRaw(anchor, &rawListInput{Size: 1000})
		require.NoError(t, err)
		rawByID := map[string]RawEventResponse{}
		for _, e := range raw.Body {
			rawByID[e.ID] = e
		}
		assert.Equal(t, inlineData, string(rawByID[inline].Data), "held in the database: returned in the list")
		assert.Empty(t, rawByID[offloaded].Data, "offloaded: left out of the list")
		assert.True(t, rawByID[offloaded].DataOffloaded)
		assert.Equal(t, len(bigData), rawByID[offloaded].DataSize)
		assert.Zero(t, store.Gets(), "lists make no store call")

		// On the wire: the new fields only where they say something.
		b, err := json.Marshal(rawByID[offloaded])
		require.NoError(t, err)
		assert.Contains(t, string(b), `"dataOffloaded":true`)
		assert.NotContains(t, string(b), `"data":`)
		assert.NotContains(t, string(b), `dataUnavailable`)
	})

	t.Run("detail", func(t *testing.T) {
		out, err := s.getByID(anchor, &apicommon.IDInput{ID: inline})
		require.NoError(t, err)
		assert.Equal(t, inlineData, string(out.Body.Data))
		assert.Equal(t, len(inlineData), out.Body.DataSize)
		b, err := json.Marshal(out.Body)
		require.NoError(t, err)
		// encoding/json compacts a raw value; the producer's key order stays.
		assert.Contains(t, string(b), `"data":{"z":1,"a":[3,1]}`)
		assert.NotContains(t, string(b), `dataUnavailable`)

		out, err = s.getByID(anchor, &apicommon.IDInput{ID: offloaded})
		require.NoError(t, err)
		assert.Equal(t, bigData, string(out.Body.Data), "the detail route returns offloaded data")
		assert.False(t, out.Body.DataUnavailable)

		raw, err := s.getDebugRaw(anchor, &apicommon.IDInput{ID: offloaded})
		require.NoError(t, err)
		assert.Equal(t, bigData, string(raw.Body.Data))
		assert.False(t, raw.Body.DataOffloaded)

		out, err = s.getByID(anchor, &apicommon.IDInput{ID: legacy})
		require.NoError(t, err)
		assert.Equal(t, legacyData, string(out.Body.Data))
		assert.Equal(t, len(legacyData), out.Body.DataSize)
	})

	t.Run("detail with the store failing", func(t *testing.T) {
		store.FailReads(errors.New("timeout"))
		t.Cleanup(func() { store.FailReads(nil) })
		out, err := s.getByID(anchor, &apicommon.IDInput{ID: offloaded})
		require.NoError(t, err, "the record is returned, not a 500")
		assert.Empty(t, out.Body.Data)
		assert.True(t, out.Body.DataUnavailable)
		assert.Equal(t, len(bigData), out.Body.DataSize)
		b, err := json.Marshal(out.Body)
		require.NoError(t, err)
		assert.Contains(t, string(b), `"dataUnavailable":true`)
		assert.NotContains(t, string(b), `"data":`)

		raw, err := s.getDebugRaw(anchor, &apicommon.IDInput{ID: offloaded})
		require.NoError(t, err)
		assert.True(t, raw.Body.DataUnavailable)

		// Data held in the database needs no store.
		out, err = s.getByID(anchor, &apicommon.IDInput{ID: inline})
		require.NoError(t, err)
		assert.Equal(t, inlineData, string(out.Body.Data))
		assert.False(t, out.Body.DataUnavailable)
	})
}
