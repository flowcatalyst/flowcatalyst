package dispatchqueue_test

import (
	"errors"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/dispatchqueue"
)

func TestParse(t *testing.T) {
	// The SPA sends "default" on every create, so case-insensitivity is what
	// keeps the existing UI working rather than 400ing it.
	cases := []struct {
		raw     string
		want    dispatchqueue.Priority
		wantErr bool
	}{
		{raw: "DEFAULT", want: dispatchqueue.PriorityDefault},
		{raw: "default", want: dispatchqueue.PriorityDefault},
		{raw: "Default", want: dispatchqueue.PriorityDefault},
		{raw: "  default  ", want: dispatchqueue.PriorityDefault},
		{raw: "HIGH_PRIORITY", want: dispatchqueue.PriorityHighPriority},
		{raw: "high_priority", want: dispatchqueue.PriorityHighPriority},
		{raw: "", want: ""},
		{raw: "   ", want: ""},
		{raw: "workers-high", wantErr: true},
		{raw: "HIGH PRIORITY", wantErr: true},
	}
	for _, c := range cases {
		got, err := dispatchqueue.Parse(c.raw)
		if c.wantErr {
			if err == nil {
				t.Errorf("Parse(%q) = %q, want an error", c.raw, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("Parse(%q) errored: %v", c.raw, err)
			continue
		}
		if got != c.want {
			t.Errorf("Parse(%q) = %q, want %q", c.raw, got, c.want)
		}
	}
}

func TestForPublishing(t *testing.T) {
	// Lenient by design: write validation cannot reach rows that already
	// exist, and a legacy value must route rather than fail.
	str := func(s string) *string { return &s }
	cases := []struct {
		name   string
		stored *string
		want   dispatchqueue.Priority
	}{
		{name: "nil (every pre-existing row)", stored: nil, want: dispatchqueue.PriorityDefault},
		{name: "blank", stored: str("  "), want: dispatchqueue.PriorityDefault},
		{name: "legacy text", stored: str("workers-high"), want: dispatchqueue.PriorityDefault},
		{name: "canonical default", stored: str("DEFAULT"), want: dispatchqueue.PriorityDefault},
		{name: "canonical high", stored: str("HIGH_PRIORITY"), want: dispatchqueue.PriorityHighPriority},
		{name: "lower-case high", stored: str("high_priority"), want: dispatchqueue.PriorityHighPriority},
	}
	for _, c := range cases {
		if got := dispatchqueue.ForPublishing(c.stored); got != c.want {
			t.Errorf("ForPublishing(%s) = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestComposeName(t *testing.T) {
	cases := []struct {
		name     string
		prefix   string
		tenant   string
		priority dispatchqueue.Priority
		sqs      bool
		want     string
	}{
		{
			name: "sqs default", prefix: "FC-staging", tenant: "acme",
			priority: dispatchqueue.PriorityDefault, sqs: true,
			want: "FC-staging-acme-DEFAULT.fifo",
		},
		{
			name: "sqs high priority keeps its underscore", prefix: "FC-staging", tenant: "acme",
			priority: dispatchqueue.PriorityHighPriority, sqs: true,
			want: "FC-staging-acme-HIGH_PRIORITY.fifo",
		},
		{
			name: "platform tenant", prefix: "FC-staging", tenant: dispatchqueue.TenantPlatform,
			priority: dispatchqueue.PriorityDefault, sqs: true,
			want: "FC-staging-platform-DEFAULT.fifo",
		},
		{
			name: "postgres takes no suffix", prefix: "", tenant: "acme",
			priority: dispatchqueue.PriorityDefault, sqs: false,
			want: "acme-DEFAULT",
		},
		{
			name: "tenant sanitisation", prefix: "FC-dev", tenant: "a_b.c d",
			priority: dispatchqueue.PriorityDefault, sqs: false,
			want: "FC-dev-a-b-c-d-DEFAULT",
		},
		{
			name: "unset priority composes as DEFAULT", prefix: "FC-dev", tenant: "acme",
			priority: "", sqs: false,
			want: "FC-dev-acme-DEFAULT",
		},
	}
	for _, c := range cases {
		got, err := dispatchqueue.ComposeName(c.prefix, c.tenant, c.priority, c.sqs)
		if err != nil {
			t.Errorf("%s: ComposeName errored: %v", c.name, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: ComposeName = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestComposeName_BlankTenantIsRefused(t *testing.T) {
	// A blank tenant would compose a lane every client shares, which is the
	// opposite of what the name exists to do.
	for _, tenant := range []string{"", "   "} {
		if _, err := dispatchqueue.ComposeName("FC-staging", tenant, dispatchqueue.PriorityDefault, true); !errors.Is(err, dispatchqueue.ErrTenantRequired) {
			t.Errorf("ComposeName(tenant=%q) error = %v, want ErrTenantRequired", tenant, err)
		}
	}
}

func TestComposeName_SQSLengthCap(t *testing.T) {
	// tnt_clients.identifier is varchar(100), so this is reachable, not
	// defensive: the name is refused so the document builder can omit that
	// one tenant instead of failing the whole document.
	long := ""
	for range 80 {
		long += "a"
	}
	_, err := dispatchqueue.ComposeName("FC-staging", long, dispatchqueue.PriorityHighPriority, true)
	var tooLong *dispatchqueue.NameTooLongError
	if !errors.As(err, &tooLong) {
		t.Fatalf("ComposeName(long tenant) error = %v, want NameTooLongError", err)
	}
	if tooLong.Tenant != long {
		t.Errorf("NameTooLongError.Tenant = %q, want the offending tenant", tooLong.Tenant)
	}

	// The same tenant is fine on the Postgres path, which has no such limit.
	if _, err := dispatchqueue.ComposeName("FC-staging", long, dispatchqueue.PriorityHighPriority, false); err != nil {
		t.Errorf("ComposeName(long tenant, postgres) errored: %v", err)
	}
}
