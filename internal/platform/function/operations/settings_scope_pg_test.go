//go:build integration

package operations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/secretref"
	"github.com/flowcatalyst/flowcatalyst-go/internal/secretref/secretreftest"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// TA3 (function settings): the function's own application is the owner of its
// SECRET/DB references, at the write and when the desired document resolves
// them.
func TestPutSetting_SecretReferenceIsScopedToTheFunctionsApplication(t *testing.T) {
	// Not parallel: it swaps the process's resolver.
	f := newTestFunctionFixture(t, "app_tsa3fn0000001", "tsa3fnapp", "scope-fn")
	aws := map[string]string{
		"prod/tsa3fnapp/stripe":  "own-secret",
		"prod/othertsa3app/db":   "other-secret",
		"prod/tsa3fnapp-admin/x": "sibling-secret",
	}
	scoped := secretreftest.New(aws, "aws-sm://prod/{app}/")
	restore := secretref.SetDefault(scoped)
	defer restore()
	f.repo.WithSecretResolver(scoped)
	ctx := context.Background()

	put := func(kind function.SettingKind, key, value string) error {
		_, err := usecaseop.RunTx(operatorCtx(), f.uow, operations.PutSetting(f.repo),
			operations.PutSettingCommand{FunctionID: f.fn.ID, Kind: kind, Key: key, Value: value}, testpg.TestEC())
		return err
	}

	require.NoError(t, put(function.SettingSecret, "STRIPE", "aws-sm://prod/tsa3fnapp/stripe"))
	for _, ref := range []string{"aws-sm://prod/othertsa3app/db", "aws-sm://prod/tsa3fnapp-admin/x"} {
		err := put(function.SettingSecret, "STOLEN", ref)
		require.Error(t, err, ref)
		assert.Equal(t, secretref.CodeNotAllowed, usecase.AsError(err).Code, ref)
		assert.Equal(t, usecase.KindValidation, usecase.AsError(err).Kind, ref)
	}
	err := put(function.SettingDB, "maindb", "aws-sm://prod/othertsa3app/db")
	require.Error(t, err)
	assert.Equal(t, secretref.CodeNotAllowed, usecase.AsError(err).Code)
	var n int
	require.NoError(t, f.pool.QueryRow(ctx, `SELECT count(*) FROM fng_settings WHERE function_id = $1 AND key IN ('STOLEN','maindb')`, f.fn.ID).Scan(&n))
	assert.Zero(t, n, "a refused write stores nothing")

	// Another application's reference already stored on this function's row
	// (inserted directly) is left out of the desired document's settings and
	// reported, never resolved.
	require.NoError(t, f.repo.UpsertSetting(ctx, &function.Setting{FunctionID: f.fn.ID, Kind: function.SettingSecret, Key: "STORED_OTHER", Value: "aws-sm://prod/othertsa3app/db"}))
	got, err := f.repo.ResolveSettingsForControl(ctx, f.fn.ID, f.fn.SecretOwner())
	require.NoError(t, err)
	assert.Equal(t, "own-secret", got.Secrets["STRIPE"])
	assert.NotContains(t, got.Secrets, "STORED_OTHER")
	require.Len(t, got.Unresolved, 1)
	assert.Equal(t, "STORED_OTHER", got.Unresolved[0].Key)
	assert.Equal(t, secretref.CodeNotAllowed, got.Unresolved[0].Code)
}

// TB3 (function settings): a stored ciphertext is not a value a caller may
// supply; the repository would keep it and the desired document decrypt it.
func TestPutSetting_RefusesAStoredCiphertext(t *testing.T) {
	f := newTestFunctionFixture(t, "app_tsb3fn0000001", "tsb3fnapp", "cipher-fn")
	for kind, key := range map[function.SettingKind]string{function.SettingSecret: "K", function.SettingDB: "maindb"} {
		for _, v := range []string{"encrypted:AAAAAAAAAAAAAAAAAAAAAAAA", "  Encrypted:AAAAAAAAAAAAAAAAAAAA", "ENCRYPTED:x"} {
			_, err := usecaseop.RunTx(operatorCtx(), f.uow, operations.PutSetting(f.repo),
				operations.PutSettingCommand{FunctionID: f.fn.ID, Kind: kind, Key: key, Value: v}, testpg.TestEC())
			testpg.RequireUsecaseError(t, err, usecase.KindValidation, "SECRET_VALUE_INVALID")
		}
	}
	var n int
	require.NoError(t, f.pool.QueryRow(context.Background(), `SELECT count(*) FROM fng_settings WHERE function_id = $1`, f.fn.ID).Scan(&n))
	assert.Zero(t, n, "nothing stored")

	// A CONFIG value is plain text, not a secret: unchanged.
	_, err := usecaseop.RunTx(operatorCtx(), f.uow, operations.PutSetting(f.repo),
		operations.PutSettingCommand{FunctionID: f.fn.ID, Kind: function.SettingConfig, Key: "NOTE", Value: "encrypted: is just text here"}, testpg.TestEC())
	require.NoError(t, err)
}
