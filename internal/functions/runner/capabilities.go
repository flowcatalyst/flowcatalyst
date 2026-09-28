package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
)

// maxHTTPResponseBytes caps an outbound response body (plan §6.3).
const maxHTTPResponseBytes = 16 << 20

// capabilities serves one invocation's host calls (engine.Host): only what the
// version's describe declares and the function's settings bind.
type capabilities struct {
	fn       *function
	ver      *version
	invID    string
	log      *slog.Logger
	emitter  Emitter
	http     *http.Client
	database DBProvider
	tx       txSet
}

// Emitter sends an event on a function's behalf (the control plane).
type Emitter interface {
	Emit(ctx context.Context, r control.EmitRequest) (*control.EmitResponse, *abi.Error)
}

func (c *capabilities) Call(ctx context.Context, op abi.Op, meta, body []byte) ([]byte, []byte, *abi.Error) {
	switch op {
	case abi.OpLog:
		return c.logCall(ctx, meta)
	case abi.OpConfigGet:
		return c.lookup(meta, c.ver.describe.Config, c.fn.snapshot().Config, "config")
	case abi.OpSecretGet:
		return c.lookup(meta, c.ver.describe.Secrets, c.fn.snapshot().Secrets, "secret")
	case abi.OpHTTPFetch:
		return c.fetch(ctx, meta, body)
	case abi.OpEventEmit:
		return c.emit(ctx, meta, body)
	case abi.OpDBQuery, abi.OpDBExec, abi.OpDBBegin, abi.OpDBCommit, abi.OpDBRollback:
		return c.dbCall(ctx, op, meta)
	}
	return nil, nil, abi.Errorf(abi.CodeUnknownOp, fmt.Sprintf("op %d is not part of ABI v1", op))
}

func decode[T any](meta []byte) (T, *abi.Error) {
	var v T
	if err := json.Unmarshal(meta, &v); err != nil {
		return v, abi.Errorf(abi.CodeBadRequest, "meta is not the expected JSON: "+err.Error())
	}
	return v, nil
}

func encode(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func (c *capabilities) logCall(ctx context.Context, meta []byte) ([]byte, []byte, *abi.Error) {
	l, aerr := decode[abi.Log](meta)
	if aerr != nil {
		return nil, nil, aerr
	}
	level := slog.LevelInfo
	switch strings.ToLower(l.Level) {
	case "debug":
		level = slog.LevelDebug
	case "warn", "warning":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	attrs := make([]any, 0, 2*len(l.Attrs)+2)
	attrs = append(attrs, "fn.invocation", c.invID)
	for k, v := range l.Attrs {
		attrs = append(attrs, "guest."+k, v)
	}
	c.log.Log(ctx, level, l.Msg, attrs...)
	return nil, nil, nil
}

func (c *capabilities) lookup(meta []byte, declared []string, values map[string]string, what string) ([]byte, []byte, *abi.Error) {
	k, aerr := decode[abi.Key](meta)
	if aerr != nil {
		return nil, nil, aerr
	}
	if !slices.Contains(declared, k.Key) {
		return nil, nil, abi.Errorf(abi.CodeNotDeclared, fmt.Sprintf("%s %q is not declared by this version", what, k.Key))
	}
	v, ok := values[k.Key]
	return encode(abi.Found{Found: ok}), []byte(v), nil
}

// hostAllowed reports whether host (no port) matches an httpAllow entry:
// exact, or `*.suffix` for strict subdomains. Entries with a port match only
// that port.
func hostAllowed(allow []string, u *url.URL) bool {
	host, port := strings.ToLower(u.Hostname()), u.Port()
	for _, a := range allow {
		aHost, aPort, hasPort := strings.Cut(a, ":")
		if hasPort && aPort != port {
			continue
		}
		if suffix, ok := strings.CutPrefix(aHost, "*."); ok {
			if strings.HasSuffix(host, "."+suffix) {
				return true
			}
			continue
		}
		if host == aHost {
			return true
		}
	}
	return false
}

// newHTTPClient builds the outbound client. Redirects are followed only to
// allowlisted hosts; the allowlist travels in the request context.
func newHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			MaxIdleConns:          256,
			MaxIdleConnsPerHost:   32,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 60 * time.Second,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("too many redirects")
			}
			allow, _ := req.Context().Value(allowKey{}).([]string)
			if !hostAllowed(allow, req.URL) {
				return fmt.Errorf("redirect to %s is not in httpAllow", req.URL.Host)
			}
			return nil
		},
	}
}

