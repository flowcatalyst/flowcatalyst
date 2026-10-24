package runtimeapi

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
)

func testTarget() *jobTarget {
	conn := "con_0000000000001"
	return &jobTarget{
		ingestion: &ingestion.Ingestion{
			ID: "ing_000000000001", ClientID: ids.ClientID("clt_000000000001"),
			Definition: ingestion.Definition{Target: ingestion.Target{
				Code: "acme:orders:order:received", TargetURL: "https://hooks.example.com/orders", Pool: "orders", Mode: ingestion.ModeBlockOnError,
			}},
		},
		source: "ingestion:acme:orders", poolID: "dpl_000000000001", connectionID: &conn,
	}
}

// Everything but the record comes from the target, whatever the record holds.
func TestJobTarget_ItemTakesEverythingButTheRecordFromTheTarget(t *testing.T) {
	tg := testTarget()
	signer := "sac_000000000001"
	tg.serviceAccount = &signer
	group, ext := "g1", "e1"
	item, problem := tg.item(JobRecord{Payload: "p", PayloadContentType: "text/csv", MessageGroup: &group, ExternalID: &ext}, "rt-1")
	require.Empty(t, problem)

	assert.Equal(t, "acme:orders:order:received", item.Code)
	assert.Equal(t, "https://hooks.example.com/orders", item.TargetURL)
	assert.Equal(t, "BLOCK_ON_ERROR", item.Mode)
	assert.Equal(t, "clt_000000000001", *item.ClientID)
	assert.Nil(t, item.ClientCode)
	assert.Equal(t, "dpl_000000000001", *item.DispatchPoolID)
	assert.Equal(t, signer, *item.ServiceAccountID)
	assert.Equal(t, "ingestion:acme:orders", *item.Source)
	assert.Equal(t, "p", *item.Payload)
	assert.Equal(t, "text/csv", item.PayloadContentType)
	assert.Equal(t, "g1", *item.MessageGroup)
	assert.Equal(t, "e1", *item.ExternalID)
	assert.Nil(t, item.SubscriptionID)
	assert.Nil(t, item.ID, "the platform mints the job's id")
}

// A record that cannot be a job says why, field by field.
func TestJobTarget_ItemRefusesWhatDoesNotFit(t *testing.T) {
	tg := testTarget()
	long := func(n int) *string { s := strings.Repeat("é", n); return &s }
	blank := "  "
	cases := map[string]struct {
		rec  JobRecord
		want string
	}{
		"no payload":        {JobRecord{}, "payload is required"},
		"content type":      {JobRecord{Payload: "p", PayloadContentType: *long(101)}, "payloadContentType"},
		"message group":     {JobRecord{Payload: "p", MessageGroup: long(201)}, "messageGroup"},
		"blank external id": {JobRecord{Payload: "p", ExternalID: &blank}, "externalId may not be blank"},
		"external id":       {JobRecord{Payload: "p", ExternalID: long(101)}, "externalId is longer"},
		"correlation id":    {JobRecord{Payload: "p", CorrelationID: long(101)}, "correlationId"},
		"subject":           {JobRecord{Payload: "p", Subject: long(501)}, "subject"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, problem := tg.item(tc.rec, "rt-1")
			assert.Contains(t, problem, tc.want)
		})
	}
	_, problem := tg.item(JobRecord{Payload: "p", MessageGroup: long(200), ExternalID: long(100), CorrelationID: long(100), Subject: long(500)}, "rt-1")
	assert.Empty(t, problem, "the limits are in characters, and inclusive")
}
