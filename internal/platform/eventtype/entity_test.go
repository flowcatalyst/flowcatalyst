package eventtype_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
)

// Entity-level tests for event-type code validation and schema versioning.

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

// TestStatusRoundTrip pins the X-06 (T, bool) idiom: known values parse
// with ok=true, and an unrecognised value is rejected (ok=false) rather
// than silently coerced to CURRENT.
func TestStatusRoundTrip(t *testing.T) {
	got, ok := eventtype.ParseStatus("CURRENT")
	assert.True(t, ok)
	assert.Equal(t, eventtype.StatusCurrent, got)

	got, ok = eventtype.ParseStatus("ARCHIVED")
	assert.True(t, ok)
	assert.Equal(t, eventtype.StatusArchived, got)

	_, ok = eventtype.ParseStatus("UNKNOWN")
	assert.False(t, ok, "an unrecognised status must not silently coerce to CURRENT")
}

// TestSourceRoundTrip mirrors TestStatusRoundTrip for Source.
func TestSourceRoundTrip(t *testing.T) {
	got, ok := eventtype.ParseSource("CODE")
	assert.True(t, ok)
	assert.Equal(t, eventtype.SourceCode, got)

	got, ok = eventtype.ParseSource("API")
	assert.True(t, ok)
	assert.Equal(t, eventtype.SourceAPI, got)

	got, ok = eventtype.ParseSource("UI")
	assert.True(t, ok)
	assert.Equal(t, eventtype.SourceUI, got)

	_, ok = eventtype.ParseSource("UNKNOWN")
	assert.False(t, ok, "an unrecognised source must not silently coerce to UI")
}

// TestSchemaTypeAcceptsAliases pins the legacy-alias behaviour (still
// accepted) alongside the X-06 rejection of anything else.
func TestSchemaTypeAcceptsAliases(t *testing.T) {
	got, ok := eventtype.ParseSchemaType("JSON_SCHEMA")
	assert.True(t, ok)
	assert.Equal(t, eventtype.SchemaJSON, got)

	got, ok = eventtype.ParseSchemaType("XSD")
	assert.True(t, ok)
	assert.Equal(t, eventtype.SchemaXSD, got)

	got, ok = eventtype.ParseSchemaType("XML_SCHEMA")
	assert.True(t, ok)
	assert.Equal(t, eventtype.SchemaXSD, got)

	got, ok = eventtype.ParseSchemaType("PROTO")
	assert.True(t, ok)
	assert.Equal(t, eventtype.SchemaProto, got)

	got, ok = eventtype.ParseSchemaType("PROTOBUF")
	assert.True(t, ok)
	assert.Equal(t, eventtype.SchemaProto, got)

	_, ok = eventtype.ParseSchemaType("UNKNOWN")
	assert.False(t, ok, "an unrecognised schema type must not silently coerce to JSON_SCHEMA")
}
