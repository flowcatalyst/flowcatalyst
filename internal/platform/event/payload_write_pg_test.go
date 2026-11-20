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
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/payloadstore"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/payloadstore/payloadtest"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// Event ingest with the payload table written (FC_PAYLOAD_TABLE_WRITES,
// docs/payload-offload-design.md, sections 3, 5 to 8): the event row carries
// payload_id and no data, its payload row the bytes as received, offloaded in
// one segment per client above the threshold; a payload that cannot be
// stored is refused for that event alone.

// writeLimits are the small limits the tests reach: offloaded above
// payloadtest.OffloadMinBytes (2048), kept in the database when the store
// fails up to 8192, refused above 16384.
func writeLimits(c *payloadstore.Config) {
	c.InlineMaxBytes = 8192
	c.MaxBytes = 16384
}

// eventRow is what msg_events and msg_payloads hold for one event.
type eventRow struct {
	data      *string
	payloadID *string
	pData     *string
	pRef      *string
	pSize     *int32
	found     bool
}

func readEventRow(t *testing.T, pool *pgxpool.Pool, id string) eventRow {
	t.Helper()
	var r eventRow
	rows, err := pool.Query(context.Background(),
		`SELECT e.data::text, e.payload_id, p.data, p.ref, p.size
		   FROM msg_events e
		   LEFT JOIN msg_payloads p ON p.id = e.payload_id AND p.created_at = e.created_at
		  WHERE e.id = $1`, id)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		r.found = true
		require.NoError(t, rows.Scan(&r.data, &r.payloadID, &r.pData, &r.pRef, &r.pSize))
	}
	require.NoError(t, rows.Err())
	return r
}

func newEvent(typ, client string, data string) event.Event {
	e := event.New(typ, "test://payload-write", "", json.RawMessage(data))
	e.CreatedAt = e.CreatedAt.Truncate(time.Microsecond)
	if client != "" {
		e.ClientID = &client
	}
	return *e
}

func writeType() string {
	return "payloadwrite:" + strings.ToLower(tsid.GenerateUntyped()) + ":event:made"
}

func TestIngest_NewShapeStoresTheBytesAsReceived(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := event.NewRepository(pool).UsePayloads(payloadtest.WritingService(t, nil, nil))
	typ := writeType()

	odd := newEvent(typ, "", oddJSON)
	empty := newEvent(typ, "", "")
	res, err := repo.Ingest(ctx, []event.Event{odd, empty})
	require.NoError(t, err)
	assert.Equal(t, 2, res.Inserted)
	assert.Equal(t, []error{nil, nil}, res.Refused)

	r := readEventRow(t, pool, odd.ID)
	assert.Nil(t, r.data, "msg_events.data is not written")
	require.NotNil(t, r.payloadID)
	require.NotNil(t, r.pData, "the payload row was inserted under the event's created_at")
	assert.Equal(t, oddJSON, *r.pData, "byte for byte")
	assert.Nil(t, r.pRef)
	assert.EqualValues(t, len(oddJSON), *r.pSize)

	// An event with no data gets "{}", as the legacy shape stores it, so its
	// subscribers receive what they always have.
	r = readEventRow(t, pool, empty.ID)
	require.NotNil(t, r.pData)
	assert.Equal(t, `{}`, *r.pData)

	// The read of one event returns the bytes.
	got, err := repo.FindRawByID(ctx, odd.ID)
	require.NoError(t, err)
	assert.Equal(t, oddJSON, string(got.Data))
}

// The switch off (and no payload service): exactly the legacy shape, no
// payload row, no new limit.
func TestIngest_SwitchOffWritesTheLegacyShape(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	off := payloadtest.Service(t, nil)
	require.False(t, off.TableWrites())
	for name, repo := range map[string]*event.Repository{
		"no service": event.NewRepository(pool),
		"switch off": event.NewRepository(pool).UsePayloads(off),
	} {
		t.Run(name, func(t *testing.T) {
			e := newEvent(writeType(), "", oddJSON)
			res, err := repo.Ingest(ctx, []event.Event{e})
			require.NoError(t, err)
			assert.Equal(t, 1, res.Inserted)
			r := readEventRow(t, pool, e.ID)
			assert.Nil(t, r.payloadID)
			require.NotNil(t, r.data)
			assert.JSONEq(t, oddJSON, *r.data)
			assert.NotEqual(t, oddJSON, *r.data, "JSONB, as before")
		})
	}
}

