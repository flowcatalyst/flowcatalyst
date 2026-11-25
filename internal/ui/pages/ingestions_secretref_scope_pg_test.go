//go:build integration

package pages

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	"github.com/flowcatalyst/flowcatalyst-go/internal/secretref"
	"github.com/flowcatalyst/flowcatalyst-go/internal/secretref/secretreftest"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// TA3 (ingestion secrets): the ingestion's own application and client own the
// secrets it writes. A reference under another application's path is refused
// at the write (SECRET_REFERENCE_NOT_ALLOWED, nothing stored), one already
// stored there does not resolve for the ingestion runtime's fetch.
func TestIngestionSecretReferenceIsScopedToItsApplication(t *testing.T) {
	// Not parallel: it swaps the process's resolver.
	p := newIngPG(t)
	ctx := context.Background()
	admin := ingAnchor(ingPermView, ingPermCreate, ingPermUpdate, ingPermDelete, ingPermPromote, ingPermSecrets, ingPermIdentity)
	configs := platformconfig.NewRepository(testpg.Pool(t))
	app := p.app.Code
	own, other := "aws-sm://prod/"+app+"/sftp", "aws-sm://prod/some-other-app/sftp"
	aws := map[string]string{"prod/" + app + "/sftp": "own-secret", "prod/some-other-app/sftp": "other-secret"}
	scoped := secretreftest.New(aws, "aws-sm://prod/{app}/")
	restore := secretref.SetDefault(scoped)
	defer restore()

	code := "vendor-" + ingUnique()
	f := p.form(code)
	for _, k := range []string{"http.auth", "http.token", "split.arrayPath"} {
		f.Del(k)
	}
	f.Set("kind", "sftp")
	f.Set("settings", "config")
	f.Set("configKey", "")
	f.Set("split.kind", "csv")
	f.Set("target.targetUrl", "")
	require.Equal(t, http.StatusOK, p.post(admin, "/ui/ingestions/new", f).code)
	i := p.find(code)
	secretName := code + "-credential"

	settings := func(credential string) url.Values {
		return url.Values{
			"sftp.host": {"sftp.vendor.example"}, "sftp.port": {"22"}, "sftp.directory": {"/out"}, "sftp.username": {"svc"},
			"sftp.credential": {credential}, "sftp.knownHosts": {ingCfgKnownHost}, "sftp.every": {"1m"},
			"config.targetUrl": {"https://app.example.com/hooks/vendor"}, "drawer": {i.ID},
		}
	}
	stored := func() *platformconfig.Config {
		c, err := configs.FindByCoordinate(ctx, app, ingestion.SecretsSection, secretName, platformconfig.ScopeClient, new(string(p.client.ID)))
		require.NoError(t, err)
		return c
	}

	refused := p.post(admin, "/ui/ingestions/"+i.ID+"/config", settings(other))
	assert.NotContains(t, refused.body, "Settings saved for this environment")
	assert.Contains(t, refused.body, `id="ings-config-error"`)
	assert.Contains(t, refused.body, "a reference must start with one of")
	assert.Nil(t, stored(), "a refused write stores nothing")

	saved := p.post(admin, "/ui/ingestions/"+i.ID+"/config", settings(own))
	require.Equal(t, http.StatusOK, saved.code, saved.body)
	assert.Contains(t, saved.body, "Settings saved for this environment")
	svc := &platformconfig.Service{Repo: configs, Refs: scoped}
	got, err := svc.FetchBoundSecretForIngestionRuntime(ctx, app, ingestion.SecretsSection, secretName, string(p.client.ID))
	require.NoError(t, err)
	assert.Equal(t, "own-secret", got.Value)

	// Another application's reference stored on this secret's row (written
	// while the allow-list was wide) is refused when the runtime fetches it.
	restoreWide := secretref.SetDefault(secretreftest.New(aws, "aws-sm://prod/"))
	require.Equal(t, http.StatusOK, p.post(admin, "/ui/ingestions/"+i.ID+"/config", settings(other)).code)
	restoreWide()
	require.Contains(t, stored().Value, "encrypted:")
	_, err = svc.FetchBoundSecretForIngestionRuntime(ctx, app, ingestion.SecretsSection, secretName, string(p.client.ID))
	require.Error(t, err)
	assert.Equal(t, secretref.CodeNotAllowed, usecase.AsError(err).Code)
}

