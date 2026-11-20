package client_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/go-sdk/client"
)

// photoSrv answers one scripted response per request and records what it saw.
type photoSrv struct {
	mu     sync.Mutex
	method string
	path   string
	query  string
	body   []byte
	header http.Header
	reply  func(w http.ResponseWriter)
	calls  int
}

func newPhotoSrv(t *testing.T, reply func(w http.ResponseWriter)) (*httptest.Server, *photoSrv) {
	t.Helper()
	p := &photoSrv{reply: reply}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		defer p.mu.Unlock()
		p.method, p.path, p.query, p.header = r.Method, r.URL.Path, r.URL.RawQuery, r.Header.Clone()
		p.body, _ = io.ReadAll(r.Body)
		p.calls++
		p.reply(w)
	}))
	t.Cleanup(srv.Close)
	return srv, p
}

func TestFieldWorkersSetPhotoSendsTheBytesAsTheyAre(t *testing.T) {
	srv, seen := newPhotoSrv(t, func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(workerJSON))
	})
	c := client.New(srv.URL)
	img := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0xFF, 0xD9}
	w, err := c.FieldWorkers().SetPhoto(context.Background(), "fwu_1", "image/jpeg", img)
	require.NoError(t, err)
	assert.Equal(t, http.MethodPut, seen.method)
	assert.Equal(t, "/api/field-workers/fwu_1/photo", seen.path)
	assert.Equal(t, img, seen.body, "the image is not JSON-encoded")
	assert.Equal(t, "image/jpeg", seen.header.Get("Content-Type"))
	assert.Equal(t, "fwk_1", w.ID)

	// No content type: the bytes decide, and it is sent as a byte stream.
	_, err = c.FieldWorkers().SetPhoto(context.Background(), "fwu_1", "", img)
	require.NoError(t, err)
	assert.Equal(t, "application/octet-stream", seen.header.Get("Content-Type"))
}

func TestFieldWorkersSetPhotoRefusalsCarryTheirCodes(t *testing.T) {
	for _, c := range []struct {
		status int
		code   string
	}{
		{413, "PHOTO_TOO_LARGE"},
		{415, "PHOTO_UNSUPPORTED_TYPE"},
		{400, "PHOTO_TYPE_MISMATCH"},
		{409, "PHOTOS_DISABLED_FOR_CLIENT"},
		{404, "PHOTOS_DISABLED"},
	} {
		srv, seen := newPhotoSrv(t, func(w http.ResponseWriter) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(c.status)
			_, _ = w.Write([]byte(`{"error":"` + c.code + `","message":"m"}`))
		})
		_, err := client.New(srv.URL).FieldWorkers().SetPhoto(context.Background(), "fwu_1", "image/jpeg", []byte{1})
		var api *client.APIError
		require.True(t, errors.As(err, &api), c.code)
		assert.Equal(t, c.status, api.StatusCode)
		assert.Equal(t, 1, seen.calls, "%s is not retried", c.code)
	}
}

func TestFieldWorkersGetPhoto(t *testing.T) {
	jpg := []byte{0xFF, 0xD8, 0xFF, 0xD9}
	srv, seen := newPhotoSrv(t, func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("ETag", `"0123456789abcdef-thumb"`)
		_, _ = w.Write(jpg)
	})
	c := client.New(srv.URL)
	img, err := c.FieldWorkers().GetPhoto(context.Background(), "fwu_1", "", "")
	require.NoError(t, err)
	assert.Equal(t, http.MethodGet, seen.method)
	assert.Equal(t, "/api/field-workers/fwu_1/photo", seen.path)
	assert.Empty(t, seen.query, "the default size is not sent")
	assert.Contains(t, seen.header.Get("Accept"), "image/jpeg")
	assert.Empty(t, seen.header.Get("If-None-Match"))
	assert.Equal(t, jpg, img.Bytes)
	assert.Equal(t, "image/jpeg", img.ContentType)
	assert.Equal(t, `"0123456789abcdef-thumb"`, img.ETag)
	assert.False(t, img.NotModified)

	_, err = c.FieldWorkers().GetPhoto(context.Background(), "fwu_1", client.PhotoFull, "")
	require.NoError(t, err)
	assert.Equal(t, "size=full", seen.query)
}

