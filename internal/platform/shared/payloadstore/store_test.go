package payloadstore

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileStore_PutAndRangeRead(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := NewFileStore(dir)
	require.NoError(t, err)
	key := "v1/clt_A/2026-11-20/0ABCDEFGHJKMN"
	obj := []byte("0123456789abcdefghij")
	require.NoError(t, s.Put(ctx, key, obj))

	b, err := os.ReadFile(filepath.Join(dir, "v1", "clt_A", "2026-11-20", "0ABCDEFGHJKMN"))
	require.NoError(t, err)
	assert.Equal(t, obj, b, "stored under the key's path")
	entries, err := os.ReadDir(filepath.Join(dir, "v1", "clt_A", "2026-11-20"))
	require.NoError(t, err)
	assert.Len(t, entries, 1, "no temporary file left behind")

	got, err := s.GetRange(ctx, key, 10, 5)
	require.NoError(t, err)
	assert.Equal(t, []byte("abcde"), got)
	got, err = s.GetRange(ctx, key, 15, 100)
	require.NoError(t, err)
	assert.Equal(t, []byte("fghij"), got, "clipped at the end")
	got, err = s.GetRange(ctx, key, 20, 5)
	require.NoError(t, err)
	assert.Empty(t, got, "past the end")

	_, err = s.GetRange(ctx, "v1/clt_A/2026-11-20/NOPE", 0, 5)
	assert.ErrorIs(t, err, ErrObjectNotFound)

	assert.Error(t, s.Put(ctx, "v1/../../escape", obj), "a key that would leave the directory")
	_, err = s.GetRange(ctx, "../escape", 0, 1)
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrObjectNotFound)
}

func TestMemoryStore(t *testing.T) {
	ctx := context.Background()
	m := NewMemoryStore()
	obj := []byte("hello world")
	require.NoError(t, m.Put(ctx, "k/1", obj))
	obj[0] = 'X'
	got, err := m.GetRange(ctx, "k/1", 0, 5)
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), got, "Put keeps its own copy")
	_, err = m.GetRange(ctx, "k/2", 0, 5)
	assert.ErrorIs(t, err, ErrObjectNotFound)
}

func TestOpenStore(t *testing.T) {
	ctx := context.Background()
	s, err := OpenStore(ctx, "")
	require.NoError(t, err)
	assert.Nil(t, s, "empty: no store")

	dir := t.TempDir()
	s, err = OpenStore(ctx, "file://"+dir)
	require.NoError(t, err)
	kind, loc := s.Describe()
	assert.Equal(t, "file", kind)
	assert.Equal(t, filepath.Clean(dir), loc)

	for _, bad := range []string{"file://relative/dir", "file://", "s3://", "s3:///prefix", "http://x", "/abs/path", "gs://b/p"} {
		s, err := OpenStore(ctx, bad)
		require.Error(t, err, bad)
		assert.Nil(t, s, bad)
	}
}

// fakeS3 records the requests and answers from its fields.
type fakeS3 struct {
	put     *s3.PutObjectInput
	putBody []byte
	putErr  error
	get     *s3.GetObjectInput
	getBody []byte
	getErr  error
}

func (f *fakeS3) PutObject(_ context.Context, in *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	f.put = in
	b, err := io.ReadAll(in.Body)
	if err != nil {
		return nil, err
	}
	f.putBody = b
	return &s3.PutObjectOutput{}, f.putErr
}

func (f *fakeS3) GetObject(_ context.Context, in *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	f.get = in
	if f.getErr != nil {
		return nil, f.getErr
	}
	return &s3.GetObjectOutput{Body: io.NopCloser(bytes.NewReader(f.getBody))}, nil
}

func responseError(status int) error {
	return &smithyhttp.ResponseError{
		Response: &smithyhttp.Response{Response: &http.Response{StatusCode: status}},
		Err:      errors.New("http error"),
	}
}

