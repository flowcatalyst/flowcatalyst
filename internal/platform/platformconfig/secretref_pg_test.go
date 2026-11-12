//go:build integration

package platformconfig_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/secretref"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// refsByMap resolves from a map; a reference mapped to "ERR:<code>" fails with
// that code, an absent one is NOT_FOUND.
type refsByMap map[string]string

func (m refsByMap) Resolve(_ context.Context, ref string, _ secretref.Owner) (string, error) {
	v, ok := m[ref]
	switch {
	case !ok:
		return "", &secretref.Error{Code: secretref.CodeNotFound, Ref: ref, Msg: "no such secret"}
	case len(v) > 4 && v[:4] == "ERR:":
		return "", &secretref.Error{Code: v[4:], Ref: ref, Msg: "fake failure"}
	}
	return v, nil
}

func setSecret(t *testing.T, svc *platformconfig.Service, app, prop, value string) error {
	t.Helper()
	secret := "SECRET"
	_, err := usecaseop.Run(adminCtx(), svc.UoW, operations.SetProperty(svc.Repo), operations.SetPropertyCommand{
		ApplicationCode: app, Section: "sftp", Property: prop, Value: value, ValueType: &secret,
	}, testpg.TestEC())
	return err
}

// A SECRET that holds a reference to a scheme nothing resolves, or one the
// allow-list does not permit, is refused at the write (400); an allowed one is
// stored encrypted like any secret, shown as the reference where a secret is
// masked, revealed to an administrator as the reference, and resolved for the
// consumers (the SDK reveal, the ingestion runtime).
func TestSecretReference_ConfigSecret(t *testing.T) {
	// Not parallel: it swaps the process's resolver (the write check).
	restore := secretref.SetDefault(secretref.New(secretref.Config{Allow: []string{"aws-sm://prod/"}}, nil))
	defer restore()
	ctx := context.Background()
	svc := newServiceFixture(t, "svcsecretref")
	svc.Refs = refsByMap{
		"aws-sm://prod/sftp#password": "pw-from-aws",
		"aws-sm://prod/gone":          "ERR:" + secretref.CodeNotFound,
		"aws-sm://prod/denied":        "ERR:" + secretref.CodeDenied,
		"aws-sm://prod/down":          "ERR:" + secretref.CodeUnavailable,
		"aws-sm://prod/nokey#x":       "ERR:" + secretref.CodeKeyMissing,
	}

	// Write checks.
	err := setSecret(t, svc, "svcsecretref", "elsewhere", "aws-sm://other/x")
	require.Error(t, err)
	assert.Equal(t, secretref.CodeNotAllowed, usecase.AsError(err).Code)
	assert.Equal(t, usecase.KindValidation, usecase.AsError(err).Kind)
	err = setSecret(t, svc, "svcsecretref", "elsewhere", "vault://kv/x")
	require.Error(t, err)
	assert.Equal(t, secretref.CodeUnsupported, usecase.AsError(err).Code)
	for prop, ref := range map[string]string{
		"ref": "aws-sm://prod/sftp#password", "gone": "aws-sm://prod/gone", "denied": "aws-sm://prod/denied",
		"down": "aws-sm://prod/down", "nokey": "aws-sm://prod/nokey#x",
	} {
		require.NoError(t, setSecret(t, svc, "svcsecretref", prop, ref), prop)
	}

	// Stored encrypted, as every config secret is.
	var stored string
	require.NoError(t, testpg.Pool(t).QueryRow(ctx,
		`SELECT value FROM app_platform_configs WHERE application_code = 'svcsecretref' AND property = 'ref'`).Scan(&stored))
	assert.Contains(t, stored, "encrypted:")
	assert.NotContains(t, stored, "aws-sm")

	// Display: a reference is shown, a real secret stays masked.
	rows, err := svc.List(ctx, revealer(), "svcsecretref")
	require.NoError(t, err)
	shown := map[string]string{}
	for _, c := range rows {
		shown[c.Property] = c.MaskedValue()
	}
	assert.Equal(t, "aws-sm://prod/sftp#password", shown["ref"])
	assert.Equal(t, "***", shown["password"])

	// The administrator's reveal shows what is stored; the SDK's resolves.
	r, err := svc.Reveal(ctx, revealer(), "svcsecretref", "sftp", "ref", "")
	require.NoError(t, err)
	assert.Equal(t, "aws-sm://prod/sftp#password", r.Value)
	r, err = svc.RevealFor(ctx, revealer(), "svcsecretref", "sftp", "ref", "", "")
	require.NoError(t, err)
	assert.Equal(t, "pw-from-aws", r.Value)
	assert.Equal(t, "s3cr3t-pw", mustReveal(t, svc, "password"), "a plain secret is unchanged")

	// The ingestion runtime's fetch resolves.
	f, err := svc.FetchBoundSecretForIngestionRuntime(ctx, "svcsecretref", "sftp", "ref", "")
	require.NoError(t, err)
	assert.Equal(t, "pw-from-aws", f.Value)

	// Failures: 409 with the reference code for not found / denied / key
	// missing; unavailable is the retryable one the endpoints answer 503.
	for prop, code := range map[string]string{
		"gone": secretref.CodeNotFound, "denied": secretref.CodeDenied, "nokey": secretref.CodeKeyMissing,
	} {
		_, err = svc.RevealFor(ctx, revealer(), "svcsecretref", "sftp", prop, "", "")
		require.Error(t, err, prop)
		assert.Equal(t, code, usecase.AsError(err).Code, prop)
		assert.Equal(t, 409, usecase.AsError(err).HTTPStatus(), prop)
		_, err = svc.FetchBoundSecretForIngestionRuntime(ctx, "svcsecretref", "sftp", prop, "")
		assert.Equal(t, code, usecase.AsError(err).Code, prop)
	}
	_, err = svc.RevealFor(ctx, revealer(), "svcsecretref", "sftp", "down", "", "")
	require.Error(t, err)
	assert.True(t, secretref.IsUnavailable(err))
	_, err = svc.FetchBoundSecretForIngestionRuntime(ctx, "svcsecretref", "sftp", "down", "")
	assert.True(t, secretref.IsUnavailable(err))

	// A reveal that did not resolve revealed nothing, so it wrote no audit row.
	var n int
	require.NoError(t, testpg.Pool(t).QueryRow(ctx, `SELECT count(*) FROM aud_logs a JOIN app_platform_configs c ON c.id = a.entity_id
		WHERE c.application_code = 'svcsecretref' AND c.property IN ('gone','denied','down','nokey') AND a.operation = 'SECRET_REVEALED'`).Scan(&n))
	assert.Zero(t, n)
}

func mustReveal(t *testing.T, svc *platformconfig.Service, prop string) string {
	t.Helper()
	r, err := svc.RevealFor(context.Background(), revealer(), "svcsecretref", "sftp", prop, "", "")
	require.NoError(t, err)
	return r.Value
}
