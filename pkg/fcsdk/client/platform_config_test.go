package client_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/client"
)

const secretJSON = `{"applicationCode":"orders","section":"smtp","property":"password","clientId":"clt_1","value":"hunter2-plaintext","updatedAt":"2026-10-23T10:00:00Z"}`

func TestRevealSecretSuccess(t *testing.T) {
	srv, seen := newMockSrv(t, secretJSON)
	c := client.New(srv.URL)

	got, err := c.PlatformConfig().RevealSecret(context.Background(), "my app", "a/b", "pw?x", "clt_1")
	require.NoError(t, err)
	assert.Equal(t, http.MethodGet, seen.method)
	assert.Equal(t, "/api/config/my app/a/b/pw?x/secret", seen.path) // decoded form
	assert.Equal(t, "clt_1", seen.query.Get("clientId"))
	assert.Equal(t, "hunter2-plaintext", got.Value)
	assert.Equal(t, "2026-10-23T10:00:00Z", got.UpdatedAt)
}

func TestRevealSecretEncodesSegmentsAndOmitsClientID(t *testing.T) {
	var rawPath, rawQuery, cacheCtl, ifNone string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawPath, rawQuery = r.URL.EscapedPath(), r.URL.RawQuery
		cacheCtl, ifNone = r.Header.Get("Cache-Control"), r.Header.Get("If-None-Match")
		_, _ = w.Write([]byte(secretJSON))
	}))
	t.Cleanup(srv.Close)
	c := client.New(srv.URL)

	_, err := c.PlatformConfig().RevealSecret(context.Background(), "my app", "a/b", "pw?x", "")
	require.NoError(t, err)
	assert.Equal(t, "/api/config/my%20app/a%2Fb/pw%3Fx/secret", rawPath)
	assert.Empty(t, rawQuery, "clientId is sent only when given")
	assert.Empty(t, cacheCtl)
	assert.Empty(t, ifNone)
}

func TestRevealSecretIsNotCached(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(secretJSON))
	}))
	t.Cleanup(srv.Close)
	c := client.New(srv.URL)
	for range 2 {
		_, err := c.PlatformConfig().RevealSecret(context.Background(), "a", "s", "p", "")
		require.NoError(t, err)
	}
	assert.Equal(t, 2, calls)
}

func TestRevealSecretErrors(t *testing.T) {
	cases := []struct {
		status int
		body   string
		check  func(t *testing.T, e *client.APIError)
	}{
		{403, `{"code":"FORBIDDEN","message":"not allowed"}`, func(t *testing.T, e *client.APIError) { assert.True(t, e.IsForbidden()) }},
		{404, `{"code":"NOT_FOUND","message":"no such property"}`, func(t *testing.T, e *client.APIError) { assert.True(t, e.IsNotFound()) }},
		{400, `{"code":"NOT_A_SECRET","message":"property is PLAIN"}`, func(t *testing.T, e *client.APIError) {
			assert.Equal(t, 400, e.StatusCode)
			assert.Equal(t, "NOT_A_SECRET", e.Code())
		}},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprint(tc.status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			t.Cleanup(srv.Close)
			c := client.New(srv.URL, client.WithRetry(1, 0))
			got, err := c.PlatformConfig().RevealSecret(context.Background(), "a", "s", "p", "")
			assert.Nil(t, got)
			var ae *client.APIError
			require.True(t, errors.As(err, &ae))
			tc.check(t, ae)
		})
	}
}

func TestSecretResponseIsRedactedWhenPrinted(t *testing.T) {
	srv, _ := newMockSrv(t, secretJSON)
	got, err := client.New(srv.URL).PlatformConfig().RevealSecret(context.Background(), "orders", "smtp", "password", "clt_1")
	require.NoError(t, err)

	for _, s := range []string{
		fmt.Sprintf("%v", got), fmt.Sprintf("%+v", got), fmt.Sprintf("%#v", got), got.String(),
		fmt.Sprintf("%v", *got), fmt.Sprint(got), fmt.Sprintf("%v", []*client.SecretResponse{got}),
	} {
		assert.NotContains(t, s, "hunter2-plaintext")
		assert.Contains(t, s, "[REDACTED]")
	}

	var sb strings.Builder
	slog.New(slog.NewTextHandler(&sb, nil)).Info("revealed", "secret", got)
	slog.New(slog.NewJSONHandler(&sb, nil)).Info("revealed", "secret", *got)
	assert.NotContains(t, sb.String(), "hunter2-plaintext")
}

func TestPlatformConfigGetAndSet(t *testing.T) {
	const masked = `{"id":"cfg_1","applicationCode":"orders","section":"smtp","property":"password","scope":"GLOBAL","valueType":"SECRET","value":"***","createdAt":"t","updatedAt":"t"}`
	srv, seen := newMockSrv(t, masked)
	c := client.New(srv.URL)

	got, err := c.PlatformConfig().Get(context.Background(), "orders", "smtp", "password", "")
	require.NoError(t, err)
	assert.Equal(t, "/api/config/orders/smtp/password", seen.path)
	assert.Empty(t, seen.query)
	assert.Equal(t, client.SecretMask, got.Value)

	_, err = c.PlatformConfig().Set(context.Background(), "orders", "smtp", "password", &client.SetConfigPropertyRequest{Value: "x", ValueType: "SECRET"})
	require.NoError(t, err)
	assert.Equal(t, http.MethodPut, seen.method)

	require.NoError(t, c.PlatformConfig().Delete(context.Background(), "orders", "smtp", "password", "clt_1"))
	assert.Equal(t, http.MethodDelete, seen.method)
	assert.Equal(t, "clt_1", seen.query.Get("clientId"))
}
