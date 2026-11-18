package public

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate"
)

// assetRepo publishes one file of the given size under the given digest.
type assetRepo struct {
	digest string
	size   int64
}

func (r assetRepo) PublicHead(context.Context, string, string, string, string, string) ([]appupdate.Update, error) {
	return nil, nil
}

func (r assetRepo) PublicAsset(_ context.Context, _, _, sha string) (string, int64, bool, error) {
	if sha != r.digest {
		return "", 0, false, nil
	}
	return "application/javascript", r.size, true, nil
}

func (r assetRepo) AssetsOf(context.Context, []string) (map[string][]appupdate.Asset, error) {
	return nil, nil
}

// bytesStore holds one file, whatever its address.
type bytesStore struct{ data []byte }

func (s bytesStore) Open(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(s.data)), nil
}

func digestOf(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// serve mounts the routes behind chi's Recoverer, as the platform's server does.
func serve(t *testing.T, repo Repository, store Store) *httptest.Server {
	t.Helper()
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	(&Server{Repo: repo, Store: store}).Mount(r, nil, nil)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}

func file(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i*7 + i/251)
	}
	return b
}

func TestAssetStreamsAFileThatMatchesItsAddress(t *testing.T) {
	for _, n := range []int{0, 1, verifyChunk - 1, verifyChunk, verifyChunk + 1, 3*verifyChunk + 17} {
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			data := file(n)
			d := digestOf(data)
			srv := serve(t, assetRepo{digest: d, size: int64(n)}, bytesStore{data: data})
			resp, err := http.Get(srv.URL + "/app-updates/acme/driver-app/assets/" + d)
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)
			got, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			assert.Equal(t, data, got)
			assert.Equal(t, "public, max-age=31536000, immutable", resp.Header.Get("Cache-Control"))
		})
	}
}

// A stored file whose bytes are not those of its address (one byte flipped, or
// cut short) is never delivered whole: the response stops short of its
// Content-Length, so a client sees a failed download and caches nothing.
func TestAssetCutsOffAFileThatDoesNotMatchItsAddress(t *testing.T) {
	good := file(3*verifyChunk + 17)
	d := digestOf(good)
	flipped := bytes.Clone(good)
	flipped[len(flipped)/2] ^= 0x01
	for name, stored := range map[string][]byte{
		"a byte changed":        flipped,
		"shorter than its size": good[:len(good)-5],
		"last chunk changed":    append(bytes.Clone(good[:len(good)-1]), good[len(good)-1]^0xff),
	} {
		t.Run(name, func(t *testing.T) {
			srv := serve(t, assetRepo{digest: d, size: int64(len(good))}, bytesStore{data: stored})
			resp, err := http.Get(srv.URL + "/app-updates/acme/driver-app/assets/" + d)
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode, "the status is sent before the file is read")
			assert.Equal(t, strconv.Itoa(len(good)), resp.Header.Get("Content-Length"))
			got, err := io.ReadAll(resp.Body)
			require.Error(t, err, "the body must end early")
			assert.True(t, errors.Is(err, io.ErrUnexpectedEOF), "got %v", err)
			assert.Less(t, len(got), len(good))
		})
	}
}

func TestCopyVerified(t *testing.T) {
	data := file(2*verifyChunk + 5)
	var out bytes.Buffer
	require.NoError(t, copyVerified(&out, bytes.NewReader(data), int64(len(data)), digestOf(data)))
	assert.Equal(t, data, out.Bytes())

	// A longer file is read only up to its registered size, which then fails the digest.
	out.Reset()
	err := copyVerified(&out, bytes.NewReader(append(bytes.Clone(data), 'x')), int64(len(data)), digestOf(append(bytes.Clone(data), 'x')))
	require.ErrorIs(t, err, errFileMismatch)
	assert.Less(t, out.Len(), len(data), "the held-back chunk is not written")
}
