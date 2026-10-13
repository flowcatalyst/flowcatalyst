//go:build integration

package operations_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// AddSchema stores what the user wrote (bug 3 in docs/ui/README.md;
// docs/platform-bug-fixes.md): a JSON Schema typed into a text field and sent
// as a JSON string is stored as the document, not double-encoded; the MIME
// type and schema type are honoured; XSD/PROTO text is stored as text.
func TestAddSchema_StoresTheDocumentWithMimeAndSchemaType(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := eventtype.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	seeded := mustCreate(t, repo, uow, "etstore:orders:order:created", "Storage")

	text, err := json.Marshal(`{"type":"object","title":"typed"}`)
	require.NoError(t, err)
	_, err = runAuthorized(uow, operations.AddSchema(repo), operations.AddSchemaCommand{
		EventTypeID: seeded.EventTypeID, Version: "1.0", Schema: text,
		MimeType: "application/json", SchemaType: "JSON_SCHEMA",
	})
	require.NoError(t, err)

	xsd := `<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"/>`
	xsdRaw, err := json.Marshal(xsd)
	require.NoError(t, err)
	_, err = runAuthorized(uow, operations.AddSchema(repo), operations.AddSchemaCommand{
		EventTypeID: seeded.EventTypeID, Version: "1.1", Schema: xsdRaw, SchemaType: "XSD",
	})
	require.NoError(t, err)

	_, err = runAuthorized(uow, operations.AddSchema(repo), operations.AddSchemaCommand{
		EventTypeID: seeded.EventTypeID, Version: "1.2", Schema: json.RawMessage(`{"type":"string"}`),
	})
	require.NoError(t, err)

	got, err := repo.FindByID(ctx, seeded.EventTypeID)
	require.NoError(t, err)
	v10 := specByVersion(t, got, "1.0")
	assert.JSONEq(t, `{"type":"object","title":"typed"}`, string(v10.SchemaContent), "the document, not a JSON string")
	assert.Equal(t, "application/json", v10.MimeType)
	assert.Equal(t, eventtype.SchemaJSON, v10.SchemaType)

	v11 := specByVersion(t, got, "1.1")
	var stored string
	require.NoError(t, json.Unmarshal(v11.SchemaContent, &stored), "XSD is stored as text")
	assert.Equal(t, xsd, stored)
	assert.Equal(t, eventtype.SchemaXSD, v11.SchemaType)
	assert.Equal(t, "application/xml", v11.MimeType, "the schema type's default MIME type")

	v12 := specByVersion(t, got, "1.2")
	assert.Equal(t, "application/schema+json", v12.MimeType, "defaults unchanged for a plain document")
	assert.Equal(t, eventtype.SchemaJSON, v12.SchemaType)
}

func TestAddSchema_RefusesSchemasThatDoNotMatchTheirType(t *testing.T) {
	t.Parallel()
	repo := eventtype.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	seeded := mustCreate(t, repo, uow, "etstorebad:orders:order:created", "Storage bad")
	notJSON, _ := json.Marshal("{not json")
	cases := []struct {
		name string
		cmd  operations.AddSchemaCommand
		code string
	}{
		{"JSON Schema text that is not JSON", operations.AddSchemaCommand{Schema: notJSON}, "INVALID_SCHEMA"},
		{"XSD that is not text", operations.AddSchemaCommand{Schema: json.RawMessage(`{"a":1}`), SchemaType: "XSD"}, "INVALID_SCHEMA"},
		{"unknown schema type", operations.AddSchemaCommand{Schema: json.RawMessage(`{}`), SchemaType: "AVRO"}, "INVALID_SCHEMA_TYPE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.cmd.EventTypeID, tc.cmd.Version = seeded.EventTypeID, "9.0"
			_, err := runAuthorized(uow, operations.AddSchema(repo), tc.cmd)
			testpg.RequireUsecaseError(t, err, usecase.KindValidation, tc.code)
		})
	}
}
