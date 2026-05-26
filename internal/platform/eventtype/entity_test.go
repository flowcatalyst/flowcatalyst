package eventtype_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
)

// Parity tests against the Rust entity tests in event_type/entity.rs.

func TestNewAcceptsValidFourPartCode(t *testing.T) {
	et, err := eventtype.New("orders:fulfillment:shipment:shipped", "Shipment Shipped")
	require.NoError(t, err)
	assert.Equal(t, "orders:fulfillment:shipment:shipped", et.Code)
	assert.Equal(t, "orders", et.Application)
	assert.Equal(t, "fulfillment", et.Subdomain)
	assert.Equal(t, "shipment", et.Aggregate)
	assert.Equal(t, "shipped", et.EventName)
	assert.Equal(t, eventtype.StatusCurrent, et.Status)
	assert.False(t, et.ClientScoped)
	assert.Empty(t, et.SpecVersions)
}

func TestNewRejectsTooFewSegments(t *testing.T) {
	for _, code := range []string{
		"orders:fulfillment:shipment",
		"orders:fulfillment",
		"orders",
		"",
	} {
		_, err := eventtype.New(code, "x")
		require.Error(t, err, "code %q", code)
	}
}

func TestNewRejectsTooManySegments(t *testing.T) {
	_, err := eventtype.New("orders:fulfillment:shipment:shipped:extra", "x")
	require.Error(t, err)
}

func TestNewRejectsEmptySegment(t *testing.T) {
	for _, code := range []string{
		"orders::shipment:shipped",
		":fulfillment:shipment:shipped",
		"orders:fulfillment:shipment:",
	} {
		_, err := eventtype.New(code, "x")
		require.Error(t, err, "code %q", code)
	}
}

func TestNewRejectsWhitespaceOnlySegment(t *testing.T) {
	_, err := eventtype.New("orders: :shipment:shipped", "x")
	require.Error(t, err)
}

func TestArchiveFlipsStatus(t *testing.T) {
	et, _ := eventtype.New("a:b:c:d", "Name")
	before := et.UpdatedAt
	time.Sleep(2 * time.Millisecond)
	et.Archive()
	assert.Equal(t, eventtype.StatusArchived, et.Status)
	assert.True(t, et.UpdatedAt.After(before))
}

func TestStatusRoundTripWithFallback(t *testing.T) {
	assert.Equal(t, eventtype.StatusCurrent, eventtype.ParseStatus("CURRENT"))
	assert.Equal(t, eventtype.StatusArchived, eventtype.ParseStatus("ARCHIVED"))
	assert.Equal(t, eventtype.StatusCurrent, eventtype.ParseStatus("UNKNOWN"))
}

func TestSourceRoundTripWithFallback(t *testing.T) {
	assert.Equal(t, eventtype.SourceCode, eventtype.ParseSource("CODE"))
	assert.Equal(t, eventtype.SourceAPI, eventtype.ParseSource("API"))
	assert.Equal(t, eventtype.SourceUI, eventtype.ParseSource("UI"))
	assert.Equal(t, eventtype.SourceUI, eventtype.ParseSource("UNKNOWN"))
}

func TestSchemaTypeAcceptsAliases(t *testing.T) {
	assert.Equal(t, eventtype.SchemaJSON, eventtype.ParseSchemaType("JSON_SCHEMA"))
	assert.Equal(t, eventtype.SchemaXSD, eventtype.ParseSchemaType("XSD"))
	assert.Equal(t, eventtype.SchemaXSD, eventtype.ParseSchemaType("XML_SCHEMA"))
	assert.Equal(t, eventtype.SchemaProto, eventtype.ParseSchemaType("PROTO"))
	assert.Equal(t, eventtype.SchemaProto, eventtype.ParseSchemaType("PROTOBUF"))
	assert.Equal(t, eventtype.SchemaJSON, eventtype.ParseSchemaType("UNKNOWN"))
}
