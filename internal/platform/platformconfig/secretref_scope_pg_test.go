//go:build integration

package platformconfig_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/secretref"
	"github.com/flowcatalyst/flowcatalyst-go/internal/secretref/secretreftest"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

func setSecretAs(f *confinementFixture, ac *auth.AuthContext, app, prop, value string, clientID *string) error {
	secret := "SECRET"
	_, err := usecaseop.Run(testpg.WithAuth(context.Background(), ac), f.svc.UoW, operations.SetProperty(f.svc.Repo), operations.SetPropertyCommand{
		ApplicationCode: app, Section: "sftp", Property: prop, Value: value, ValueType: &secret, ClientID: clientID,
	}, testpg.TestEC())
	return err
}

func requireNotAllowed(t *testing.T, err error, msg string) {
	t.Helper()
	require.Error(t, err, msg)
	assert.Equal(t, secretref.CodeNotAllowed, usecase.AsError(err).Code, msg)
	assert.Equal(t, usecase.KindValidation, usecase.AsError(err).Kind, msg)
}

// TA2: with a scoped allow-list, an application's service account may set and
// reveal a reference under its own application's path, may not set one under
// another application's, and a reference to another application's secret that
// is already stored on its row does not resolve.
func TestSecretReference_ScopedToTheOwningApplication(t *testing.T) {
	// Not parallel: it swaps the process's resolver (the write check).
	const a, b = "tsa2-app-a", "tsa2-app-b"
	fa := newConfinementFixture(t, a)
	_ = newConfinementFixture(t, b)
	aws := map[string]string{
		"prod/" + a + "/db": `{"password":"a-secret"}`,
		"prod/" + b + "/db": `{"password":"b-secret"}`,
	}
	scoped := secretreftest.New(aws, "aws-sm://prod/{app}/")
	restore := secretref.SetDefault(scoped)
	defer restore()
	fa.svc.Refs = scoped
	ctx := context.Background()

	// Own application: set, and revealed resolved.
	own := "aws-sm://prod/" + a + "/db#password"
	require.NoError(t, setSecretAs(fa, fa.saApp, a, "db", own, nil))
	r, err := fa.svc.RevealFor(ctx, fa.saApp, a, "sftp", "db", "", "")
	require.NoError(t, err)
	assert.Equal(t, "a-secret", r.Value)

	// Another application's path: refused at the write, nothing stored.
	other := "aws-sm://prod/" + b + "/db#password"
	requireNotAllowed(t, setSecretAs(fa, fa.saApp, a, "stolen", other, nil), "another application's reference")
	requireNotAllowed(t, setSecretAs(fa, fa.saApp, a, "stolen", "aws-sm://prod/"+a+"-admin/db", nil), "a sibling name")
	got, err := fa.svc.Repo.FindByCoordinate(ctx, a, "sftp", "stolen", "GLOBAL", nil)
	require.NoError(t, err)
	assert.Nil(t, got, "a refused write stores nothing")

	// A reference to another application's secret stored on A's row (written
	// while the allow-list was wide) does not resolve for A, at reveal or at the
	// ingestion runtime's fetch, however it got there.
	wide := secretreftest.New(aws, "aws-sm://prod/")
	restoreWide := secretref.SetDefault(wide)
	require.NoError(t, setSecretAs(fa, fa.saApp, a, "stored-other", other, nil))
	restoreWide()
	_, err = fa.svc.RevealFor(ctx, fa.saApp, a, "sftp", "stored-other", "", "")
	require.Error(t, err)
	assert.Equal(t, secretref.CodeNotAllowed, usecase.AsError(err).Code)
	_, err = fa.svc.FetchBoundSecretForIngestionRuntime(ctx, a, "sftp", "stored-other", "")
	require.Error(t, err)
	assert.Equal(t, secretref.CodeNotAllowed, usecase.AsError(err).Code)
	for _, v := range aws {
		assert.NotContains(t, err.Error(), v, "a refusal carries no value")
	}
}

// With {client} too: a client-scoped value resolves for its own client id only,
// and the application-wide value (no client) never matches such an entry.
func TestSecretReference_ScopedToTheOwningClient(t *testing.T) {
	const a = "tsa2-app-c"
	fa := newConfinementFixture(t, a)
	aws := map[string]string{
		"prod/" + a + "/" + fa.a + "/db": "client-a-secret",
		"prod/" + a + "/" + fa.b + "/db": "client-b-secret",
	}
	s := secretreftest.New(aws, "aws-sm://prod/{app}/{client}/")
	restore := secretref.SetDefault(s)
	defer restore()
	fa.svc.Refs = s
	ctx := context.Background()

	require.NoError(t, setSecretAs(fa, fa.saApp, a, "db", "aws-sm://prod/"+a+"/"+fa.a+"/db", &fa.a))
	r, err := fa.svc.RevealFor(ctx, fa.saApp, a, "sftp", "db", fa.a, "")
	require.NoError(t, err)
	assert.Equal(t, "client-a-secret", r.Value)

	requireNotAllowed(t, setSecretAs(fa, fa.saApp, a, "db2", "aws-sm://prod/"+a+"/"+fa.b+"/db", &fa.a), "another client's path on this client's row")
	requireNotAllowed(t, setSecretAs(fa, fa.saApp, a, "db3", "aws-sm://prod/"+a+"/"+fa.a+"/db", nil), "the application-wide value has no client")
}
