package dispatch_test

import (
	"strings"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatch"
)

const sqsURL = "https://sqs.eu-west-1.amazonaws.com/123456789012/FC-staging-dispatch.fifo"

func TestResolveSettings_Postgres(t *testing.T) {
	// Anything other than SQS — including unset — names Postgres-backed
	// queues, and needs no prefix.
	for _, queueType := range []string{"", "postgres", "POSTGRES"} {
		s, err := dispatch.ResolveSettings(queueType, "", "", "", "postgresql://user@localhost:5432/fc")
		if err != nil {
			t.Fatalf("ResolveSettings(%q) errored: %v", queueType, err)
		}
		if s.SQS {
			t.Errorf("ResolveSettings(%q).SQS = true, want false", queueType)
		}
		// The queue registry only ever registers the "postgres" scheme, so a
		// document naming postgresql:// queues would be refused outright.
		if s.DatabaseURL != "postgres://user@localhost:5432/fc" {
			t.Errorf("DatabaseURL = %q, want the postgres:// form", s.DatabaseURL)
		}
		if got := s.QueueURIFor("acme-DEFAULT"); got != s.DatabaseURL {
			t.Errorf("QueueURIFor = %q, want the shared database URL", got)
		}
	}
}

func TestResolveSettings_SQS(t *testing.T) {
	s, err := dispatch.ResolveSettings("sqs", sqsURL, "", "FC-staging", "postgresql://x@localhost/fc")
	if err != nil {
		t.Fatalf("ResolveSettings errored: %v", err)
	}
	if !s.SQS {
		t.Fatal("SQS = false, want true (matched ignoring case)")
	}
	if s.SQSAccountID != "123456789012" {
		t.Errorf("SQSAccountID = %q", s.SQSAccountID)
	}
	if s.SQSRegion != "eu-west-1" {
		t.Errorf("SQSRegion = %q", s.SQSRegion)
	}
	// The queue the URL names is itself unused: it supplies the account and
	// region this platform composes its OWN per-tenant queue URLs with.
	want := "https://sqs.eu-west-1.amazonaws.com/123456789012/FC-staging-acme-DEFAULT.fifo"
	if got := s.QueueURIFor("FC-staging-acme-DEFAULT.fifo"); got != want {
		t.Errorf("QueueURIFor = %q, want %q", got, want)
	}
	if s.DatabaseURL != "" {
		t.Errorf("DatabaseURL = %q, want empty for an SQS deployment", s.DatabaseURL)
	}
}

func TestResolveSettings_ExplicitRegionWins(t *testing.T) {
	s, err := dispatch.ResolveSettings("SQS", sqsURL, "us-east-2", "FC-prod", "")
	if err != nil {
		t.Fatalf("ResolveSettings errored: %v", err)
	}
	if s.SQSRegion != "us-east-2" {
		t.Errorf("SQSRegion = %q, want the explicitly configured region", s.SQSRegion)
	}
}

// A deployed environment that asks for SQS without the pieces needed to name
// and address its queues must fail at boot. The alternative is a queue named
// literally "FC-{env}-..." or an endpoint like
// "https://sqs..amazonaws.com//FC-staging-acme-DEFAULT.fifo", which the router
// still recognises as SQS and only fails on at poll time.
func TestResolveSettings_SQSRefusals(t *testing.T) {
	cases := []struct {
		name        string
		url         string
		region      string
		prefix      string
		wantMessage string
	}{
		{name: "no prefix", url: sqsURL, prefix: "", wantMessage: "FC_DISPATCH_QUEUE_PREFIX is required"},
		{name: "blank prefix", url: sqsURL, prefix: "   ", wantMessage: "FC_DISPATCH_QUEUE_PREFIX is required"},
		{name: "no url at all", url: "", prefix: "FC-staging", wantMessage: "account id and region"},
		{name: "url with no account segment", url: "https://sqs.eu-west-1.amazonaws.com/", prefix: "FC-staging", wantMessage: "account id and region"},
		{name: "non-sqs url and no region", url: "https://example.test/queue", prefix: "FC-staging", wantMessage: "account id and region"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := dispatch.ResolveSettings("SQS", c.url, c.region, c.prefix, "")
			if err == nil {
				t.Fatalf("ResolveSettings(%s) succeeded, want a startup refusal", c.name)
			}
			if !strings.Contains(err.Error(), c.wantMessage) {
				t.Errorf("error = %v, want it to mention %q", err, c.wantMessage)
			}
		})
	}
}
