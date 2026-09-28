package fn

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

// driverName is the database/sql driver name the fn SDK registers, so
// guest code can write sql.Open("fc", "main") (plan §9). DBBinding.SQL
// wraps this for the common case of one *sql.DB per declared binding.
const driverName = "fc"

func init() {
	sql.Register(driverName, fcDriver{})
}

// SQL returns a *sql.DB for this binding, backed by the "fc" driver (host
// ops 6-10). The *sql.DB is created lazily and cached: repeated calls
// return the same instance.
func (d *DBBinding) SQL() *sql.DB {
	d.once.Do(func() {
		// sql.Open never dials; it only validates the driver name, which
		// this package always registers in init(). The error is therefore
		// unreachable in practice, but checked defensively rather than
		// ignored.
		db, err := sql.Open(driverName, d.name)
		if err != nil {
			panic("fn: db: " + err.Error())
		}
		d.db = db
	})
	return d.db
}

// --- driver.Driver -----------------------------------------------------

type fcDriver struct{}

func (fcDriver) Open(name string) (driver.Conn, error) {
	return &fcConn{db: name}, nil
}

// --- driver.Conn ---------------------------------------------------------

type fcConn struct {
	db string
	tx string // non-empty while a transaction is open on this connection
}

func (c *fcConn) Prepare(query string) (driver.Stmt, error) {
	return &fcStmt{conn: c, query: query}, nil
}

func (c *fcConn) Close() error { return nil }

