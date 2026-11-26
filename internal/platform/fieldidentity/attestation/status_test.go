package attestation_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity/attestation"
)

// googleExample is the example list of Google's documentation
// (developer.android.com/privacy-and-security/security-key-attestation).
const googleExample = `{
  "entries": {
    "2c8cdddfd5e03bfc": {"status": "REVOKED", "expires": "2020-11-13", "reason": "KEY_COMPROMISE", "comment": "Key stored on unsecure system"},
    "c8966fcb2fbb0d7a": {"status": "SUSPENDED", "reason": "SOFTWARE_FLAW", "comment": "Bug in keystore causes this key malfunction b/555555"}
  }
}`

func TestParseStatusList(t *testing.T) {
	t.Parallel()
	l, err := attestation.ParseStatusList([]byte(googleExample))
	require.NoError(t, err)
	assert.Equal(t, 2, l.Len())
	st, ok := l.Status("2c8cdddfd5e03bfc")
	assert.True(t, ok)
	assert.Equal(t, attestation.RevocationRevoked, st)
	st, _ = l.Status("c8966fcb2fbb0d7a")
	assert.Equal(t, attestation.RevocationSuspended, st)
	_, ok = l.Status("1001")
	assert.False(t, ok)
	serial, listed := attestation.ListedSerial(l, []string{"1001", "c8966fcb2fbb0d7a"})
	assert.True(t, listed)
	assert.Equal(t, "c8966fcb2fbb0d7a", serial)

	empty, err := attestation.ParseStatusList([]byte(`{"entries":{}}`))
	require.NoError(t, err)
	assert.Zero(t, empty.Len())

	// Only a document that is not the shape at all is refused.
	for name, doc := range map[string]string{
		"not json":          `nope`,
		"no entries":        `{}`,
		"entries not a map": `{"entries":[]}`,
		"a list":            `[]`,
	} {
		_, err := attestation.ParseStatusList([]byte(doc))
		assert.Error(t, err, name)
	}
}

// Inside a well-shaped document the reading leans to revoked: serials are
// normalised, an unknown or missing status is revoked, and a key that is not a
// serial is skipped.
func TestParseStatusListLeansToRevoked(t *testing.T) {
	t.Parallel()
	l, err := attestation.ParseStatusList([]byte(`{"entries":{
		"00ABC":   {"status":"SUSPENDED"},
		"0DEF0":   {"status":"FINE"},
		"1234":    {},
		"5678":    "not an object",
		"9abc":    {"status":"REVOKED"},
		"0000":    {"status":"REVOKED"},
		"not hex": {"status":"REVOKED"},
		"":        {"status":"REVOKED"}
	}}`))
	require.NoError(t, err)
	assert.Equal(t, 2, l.Skipped(), "the keys that are not serials")
	for serial, want := range map[string]attestation.RevocationState{
		"abc": attestation.RevocationSuspended, "def0": attestation.RevocationRevoked,
		"1234": attestation.RevocationRevoked, "5678": attestation.RevocationRevoked,
		"9abc": attestation.RevocationRevoked, "0": attestation.RevocationRevoked,
	} {
		st, ok := l.Status(serial)
		assert.True(t, ok, serial)
		assert.Equal(t, want, st, serial)
	}
	// A serial is looked up in the same form, however the certificate spells it.
	st, ok := l.Status("000ABC")
	assert.True(t, ok)
	assert.Equal(t, attestation.RevocationSuspended, st)
	_, ok = l.Status("not hex")
	assert.False(t, ok)

	// Two spellings of one serial: revoked wins over suspended.
	dup, err := attestation.ParseStatusList([]byte(`{"entries":{"0abc":{"status":"REVOKED"},"abc":{"status":"SUSPENDED"}}}`))
	require.NoError(t, err)
	st, _ = dup.Status("abc")
	assert.Equal(t, attestation.RevocationRevoked, st)
}

func TestStatusCacheAge(t *testing.T) {
	t.Parallel()
	c := attestation.NewStatusCache(48 * time.Hour)
	_, ok := c.Usable(testNow)
	assert.False(t, ok, "never fetched")
	l, err := attestation.ParseStatusList([]byte(googleExample))
	require.NoError(t, err)
	c.Set(l, testNow)
	got, ok := c.Usable(testNow.Add(48 * time.Hour))
	assert.True(t, ok)
	assert.Equal(t, 2, got.Len())
	_, ok = c.Usable(testNow.Add(48*time.Hour + time.Second))
	assert.False(t, ok, "older than the maximum age")
	var nilCache *attestation.StatusCache
	_, ok = nilCache.Usable(testNow)
	assert.False(t, ok)
}

