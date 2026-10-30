package client_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/go-sdk/client"
)

func TestWithOnBehalfOfSendsTheHeaderAndRefusesBadValues(t *testing.T) {
	var got []string
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		got = append(got, r.Header.Get(client.OnBehalfOfHeader))
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	c := client.New(srv.URL)

	require.NoError(t, c.PlatformConfig().Delete(client.WithOnBehalfOf(context.Background(), " prn_alice "), "orders", "ftp", "password", ""))
	require.NoError(t, c.PlatformConfig().Delete(context.Background(), "orders", "ftp", "password", ""))
	require.NoError(t, c.PlatformConfig().Delete(client.WithOnBehalfOf(context.Background(), "  "), "orders", "ftp", "password", ""))
	assert.Equal(t, []string{"prn_alice", "", ""}, got, "trimmed when given, absent when not given or blank")

	for _, bad := range []string{strings.Repeat("a", 257), "alice\nbob", "a\x7fb"} {
		err := c.PlatformConfig().Delete(client.WithOnBehalfOf(context.Background(), bad), "orders", "ftp", "password", "")
		require.ErrorIs(t, err, client.ErrInvalidOnBehalfOf)
	}
	assert.Equal(t, 3, calls, "a bad value is refused before anything is sent")
}
