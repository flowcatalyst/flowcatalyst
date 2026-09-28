package runner

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
)

// Query result caps (plan §6.3).
const (
	maxRows     = 10_000
	maxRowBytes = 8 << 20
)

// DBProvider hands out the runner-owned pool for a DSN.
type DBProvider interface {
	Pool(ctx context.Context, dsn string) (*pgxpool.Pool, error)
}

// dbPools shares one pgxpool per distinct DSN across every function on the
// runner, bounded in count and closed when idle.
type dbPools struct {
	mu       sync.Mutex
	pools    map[string]*dbPool
	maxPools int
	maxConns int32
}

type dbPool struct {
	pool     *pgxpool.Pool
	lastUsed time.Time
}

func newDBPools(maxPools int, maxConns int32) *dbPools {
	return &dbPools{pools: map[string]*dbPool{}, maxPools: maxPools, maxConns: maxConns}
}

func (d *dbPools) Pool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if p, ok := d.pools[dsn]; ok {
		p.lastUsed = time.Now()
		return p.pool, nil
	}
	if len(d.pools) >= d.maxPools {
		// Close the least recently used pool to make room.
		var oldest string
		for k, p := range d.pools {
			if oldest == "" || p.lastUsed.Before(d.pools[oldest].lastUsed) {
				oldest = k
			}
		}
		d.pools[oldest].pool.Close()
		delete(d.pools, oldest)
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, errors.New("the database binding's DSN is not valid") // never echo the DSN
	}
	cfg.MaxConns = d.maxConns
	cfg.MaxConnIdleTime = 5 * time.Minute
	p, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, errors.New("the database could not be reached")
	}
	d.pools[dsn] = &dbPool{pool: p, lastUsed: time.Now()}
	return p, nil
}

// closeIdle closes pools unused for longer than ttl.
func (d *dbPools) closeIdle(ttl time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for k, p := range d.pools {
		if time.Since(p.lastUsed) > ttl {
			p.pool.Close()
			delete(d.pools, k)
		}
	}
}

func (d *dbPools) closeAll() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for k, p := range d.pools {
		p.pool.Close()
		delete(d.pools, k)
	}
}

// txSet is an invocation's open transactions. Whatever is still open when the
// invocation ends is rolled back.
type txSet struct {
	mu  sync.Mutex
	txs map[string]openTx
}

type openTx struct {
	db string
	tx pgx.Tx
}

func (t *txSet) add(db string, tx pgx.Tx) string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	id := hex.EncodeToString(b)
	t.mu.Lock()
	if t.txs == nil {
		t.txs = map[string]openTx{}
	}
	t.txs[id] = openTx{db: db, tx: tx}
	t.mu.Unlock()
	return id
}

func (t *txSet) take(id string) (openTx, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	o, ok := t.txs[id]
	delete(t.txs, id)
	return o, ok
}

func (t *txSet) get(id string) (openTx, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	o, ok := t.txs[id]
	return o, ok
}

func (t *txSet) rollbackAll() {
	t.mu.Lock()
	txs := t.txs
	t.txs = nil
	t.mu.Unlock()
	for _, o := range txs {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = o.tx.Rollback(ctx)
		cancel()
	}
}