func (c *fcConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

func (c *fcConn) BeginTx(ctx context.Context, _ driver.TxOptions) (driver.Tx, error) {
	mb, err := json.Marshal(dbBeginMeta{DB: c.db})
	if err != nil {
		return nil, err
	}
	rm, _, hostErr, err := currentHost.call(opDBBegin, mb, nil)
	if err != nil {
		return nil, err
	}
	if hostErr != nil {
		return nil, hostErr
	}
	var out struct {
		Tx string `json:"tx"`
	}
	if len(rm) > 0 {
		if uerr := json.Unmarshal(rm, &out); uerr != nil {
			return nil, uerr
		}
	}
	c.tx = out.Tx
	return &fcTx{conn: c}, nil
}

func (c *fcConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	params, err := convertParams(args)
	if err != nil {
		return nil, err
	}
	mb, err := json.Marshal(dbQueryMeta{DB: c.db, SQL: query, Params: params, Tx: c.tx})
	if err != nil {
		return nil, err
	}
	_, rb, hostErr, err := currentHost.call(opDBQuery, mb, nil)
	if err != nil {
		return nil, err
	}
	if hostErr != nil {
		return nil, hostErr
	}
	return parseRowsJSON(rb)
}

func (c *fcConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	params, err := convertParams(args)
	if err != nil {
		return nil, err
	}
	mb, err := json.Marshal(dbQueryMeta{DB: c.db, SQL: query, Params: params, Tx: c.tx})
	if err != nil {
		return nil, err
	}
	rm, _, hostErr, err := currentHost.call(opDBExec, mb, nil)
	if err != nil {
		return nil, err
	}
	if hostErr != nil {
		return nil, hostErr
	}
	var out struct {
		RowsAffected int64 `json:"rowsAffected"`
	}
	if len(rm) > 0 {
		if uerr := json.Unmarshal(rm, &out); uerr != nil {
			return nil, uerr
		}
	}
	return fcResult{rowsAffected: out.RowsAffected}, nil
}

// --- driver.Stmt (fallback for callers that go through database/sql's
// legacy Prepare/Exec/Query path instead of the *Context variants above)

type fcStmt struct {
	conn  *fcConn
	query string
}

func (s *fcStmt) Close() error  { return nil }
func (s *fcStmt) NumInput() int { return -1 } // let database/sql skip arg-count validation

func (s *fcStmt) Exec(args []driver.Value) (driver.Result, error) {
	return s.conn.ExecContext(context.Background(), s.query, valuesToNamed(args))
}

func (s *fcStmt) Query(args []driver.Value) (driver.Rows, error) {
	return s.conn.QueryContext(context.Background(), s.query, valuesToNamed(args))
}

func valuesToNamed(args []driver.Value) []driver.NamedValue {
	nv := make([]driver.NamedValue, len(args))
	for i, a := range args {
		nv[i] = driver.NamedValue{Ordinal: i + 1, Value: a}
	}
	return nv
}

// --- driver.Tx -------------------------------------------------------------

type fcTx struct{ conn *fcConn }

func (t *fcTx) Commit() error {
	mb, err := json.Marshal(dbTxMeta{Tx: t.conn.tx})
	if err != nil {
		return err
	}
	_, _, hostErr, err := currentHost.call(opDBCommit, mb, nil)
	t.conn.tx = ""
	if err != nil {
		return err
	}
	if hostErr != nil {
		return hostErr
	}
	return nil
}

func (t *fcTx) Rollback() error {
	mb, err := json.Marshal(dbTxMeta{Tx: t.conn.tx})
	if err != nil {
		return err
	}
	_, _, hostErr, err := currentHost.call(opDBRollback, mb, nil)
	t.conn.tx = ""
	if err != nil {
		return err
	}
	if hostErr != nil {
		return hostErr
	}
	return nil
}

// --- driver.Result -----------------------------------------------------

type fcResult struct{ rowsAffected int64 }

func (r fcResult) LastInsertId() (int64, error) {
	return 0, errors.New("fn: LastInsertId is not supported by the fc driver")
}
func (r fcResult) RowsAffected() (int64, error) { return r.rowsAffected, nil }

// --- request/response meta shapes (plan §5.3 ops 6-10) --------------------

type dbQueryMeta struct {
	DB     string `json:"db"`
	SQL    string `json:"sql"`
	Params []any  `json:"params,omitempty"`
	Tx     string `json:"tx,omitempty"`
}

type dbBeginMeta struct {
	DB string `json:"db"`
}

type dbTxMeta struct {
	Tx string `json:"tx"`
}

// convertParams maps database/sql's bound values onto the JSON param types
// the ABI carries. database/sql's default converter has already reduced
// args to int64, float64, bool, []byte, string, time.Time or nil before
// this is called.
//
// Decision (plan §5.3 leaves this unspecified): []byte params are
// base64-encoded (standard encoding) into a JSON string. The host side must
// mirror this to support bytea columns; there is no type tag in the wire
// params array to distinguish an intentional string from a base64'd blob.
// time.Time is encoded as RFC3339Nano, matching the task's explicit choice.
func convertParams(args []driver.NamedValue) ([]any, error) {
	if len(args) == 0 {
		return nil, nil
	}
	out := make([]any, len(args))
	for i, a := range args {
		switch v := a.Value.(type) {
		case nil:
			out[i] = nil
		case int64:
			out[i] = v
		case float64:
			out[i] = v
		case bool:
			out[i] = v
		case string:
			out[i] = v
		case []byte:
			out[i] = base64.StdEncoding.EncodeToString(v)
		case time.Time:
			out[i] = v.Format(time.RFC3339Nano)
		default:
			return nil, fmt.Errorf("fn: db: unsupported parameter type %T at position %d", a.Value, i+1)
		}
	}
	return out, nil
}

// --- row decoding: preserves column order per row -------------------------

// fcRows implements driver.Rows over the host's JSON array-of-objects
// result (plan §5.3 op 6: "body = JSON array of row objects"). Column
// order is taken from the first row's key order, per the task's decision
// that "the host will emit keys in select order".
type fcRows struct {
	columns []string
	rows    [][]driver.Value
	idx     int
}

func (r *fcRows) Columns() []string { return r.columns }
func (r *fcRows) Close() error      { return nil }

func (r *fcRows) Next(dest []driver.Value) error {
	if r.idx >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.idx])
	r.idx++
	return nil
}