func TestFieldWorkersGetPhotoNotModified(t *testing.T) {
	srv, seen := newPhotoSrv(t, func(w http.ResponseWriter) {
		w.Header().Set("ETag", `"0123456789abcdef-thumb"`)
		w.WriteHeader(http.StatusNotModified)
	})
	img, err := client.New(srv.URL).FieldWorkers().GetPhoto(context.Background(), "fwu_1", client.PhotoThumb, `"0123456789abcdef-thumb"`)
	require.NoError(t, err)
	assert.Equal(t, `"0123456789abcdef-thumb"`, seen.header.Get("If-None-Match"))
	assert.True(t, img.NotModified)
	assert.Nil(t, img.Bytes)
	assert.Equal(t, `"0123456789abcdef-thumb"`, img.ETag)
}

func TestFieldWorkersGetPhotoNone(t *testing.T) {
	srv, _ := newPhotoSrv(t, func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"PHOTO_NOT_FOUND","message":"this worker has no photo"}`))
	})
	_, err := client.New(srv.URL).FieldWorkers().GetPhoto(context.Background(), "fwu_1", "", "")
	var api *client.APIError
	require.True(t, errors.As(err, &api))
	assert.True(t, api.IsNotFound())
	assert.Equal(t, "PHOTO_NOT_FOUND", api.Code())
}

func TestFieldWorkersRemovePhoto(t *testing.T) {
	srv, seen := newMockSrv(t, workerJSON)
	w, err := client.New(srv.URL).FieldWorkers().RemovePhoto(context.Background(), "fwu_1")
	require.NoError(t, err)
	assert.Equal(t, http.MethodDelete, seen.method)
	assert.Equal(t, "/api/field-workers/fwu_1/photo", seen.path)
	assert.Equal(t, "fwk_1", w.ID)
}

func TestWorkerResponseCarriesThePhoto(t *testing.T) {
	srv, _ := newMockSrv(t, `{"id":"fwu_1","clientId":"c","workerCode":"W","name":"W","status":"ACTIVE","apps":[],"photo":{"version":"0123456789abcdef","updatedAt":"2026-11-20T10:00:00Z"}}`)
	w, err := client.New(srv.URL).FieldWorkers().Get(context.Background(), "fwu_1")
	require.NoError(t, err)
	require.NotNil(t, w.Photo)
	assert.Equal(t, "0123456789abcdef", w.Photo.Version)
	srv2, _ := newMockSrv(t, `{"id":"fwu_1","clientId":"c","workerCode":"W","name":"W","status":"ACTIVE","apps":[],"photo":null}`)
	w, err = client.New(srv2.URL).FieldWorkers().Get(context.Background(), "fwu_1")
	require.NoError(t, err)
	assert.Nil(t, w.Photo)
}

func TestFieldPoliciesDeletePhotos(t *testing.T) {
	srv, seen := newMockSrv(t, `{"removed":3}`)
	r, err := client.New(srv.URL).FieldPolicies().DeletePhotos(context.Background(), "clt_1")
	require.NoError(t, err)
	assert.Equal(t, http.MethodDelete, seen.method)
	assert.Equal(t, "/api/field-policies/clt_1/photos", seen.path)
	assert.Equal(t, 3, r.Removed)
}

// A PUT replaces the policy: a request that names neither photo setting sends
// neither, so the platform defaults them (photosEnabled true,
// workerMaySetOwnPhoto false); the settings the caller sets are sent.
func TestFieldPolicyPutSendsThePhotoSettingsOnlyWhenSet(t *testing.T) {
	srv, seen := newMockSrv(t, `{"clientId":"clt_1","photosEnabled":true,"workerMaySetOwnPhoto":false}`)
	c := client.New(srv.URL)
	_, err := c.FieldPolicies().Put(context.Background(), "clt_1", &client.PutFieldPolicyRequest{})
	require.NoError(t, err)
	assert.JSONEq(t, `{"requireAttestation":false}`, seen.body)
	_, err = c.FieldPolicies().Put(context.Background(), "clt_1", &client.PutFieldPolicyRequest{PhotosEnabled: new(false), WorkerMaySetOwnPhoto: true})
	require.NoError(t, err)
	assert.JSONEq(t, `{"requireAttestation":false,"photosEnabled":false,"workerMaySetOwnPhoto":true}`, seen.body)
	p, err := c.FieldPolicies().Get(context.Background(), "clt_1")
	require.NoError(t, err)
	assert.True(t, p.PhotosEnabled)
	assert.False(t, p.WorkerMaySetOwnPhoto)
}