// querier is what a statement runs on: a pool or a transaction.
type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func (c *capabilities) dbCall(ctx context.Context, op abi.Op, meta []byte) ([]byte, []byte, *abi.Error) {
	if c.database == nil {
		return nil, nil, abi.Errorf(abi.CodeCapabilityUnavailable, "databases are not available on this runner")
	}
	switch op {
	case abi.OpDBBegin:
		b, aerr := decode[abi.DBBegin](meta)
		if aerr != nil {
			return nil, nil, aerr
		}
		pool, aerr := c.pool(ctx, b.DB)
		if aerr != nil {
			return nil, nil, aerr
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			return nil, nil, dbError(ctx, err)
		}
		return encode(abi.DBTx{Tx: c.tx.add(b.DB, tx)}), nil, nil
	case abi.OpDBCommit, abi.OpDBRollback:
		t, aerr := decode[abi.DBTx](meta)
		if aerr != nil {
			return nil, nil, aerr
		}
		o, ok := c.tx.take(t.Tx)
		if !ok {
			return nil, nil, abi.Errorf(abi.CodeDBTxUnknown, "no open transaction with that id in this invocation")
		}
		var err error
		if op == abi.OpDBCommit {
			err = o.tx.Commit(ctx)
		} else {
			err = o.tx.Rollback(ctx)
		}
		if err != nil {
			return nil, nil, dbError(ctx, err)
		}
		return nil, nil, nil
	}

	st, aerr := decodeStatement(meta)
	if aerr != nil {
		return nil, nil, aerr
	}
	var q querier
	if st.Tx != "" {
		o, ok := c.tx.get(st.Tx)
		if !ok || o.db != st.DB {
			return nil, nil, abi.Errorf(abi.CodeDBTxUnknown, "no open transaction with that id on that database in this invocation")
		}
		q = o.tx
	} else {
		pool, aerr := c.pool(ctx, st.DB)
		if aerr != nil {
			return nil, nil, aerr
		}
		q = pool
	}
	if _, ok := ctx.Deadline(); ok && ctx.Err() != nil {
		return nil, nil, abi.Errorf(abi.CodeDBTimeout, "no time left before the invocation's deadline")
	}
	sql := rewritePlaceholders(st.SQL)
	if op == abi.OpDBExec {
		tag, err := q.Exec(ctx, sql, st.Params...)
		if err != nil {
			return nil, nil, dbError(ctx, err)
		}
		return encode(abi.DBExecuted{RowsAffected: tag.RowsAffected()}), nil, nil
	}
	rows, err := q.Query(ctx, sql, st.Params...)
	if err != nil {
		return nil, nil, dbError(ctx, err)
	}
	defer rows.Close()
	body, truncated, err := rowsJSON(rows)
	if err != nil {
		return nil, nil, dbError(ctx, err)
	}
	return encode(abi.DBRows{Truncated: truncated}), body, nil
}

func (c *capabilities) pool(ctx context.Context, db string) (*pgxpool.Pool, *abi.Error) {
	if !slices.Contains(c.ver.describe.DB, db) {
		return nil, abi.Errorf(abi.CodeNotDeclared, fmt.Sprintf("database %q is not declared by this version", db))
	}
	dsn, ok := c.fn.snapshot().DB[db]
	if !ok || dsn == "" {
		return nil, abi.Errorf(abi.CodeNotDeclared, fmt.Sprintf("database %q has no binding on this function", db))
	}
	p, err := c.database.Pool(ctx, dsn)
	if err != nil {
		return nil, abi.Errorf(abi.CodeDBUnavailable, err.Error())
	}
	return p, nil
}

// decodeStatement decodes params keeping integers exact: an integer goes to
// the server as int8, any other number as exact numeric text.
func decodeStatement(meta []byte) (abi.DBStatement, *abi.Error) {
	var st abi.DBStatement
	dec := json.NewDecoder(bytes.NewReader(meta))
	dec.UseNumber()
	if err := dec.Decode(&st); err != nil {
		return st, abi.Errorf(abi.CodeBadRequest, "meta is not a statement: "+err.Error())
	}
	if st.DB == "" || strings.TrimSpace(st.SQL) == "" {
		return st, abi.Errorf(abi.CodeBadRequest, "db and sql are required")
	}
	for i, p := range st.Params {
		switch v := p.(type) {
		case json.Number:
			if n, err := v.Int64(); err == nil {
				st.Params[i] = n
			} else {
				st.Params[i] = v.String()
			}
		case string, bool, nil:
		default:
			return st, abi.Errorf(abi.CodeBadRequest, fmt.Sprintf("params[%d] must be a string, number, boolean or null", i))
		}
	}
	return st, nil
}

// rewritePlaceholders turns `?` placeholders into $1..$n, leaving `?` inside
// string literals, quoted identifiers, dollar-quoted strings and comments alone.
func rewritePlaceholders(sql string) string {
	var b strings.Builder
	b.Grow(len(sql) + 8)
	n := 0
	for i := 0; i < len(sql); i++ {
		ch := sql[i]
		switch {
		case ch == '\'' || ch == '"':
			j := i + 1
			for j < len(sql) {
				if sql[j] == ch {
					if j+1 < len(sql) && sql[j+1] == ch { // doubled quote escape
						j += 2
						continue
					}
					break
				}
				j++
			}
			end := min(j+1, len(sql))
			b.WriteString(sql[i:end])
			i = end - 1
		case ch == '-' && i+1 < len(sql) && sql[i+1] == '-':
			j := strings.IndexByte(sql[i:], '\n')
			if j < 0 {
				j = len(sql) - i
			}
			b.WriteString(sql[i : i+j])
			i += j - 1
		case ch == '/' && i+1 < len(sql) && sql[i+1] == '*':
			j := strings.Index(sql[i+2:], "*/")
			end := len(sql)
			if j >= 0 {
				end = i + 2 + j + 2
			}
			b.WriteString(sql[i:end])
			i = end - 1
		case ch == '$':
			// $tag$ ... $tag$ dollar quoting ($1 style params pass through).
			j := i + 1
			for j < len(sql) && (sql[j] == '_' || sql[j] >= 'a' && sql[j] <= 'z' || sql[j] >= 'A' && sql[j] <= 'Z' || (j > i+1 && sql[j] >= '0' && sql[j] <= '9')) {
				j++
			}
			if j < len(sql) && sql[j] == '$' {
				tag := sql[i : j+1]
				k := strings.Index(sql[j+1:], tag)
				end := len(sql)
				if k >= 0 {
					end = j + 1 + k + len(tag)
				}
				b.WriteString(sql[i:end])
				i = end - 1
			} else {
				b.WriteByte(ch)
			}
		case ch == '?':
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
		default:
			b.WriteByte(ch)
		}
	}
	return b.String()
}

