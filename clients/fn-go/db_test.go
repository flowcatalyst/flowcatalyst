//go:build !wasip1

package fn

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestDBQueryPreservesColumnOrderAndTypes(t *testing.T) {
	fake := &FakeHost{Handler: func(op int32, meta, body []byte) ([]byte, []byte, *Error, error) {
		if op != int32(opDBQuery) {
			t.Fatalf("op = %d, want db.query", op)
		}
		var m dbQueryMeta
		_ = json.Unmarshal(meta, &m)
		if m.DB != "main" {
			t.Fatalf("db = %q", m.DB)
		}
		if m.SQL != "select id, name, active from widgets where id = ?" {
			t.Fatalf("sql = %q", m.SQL)
		}
		if len(m.Params) != 1 || m.Params[0].(float64) != 7 {
			t.Fatalf("params = %v", m.Params)
		}
		rows := `[{"z_last":"z","id":1,"name":"a","active":true},{"z_last":"y","id":2,"name":"b","active":false}]`
		rm, _ := json.Marshal(struct {
			Truncated bool `json:"truncated"`
		}{})
		return rm, []byte(rows), nil, nil
	}}
	defer SetHost(fake)()

	db := DB("main").SQL()
	rows, err := db.QueryContext(context.Background(), "select id, name, active from widgets where id = ?", 7)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		t.Fatalf("Columns: %v", err)
	}
	wantCols := []string{"z_last", "id", "name", "active"}
	if len(cols) != len(wantCols) {
		t.Fatalf("cols = %v, want %v", cols, wantCols)
	}
	for i, c := range wantCols {
		if cols[i] != c {
			t.Fatalf("cols[%d] = %q, want %q (column order must follow the host's key order, not be re-sorted)", i, cols[i], c)
		}
	}

	var n int
	var zLast, name string
	var active bool
	if !rows.Next() {
		t.Fatal("expected a row")
	}
	if err := rows.Scan(&zLast, &n, &name, &active); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if zLast != "z" || n != 1 || name != "a" || active != true {
		t.Fatalf("row 1 = %q %d %q %v", zLast, n, name, active)
	}
	if !rows.Next() {
		t.Fatal("expected a second row")
	}
	if err := rows.Scan(&zLast, &n, &name, &active); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if zLast != "y" || n != 2 || name != "b" || active != false {
		t.Fatalf("row 2 = %q %d %q %v", zLast, n, name, active)
	}
	if rows.Next() {
		t.Fatal("expected exactly two rows")
	}
}

func TestDBQueryEmptyResult(t *testing.T) {
	fake := &FakeHost{Handler: func(op int32, meta, body []byte) ([]byte, []byte, *Error, error) {
		return nil, []byte(`[]`), nil, nil
	}}
	defer SetHost(fake)()

	db := DB("main").SQL()
	rows, err := db.QueryContext(context.Background(), "select 1 where false")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("want zero rows")
	}
}

func TestDBExecRowsAffected(t *testing.T) {
	fake := &FakeHost{Handler: func(op int32, meta, body []byte) ([]byte, []byte, *Error, error) {
		if op != int32(opDBExec) {
			t.Fatalf("op = %d, want db.exec", op)
		}
		rm, _ := json.Marshal(struct {
			RowsAffected int64 `json:"rowsAffected"`
		}{RowsAffected: 3})
		return rm, nil, nil, nil
	}}
	defer SetHost(fake)()

	db := DB("main").SQL()
	res, err := db.ExecContext(context.Background(), "update widgets set active = ?", true)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	n, err := res.RowsAffected()
	if err != nil || n != 3 {
		t.Fatalf("RowsAffected = %d, %v, want 3", n, err)
	}
}

func TestDBParamEncoding(t *testing.T) {
	var gotParams []any
	fake := &FakeHost{Handler: func(op int32, meta, body []byte) ([]byte, []byte, *Error, error) {
		var m dbQueryMeta
		_ = json.Unmarshal(meta, &m)
		gotParams = m.Params
		rm, _ := json.Marshal(struct {
			RowsAffected int64 `json:"rowsAffected"`
		}{})
		return rm, nil, nil, nil
	}}
	defer SetHost(fake)()

	when := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	db := DB("main").SQL()
	_, err := db.ExecContext(context.Background(), "insert into x (a,b,c,d,e) values (?,?,?,?,?)",
		"str", int64(5), true, when, []byte{0xDE, 0xAD, 0xBE, 0xEF})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if len(gotParams) != 5 {
		t.Fatalf("params = %v", gotParams)
	}
	if gotParams[0] != "str" {
		t.Errorf("param0 = %v, want str", gotParams[0])
	}
	if gotParams[1].(float64) != 5 {
		t.Errorf("param1 = %v, want 5", gotParams[1])
	}
	if gotParams[2] != true {
		t.Errorf("param2 = %v, want true", gotParams[2])
	}
	if gotParams[3] != when.Format(time.RFC3339Nano) {
		t.Errorf("param3 = %v, want RFC3339Nano %s", gotParams[3], when.Format(time.RFC3339Nano))
	}
	if gotParams[4] != "3q2+7w==" { // base64 of 0xDEADBEEF
		t.Errorf("param4 = %v, want base64 of the bytes", gotParams[4])
	}
}

