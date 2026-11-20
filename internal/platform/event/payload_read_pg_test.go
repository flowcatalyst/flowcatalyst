//go:build integration

package event_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/event"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/payloadstore/payloadtest"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// The readers of an event's data accept both shapes
// (docs/payload-offload-design.md, section 10, step 1): a legacy row has its
// data in its own column, a row with a payload_id in its msg_payloads row.
// These tests write the rows of either shape themselves, so each reader is
// tested on exactly the shape it meets; the writers (behind
// FC_PAYLOAD_TABLE_WRITES) have tests of their own.

// oddJSON is data JSONB would have changed: keys out of order, a repeated key
// and spacing. A reader of the payload table returns it byte for byte.
const oddJSON = `{"z": 1,"a":  [3, 1] , "z":2}`

// largeOddJSON is oddJSON's shape, large enough to be offloaded.
func largeOddJSON() string {
	return `{"z": 1, "pad":"` + strings.Repeat("x", 3*payloadtest.OffloadMinBytes) + `" ,"a":2}`
}

// evSeed writes one event as both tables hold it: msg_events and its
// msg_events_read row, with the same created_at. payloadID nil writes the
// legacy shape with legacy as its data; otherwise data is NULL and the rows
// point at the payload.
type evSeed struct {
	pool      *pgxpool.Pool
	typ       string
	createdAt time.Time
}

func (s evSeed) put(t *testing.T, id string, legacy *string, payloadID *string) {
	t.Helper()
	ctx := context.Background()
	_, err := s.pool.Exec(ctx,
		`INSERT INTO msg_events (id, spec_version, type, source, time, data, deduplication_id, created_at, payload_id)
		 VALUES ($1, '1.0', $2, 'test://payload', $3, $4::jsonb, $1, $3, $5)`,
		id, s.typ, s.createdAt, legacy, payloadID)
	require.NoError(t, err)
	_, err = s.pool.Exec(ctx,
		`INSERT INTO msg_events_read (id, spec_version, type, source, time, data, deduplication_id, created_at, payload_id)
		 VALUES ($1, '1.0', $2, 'test://payload', $3, $4, $1, $3, $5)`,
		id, s.typ, s.createdAt, legacy, payloadID)
	require.NoError(t, err)
}

func newSeed(t *testing.T, pool *pgxpool.Pool) evSeed {
	sfx := strings.ToLower(tsid.GenerateUntyped())
	// Newer than anything else in the shared database, so FindRecentRaw's
	// page holds these rows.
	return evSeed{pool: pool, typ: "payloadread:" + sfx + ":event:made", createdAt: time.Now().UTC().Add(2 * time.Hour).Truncate(time.Microsecond)}
}

func byID(rows []event.Event, id string) *event.Event {
	for i := range rows {
		if rows[i].ID == id {
			return &rows[i]
		}
	}
	return nil
}

func TestEventReads_LegacyRowReadsAsBefore(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	store := payloadtest.NewCountingStore()
	repo := event.NewRepository(pool).UsePayloads(payloadtest.Service(t, store))
	s := newSeed(t, pool)
	id := tsid.GenerateUntyped()
	legacy := `{"b":1,"a":2}`
	s.put(t, id, &legacy, nil)

	// The read table holds the projection's text, the write table JSONB's.
	readText := legacy
	var rawText string
	require.NoError(t, pool.QueryRow(ctx, `SELECT data::text FROM msg_events WHERE id = $1`, id).Scan(&rawText))

	e, err := repo.FindByID(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, e)
	assert.Equal(t, readText, string(e.Data))
	assert.Equal(t, len(readText), e.DataSize)
	assert.Nil(t, e.StoredData)
	assert.False(t, e.DataOffloaded)
	assert.False(t, e.DataUnavailable)

	rows, err := repo.FindWithFilters(ctx, event.FilterParams{Types: []string{s.typ}})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, readText, string(rows[0].Data))
	assert.Equal(t, len(readText), rows[0].DataSize)

	raw, err := repo.FindRawByID(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, raw)
	assert.JSONEq(t, rawText, string(raw.Data))
	assert.Equal(t, len(raw.Data), raw.DataSize)

	recent, err := repo.FindRecentRaw(ctx, 50, nil)
	require.NoError(t, err)
	got := byID(recent, id)
	require.NotNil(t, got)
	assert.JSONEq(t, rawText, string(got.Data))

	byDedup, err := repo.FindByDeduplicationID(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, byDedup)
	assert.Equal(t, id, byDedup.ID)

	assert.Zero(t, store.Gets(), "a legacy row never touches the store")
}

