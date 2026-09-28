package fn

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type httpFetchMeta struct {
	Method    string              `json:"method"`
	URL       string              `json:"url"`
	Headers   map[string][]string `json:"headers,omitempty"`
	TimeoutMs int64               `json:"timeoutMs,omitempty"`
}

type httpFetchRespMeta struct {
	Status  int                 `json:"status"`
	Headers map[string][]string `json:"headers,omitempty"`
}

// roundTripper implements http.RoundTripper over host op 4 (http.fetch).
type roundTripper struct{}

func (roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	var body []byte
	if req.Body != nil {
		b, err := io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return nil, err
		}
		body = b
	}

	var timeoutMs int64
	if dl, ok := req.Context().Deadline(); ok {
		timeoutMs = max(time.Until(dl).Milliseconds(), 0)
	}

	meta := httpFetchMeta{
		Method:    req.Method,
		URL:       req.URL.String(),
		Headers:   map[string][]string(req.Header),
		TimeoutMs: timeoutMs,
	}
	mb, err := json.Marshal(meta)
	if err != nil {
		return nil, err
	}

	rm, rb, hostErr, err := currentHost.call(opHTTPFetch, mb, body)
	if err != nil {
		return nil, err
	}
	if hostErr != nil {
		return nil, hostErr
	}

	var rmeta httpFetchRespMeta
	if len(rm) > 0 {
		if uerr := json.Unmarshal(rm, &rmeta); uerr != nil {
			return nil, uerr
		}
	}

	return &http.Response{
		Status:        fmt.Sprintf("%d %s", rmeta.Status, http.StatusText(rmeta.Status)),
		StatusCode:    rmeta.Status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        http.Header(rmeta.Headers),
		Body:          io.NopCloser(bytes.NewReader(rb)),
		ContentLength: int64(len(rb)),
		Request:       req,
	}, nil
}

// HTTPClient returns an *http.Client whose RoundTripper runs outbound
// requests through host op 4 (http.fetch). The request's context deadline,
// if any, is passed to the host as timeoutMs. Only hosts declared with
// HTTPAllow (and matched by the runner's allowlist, including on redirect)
// may be reached.
func HTTPClient() *http.Client {
	return &http.Client{Transport: roundTripper{}}
}
