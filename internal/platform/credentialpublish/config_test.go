package credentialpublish

import (
	"maps"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/go-sdk/usecase"
)

func getenvOf(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestConfigFromEnv_unsetIsOff(t *testing.T) {
	c, err := configFromGetenv(getenvOf(nil))
	require.NoError(t, err)
	assert.False(t, c.Enabled())
	// Other settings do not matter while the store is unset.
	c, err = configFromGetenv(getenvOf(map[string]string{"FC_CREDENTIAL_PUBLISH_PREFIX": "bad"}))
	require.NoError(t, err)
	assert.False(t, c.Enabled())
}

func TestConfigFromEnv_defaults(t *testing.T) {
	c, err := configFromGetenv(getenvOf(map[string]string{
		"FC_CREDENTIAL_PUBLISH": "aws-ps", "FC_CREDENTIAL_PUBLISH_PREFIX": "/flowcatalyst/np/",
	}))
	require.NoError(t, err)
	assert.True(t, c.Enabled())
	assert.Equal(t, DefaultServiceAccountPath, c.ServiceAccountPath)
	assert.Equal(t, DefaultOAuthClientPath, c.OAuthClientPath)
	assert.False(t, c.DefaultOn)
	assert.Empty(t, c.KMSKey)
}

func TestConfigFromEnv_valid(t *testing.T) {
	c, err := configFromGetenv(getenvOf(map[string]string{
		"FC_CREDENTIAL_PUBLISH": "aws-ps", "FC_CREDENTIAL_PUBLISH_PREFIX": "/fc/",
		"FC_CREDENTIAL_PUBLISH_SERVICE_ACCOUNT_PATH": "{prefix}apps/{app}/{account}/{credential}",
		"FC_CREDENTIAL_PUBLISH_DEFAULT":              "ON",
		"FC_CREDENTIAL_PUBLISH_KMS_KEY":              "alias/fc",
	}))
	require.NoError(t, err)
	assert.True(t, c.DefaultOn)
	assert.Equal(t, "alias/fc", c.KMSKey)
}

func TestConfigFromEnv_invalid(t *testing.T) {
	base := map[string]string{"FC_CREDENTIAL_PUBLISH": "aws-ps", "FC_CREDENTIAL_PUBLISH_PREFIX": "/fc/"}
	with := func(k, v string) map[string]string {
		m := map[string]string{}
		maps.Copy(m, base)
		m[k] = v
		return m
	}
	cases := map[string]map[string]string{
		"unknown store":          with("FC_CREDENTIAL_PUBLISH", "vault"),
		"no prefix":              {"FC_CREDENTIAL_PUBLISH": "aws-ps"},
		"prefix no leading /":    with("FC_CREDENTIAL_PUBLISH_PREFIX", "fc/"),
		"prefix no trailing /":   with("FC_CREDENTIAL_PUBLISH_PREFIX", "/fc"),
		"prefix empty segment":   with("FC_CREDENTIAL_PUBLISH_PREFIX", "/fc//np/"),
		"prefix bad character":   with("FC_CREDENTIAL_PUBLISH_PREFIX", "/fc np/"),
		"bad default":            with("FC_CREDENTIAL_PUBLISH_DEFAULT", "maybe"),
		"template no prefix":     with("FC_CREDENTIAL_PUBLISH_SERVICE_ACCOUNT_PATH", "sa/{account}/{credential}"),
		"template no credential": with("FC_CREDENTIAL_PUBLISH_SERVICE_ACCOUNT_PATH", "{prefix}sa/{account}"),
		"partial placeholder":    with("FC_CREDENTIAL_PUBLISH_SERVICE_ACCOUNT_PATH", "{prefix}sa-{account}/{credential}"),
		"unknown placeholder":    with("FC_CREDENTIAL_PUBLISH_SERVICE_ACCOUNT_PATH", "{prefix}{tenant}/{credential}"),
		"wrong owner's one":      with("FC_CREDENTIAL_PUBLISH_SERVICE_ACCOUNT_PATH", "{prefix}{oauthClient}/{credential}"),
		"empty template segment": with("FC_CREDENTIAL_PUBLISH_OAUTH_CLIENT_PATH", "{prefix}a//{credential}"),
	}
	for name, m := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := configFromGetenv(getenvOf(m))
			assert.Error(t, err)
		})
	}
}

func testConfig() Config {
	return Config{
		Store: StoreAWSParameterStore, Prefix: "/flowcatalyst/np/",
		ServiceAccountPath: DefaultServiceAccountPath, OAuthClientPath: DefaultOAuthClientPath,
	}
}