// The fetcher keeps the last good copy when a fetch fails or answers a broken
// document, and hands each good copy on; the test talks to a local server only.
func TestRunStatusFetcherKeepsTheLastGoodCopy(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		switch calls.Add(1) {
		case 1:
			_, _ = w.Write([]byte(googleExample))
		case 2:
			_, _ = w.Write([]byte(`{"entries":["not","a","map"]}`))
		default:
			w.WriteHeader(http.StatusBadGateway)
		}
	}))
	defer srv.Close()
	cache := attestation.NewStatusCache(time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var handed atomic.Int32
	go func() {
		attestation.RunStatusFetcher(ctx, srv.Client(), srv.URL, cache, 20*time.Millisecond,
			func(_ context.Context, l *attestation.StatusList) {
				if l.Len() == 2 {
					handed.Add(1)
				}
			})
		close(done)
	}()
	require.Eventually(t, func() bool { return calls.Load() >= 4 }, 5*time.Second, 10*time.Millisecond)
	cancel()
	<-done
	l, ok := cache.Usable(time.Now())
	require.True(t, ok)
	assert.Equal(t, 2, l.Len(), "the broken and the failed fetches did not replace the good copy")
	assert.Equal(t, int32(1), handed.Load(), "only the good copy is handed on, once")
}

// The list is fetched over https only, and a redirect may not leave the host
// or drop to plain http.
func TestFetchStatusHTTPSOnly(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(googleExample))
	}))
	defer plain.Close()
	other := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(googleExample))
	}))
	defer other.Close()
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/same":
			http.Redirect(w, r, srv.URL+"/list", http.StatusFound)
		case "/other-host":
			http.Redirect(w, r, other.URL+"/list", http.StatusFound)
		case "/plain":
			http.Redirect(w, r, plain.URL+"/list", http.StatusFound)
		case "/loop":
			http.Redirect(w, r, srv.URL+"/loop", http.StatusFound)
		default:
			_, _ = w.Write([]byte(googleExample))
		}
	}))
	defer srv.Close()
	client := srv.Client()

	l, err := attestation.FetchStatus(ctx, client, srv.URL+"/same")
	require.NoError(t, err, "a redirect on the same host, over https")
	assert.Equal(t, 2, l.Len())
	for _, path := range []string{"/other-host", "/plain", "/loop"} {
		_, err := attestation.FetchStatus(ctx, client, srv.URL+path)
		assert.Error(t, err, path)
	}
	_, err = attestation.FetchStatus(ctx, plain.Client(), plain.URL)
	assert.ErrorContains(t, err, "https", "a plain http address is not fetched")

	for raw, ok := range map[string]bool{
		attestation.DefaultStatusURL:      true,
		"https://status.example.com/list": true,
		"http://status.example.com/list":  false,
		"http://127.0.0.1:8443/status":    false,
		"ftp://status.example.com/list":   false,
		"https:///no-host":                false,
		"status.example.com/list":         false,
	} {
		err := attestation.ValidateStatusURL(raw)
		assert.Equal(t, ok, err == nil, raw)
	}
}

// While the copy is stale every failed fetch is an error in the log (each
// unlock under REQUIRED is refused then); while it is usable a warning.
func TestRunStatusFetcherLogsAStaleCopyAsAnError(t *testing.T) { //nolint:paralleltest // swaps the default logger
	var buf syncBuffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	var calls atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	cache := attestation.NewStatusCache(time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		attestation.RunStatusFetcher(ctx, srv.Client(), srv.URL, cache, 20*time.Millisecond, nil)
		close(done)
	}()
	require.Eventually(t, func() bool { return calls.Load() >= 3 }, 5*time.Second, 10*time.Millisecond)
	cancel()
	<-done
	out := buf.String()
	assert.GreaterOrEqual(t, strings.Count(out, `"level":"ERROR"`), 3, "one error per failed fetch with no usable copy: %s", out)
	assert.NotContains(t, out, `"level":"WARN"`)

	// With a usable copy, a failure is a warning.
	buf.Reset()
	good, err := attestation.ParseStatusList([]byte(googleExample))
	require.NoError(t, err)
	cache.Set(good, time.Now().UTC())
	calls.Store(0)
	ctx, cancel = context.WithCancel(context.Background())
	done = make(chan struct{})
	go func() {
		attestation.RunStatusFetcher(ctx, srv.Client(), srv.URL, cache, 20*time.Millisecond, nil)
		close(done)
	}()
	require.Eventually(t, func() bool { return calls.Load() >= 2 }, 5*time.Second, 10*time.Millisecond)
	cancel()
	<-done
	out = buf.String()
	assert.Contains(t, out, `"level":"WARN"`)
	assert.NotContains(t, out, `"level":"ERROR"`)
}

// syncBuffer is a bytes.Buffer safe for the fetcher's goroutine and the test.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func (s *syncBuffer) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.b.Reset()
}