func TestEventReads_NewShapeHeldInTheDatabase(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	store := payloadtest.NewCountingStore()
	svc := payloadtest.Service(t, store)
	repo := event.NewRepository(pool).UsePayloads(svc)
	s := newSeed(t, pool)
	id := tsid.GenerateUntyped()
	row := payloadtest.Store(t, svc, pool, "", s.createdAt, []byte(oddJSON))
	require.False(t, row.Offloaded())
	s.put(t, id, nil, &row.ID)

	e, err := repo.FindByID(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, e)
	assert.Equal(t, oddJSON, string(e.Data), "the producer's bytes, not JSONB's")
	assert.Equal(t, len(oddJSON), e.DataSize)
	assert.False(t, e.DataOffloaded)

	rows, err := repo.FindWithFilters(ctx, event.FilterParams{Types: []string{s.typ}})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, oddJSON, string(rows[0].Data))
	assert.Equal(t, len(oddJSON), rows[0].DataSize)

	raw, err := repo.FindRawByIDScoped(ctx, id, nil)
	require.NoError(t, err)
	require.NotNil(t, raw)
	assert.Equal(t, oddJSON, string(raw.Data))

	recent, err := repo.FindRecentRaw(ctx, 50, nil)
	require.NoError(t, err)
	got := byID(recent, id)
	require.NotNil(t, got)
	assert.Equal(t, oddJSON, string(got.Data))
	assert.Equal(t, len(oddJSON), got.DataSize)

	assert.Zero(t, store.Gets(), "data held in the database needs no store")
}

func TestEventReads_NewShapeOffloaded(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	store := payloadtest.NewCountingStore()
	svc := payloadtest.Service(t, store)
	repo := event.NewRepository(pool).UsePayloads(svc)
	s := newSeed(t, pool)
	data := largeOddJSON()
	const n = 3
	ids := make([]string, n)
	for i := range ids {
		ids[i] = tsid.GenerateUntyped()
		row := payloadtest.Store(t, svc, pool, "", s.createdAt, []byte(data))
		require.True(t, row.Offloaded())
		s.put(t, ids[i], nil, &row.ID)
	}

	// Lists: the data is left out and says so, and the store is never read.
	rows, err := repo.FindWithFilters(ctx, event.FilterParams{Types: []string{s.typ}})
	require.NoError(t, err)
	require.Len(t, rows, n)
	for _, e := range rows {
		assert.Nil(t, e.Data, e.ID)
		assert.True(t, e.DataOffloaded, e.ID)
		assert.Equal(t, len(data), e.DataSize, e.ID)
	}
	recent, err := repo.FindRecentRaw(ctx, 50, nil)
	require.NoError(t, err)
	for _, id := range ids {
		got := byID(recent, id)
		require.NotNil(t, got, id)
		assert.Nil(t, got.Data)
		assert.True(t, got.DataOffloaded)
		assert.Equal(t, len(data), got.DataSize)
	}
	byDedup, err := repo.FindByDeduplicationID(ctx, ids[0])
	require.NoError(t, err)
	require.NotNil(t, byDedup)
	assert.True(t, byDedup.DataOffloaded, "the dedup lookup answers an id; it leaves the data out")
	assert.Zero(t, store.Gets(), "a list makes no store call")

	// Detail reads resolve it: one read each.
	e, err := repo.FindByID(ctx, ids[0])
	require.NoError(t, err)
	require.NotNil(t, e)
	assert.Equal(t, data, string(e.Data))
	assert.Equal(t, len(data), e.DataSize)
	assert.False(t, e.DataOffloaded)
	assert.False(t, e.DataUnavailable)
	assert.EqualValues(t, 1, store.Gets())

	raw, err := repo.FindRawByIDScoped(ctx, ids[1], nil)
	require.NoError(t, err)
	require.NotNil(t, raw)
	assert.Equal(t, data, string(raw.Data))
	assert.EqualValues(t, 2, store.Gets())
}

