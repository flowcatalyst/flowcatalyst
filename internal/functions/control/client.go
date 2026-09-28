package control

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
)

// TokenSource returns a bearer token for the runner's service account.
type TokenSource func(ctx context.Context) (string, error)

// Client is the runner's side of the control plane.
type Client struct {
	base  string
	token TokenSource
	http  *http.Client
}

// NewClient builds a client for the platform at baseURL. httpClient may be
// nil; it must not have a timeout shorter than MaxWait (long-polls).
func NewClient(baseURL string, token TokenSource, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: MaxWait + 30*time.Second}
	}
	return &Client{base: strings.TrimRight(baseURL, "/"), token: token, http: httpClient}
}

// ErrStatus is a non-success answer from the platform.
type ErrStatus struct {
	Status  int
	Code    string
	Message string
}

func (e *ErrStatus) Error() string {
	return fmt.Sprintf("control plane: HTTP %d %s: %s", e.Status, e.Code, e.Message)
}

func (c *Client) do(ctx context.Context, method, path string, body any, header http.Header) (*http.Response, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rd)
	if err != nil {
		return nil, err
	}
	maps.Copy(req.Header, header)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	tok, err := c.token(ctx)
	if err != nil {
		return nil, fmt.Errorf("control plane: token: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	return c.http.Do(req)
}

func statusError(resp *http.Response) error {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	var e struct {
		Code    string `json:"error"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(b, &e)
	if e.Message == "" {
		e.Message = strings.TrimSpace(string(b))
	}
	return &ErrStatus{Status: resp.StatusCode, Code: e.Code, Message: e.Message}
}

// Desired long-polls the pool's document. With etag set, the platform holds
// the request up to wait and answers 304 (changed=false) if nothing changed.
func (c *Client) Desired(ctx context.Context, pool, etag string, wait time.Duration) (d *Desired, newETag string, changed bool, err error) {
	q := url.Values{"pool": {pool}}
	if wait > 0 {
		q.Set("wait", strconv.Itoa(int(min(wait, MaxWait).Seconds())))
	}
	h := http.Header{}
	if etag != "" {
		h.Set("If-None-Match", etag)
	}
	resp, err := c.do(ctx, http.MethodGet, PathDesired+"?"+q.Encode(), nil, h)
	if err != nil {
		return nil, "", false, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusNotModified:
		return nil, etag, false, nil
	case http.StatusOK:
		var doc Desired
		if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
			return nil, "", false, fmt.Errorf("control plane: desired document: %w", err)
		}
		return &doc, resp.Header.Get("ETag"), true, nil
	default:
		return nil, "", false, statusError(resp)
	}
}

// Heartbeat reports the runner's state.
func (c *Client) Heartbeat(ctx context.Context, hb Heartbeat) error {
	resp, err := c.do(ctx, http.MethodPost, PathHeartbeat, hb, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return statusError(resp)
	}
	return nil
}

// Artifact downloads an artifact by digest. The platform either streams it or
// redirects to storage; Go's client drops the Authorization header on a
// cross-host redirect, so storage never sees the runner's token.
func (c *Client) Artifact(ctx context.Context, digest string) (io.ReadCloser, error) {
	resp, err := c.do(ctx, http.MethodGet, PathArtifacts+"/"+url.PathEscape(digest), nil, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, statusError(resp)
	}
	return resp.Body, nil
}

// Emit asks the platform to ingest an event for a function. A refusal is an
// *abi.Error the guest sees: UNAVAILABLE when the platform could not be
// reached or failed (retryable), NOT_ALLOWED / BAD_REQUEST otherwise.
func (c *Client) Emit(ctx context.Context, r EmitRequest) (*EmitResponse, *abi.Error) {
	resp, err := c.do(ctx, http.MethodPost, PathEvents, r, nil)
	if err != nil {
		if ctx.Err() != nil {
			return nil, abi.Errorf(abi.CodeDeadline, "the invocation's deadline passed while emitting")
		}
		return nil, abi.Errorf(abi.CodeUnavailable, "the platform could not be reached")
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 == 2 {
		var out EmitResponse
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return nil, abi.Errorf(abi.CodeUnavailable, "the platform's answer was unreadable")
		}
		return &out, nil
	}
	if se, ok := errors.AsType[*ErrStatus](statusError(resp)); ok {
		switch {
		case se.Status >= 500:
			return nil, abi.Errorf(abi.CodeUnavailable, se.Message)
		case se.Status == http.StatusForbidden:
			return nil, abi.Errorf(abi.CodeNotAllowed, se.Message)
		default:
			return nil, abi.Errorf(abi.CodeBadRequest, se.Message)
		}
	}
	return nil, abi.Errorf(abi.CodeUnavailable, "unexpected answer")
}
