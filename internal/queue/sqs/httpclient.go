package sqs

import (
	"io"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

// plainBodyClient hides io.WriterTo on the request body before handing the
// request to the SDK's HTTP client.
//
// smithy-go wraps a request body so it can close it as soon as the response
// headers are back (transport/http ClientHandler). net/http, having written
// Content-Length bytes, reads the body once more to check nothing follows. If
// that last read lands after the close, the wrapper's WriteTo answers
// (0, io.EOF) — an error, where its Read answers a clean end — so net/http
// treats the request write as failed and closes the connection while the
// response body is still being read. The SDK then sees "200 ... failed to
// decode response body ... use of closed network connection" and retries.
//
// For ReceiveMessage that retry loses a batch: the broker has already handed
// the messages out, and they stay invisible until their visibility timeout
// (measured: 16-24 batches of ten per 500k messages at 4 CPUs against a broker
// that answers in microseconds; none at 1-2 CPUs). It needs a response that
// beats the writer goroutine's last read, so real SQS latency all but rules it
// out, but the cost when it happens is a two-minute stall for those messages.
//
// Without WriteTo, io.Copy falls back to Read, which reports a closed body as a
// plain EOF, and the write completes normally.
type plainBodyClient struct{ inner sqs.HTTPClient }

func (c plainBodyClient) Do(r *http.Request) (*http.Response, error) {
	if r.Body != nil && r.Body != http.NoBody {
		if _, ok := r.Body.(io.WriterTo); ok {
			r.Body = readCloserOnly{r.Body}
		}
	}
	return c.inner.Do(r)
}

// readCloserOnly exposes Read and Close and nothing else.
type readCloserOnly struct{ rc io.ReadCloser }

func (b readCloserOnly) Read(p []byte) (int, error) { return b.rc.Read(p) }
func (b readCloserOnly) Close() error               { return b.rc.Close() }
