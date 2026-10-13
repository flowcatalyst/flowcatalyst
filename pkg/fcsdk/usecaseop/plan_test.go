package usecaseop

import (
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/outbox"
)

type testEvent struct{}

func (testEvent) EventID() string             { return "0ABCDEFGHJKMN" }
func (testEvent) EventType() string           { return "shop:catalog:thing:saved" }
func (testEvent) SpecVersion() string         { return "1.0" }
func (testEvent) Source() string              { return "shop:catalog" }
func (testEvent) Subject() string             { return "catalog.thing.1" }
func (testEvent) Time() time.Time             { return time.Unix(0, 0) }
func (testEvent) PrincipalID() string         { return "" }
func (testEvent) CorrelationID() string       { return "" }
func (testEvent) CausationID() string         { return "" }
func (testEvent) ExecutionID() string         { return "" }
func (testEvent) MessageGroup() string        { return "" }
func (testEvent) ToDataJSON() ([]byte, error) { return []byte(`{}`), nil }

func jobIDs(p Plan[testEvent]) []string {
	var out []string
	for _, j := range p.(plan[testEvent]).jobs {
		out = append(out, j.ID)
	}
	return out
}

// Dispatch returns a new plan: the plan it is called on keeps its jobs, and
// two plans grown from one never share (or overwrite) each other's jobs.
func TestDispatchDoesNotChangeThePlanItIsCalledOn(t *testing.T) {
	job := func(id string) outbox.DispatchJob {
		return outbox.NewDispatchJob("shop:x", "https://a.example.com").WithID(id)
	}
	base := Emit(testEvent{})
	one := base.Dispatch(job("a"))
	left := one.Dispatch(job("b"))
	right := one.Dispatch(job("c"), job("d"))

	for name, c := range map[string]struct {
		got  []string
		want []string
	}{
		"base": {jobIDs(base), nil}, "one": {jobIDs(one), []string{"a"}},
		"left": {jobIDs(left), []string{"a", "b"}}, "right": {jobIDs(right), []string{"a", "c", "d"}},
	} {
		if len(c.got) != len(c.want) {
			t.Fatalf("%s: %v, want %v", name, c.got, c.want)
		}
		for i := range c.got {
			if c.got[i] != c.want[i] {
				t.Fatalf("%s: %v, want %v", name, c.got, c.want)
			}
		}
	}
}
