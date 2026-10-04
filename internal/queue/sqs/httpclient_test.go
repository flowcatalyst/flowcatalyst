package sqs

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingHTTPClient struct{ got *http.Request }

func (c *recordingHTTPClient) Do(r *http.Request) (*http.Response, error) {
	c.got = r
	return &http.Response{StatusCode: 200, Body: http.NoBody, Header: http.Header{}}, nil
}

// A body that offers WriteTo reaches the transport without it; a body with no
// WriteTo, no body and http.NoBody are passed through untouched.
func TestPlainBodyClientHidesWriteTo(t *testing.T) {
	rec := &recordingHTTPClient{}
	c := plainBodyClient{inner: rec}

	withWriteTo := io.NopCloser(bytes.NewReader([]byte("abc"))) // NopCloser keeps WriterTo
	_, isWT := withWriteTo.(io.WriterTo)
	require.True(t, isWT, "precondition: the body offers WriteTo")
	req, _ := http.NewRequest(http.MethodPost, "http://x/", nil)
	req.Body = withWriteTo
	_, err := c.Do(req)
	require.NoError(t, err)
	_, still := rec.got.Body.(io.WriterTo)
	assert.False(t, still, "WriteTo must be hidden")
	b, err := io.ReadAll(rec.got.Body)
	require.NoError(t, err)
	assert.Equal(t, "abc", string(b))
	assert.NoError(t, rec.got.Body.Close())

	plain := readCloserOnly{io.NopCloser(bytes.NewReader(nil))}
	req2, _ := http.NewRequest(http.MethodPost, "http://x/", nil)
	req2.Body = plain
	_, _ = c.Do(req2)
	assert.Equal(t, plain, rec.got.Body, "a body without WriteTo is passed through")

	req3, _ := http.NewRequest(http.MethodGet, "http://x/", nil)
	_, _ = c.Do(req3)
	assert.Nil(t, rec.got.Body)
	req4, _ := http.NewRequest(http.MethodPost, "http://x/", http.NoBody)
	_, _ = c.Do(req4)
	assert.Equal(t, http.NoBody, rec.got.Body)
}

// The condition itself, without timing: a request body built by smithy-go,
// closed (as its client handler does once the response headers are back) and
// then copied once more (as net/http does after writing Content-Length bytes).
// smithy's body answers that copy with io.EOF as an ERROR, which is what makes
// net/http close the connection; behind plainBodyClient the same copy ends
// cleanly.
func TestClosedSmithyBodyCopiesCleanlyBehindPlainBodyClient(t *testing.T) {
	build := func() *http.Request {
		sr := smithyhttp.NewStackRequest().(*smithyhttp.Request)
		sr.URL, _ = url.Parse("http://x/")
		sr.Method = http.MethodPost
		var err error
		sr, err = sr.SetStream(bytes.NewReader([]byte(`{"QueueUrl":"q"}`)))
		require.NoError(t, err)
		sr.ContentLength = 16
		return sr.Build(context.Background())
	}

	raw := build()
	require.NoError(t, raw.Body.Close())
	_, rawErr := io.Copy(io.Discard, raw.Body)
	if rawErr == nil {
		t.Log("smithy-go no longer returns an error here; plainBodyClient may be unnecessary")
	}

	rec := &recordingHTTPClient{}
	_, err := plainBodyClient{inner: rec}.Do(build())
	require.NoError(t, err)
	require.NoError(t, rec.got.Body.Close())
	_, err = io.Copy(io.Discard, rec.got.Body)
	assert.NoError(t, err, "a closed body must read as a clean end, or net/http closes the connection")
}

// A best-effort stress guard for the same thing end to end. It does NOT
// reproduce the race on every machine (it needs a response that beats the
// writer goroutine's last read; it showed on Linux at 4 CPUs, not on macOS
// loopback), so the test above is the one that pins the behaviour.
// Against a server that answers at once, with several Ps, no
// ReceiveMessage attempt may fail. Before plainBodyClient the SDK closed the
// request body while net/http was still doing its last read of it, net/http
// closed the connection, and the response (with its messages) was lost to
// "use of closed network connection".
func TestReceiveResponsesAreNotLostToTheRequestBodyRace(t *testing.T) {
	if runtime.GOMAXPROCS(0) < 4 {
		t.Skip("needs at least 4 Ps to open the race window")
	}
	if testing.Short() {
		t.Skip("stress test")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	client := newSQSClient(aws.Config{
		Region:           "us-east-1",
		Credentials:      aws.AnonymousCredentials{},
		BaseEndpoint:     aws.String(srv.URL),
		RetryMaxAttempts: 1, // a failed attempt must surface, not be retried away
	})
	const workers, perWorker = 16, 2000
	var failures atomic.Int64
	var firstErr atomic.Value
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for range perWorker {
				_, err := client.ReceiveMessage(context.Background(), &sqs.ReceiveMessageInput{
					QueueUrl:            aws.String(srv.URL + "/000000000000/q"),
					MaxNumberOfMessages: 10,
				})
				if err != nil {
					failures.Add(1)
					firstErr.CompareAndSwap(nil, err.Error())
				}
			}
		})
	}
	wg.Wait()
	assert.Zero(t, failures.Load(), "lost responses out of %d; first error: %v", workers*perWorker, firstErr.Load())
}