func TestBasePath(t *testing.T) {
	c := testConfig()
	p, err := c.BasePath(OwnerServiceAccount, Vars{Account: "orders"})
	require.NoError(t, err)
	assert.Equal(t, "/flowcatalyst/np/service-accounts/orders/", p)

	p, err = c.BasePath(OwnerOAuthClient, Vars{OAuthClient: "my.client_1"})
	require.NoError(t, err)
	assert.Equal(t, "/flowcatalyst/np/oauth-clients/my.client_1/", p)

	c.ServiceAccountPath = "{prefix}{app}/{account}/{credential}"
	p, err = c.BasePath(OwnerServiceAccount, Vars{Account: "orders", App: "shop"})
	require.NoError(t, err)
	assert.Equal(t, "/flowcatalyst/np/shop/orders/", p)

	// {app} for an account without an application.
	_, err = c.BasePath(OwnerServiceAccount, Vars{Account: "orders"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "{app}")

	// A value that is not one path segment cannot climb out of its place.
	c = testConfig()
	for _, bad := range []string{"a/b", "..", "a b", ""} {
		_, err = c.BasePath(OwnerServiceAccount, Vars{Account: bad})
		assert.Error(t, err, "%q", bad)
	}

	// Missing templates fall back to the defaults (a hand-built Config).
	p, err = Config{Prefix: "/x/"}.BasePath(OwnerServiceAccount, Vars{Account: "a"})
	require.NoError(t, err)
	assert.Equal(t, "/x/service-accounts/a/", p)

	_, err = c.BasePath("NOPE", Vars{})
	assert.Error(t, err)
}

func TestValidateBasePath(t *testing.T) {
	c := testConfig()
	assert.NoError(t, c.ValidateBasePath("/flowcatalyst/np/custom/orders/"))
	assert.NoError(t, c.ValidateBasePath("/flowcatalyst/np/"))
	for name, p := range map[string]string{
		"outside the prefix": "/flowcatalyst/prod/x/",
		"prefix sibling":     "/flowcatalyst/np-other/x/",
		"no leading slash":   "flowcatalyst/np/x/",
		"no trailing slash":  "/flowcatalyst/np/x",
		"empty segment":      "/flowcatalyst/np//x/",
		"bad character":      "/flowcatalyst/np/x y/",
		"dot dot":            "/flowcatalyst/np/../x/",
		"too long":           "/flowcatalyst/np/" + strings.Repeat("a", 1000) + "/",
		"too deep":           "/flowcatalyst/np/" + strings.Repeat("a/", 14),
		"wildcard":           "/flowcatalyst/np/*/",
		"unicode":            "/flowcatalyst/np/é/",
	} {
		assert.Error(t, c.ValidateBasePath(p), name)
	}
}

func TestReference(t *testing.T) {
	assert.Equal(t, "aws-ps:///flowcatalyst/np/service-accounts/orders/auth-token",
		Reference("/flowcatalyst/np/service-accounts/orders/auth-token"))
}

func TestPendingEmpty(t *testing.T) {
	assert.True(t, Pending{}.Empty())
	assert.False(t, Pending{Enable: &EnableRequest{}}.Empty())
	assert.False(t, Pending{Changes: []Change{{Credential: CredClientID}}}.Empty())
}

func TestBackoff(t *testing.T) {
	assert.Equal(t, "30s", backoff(1).String())
	assert.Equal(t, "1m0s", backoff(2).String())
	assert.Equal(t, "2m0s", backoff(3).String())
	assert.Equal(t, "1h0m0s", backoff(20).String())
}

func TestAccountSegment(t *testing.T) {
	for in, want := range map[string]string{
		"orders":     "orders",
		"app:orders": "app.orders",
		"":           "",
	} {
		assert.Equal(t, want, AccountSegment(in), in)
	}
}

func TestConfigWant(t *testing.T) {
	yes, no := true, false
	off := Config{}
	on := Config{Store: StoreAWSParameterStore}
	onDefault := Config{Store: StoreAWSParameterStore, DefaultOn: true}
	for name, tc := range map[string]struct {
		cfg  Config
		req  *bool
		want bool
		code string
	}{
		"nil, not configured":   {off, nil, false, ""},
		"nil, configured, off":  {on, nil, false, ""},
		"nil, configured, on":   {onDefault, nil, true, ""},
		"false, default on":     {onDefault, &no, false, ""},
		"true, configured":      {on, &yes, true, ""},
		"true, not configured":  {off, &yes, false, CodeUnavailable},
		"false, not configured": {off, &no, false, ""},
	} {
		got, err := tc.cfg.Want(tc.req)
		assert.Equal(t, tc.want, got, name)
		if tc.code == "" {
			assert.NoError(t, err, name)
			continue
		}
		if ue := usecase.AsError(err); assert.NotNil(t, ue, name) {
			assert.Equal(t, tc.code, ue.Code, name)
		}
	}
}