// The settings form's "Stored as" choice: a secret entered as a reference to
// Parameter Store is stored as that reference, shown in the drawer with its
// store, offered back for editing, and a refused one is the field's error.
func TestIngestionSecretStoredAsReference(t *testing.T) {
	// Not parallel: it swaps the process's resolver.
	p := newIngPG(t)
	ctx := context.Background()
	admin := ingAnchor(ingPermView, ingPermCreate, ingPermUpdate, ingPermDelete, ingPermPromote, ingPermSecrets, ingPermIdentity)
	configs := platformconfig.NewRepository(testpg.Pool(t))
	app := p.app.Code
	path := "/shared/" + app + "/sftp-password"
	restore := secretref.SetDefault(secretreftest.New(map[string]string{path: "from-parameter-store"}, "aws-ps:///shared/"))
	defer restore()

	code := "vendor-" + ingUnique()
	f := p.form(code)
	for _, k := range []string{"http.auth", "http.token", "split.arrayPath"} {
		f.Del(k)
	}
	f.Set("kind", "sftp")
	f.Set("settings", "config")
	f.Set("configKey", "")
	f.Set("split.kind", "csv")
	f.Set("target.targetUrl", "")
	require.Equal(t, http.StatusOK, p.post(admin, "/ui/ingestions/new", f).code)
	i := p.find(code)
	secretName := code + "-credential"

	settings := func(source, ref string) url.Values {
		return url.Values{
			"sftp.host": {"sftp.vendor.example"}, "sftp.port": {"22"}, "sftp.directory": {"/out"}, "sftp.username": {"svc"},
			"sftp.credentialSource": {source}, "sftp.credential": {"typed-but-not-chosen"}, "sftp.credentialRefAwsPs": {ref},
			"sftp.knownHosts": {ingCfgKnownHost}, "sftp.every": {"1m"},
			"config.targetUrl": {"https://app.example.com/hooks/vendor"}, "drawer": {i.ID},
		}
	}

	// The form offers the choice, with the value first.
	form := p.do(admin, req{target: "/ui/ingestions/" + i.ID + "/config", datastar: true}).body
	assert.Contains(t, form, `name="sftp.credentialSource"`)
	assert.Contains(t, form, `<option value="value" selected`)

	// Outside the allow-list: refused on the field, the reference kept, nothing stored.
	refused := p.post(admin, "/ui/ingestions/"+i.ID+"/config", settings("aws-ps", "/elsewhere/x"))
	assert.NotContains(t, refused.body, "Settings saved for this environment")
	assert.Contains(t, refused.body, `id="ings-config-sftp-credential-err"`)
	assert.Contains(t, refused.body, `value="/elsewhere/x"`)
	assert.Contains(t, refused.body, "a reference must start with one of")
	assert.NotContains(t, refused.body, "typed-but-not-chosen")

	// Parameter Store chosen but left empty: the field says what to enter.
	empty := p.post(admin, "/ui/ingestions/"+i.ID+"/config", settings("aws-ps", ""))
	assert.Contains(t, empty.body, "Enter the parameter&#39;s name.")

	saved := p.post(admin, "/ui/ingestions/"+i.ID+"/config", settings("aws-ps", path))
	require.Equal(t, http.StatusOK, saved.code, saved.body)
	assert.Contains(t, saved.body, "Settings saved for this environment")
	got, err := (&platformconfig.Service{Repo: configs, Refs: secretref.Default()}).FetchBoundSecretForIngestionRuntime(ctx, app, ingestion.SecretsSection, secretName, string(p.client.ID))
	require.NoError(t, err)
	assert.Equal(t, "from-parameter-store", got.Value, "the typed value was not the one chosen")

	// The drawer shows the reference with its store; the edit form offers it back.
	drawer := p.do(admin, req{target: "/ui/ingestions/" + i.ID, datastar: true}).body
	assert.Contains(t, drawer, `data-testid="ings-config-reference"`)
	assert.Contains(t, drawer, "AWS Parameter Store")
	assert.Contains(t, drawer, "aws-ps://"+path)
	form = p.do(admin, req{target: "/ui/ingestions/" + i.ID + "/config", datastar: true}).body
	assert.Contains(t, form, `<option value="aws-ps" selected`)
	assert.Contains(t, form, `value="`+path+`"`)
	assert.NotContains(t, form, "from-parameter-store")
}