type allowKey struct{}

var hopHeaders = []string{"Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade", "Host"}

func (c *capabilities) fetch(ctx context.Context, meta, body []byte) ([]byte, []byte, *abi.Error) {
	r, aerr := decode[abi.HTTPRequest](meta)
	if aerr != nil {
		return nil, nil, aerr
	}
	u, err := url.Parse(r.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, nil, abi.Errorf(abi.CodeBadRequest, "url must be an absolute http(s) URL")
	}
	allow := c.ver.describe.HTTPAllow
	if !hostAllowed(allow, u) {
		return nil, nil, abi.Errorf(abi.CodeNotAllowed, fmt.Sprintf("%s is not in httpAllow", u.Host))
	}
	if r.TimeoutMs > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(r.TimeoutMs)*time.Millisecond)
		defer cancel()
	}
	method := r.Method
	if method == "" {
		method = http.MethodGet
	}
	req, err := http.NewRequestWithContext(context.WithValue(ctx, allowKey{}, allow), method, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, nil, abi.Errorf(abi.CodeBadRequest, err.Error())
	}
	for k, vs := range r.Headers {
		req.Header[http.CanonicalHeaderKey(k)] = vs
	}
	for _, h := range hopHeaders {
		req.Header.Del(h)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, nil, abi.Errorf(abi.CodeDeadline, "the request did not finish before its deadline")
		}
		var ue *url.Error
		if errors.As(err, &ue) && strings.Contains(ue.Err.Error(), "httpAllow") {
			return nil, nil, abi.Errorf(abi.CodeNotAllowed, ue.Err.Error())
		}
		return nil, nil, abi.Errorf(abi.CodeUnavailable, err.Error())
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxHTTPResponseBytes+1))
	if err != nil {
		if ctx.Err() != nil {
			return nil, nil, abi.Errorf(abi.CodeDeadline, "the response did not finish before its deadline")
		}
		return nil, nil, abi.Errorf(abi.CodeUnavailable, err.Error())
	}
	if len(data) > maxHTTPResponseBytes {
		return nil, nil, abi.Errorf(abi.CodeTooLarge, "the response body is over 16 MiB")
	}
	return encode(abi.HTTPResponse{Status: resp.StatusCode, Headers: resp.Header}), data, nil
}

func (c *capabilities) emit(ctx context.Context, meta, body []byte) ([]byte, []byte, *abi.Error) {
	ev, aerr := decode[abi.Event](meta)
	if aerr != nil {
		return nil, nil, aerr
	}
	if !slices.Contains(c.ver.describe.Emits, ev.Type) {
		return nil, nil, abi.Errorf(abi.CodeNotDeclared, fmt.Sprintf("event type %q is not in this version's emits", ev.Type))
	}
	if ev.DedupID == "" {
		return nil, nil, abi.Errorf(abi.CodeBadRequest, "dedupId is required")
	}
	if ev.Source == "" {
		ev.Source = "function:" + c.fn.address
	}
	out, aerr := c.emitter.Emit(ctx, control.EmitRequest{FunctionID: c.fn.id, Version: c.ver.number, Event: ev, Data: body})
	if aerr != nil {
		return nil, nil, aerr
	}
	return encode(abi.Emitted{EventID: out.EventID}), nil, nil
}