func TestDBTransactionCommit(t *testing.T) {
	var ops []int32
	var seenTx string
	fake := &FakeHost{Handler: func(op int32, meta, body []byte) ([]byte, []byte, *Error, error) {
		ops = append(ops, op)
		switch opCode(op) {
		case opDBBegin:
			rm, _ := json.Marshal(struct {
				Tx string `json:"tx"`
			}{Tx: "tx_1"})
			return rm, nil, nil, nil
		case opDBExec:
			var m dbQueryMeta
			_ = json.Unmarshal(meta, &m)
			seenTx = m.Tx
			rm, _ := json.Marshal(struct {
				RowsAffected int64 `json:"rowsAffected"`
			}{RowsAffected: 1})
			return rm, nil, nil, nil
		case opDBCommit:
			var m dbTxMeta
			_ = json.Unmarshal(meta, &m)
			if m.Tx != "tx_1" {
				t.Fatalf("commit tx = %q, want tx_1", m.Tx)
			}
			return nil, nil, nil, nil
		}
		return nil, nil, nil, nil
	}}
	defer SetHost(fake)()

	db := DB("main").SQL()
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	if _, err := tx.ExecContext(context.Background(), "update x set y = 1"); err != nil {
		t.Fatalf("Exec in tx: %v", err)
	}
	if seenTx != "tx_1" {
		t.Fatalf("exec inside tx did not carry the tx id: got %q", seenTx)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	want := []int32{int32(opDBBegin), int32(opDBExec), int32(opDBCommit)}
	if len(ops) != len(want) {
		t.Fatalf("ops = %v, want %v", ops, want)
	}
	for i := range want {
		if ops[i] != want[i] {
			t.Fatalf("ops[%d] = %d, want %d", i, ops[i], want[i])
		}
	}
}

func TestDBTransactionRollback(t *testing.T) {
	rolledBack := false
	fake := &FakeHost{Handler: func(op int32, meta, body []byte) ([]byte, []byte, *Error, error) {
		switch opCode(op) {
		case opDBBegin:
			rm, _ := json.Marshal(struct {
				Tx string `json:"tx"`
			}{Tx: "tx_2"})
			return rm, nil, nil, nil
		case opDBRollback:
			rolledBack = true
			return nil, nil, nil, nil
		}
		return nil, nil, nil, nil
	}}
	defer SetHost(fake)()

	db := DB("main").SQL()
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("BeginTx: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if !rolledBack {
		t.Fatal("rollback op was not called")
	}
}

func TestDBQueryHostErrorClassified(t *testing.T) {
	fake := &FakeHost{Handler: func(op int32, meta, body []byte) ([]byte, []byte, *Error, error) {
		return nil, nil, &Error{Code: ErrCodeDBConstraint, Message: "unique violation"}, nil
	}}
	defer SetHost(fake)()

	db := DB("main").SQL()
	_, err := db.ExecContext(context.Background(), "insert into x values (1)")
	if err == nil {
		t.Fatal("want error")
	}
	var fe *Error
	if e, ok := asFcError(err); ok {
		fe = e
	}
	if fe == nil || fe.Code != ErrCodeDBConstraint {
		t.Fatalf("err = %v, want *Error{Code: DB_CONSTRAINT}", err)
	}
}

func asFcError(err error) (*Error, bool) {
	if err == nil {
		return nil, false
	}
	fe := &Error{}
	ok := errors.As(err, &fe)
	return fe, ok
}

func TestDBBindingSQLIsCached(t *testing.T) {
	b := DB("main")
	first, second := b.SQL(), b.SQL()
	if first != second {
		t.Error("DBBinding.SQL() should return the same *sql.DB on repeated calls")
	}
}

var _ = sql.ErrNoRows // ensure database/sql import stays even if scans change
