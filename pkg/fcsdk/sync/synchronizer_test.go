package sync_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/client"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/sync"
)

func TestSynchronizerSkipsEmptyCategories(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_ = json.NewEncoder(w).Encode(client.SyncResult{ApplicationCode: "x"})
	}))
	defer srv.Close()

	c := client.New(srv.URL)
	s := sync.NewSynchronizer(c)
	set := sync.ForApplication("orders") // no categories populated
	out := s.Sync(context.Background(), set, sync.DefaultOptions())

	assert.Equal(t, int32(0), hits.Load(), "no category enabled → no HTTP calls")
	assert.False(t, out.HasErrors())
}

func TestSynchronizerRunsRolesOnlyWhenConfigured(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		_ = json.NewEncoder(w).Encode(client.SyncResult{
			ApplicationCode: "orders",
			Created:         1,
			SyncedCodes:     []string{"admin"},
		})
	}))
	defer srv.Close()

	c := client.New(srv.URL)
	s := sync.NewSynchronizer(c)
	set := sync.ForApplication("orders").AddRole(
		sync.MakeRole("admin").WithDisplayName("Admin"),
	)
	out := s.Sync(context.Background(), set, sync.RolesOnly())

	require.NotNil(t, out.Roles)
	assert.Equal(t, uint32(1), out.Roles.Created)
	assert.Equal(t, []string{"admin"}, out.Roles.SyncedCodes)
	assert.False(t, out.HasErrors())
	assert.Equal(t, []string{"/api/applications/orders/roles/sync"}, paths)
}

func TestSynchronizerCapturesPerCategoryError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"code":"DENY","message":"nope"}`))
	}))
	defer srv.Close()

	c := client.New(srv.URL, client.WithRetry(1, 0)) // skip retry to keep the test fast
	s := sync.NewSynchronizer(c)
	set := sync.ForApplication("orders").AddRole(sync.MakeRole("admin"))
	out := s.Sync(context.Background(), set, sync.RolesOnly())

	require.NotNil(t, out.Roles)
	assert.True(t, out.HasErrors())
	assert.Contains(t, out.Roles.Error, "403")
	errs := out.Errors()
	assert.Equal(t, 1, len(errs))
	assert.Contains(t, errs, "roles")
}

func TestSynchronizerSendsRemoveUnlistedQuery(t *testing.T) {
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode(client.SyncResult{})
	}))
	defer srv.Close()

	c := client.New(srv.URL)
	s := sync.NewSynchronizer(c)
	set := sync.ForApplication("orders").AddRole(sync.MakeRole("admin"))
	_ = s.Sync(context.Background(), set, sync.Options{SyncRoles: true, RemoveUnlisted: true})

	assert.Equal(t, "removeUnlisted=true", seen)
}
