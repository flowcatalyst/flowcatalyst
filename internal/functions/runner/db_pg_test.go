//go:build integration

package runner

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

func dbFunction(t *testing.T) control.Function {
	var d map[string]any
	if err := json.Unmarshal([]byte(describeDoc), &d); err != nil {
		t.Fatal(err)
	}
	d["db"] = []string{"main"}
	doc, _ := json.Marshal(d)
	v := ver(1, control.RoleLive)
	v.Describe = doc
	fn := fnDoc(v)
	fn.DB = map[string]string{"main": testpg.Pool(t).Config().ConnString()}
	return fn
}

func TestDBCapability(t *testing.T) {
	h := start(t, dbFunction(t))
	table := fmt.Sprintf("fn_db_test_%d", time.Now().UnixNano())
	stmt := func(sql string, params ...any) abi.DBStatement {
		return abi.DBStatement{DB: "main", SQL: sql, Params: params}
	}

	code, meta, _ := hostCall(t, h, abi.OpDBExec, stmt("create table "+table+" (id int8 primary key, name text, amount numeric, at timestamptz, data jsonb, ok bool)"), "")
	if code != "0" {
		t.Fatalf("create: %s", meta)
	}
	t.Cleanup(func() { _, _ = testpg.Pool(t).Exec(t.Context(), "drop table if exists "+table) })

	code, meta, _ = hostCall(t, h, abi.OpDBExec, stmt("insert into "+table+" values (?, ?, ?, ?, ?, ?)",
		42, "it's ?", "12.50", "2026-09-24T10:15:30Z", `{"a":1}`, true), "")
	if code != "0" || meta != `{"rowsAffected":1}` {
		t.Fatalf("insert: %s %s", code, meta)
	}

	code, meta, body := hostCall(t, h, abi.OpDBQuery, stmt("select id, name, amount, at, data, ok, null as nothing from "+table+" where id = ?", 42), "")
	if code != "0" || meta != `{"truncated":false}` {
		t.Fatalf("select: %s %s", code, meta)
	}
	want := `[{"id":42,"name":"it's ?","amount":"12.50","at":"2026-09-24T10:15:30Z","data":{"a":1},"ok":true,"nothing":null}]`
	if body != want {
		t.Fatalf("rows\n got %s\nwant %s", body, want)
	}

	if code, meta, _ := hostCall(t, h, abi.OpDBExec, stmt("insert into "+table+" (id) values (?)", 42), ""); code != "1" || !strings.Contains(meta, abi.CodeDBConstraint) {
		t.Errorf("duplicate key: %s %s", code, meta)
	}
	if code, meta, _ := hostCall(t, h, abi.OpDBQuery, stmt("selec 1"), ""); code != "1" || !strings.Contains(meta, abi.CodeDBSyntax) {
		t.Errorf("syntax error: %s %s", code, meta)
	}
	if code, meta, _ := hostCall(t, h, abi.OpDBQuery, stmt("select 1", map[string]any{"x": 1}), ""); code != "1" || !strings.Contains(meta, abi.CodeBadRequest) {
		t.Errorf("object param: %s %s", code, meta)
	}

	code, meta, body = hostCall(t, h, abi.OpDBQuery, stmt("select g from generate_series(1, 10005) g"), "")
	var rows []map[string]any
	_ = json.Unmarshal([]byte(body), &rows)
	if code != "0" || meta != `{"truncated":true}` || len(rows) != maxRows {
		t.Errorf("truncation: %s %s rows=%d", code, meta, len(rows))
	}

	// A transaction lives only as long as the invocation that opened it.
	code, meta, _ = hostCall(t, h, abi.OpDBBegin, abi.DBBegin{DB: "main"}, "")
	var tx abi.DBTx
	_ = json.Unmarshal([]byte(meta), &tx)
	if code != "0" || tx.Tx == "" {
		t.Fatalf("begin: %s %s", code, meta)
	}
	if code, meta, _ := hostCall(t, h, abi.OpDBCommit, tx, ""); code != "1" || !strings.Contains(meta, abi.CodeDBTxUnknown) {
		t.Errorf("commit of another invocation's tx: %s %s", code, meta)
	}
	if code, meta, _ := hostCall(t, h, abi.OpDBQuery, abi.DBStatement{DB: "main", SQL: "select 1", Tx: tx.Tx}, ""); code != "1" || !strings.Contains(meta, abi.CodeDBTxUnknown) {
		t.Errorf("query in another invocation's tx: %s %s", code, meta)
	}
}
