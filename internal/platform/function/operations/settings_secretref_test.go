package operations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/secretref"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// A SECRET/DB write that is a secret reference is checked against the
// allow-list and the resolvable schemes at the write (Validate runs before
// anything touches the database); CONFIG values are never checked.
func TestPutSetting_ValidatesSecretReferences(t *testing.T) {
	// Not parallel: it swaps the process's resolver.
	restore := secretref.SetDefault(secretref.New(secretref.Config{Allow: []string{"aws-sm://prod/"}}, nil))
	defer restore()
	validate := operations.PutSetting(nil).Validate

	put := func(kind function.SettingKind, value string) error {
		return validate(context.Background(), operations.PutSettingCommand{FunctionID: "fn_1", Kind: kind, Key: "main", Value: value})
	}
	codeOf := func(err error) string {
		require.Error(t, err)
		ue := usecase.AsError(err)
		require.NotNil(t, ue)
		assert.Equal(t, usecase.KindValidation, ue.Kind, "a refusal is a 400")
		return ue.Code
	}

	assert.NoError(t, put(function.SettingSecret, "aws-sm://prod/stripe#key"))
	assert.NoError(t, put(function.SettingSecret, "an ordinary secret"))
	assert.NoError(t, put(function.SettingDB, "postgres://u:p@h/db"), "a DSN is not a reference")

	assert.Equal(t, secretref.CodeNotAllowed, codeOf(put(function.SettingSecret, "aws-sm://other/db")))
	assert.Equal(t, secretref.CodeNotAllowed, codeOf(put(function.SettingDB, "aws-ps:///anything")))
	for _, v := range []string{"gcp-sm://p/s", "vault://x#y", "env://HOME"} {
		assert.Equal(t, secretref.CodeUnsupported, codeOf(put(function.SettingSecret, v)), v)
	}
	assert.NoError(t, put(function.SettingConfig, "aws-sm://other/db"), "CONFIG is plain text, not a reference")
}