// rowsJSON renders rows as a JSON array of objects, keys in select order,
// stopping at maxRows or maxRowBytes.
func rowsJSON(rows pgx.Rows) ([]byte, bool, error) {
	fields := rows.FieldDescriptions()
	keys := make([][]byte, len(fields))
	for i, f := range fields {
		keys[i], _ = json.Marshal(f.Name)
	}
	var buf bytes.Buffer
	buf.WriteByte('[')
	count, truncated := 0, false
	for rows.Next() {
		if count == maxRows || buf.Len() > maxRowBytes {
			truncated = true
			break
		}
		vals, err := rows.Values()
		if err != nil {
			return nil, false, err
		}
		if count > 0 {
			buf.WriteByte(',')
		}
		buf.WriteByte('{')
		for i, v := range vals {
			if i > 0 {
				buf.WriteByte(',')
			}
			buf.Write(keys[i])
			buf.WriteByte(':')
			buf.Write(jsonValue(v))
		}
		buf.WriteByte('}')
		count++
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	buf.WriteByte(']')
	return buf.Bytes(), truncated, nil
}

func jsonValue(v any) []byte {
	switch x := v.(type) {
	case nil:
		return []byte("null")
	case time.Time:
		b, _ := json.Marshal(x.UTC().Format(time.RFC3339Nano))
		return b
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			b, _ := json.Marshal(strconv.FormatFloat(x, 'g', -1, 64))
			return b
		}
	case float32:
		if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) {
			b, _ := json.Marshal(strconv.FormatFloat(float64(x), 'g', -1, 32))
			return b
		}
	case pgtype.Numeric:
		if !x.Valid {
			return []byte("null")
		}
		t, err := x.Value()
		if err == nil {
			b, _ := json.Marshal(t)
			return b
		}
	case [16]byte: // uuid
		b, _ := json.Marshal(fmt.Sprintf("%x-%x-%x-%x-%x", x[0:4], x[4:6], x[6:8], x[8:10], x[10:16]))
		return b
	case fmt.Stringer:
		b, _ := json.Marshal(x.String())
		return b
	}
	b, err := json.Marshal(v)
	if err != nil {
		b, _ = json.Marshal(fmt.Sprint(v))
	}
	return b
}

// dbError maps a driver error to an ABI error. The message is the driver's;
// the SQL and its parameters are never included.
func dbError(ctx context.Context, err error) *abi.Error {
	if ctx.Err() != nil {
		return abi.Errorf(abi.CodeDBTimeout, "the statement did not finish before the invocation's deadline")
	}
	if pg, ok := errors.AsType[*pgconn.PgError](err); ok {
		switch {
		case strings.HasPrefix(pg.Code, "23"):
			return abi.Errorf(abi.CodeDBConstraint, pg.Message)
		case strings.HasPrefix(pg.Code, "42"):
			return abi.Errorf(abi.CodeDBSyntax, pg.Message)
		case pg.Code == "57014":
			return abi.Errorf(abi.CodeDBTimeout, pg.Message)
		case strings.HasPrefix(pg.Code, "08"), strings.HasPrefix(pg.Code, "53"), strings.HasPrefix(pg.Code, "57"):
			return abi.Errorf(abi.CodeDBUnavailable, pg.Message)
		}
		return abi.Errorf(abi.CodeDBError, pg.Message)
	}
	if pgconn.SafeToRetry(err) || errors.Is(err, pgx.ErrTxClosed) {
		return abi.Errorf(abi.CodeDBUnavailable, err.Error())
	}
	return abi.Errorf(abi.CodeDBError, err.Error())
}
