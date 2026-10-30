package outboxsql

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/go-sdk/outbox"
)

type recorder struct {
	queries []string
	args    [][]any
}

func (r *recorder) ExecContext(_ context.Context, q string, args ...any) (sql.Result, error) {
	r.queries = append(r.queries, q)
	r.args = append(r.args, args)
	return nil, nil
}

type evt struct{}

func (evt) EventID() string             { return "0ABCDEFGHJKMN" }
func (evt) EventType() string           { return "shop:sales:order:placed" }
func (evt) SpecVersion() string         { return "1.0" }
func (evt) Source() string              { return "shop:sales" }
func (evt) Subject() string             { return "sales.order.ord_1" }
func (evt) Time() time.Time             { return time.Unix(0, 0) }
func (evt) PrincipalID() string         { return "prn_1" }
func (evt) CorrelationID() string       { return "" }
func (evt) CausationID() string         { return "" }
func (evt) ExecutionID() string         { return "" }
func (evt) MessageGroup() string        { return "sales:order:ord_1" }
func (evt) ToDataJSON() ([]byte, error) { return []byte(`{"a":1}`), nil }

func TestWriterWritesTheSharedRows(t *testing.T) {
	now := time.Date(2026, 10, 8, 10, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		dialect Dialect
		marker  string
	}{{DialectMySQL, "?"}, {DialectPostgres, "$10"}} {
		w, err := NewWriter(c.dialect, WriterConfig{ClientID: "clt_1", ClientCode: "acme", Now: func() time.Time { return now }})
		require.NoError(t, err)
		rec := &recorder{}
		ctx := context.Background()
		require.NoError(t, w.WriteEvent(ctx, rec, evt{}))
		require.NoError(t, w.WriteAudit(ctx, rec, evt{}, nil))
		require.NoError(t, w.WriteDispatchJob(ctx, rec, outbox.NewDispatchJob("shop:x", "https://a.example.com").WithID("job-1")))
		require.Error(t, w.WriteDispatchJob(ctx, rec, outbox.NewDispatchJob("shop:x", "ftp://a.example.com")))
		require.Len(t, rec.queries, 3, "an invalid job is not written")
		for _, q := range rec.queries {
			assert.Contains(t, q, c.marker)
			assert.True(t, strings.HasPrefix(q, "INSERT INTO outbox_messages (\n"))
		}

		want, err := outbox.EventRow(outbox.Config{ClientID: "clt_1", ClientCode: "acme", Now: func() time.Time { return now }}, evt{})
		require.NoError(t, err)
		assert.Equal(t, want.Args(), rec.args[0], "the same row outboxpgx writes")
		assert.Equal(t, "AUDIT_LOG", rec.args[1][1])
		assert.Equal(t, "job-1", rec.args[2][0])
		assert.Equal(t, "DISPATCH_JOB", rec.args[2][1])
	}
	_, err := NewWriter(DialectPostgres, WriterConfig{TableName: "bad name"})
	require.Error(t, err)
}