// parseRowsJSON decodes a JSON array of row objects into an fcRows,
// preserving each row's key order via streaming tokens (encoding/json's
// map decoding does not preserve key order, so this cannot use json.Unmarshal
// into map[string]any for the top-level row object).
func parseRowsJSON(data []byte) (*fcRows, error) {
	if len(data) == 0 {
		return &fcRows{}, nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()

	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '[' {
		return nil, fmt.Errorf("fn: db: expected a JSON array of rows, got %v", tok)
	}

	var columns []string
	colIndex := make(map[string]int)
	var rowsOut [][]driver.Value

	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return nil, err
		}
		if d, ok := t.(json.Delim); !ok || d != '{' {
			return nil, fmt.Errorf("fn: db: expected a row object, got %v", t)
		}
		row := make([]driver.Value, len(columns))
		for dec.More() {
			keyTok, err := dec.Token()
			if err != nil {
				return nil, err
			}
			key, ok := keyTok.(string)
			if !ok {
				return nil, fmt.Errorf("fn: db: expected a string field name, got %v", keyTok)
			}
			valTok, err := dec.Token()
			if err != nil {
				return nil, err
			}
			val, err := decodeJSONValue(dec, valTok)
			if err != nil {
				return nil, err
			}
			dv, err := toDriverValue(val)
			if err != nil {
				return nil, err
			}
			idx, ok := colIndex[key]
			if !ok {
				idx = len(columns)
				columns = append(columns, key)
				colIndex[key] = idx
				row = append(row, nil)
			}
			row[idx] = dv
		}
		if _, err := dec.Token(); err != nil { // consume closing '}'
			return nil, err
		}
		rowsOut = append(rowsOut, row)
	}
	if _, err := dec.Token(); err != nil { // consume closing ']'
		return nil, err
	}

	for i := range rowsOut {
		for len(rowsOut[i]) < len(columns) {
			rowsOut[i] = append(rowsOut[i], nil)
		}
	}
	return &fcRows{columns: columns, rows: rowsOut}, nil
}

// decodeJSONValue fully decodes a JSON value (scalar or nested
// object/array) starting from an already-read token. Nested
// objects/arrays use plain maps/slices: only the top-level row object's key
// order matters (it defines the SQL column order), not the internal
// ordering of a JSON/JSONB column's own value.
func decodeJSONValue(dec *json.Decoder, tok json.Token) (any, error) {
	d, ok := tok.(json.Delim)
	if !ok {
		return tok, nil // nil, bool, json.Number or string
	}
	switch d {
	case '{':
		m := map[string]any{}
		for dec.More() {
			kt, err := dec.Token()
			if err != nil {
				return nil, err
			}
			k, ok := kt.(string)
			if !ok {
				return nil, fmt.Errorf("fn: db: expected a string field name, got %v", kt)
			}
			vt, err := dec.Token()
			if err != nil {
				return nil, err
			}
			v, err := decodeJSONValue(dec, vt)
			if err != nil {
				return nil, err
			}
			m[k] = v
		}
		if _, err := dec.Token(); err != nil {
			return nil, err
		}
		return m, nil
	case '[':
		var arr []any
		for dec.More() {
			vt, err := dec.Token()
			if err != nil {
				return nil, err
			}
			v, err := decodeJSONValue(dec, vt)
			if err != nil {
				return nil, err
			}
			arr = append(arr, v)
		}
		if _, err := dec.Token(); err != nil {
			return nil, err
		}
		return arr, nil
	default:
		return nil, fmt.Errorf("fn: db: unexpected JSON delimiter %v", d)
	}
}

// toDriverValue converts a decodeJSONValue result into a database/sql
// driver.Value. Nested objects/arrays (JSON/JSONB columns) are re-marshaled
// to a JSON string: the SDK exposes them as text, leaving structured
// decoding to the caller (documented decision; the ABI carries no per-column
// type information).
func toDriverValue(v any) (driver.Value, error) {
	switch x := v.(type) {
	case nil:
		return nil, nil
	case bool:
		return x, nil
	case string:
		return x, nil
	case json.Number:
		if i, err := x.Int64(); err == nil {
			return i, nil
		}
		f, err := x.Float64()
		if err != nil {
			return nil, fmt.Errorf("fn: db: unparseable row number %q: %w", x.String(), err)
		}
		return f, nil
	case map[string]any, []any:
		b, err := json.Marshal(x)
		if err != nil {
			return nil, err
		}
		return string(b), nil
	default:
		return nil, fmt.Errorf("fn: db: unsupported row value type %T", v)
	}
}