func TestEventReads_StoreFailureOnDetail(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	store := payloadtest.NewCountingStore()
	svc := payloadtest.Service(t, store)
	repo := event.NewRepository(pool).UsePayloads(svc)
	s := newSeed(t, pool)
	data := largeOddJSON()
	failing := tsid.GenerateUntyped()
	row := payloadtest.Store(t, svc, pool, "", s.createdAt, []byte(data))
	s.put(t, failing, nil, &row.ID)
	missing := tsid.GenerateUntyped()
	gone := payloadtest.Store(t, svc, pool, "", s.createdAt, []byte(data))
	s.put(t, missing, nil, &gone.ID)
	store.Delete(gone.Ref.Key)

	store.FailReads(errors.New("connection reset"))
	for _, find := range []func() (*event.Event, error){
		func() (*event.Event, error) { return repo.FindByID(ctx, failing) },
		func() (*event.Event, error) { return repo.FindRawByIDScoped(ctx, failing, nil) },
	} {
		e, err := find()
		require.NoError(t, err, "the record is still returned")
		require.NotNil(t, e)
		assert.Nil(t, e.Data)
		assert.True(t, e.DataUnavailable)
		assert.False(t, e.DataOffloaded)
		assert.Equal(t, len(data), e.DataSize)
	}
	store.FailReads(nil)

	// A payload whose object is gone: the record, without data.
	e, err := repo.FindByID(ctx, missing)
	require.NoError(t, err)
	require.NotNil(t, e)
	assert.Nil(t, e.Data)
	assert.True(t, e.DataUnavailable)

	// A repository with no payload service reads legacy and inline rows; an
	// offloaded one is unavailable, not an error.
	bare := event.NewRepository(pool)
	e, err = bare.FindByID(ctx, failing)
	require.NoError(t, err)
	require.NotNil(t, e)
	assert.True(t, e.DataUnavailable)
}

// The list's page is chosen on the read table first and only then joined to
// the payload rows, one primary-key probe per row returned: the join sits
// above the LIMIT, and the payload table is reached by its primary key.
func TestEventList_JoinIsAPrimaryKeyProbeAboveTheLimit(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	q, args := event.ListSQL(event.FilterParams{Types: []string{"plan:test:event:made"}, Limit: 50})
	var plan []map[string]any
	var raw []byte
	require.NoError(t, pool.QueryRow(ctx, "EXPLAIN (FORMAT JSON) "+q, args...).Scan(&raw))
	require.NoError(t, json.Unmarshal(raw, &plan))
	require.Len(t, plan, 1)
	root, _ := plan[0]["Plan"].(map[string]any)
	join := findNode(root, func(n map[string]any) bool { return n["Node Type"] == "Nested Loop" })
	require.NotNil(t, join, "the payload join is a nested loop: %s", raw)
	kids, _ := join["Plans"].([]any)
	require.Len(t, kids, 2)
	outer, _ := kids[0].(map[string]any)
	assert.NotNil(t, findNode(outer, func(n map[string]any) bool { return n["Node Type"] == "Limit" }),
		"the page (LIMIT) is the join's outer side: %s", raw)
	inner, _ := kids[1].(map[string]any)
	assert.Nil(t, findNode(inner, func(n map[string]any) bool { return n["Node Type"] == "Limit" }))
	probe := findNode(inner, func(n map[string]any) bool {
		name, _ := n["Index Name"].(string)
		return strings.HasPrefix(name, "msg_payloads") && strings.HasSuffix(name, "_pkey")
	})
	empty := findNode(inner, func(n map[string]any) bool {
		// No partition yet for the bounds: the planner may prove the probe
		// empty instead.
		return n["Node Type"] == "Result"
	})
	assert.True(t, probe != nil || empty != nil, "the payload row is read by its primary key: %s", raw)
}

func findNode(n map[string]any, match func(map[string]any) bool) map[string]any {
	if n == nil {
		return nil
	}
	if match(n) {
		return n
	}
	kids, _ := n["Plans"].([]any)
	for _, k := range kids {
		if m, ok := k.(map[string]any); ok {
			if got := findNode(m, match); got != nil {
				return got
			}
		}
	}
	return nil
}