// Offloaded payloads of one batch are one segment per client: one store write
// for a client's several oversized payloads, two for two clients; each row
// holds its reference and the read resolves it.
func TestIngest_OffloadsOneSegmentPerClient(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	big := func(c byte) string {
		return `{"pad":"` + strings.Repeat(string(c), 3*payloadtest.OffloadMinBytes) + `"}`
	}

	t.Run("one client", func(t *testing.T) {
		store := payloadtest.NewCountingStore()
		repo := event.NewRepository(pool).UsePayloads(payloadtest.WritingService(t, store, nil))
		typ := writeType()
		evs := []event.Event{newEvent(typ, "clt_ONE", big('a')), newEvent(typ, "clt_ONE", `{"small":1}`), newEvent(typ, "clt_ONE", big('b')), newEvent(typ, "clt_ONE", big('c'))}
		res, err := repo.Ingest(ctx, evs)
		require.NoError(t, err)
		assert.Equal(t, 4, res.Inserted)
		assert.EqualValues(t, 1, store.Puts(), "one segment for the client's three oversized payloads")
		for i, e := range evs {
			r := readEventRow(t, pool, e.ID)
			if i == 1 {
				assert.Nil(t, r.pRef, "under the threshold: in the database")
				continue
			}
			require.NotNil(t, r.pRef, "offloaded: a reference")
			assert.Nil(t, r.pData)
			got, err := repo.FindRawByID(ctx, e.ID)
			require.NoError(t, err)
			assert.False(t, got.DataUnavailable)
			assert.Equal(t, string(e.Data), string(got.Data))
		}
	})

	t.Run("two clients", func(t *testing.T) {
		store := payloadtest.NewCountingStore()
		repo := event.NewRepository(pool).UsePayloads(payloadtest.WritingService(t, store, nil))
		typ := writeType()
		evs := []event.Event{newEvent(typ, "clt_ONE", big('d')), newEvent(typ, "clt_TWO", big('e')), newEvent(typ, "clt_ONE", big('f'))}
		_, err := repo.Ingest(ctx, evs)
		require.NoError(t, err)
		assert.EqualValues(t, 2, store.Puts())
		keys := store.Keys()
		require.Len(t, keys, 2)
		prefixes := []string{strings.Split(keys[0], "/")[1], strings.Split(keys[1], "/")[1]}
		assert.ElementsMatch(t, []string{"clt_ONE", "clt_TWO"}, prefixes, "a segment's key starts with its client")
	})
}

// The store cannot be written: payloads up to the inline maximum are kept in
// the database; a larger one is refused for now (retryable); too large is
// refused for good, and so is data that is not UTF-8 text. The rest of the
// batch is stored, and each refusal is in its own position.
func TestIngest_RefusalsArePerEvent(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	store := payloadtest.NewCountingStore()
	store.FailWrites(errors.New("bucket unreachable"))
	repo := event.NewRepository(pool).UsePayloads(payloadtest.WritingService(t, store, writeLimits))
	typ := writeType()
	pad := func(n int) string { return `"` + strings.Repeat("p", n-2) + `"` }

	evs := []event.Event{
		newEvent(typ, "", `{"ok":1}`),
		newEvent(typ, "", pad(6000)),  // offloadable, fits the database: kept inline
		newEvent(typ, "", pad(10000)), // offloadable, too large for the database: retryable
		newEvent(typ, "", pad(20000)), // over the largest payload: refused
		newEvent(typ, "", "\"a\xffb\""),
		newEvent(typ, "", `{"ok":2}`),
	}
	res, err := repo.Ingest(ctx, evs)
	require.NoError(t, err)
	assert.Equal(t, 3, res.Inserted)
	require.Len(t, res.Refused, 6)
	assert.NoError(t, res.Refused[0])
	assert.NoError(t, res.Refused[1])
	assert.ErrorIs(t, res.Refused[2], payloadstore.ErrStoreUnavailable)
	assert.ErrorIs(t, res.Refused[3], payloadstore.ErrTooLarge)
	assert.ErrorIs(t, res.Refused[4], payloadstore.ErrNotText)
	assert.NoError(t, res.Refused[5])

	r := readEventRow(t, pool, evs[1].ID)
	require.NotNil(t, r.pData, "kept in the database while the store is down")
	assert.Nil(t, r.pRef)
	for _, i := range []int{2, 3, 4} {
		assert.False(t, readEventRow(t, pool, evs[i].ID).found, "refused event %d is not stored", i)
	}
	assert.True(t, readEventRow(t, pool, evs[5].ID).found)

	// InsertBatch answers the first refusal.
	_, err = repo.InsertBatch(ctx, []event.Event{newEvent(typ, "", pad(20000))})
	assert.ErrorIs(t, err, payloadstore.ErrTooLarge)
}

// A dropped duplicate leaves no payload row: one dropped by the
// deduplication lookup is never prepared, and one dropped by the insert's ON
// CONFLICT (here a repeated id under the same created_at) does not insert the
// payload row its statement carries.
func TestIngest_DuplicateLeavesNoPayloadRow(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := event.NewRepository(pool).UsePayloads(payloadtest.WritingService(t, nil, nil))
	typ := writeType()

	first := newEvent(typ, "", `{"n":1}`)
	first.DeduplicationID = "dedup-" + first.ID
	_, err := repo.Ingest(ctx, []event.Event{first})
	require.NoError(t, err)
	require.Equal(t, 1, payloadRowsAt(t, pool, first.CreatedAt))

	again := newEvent(typ, "", `{"n":2}`)
	again.DeduplicationID = first.DeduplicationID
	res, err := repo.Ingest(ctx, []event.Event{again})
	require.NoError(t, err)
	assert.Zero(t, res.Inserted)
	assert.Zero(t, payloadRowsAt(t, pool, again.CreatedAt), "a deduplicated event is not prepared")

	// The same id and created_at, a different deduplication id: the lookup
	// passes it and the event insert's ON CONFLICT drops it.
	clash := first
	clash.DeduplicationID = "other-" + first.ID
	clash.Data = json.RawMessage(`{"n":3}`)
	res, err = repo.Ingest(ctx, []event.Event{clash})
	require.NoError(t, err)
	assert.Zero(t, res.Inserted)
	assert.Equal(t, 1, payloadRowsAt(t, pool, first.CreatedAt), "no payload row without its event row")
	r := readEventRow(t, pool, first.ID)
	require.NotNil(t, r.pData)
	assert.Equal(t, `{"n":1}`, *r.pData)
}

// payloadRowsAt counts the payload rows stored under createdAt (an event's).
func payloadRowsAt(t *testing.T, pool *pgxpool.Pool, createdAt time.Time) int {
	t.Helper()
	var n int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT count(*) FROM msg_payloads WHERE created_at = $1`, createdAt).Scan(&n))
	return n
}
