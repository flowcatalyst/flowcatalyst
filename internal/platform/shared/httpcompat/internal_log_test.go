package httpcompat_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/artifact"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// failingStore is an in-memory artifact.Store whose Exists/PresignGet fail
// the way the AWS SDK does.
type failingStore struct{ err error }

func (f failingStore) Put(context.Context, string, io.Reader) error        { return f.err }
func (f failingStore) Open(context.Context, string) (io.ReadCloser, error) { return nil, f.err }
func (f failingStore) Exists(context.Context, string) (bool, error)        { return false, f.err }
func (f failingStore) Delete(context.Context, string) error                { return f.err }
func (f failingStore) Describe() (string, string, string)                  { return "s3", "my-bucket", "fn/art" }

func (f failingStore) PresignGet(context.Context, string, time.Duration) (string, bool, error) {
	return "", false, f.err
}

func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })
	return &buf
}

const dig = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

// TestInternalErrorsFromHumaHandlersAreLogged: a usecase.Internal returned
// from a huma handler must leave one Error log line carrying the full cause,
// the store diagnostics and the request path, while the wire body stays
// generic and nothing secret is logged.
func TestInternalErrorsFromHumaHandlersAreLogged(t *testing.T) {
	httpcompat.Init()
	buf := captureLogs(t)

	cause := errors.New("operation error S3: HeadObject, https response error StatusCode: 403, api error AccessDenied: " +
		`Get "https://b.s3.amazonaws.com/k?X-Amz-Signature=SECRETSIG&X-Amz-Credential=AKIASECRET"`)
	store := artifact.WithDiagnostics(failingStore{err: cause})

	_, api := humatest.New(t)
	huma.Register(api, huma.Operation{OperationID: "x", Method: "GET", Path: "/x"},
		func(ctx context.Context, _ *struct{}) (*struct{}, error) {
			if _, err := store.Exists(ctx, dig); err != nil {
				return nil, usecase.Internal("ARTIFACT_STORE", "exists check failed", err)
			}
			return nil, nil
		})
	huma.Register(api, huma.Operation{OperationID: "y", Method: "GET", Path: "/y"},
		func(ctx context.Context, _ *struct{}) (*struct{}, error) {
			if _, _, err := store.PresignGet(ctx, dig, time.Minute); err != nil {
				return nil, usecase.Internal("ARTIFACT_STORE", "presign failed", err)
			}
			return nil, nil
		})

	for _, path := range []string{"/x", "/y"} {
		resp := api.Get(path)
		require.Equal(t, http.StatusInternalServerError, resp.Code)
		assert.NotContains(t, resp.Body.String(), "AccessDenied", "wire body must stay generic")
	}

	out := buf.String()
	assert.Equal(t, 2, strings.Count(out, "internal error response"), "exactly one line per 500: %s", out)
	for _, want := range []string{
		"level=ERROR", "ARTIFACT_STORE", "exists check failed", "presign failed",
		"AccessDenied", "StatusCode: 403",
		"store_kind=s3", "bucket=my-bucket", "prefix=fn/art", "digest=" + dig,
		"store_op=exists", "store_op=presign", "path=/x", "path=/y", "method=GET",
	} {
		assert.Contains(t, out, want)
	}
	for _, secret := range []string{"SECRETSIG", "AKIASECRET", "X-Amz-Signature=S"} {
		assert.NotContains(t, out, secret)
	}
}

func TestNon5xxHumaErrorsAreNotLoggedAsInternal(t *testing.T) {
	httpcompat.Init()
	buf := captureLogs(t)
	_, api := humatest.New(t)
	huma.Register(api, huma.Operation{OperationID: "z", Method: "GET", Path: "/z"},
		func(context.Context, *struct{}) (*struct{}, error) {
			return nil, usecase.Validation("BAD", "nope")
		})
	require.Equal(t, http.StatusBadRequest, api.Get("/z").Code)
	assert.NotContains(t, buf.String(), "internal error response")
}

func TestStoreErrorPreservesSentinels(t *testing.T) {
	s := artifact.WithDiagnostics(failingStore{err: artifact.ErrNotFound})
	_, err := s.Open(context.Background(), dig)
	assert.ErrorIs(t, err, artifact.ErrNotFound)
	var se *artifact.StoreError
	assert.False(t, errors.As(err, &se))
}
