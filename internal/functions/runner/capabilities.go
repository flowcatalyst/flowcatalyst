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
	"github.com/flowcatalyst/flowcatalyst-go/internal/netguard"
)

// maxHTTPResponseBytes caps an outbound response body (plan §6.3).
const maxHTTPResponseBytes = 16 << 20

// capabilities serves one invocation's host calls (engine.Host): only what the
// version's describe declares and the function's settings bind.
type capabilities struct {
	fn      *function
	ver     *version
	invID   string
	log     *slog.Logger
	emitter Emitter
	// dispatcher creates dispatch jobs for the dispatch capability; correlation
	// is the calling request's X-Correlation-Id ("" when it had none).
	dispatcher  Dispatcher
	correlation string
	http        *http.Client
	database    DBProvider
	tx          txSet
	// trace is the invocation's trace state; nil when no context is live for it.
	trace *invTrace
}

// Emitter sends an event on a function's behalf (the control plane).
type Emitter interface {
	Emit(ctx context.Context, r control.EmitRequest) (*control.EmitResponse, *abi.Error)
}

// Dispatcher creates a dispatch job on a function's behalf (the control plane).
type Dispatcher interface {
	Dispatch(ctx context.Context, r control.DispatchRequest) (*control.DispatchResponse, *abi.Error)
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
	case abi.OpDispatch:
		return c.dispatch(ctx, meta, body)
	case abi.OpTrace:
		return c.traceOp(meta)
	case abi.OpTracing:
		return c.tracingOp(meta)
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
	c.trace.logRecord(levelName(level), l.Msg, l.Attrs)
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

// hostAllowed reports whether u matches an httpAllow entry: exact host, or
// `*.suffix` for strict subdomains. An entry with a port matches only that
// port; an entry without one matches only the default web ports (80 and 443),
// so declaring a host does not open every service on it.
func hostAllowed(allow []string, u *url.URL) bool {
	host, port := strings.ToLower(u.Hostname()), u.Port()
	if port == "" {
		port = "80"
		if u.Scheme == "https" {
			port = "443"
		}
	}
	for _, a := range allow {
		aHost, aPort, hasPort := strings.Cut(strings.ToLower(a), ":")
		if hasPort {
			if aPort != port {
				continue
			}
		} else if port != "80" && port != "443" {
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

// newHTTPClient builds the outbound client under the process-wide delivery
// policy (netguard.Default: loopback only with FC_DELIVERY_ALLOW_LOOPBACK).
func newHTTPClient() *http.Client { return newHTTPClientWith(netguard.Default) }

// newHTTPClientWith builds the outbound client. The allowlist is a hostname
// check; the dial guard is the authoritative one: it vets the address actually
// connected to after DNS resolution, so an allowlisted name that resolves (or
// rebinds) to loopback, private, link-local or metadata addresses is refused,
// on every redirect hop too. Redirects are followed only to allowlisted hosts;
// the allowlist travels in the request context.
//
// Proxy is deliberately unset: a proxy would receive the connection instead of
// the target, so the dial-time address check would vet the proxy, not the
// destination.
func newHTTPClientWith(policy *netguard.Policy) *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	return &http.Client{
		Transport: &http.Transport{
			DialContext:           policy.DialContext(dialer),
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
		if errors.Is(err, netguard.ErrBlocked) {
			return nil, nil, abi.Errorf(abi.CodeNotAllowed, err.Error())
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

// maxDispatchPayloadBytes caps a dispatch job's payload; a job is a message,
// not a transfer. The platform enforces the same bound.
const maxDispatchPayloadBytes = 256 << 10

// dispatch serves the dispatch capability: the (target, path) must be declared
// by this version, as emit's type must. The platform repeats the check against
// its own copy of the describe document and does everything else (ownership,
// target resolution, URL, signing identity, the job rules): nothing the guest
// sends decides where a job goes.
func (c *capabilities) dispatch(ctx context.Context, meta, body []byte) ([]byte, []byte, *abi.Error) {
	// Strict: a dispatch meta key the runner does not know (a `pool`, say)
	// is a refusal, never silently dropped: the pool is the describe's, and a
	// guest that tries to pick one must hear that it cannot.
	dec := json.NewDecoder(bytes.NewReader(meta))
	dec.DisallowUnknownFields()
	var d abi.Dispatch
	if err := dec.Decode(&d); err != nil {
		return nil, nil, abi.Errorf(abi.CodeBadRequest, "dispatch meta is not the expected JSON: "+err.Error())
	}
	if _, ok := c.ver.describe.DispatchFor(d.Target, d.Path); !ok {
		return nil, nil, abi.Errorf(abi.CodeNotDeclared, fmt.Sprintf("dispatch target %s%s is not in this version's dispatches", d.Target, d.Path))
	}
	if len(body) > maxDispatchPayloadBytes {
		return nil, nil, abi.Errorf(abi.CodeTooLarge, "the dispatch payload is over 256 KiB")
	}
	out, aerr := c.dispatcher.Dispatch(ctx, control.DispatchRequest{
		FunctionID: c.fn.id, Version: c.ver.number, InvocationID: c.invID,
		RequestCorrelationID: c.correlation, Dispatch: d, Payload: body,
	})
	if aerr != nil {
		return nil, nil, aerr
	}
	return encode(abi.Dispatched{JobID: out.JobID}), nil, nil
}
