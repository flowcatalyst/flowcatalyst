package outboxpgx

import (
	"strings"
	"testing"
	"time"
)

type webhookCmd struct {
	Code               string            `json:"code"`
	WebhookCredentials map[string]string `json:"webhookCredentials"`
}

// The outbox audit payload never carries a secret: the command is redacted
// before it is serialised into the row an app's outbox relays to the platform.
func TestAuditPayloadIsRedactedBeforeItReachesTheOutbox(t *testing.T) {
	payload, err := buildAuditPayload(testEvent{}, webhookCmd{Code: "sa-1",
		WebhookCredentials: map[string]string{"token": "leaked-token", "signingSecret": "leaked-secret", "authType": "HMAC_SIGNATURE"}})
	if err != nil {
		t.Fatal(err)
	}
	s := string(payload)
	for _, leak := range []string{"leaked-token", "leaked-secret"} {
		if strings.Contains(s, leak) {
			t.Fatalf("%s reached the outbox payload: %s", leak, s)
		}
	}
	if !strings.Contains(s, "HMAC_SIGNATURE") {
		t.Fatalf("non-secret field lost: %s", s)
	}
}

// testEvent is the smallest DomainEvent the audit payload builder reads.
type testEvent struct{}

func (testEvent) EventID() string             { return "evt_test" }
func (testEvent) EventType() string           { return "app:test:thing:created" }
func (testEvent) SpecVersion() string         { return "1.0" }
func (testEvent) Source() string              { return "app:test" }
func (testEvent) Subject() string             { return "app.thing.thg_1" }
func (testEvent) Time() time.Time             { return time.Unix(0, 0) }
func (testEvent) PrincipalID() string         { return "prn_1" }
func (testEvent) CorrelationID() string       { return "" }
func (testEvent) CausationID() string         { return "" }
func (testEvent) ExecutionID() string         { return "" }
func (testEvent) MessageGroup() string        { return "" }
func (testEvent) ToDataJSON() ([]byte, error) { return []byte("{}"), nil }