func TestS3Store_PutPrefixesTheKey(t *testing.T) {
	f := &fakeS3{}
	s := newS3StoreWithClient(f, "payloads-bucket", "/fc/payloads/")
	require.NoError(t, s.Put(context.Background(), "v1/clt_A/2026-11-20/K", []byte("segment")))
	assert.Equal(t, "payloads-bucket", *f.put.Bucket)
	assert.Equal(t, "fc/payloads/v1/clt_A/2026-11-20/K", *f.put.Key)
	assert.Equal(t, int64(7), *f.put.ContentLength)
	assert.Equal(t, []byte("segment"), f.putBody)

	noPrefix := newS3StoreWithClient(f, "b", "")
	require.NoError(t, noPrefix.Put(context.Background(), "v1/_/2026-11-20/K", []byte("x")))
	assert.Equal(t, "v1/_/2026-11-20/K", *f.put.Key)

	f.putErr = errors.New("AccessDenied: no")
	err := s.Put(context.Background(), "v1/_/2026-11-20/K", []byte("x"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "access denied")
	assert.Contains(t, err.Error(), "fc/payloads/v1/_/2026-11-20/K")

	assert.Error(t, s.Put(context.Background(), "../x", []byte("x")), "bad key refused before any request")
}

func TestS3Store_GetRange(t *testing.T) {
	f := &fakeS3{getBody: []byte("0123456789")}
	s := newS3StoreWithClient(f, "b", "pfx")
	got, err := s.GetRange(context.Background(), "v1/_/2026-11-20/K", 100, 10)
	require.NoError(t, err)
	assert.Equal(t, []byte("0123456789"), got)
	assert.Equal(t, "bytes=100-109", *f.get.Range, "inclusive end")
	assert.Equal(t, "pfx/v1/_/2026-11-20/K", *f.get.Key)
	assert.Equal(t, "b", *f.get.Bucket)

	// Never more than the range, whatever the body holds.
	got, err = s.GetRange(context.Background(), "v1/_/2026-11-20/K", 0, 4)
	require.NoError(t, err)
	assert.Equal(t, []byte("0123"), got)
	assert.Equal(t, "bytes=0-3", *f.get.Range)

	for name, e := range map[string]error{
		"NoSuchKey": &types.NoSuchKey{},
		"NotFound":  &types.NotFound{},
		"bare 404":  responseError(404),
	} {
		f.getErr = e
		_, err := s.GetRange(context.Background(), "v1/_/2026-11-20/K", 0, 4)
		assert.ErrorIs(t, err, ErrObjectNotFound, name)
	}

	for name, e := range map[string]error{
		"InvalidRange": &smithy.GenericAPIError{Code: "InvalidRange"},
		"bare 416":     responseError(416),
	} {
		f.getErr = e
		got, err := s.GetRange(context.Background(), "v1/_/2026-11-20/K", 0, 4)
		require.NoError(t, err, name)
		assert.Empty(t, got, name)
	}

	f.getErr = responseError(503)
	_, err = s.GetRange(context.Background(), "v1/_/2026-11-20/K", 0, 4)
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrObjectNotFound)

	_, err = s.GetRange(context.Background(), "v1/_/2026-11-20/K", 0, 0)
	assert.Error(t, err, "an empty range is refused")
}

// The S3 store's failures surface through Resolve as "unavailable", its
// not-found as "missing".
func TestS3Store_ThroughResolve(t *testing.T) {
	f := &fakeS3{}
	s := newTestService(t, DefaultConfig(), newS3StoreWithClient(f, "b", "pfx"))
	payload := payloadOf(9000, 7)
	out := s.Prepare(context.Background(), []Item{{ClientID: "clt_A", Data: payload}})
	require.NoError(t, out[0].Err)
	require.True(t, out[0].Row.Offloaded())
	f.getBody = f.putBody[out[0].Row.Ref.Offset : out[0].Row.Ref.Offset+out[0].Row.Ref.Length]
	ref := out[0].Row.Ref.String()
	got, err := s.Resolve(context.Background(), Stored{ID: out[0].Row.ID, Ref: &ref, Size: len(payload)})
	require.NoError(t, err)
	assert.Equal(t, payload, got)

	other := Ref{Key: "v1/_/2026-11-20/K2", Offset: 0, Length: 10, Codec: CodecZstd}.String()
	f.getErr = &types.NoSuchKey{}
	_, err = s.Resolve(context.Background(), Stored{ID: "p2", Ref: &other, Size: 10})
	assert.ErrorIs(t, err, ErrMissing)
	f.getErr = responseError(500)
	_, err = s.Resolve(context.Background(), Stored{ID: "p3", Ref: &other, Size: 10})
	assert.ErrorIs(t, err, ErrStoreUnavailable)
}
