package repocommon

import (
	"errors"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestOne(t *testing.T) {
	t.Parallel()

	type row struct{ ID string }

	if got, err := One(row{}, pgx.ErrNoRows, "things FindByID"); got != nil || err != nil {
		t.Errorf("One(ErrNoRows) = (%v, %v), want (nil, nil)", got, err)
	}

	boom := errors.New("boom")
	got, err := One(row{}, boom, "things FindByID")
	if got != nil {
		t.Errorf("One(error) returned row %v, want nil", got)
	}
	if err == nil || err.Error() != "things FindByID: boom" || !errors.Is(err, boom) {
		t.Errorf("One(error) = %v, want wrapped 'things FindByID: boom'", err)
	}

	hit, err := One(row{ID: "x"}, nil, "things FindByID")
	if err != nil || hit == nil || hit.ID != "x" {
		t.Errorf("One(hit) = (%v, %v), want (&{x}, nil)", hit, err)
	}
}

func TestFilter(t *testing.T) {
	t.Parallel()

	var f Filter
	if f.Where() != "" || len(f.Args()) != 0 {
		t.Errorf("zero Filter = (%q, %v), want empty", f.Where(), f.Args())
	}

	f.EqPtr("application", nil) // nil → no condition
	status := "CURRENT"
	f.EqPtr("status", &status)
	f.Eq("source", "SDK")
	f.Any("client_id", []string{"c1", "c2"})
	f.Any("subdomain", nil) // empty → no condition
	f.Clause("(client_id IS NULL OR client_id = ANY($%d))", []string{"c3"})
	if n := f.Arg(int64(20)); n != 5 {
		t.Errorf("Arg index = %d, want 5", n)
	}

	wantWhere := " WHERE status = $1 AND source = $2 AND client_id = ANY($3) AND (client_id IS NULL OR client_id = ANY($4))"
	if f.Where() != wantWhere {
		t.Errorf("Where() = %q, want %q", f.Where(), wantWhere)
	}
	wantArgs := []any{"CURRENT", "SDK", []string{"c1", "c2"}, []string{"c3"}, int64(20)}
	if !reflect.DeepEqual(f.Args(), wantArgs) {
		t.Errorf("Args() = %v, want %v", f.Args(), wantArgs)
	}
}
