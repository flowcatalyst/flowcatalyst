package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/engine"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

var (
	errPreparing = errors.New("the version is still being prepared")
	errFailed    = errors.New("the version could not be loaded")
	errUnloaded  = errors.New("the version has been unloaded")
)

// Default limits when the platform sends none (plan §7.1).
const (
	defaultMemoryMB       = 64
	defaultMaxConcurrency = 16
	defaultTimeout        = 30 * time.Second
	defaultMaxBodyBytes   = 1 << 20
)

// Handler is the runner's invocation entry: /fn/{address}[@{alias}|@v{n}]/{path...}.
func (r *Runner) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/fn/{target}/{path...}", r.serveInvoke)
	mux.HandleFunc("/fn/{target}", r.serveInvoke)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		if !r.ready.Load() {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	return mux
}

func writeError(w http.ResponseWriter, status int, code, msg string, retryAfter time.Duration) {
	if retryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code, "message": msg})
}

// target is the parsed {target} path segment.
type target struct {
	address string
	alias   string // named alias; "" = live
	version int    // explicit version; 0 = none
}

func parseTarget(s string) (target, bool) {
	addr, sel, has := strings.Cut(s, "@")
	t := target{address: addr}
	if !has {
		return t, addr != ""
	}
	if n, ok := strings.CutPrefix(sel, "v"); ok {
		if v, err := strconv.Atoi(n); err == nil && v > 0 {
			t.version = v
			return t, true
		}
	}
	if sel == "" {
		return t, false
	}
	t.alias = sel
	return t, true
}

func (r *Runner) serveInvoke(w http.ResponseWriter, req *http.Request) {
	t, ok := parseTarget(req.PathValue("target"))
	if !ok {
		writeError(w, http.StatusNotFound, "FUNCTION_NOT_FOUND", "no such function", 0)
		return
	}
	r.invoke(w, req, t, "/"+req.PathValue("path"), false)
}

// invoke runs one call to target t at path, from the private entry or (public
// true) the public one.
func (r *Runner) invoke(w http.ResponseWriter, req *http.Request, t target, path string, public bool) {
	start := time.Now()
	fn, ver, rerr := r.resolve(t)
	if rerr != nil {
		rerr.write(w)
		r.metrics.observe(t.address, 0, rerr.code, start)
		return
	}
	settings := fn.snapshot()
	outcome := "ok"
	defer func() { r.metrics.observe(fn.address, ver.number, outcome, start) }()

	// Explicit versions are an operator tool: platform token with
	// version:invoke and reach; the endpoint's own auth is not applied.
	var caller *abi.Caller
	if t.version > 0 {
		c, err := r.tokens.Verify(req.Context(), req.Header.Get("Authorization"))
		if err != nil {
			outcome = "unauthenticated"
			writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "a platform bearer token is required", 0)
			return
		}
		if !hasPermission(c.Permissions, PermVersionInvoke) || !canReach(c, settings.ClientID) {
			outcome = "forbidden"
			writeError(w, http.StatusForbidden, "FORBIDDEN", "invoking an explicit version needs "+PermVersionInvoke, 0)
			return
		}
		caller = c
	}

	// A CORS preflight is matched by the method it asks about.
	matchMethod := req.Method
	if req.Method == http.MethodOptions {
		if want := req.Header.Get("Access-Control-Request-Method"); want != "" {
			matchMethod = want
		}
	}
	m, err := ver.router.Match(matchMethod, path)
	if err != nil {
		outcome = "no_route"
		if errors.Is(err, abi.ErrMethodNotAllowed) {
			writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "the endpoint does not accept "+req.Method, 0)
			return
		}
		writeError(w, http.StatusNotFound, "ROUTE_NOT_FOUND", "no endpoint matches "+path, 0)
		return
	}
	ep := m.Endpoint

	if ep.CORS != nil && handleCORS(w, req, ep.CORS) {
		outcome = "preflight"
		return
	}

	maxBody := settings.Limits.MaxBodyBytes
	if maxBody <= 0 {
		maxBody = defaultMaxBodyBytes
	}
	if ep.MaxBodyBytes != nil {
		maxBody = *ep.MaxBodyBytes
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, req.Body, maxBody))
	if err != nil {
		outcome = "too_large"
		writeError(w, http.StatusRequestEntityTooLarge, "BODY_TOO_LARGE", fmt.Sprintf("the body is over %d bytes", maxBody), 0)
		return
	}

	if caller == nil {
		switch ep.Auth {
		case abi.AuthWebhook:
			if !verifyWebhook(settings.WebhookSecret, req, body) {
				outcome = "unauthenticated"
				writeError(w, http.StatusUnauthorized, "INVALID_SIGNATURE", "the delivery signature did not verify", 0)
				return
			}
			caller = &abi.Caller{Kind: abi.CallerWebhook}
		case abi.AuthPlatform:
			c, err := r.tokens.Verify(req.Context(), req.Header.Get("Authorization"))
			if err != nil {
				outcome = "unauthenticated"
				writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "a platform bearer token is required", 0)
				return
			}
			caller = c
		default:
			caller = &abi.Caller{Kind: abi.CallerAnonymous}
		}
	}

	release, ok := fn.tryAcquire()
	if !ok {
		outcome = "busy"
		writeError(w, http.StatusTooManyRequests, "FUNCTION_BUSY", "the function is at its concurrency limit", time.Second)
		return
	}
	defer release()
	ver.inflight.Add(1)
	defer ver.inflight.Done()
	ver.touch()

	timeout := time.Duration(settings.Limits.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	if ep.TimeoutMs != nil {
		timeout = min(timeout, time.Duration(*ep.TimeoutMs)*time.Millisecond)
	}
	ctx, cancel := context.WithTimeout(req.Context(), timeout)
	defer cancel()
	deadline, _ := ctx.Deadline()

	p, err := ver.acquirePool(ctx, r)
	if err != nil {
		outcome = "unavailable"
		writeError(w, http.StatusServiceUnavailable, "FUNCTION_UNAVAILABLE", err.Error(), 5*time.Second)
		return
	}
	inst, err := p.get(ctx)
	if err != nil {
		outcome = "no_memory"
		if !errors.Is(err, engine.ErrNoMemory) {
			outcome = "load_failed"
			r.log.Error("function instantiate failed", "fn.address", fn.address, "fn.version", ver.number, "err", err)
		}
		writeError(w, http.StatusServiceUnavailable, "FUNCTION_UNAVAILABLE", "the runner has no capacity for this call right now", time.Second)
		return
	}

	invID := tsid.GenerateUntyped()
	headers := req.Header.Clone()
	if ep.Auth == abi.AuthPlatform || t.version > 0 {
		// The caller is proven; never hand the guest a credential it could
		// replay against the platform.
		headers.Del("Authorization")
	}
	for _, h := range hopHeaders {
		headers.Del(h)
	}
	params := m.PathParams
	if params == nil {
		params = map[string]string{} // always an object on the wire, never null
	}
	frame, err := abi.MarshalFrame(abi.Request{
		ID:             invID,
		Address:        fn.address,
		Version:        ver.number,
		Method:         req.Method,
		Path:           path,
		RawQuery:       req.URL.RawQuery,
		Headers:        headers,
		Route:          ep.Pattern(),
		PathParams:     params,
		Caller:         *caller,
		DeadlineUnixMs: deadline.UnixMilli(),
		Host:           req.Host,
		RemoteAddr:     r.clientAddr(req, public),
		Public:         public,
	}, body)
	if err != nil {
		p.put(inst)
		outcome = "error"
		writeError(w, http.StatusInternalServerError, "FUNCTION_FAILED", "the request could not be encoded", 0)
		return
	}

	caps := &capabilities{fn: fn, ver: ver, invID: invID, log: r.guestLogger(fn, ver), emitter: r.cp, http: r.httpClient, database: r.db}
	out, herr := inst.Handle(ctx, frame, caps)
	caps.tx.rollbackAll()
	p.put(inst)
	if herr != nil {
		switch {
		case errors.Is(herr, engine.ErrDeadline):
			outcome = "timeout"
			writeError(w, http.StatusGatewayTimeout, "FUNCTION_TIMEOUT", "the function did not finish before its deadline", 0)
		case errors.Is(herr, engine.ErrOutOfMemory):
			outcome = "out_of_memory"
			r.log.Warn("function ran out of memory", "fn.address", fn.address, "fn.version", ver.number, "fn.invocation", invID)
			writeError(w, http.StatusInternalServerError, "FUNCTION_FAILED", "the function failed", 0)
		default:
			outcome = "failed"
			r.log.Warn("function failed", "fn.address", fn.address, "fn.version", ver.number, "fn.invocation", invID, "err", herr)
			writeError(w, http.StatusInternalServerError, "FUNCTION_FAILED", "the function failed", 0)
		}
		return
	}
	var resp abi.Response
	respBody, err := abi.UnmarshalFrame(out, &resp)
	if err != nil || resp.Status < 100 || resp.Status > 999 {
		outcome = "malformed"
		r.log.Warn("function returned a malformed response", "fn.address", fn.address, "fn.version", ver.number, "fn.invocation", invID, "err", err)
		writeError(w, http.StatusInternalServerError, "FUNCTION_FAILED", "the function failed", 0)
		return
	}
	for k, vs := range resp.Headers {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	for _, h := range hopHeaders {
		w.Header().Del(h)
	}
	w.Header().Del("Content-Length")
	w.Header().Set("X-FlowCatalyst-Invocation", invID)
	w.WriteHeader(resp.Status)
	_, _ = w.Write(respBody)
	outcome = statusClass(resp.Status)
}

func statusClass(s int) string {
	return strconv.Itoa(s/100) + "xx"
}

// resolveError is a lookup miss with its HTTP answer.
type resolveError struct {
	status     int
	code, msg  string
	retryAfter time.Duration
}

func (e *resolveError) write(w http.ResponseWriter) {
	writeError(w, e.status, e.code, e.msg, e.retryAfter)
}

// resolve finds the function and the version a target names.
func (r *Runner) resolve(t target) (*function, *version, *resolveError) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	fn, ok := r.functions[t.address]
	if !ok {
		return nil, nil, &resolveError{http.StatusNotFound, "FUNCTION_NOT_FOUND", "no such function", 0}
	}
	var n int
	switch {
	case t.version > 0:
		n = t.version
	case t.alias != "":
		v, ok := fn.aliases[t.alias]
		if !ok {
			return nil, nil, &resolveError{http.StatusNotFound, "ALIAS_NOT_FOUND", "no such alias", 0}
		}
		n = v
	default:
		n = fn.serving
		if n == 0 {
			n = fn.live
		}
	}
	ver, ok := fn.versions[n]
	if !ok || n == 0 {
		if t.version > 0 {
			return nil, nil, &resolveError{http.StatusNotFound, "VERSION_NOT_FOUND", "no such version on this runner", 0}
		}
		return nil, nil, &resolveError{http.StatusServiceUnavailable, "FUNCTION_UNAVAILABLE", "the function has no loaded version", 5 * time.Second}
	}
	//exhaustive:ignore ready and evicted versions are servable; only the unusable states need a reply
	switch st, reason := ver.currentState(); st {
	case statePreparing:
		return nil, nil, &resolveError{http.StatusServiceUnavailable, "FUNCTION_PREPARING", "the version is still being prepared", 2 * time.Second}
	case stateFailed:
		return nil, nil, &resolveError{http.StatusServiceUnavailable, "FUNCTION_UNAVAILABLE", "the version could not be loaded: " + reason, 30 * time.Second}
	case stateClosed:
		return nil, nil, &resolveError{http.StatusServiceUnavailable, "FUNCTION_UNAVAILABLE", "the version was unloaded", time.Second}
	}
	return fn, ver, nil
}

// handleCORS answers CORS for an endpoint that declares a policy. It returns
// true when it has fully answered (a preflight).
func handleCORS(w http.ResponseWriter, req *http.Request, c *abi.CORS) bool {
	origin := req.Header.Get("Origin")
	if origin == "" {
		return false
	}
	allowed := false
	for _, o := range c.Origins {
		if o == "*" || strings.EqualFold(o, origin) {
			allowed = true
			break
		}
	}
	if !allowed {
		if req.Method == http.MethodOptions && req.Header.Get("Access-Control-Request-Method") != "" {
			w.WriteHeader(http.StatusForbidden)
			return true
		}
		return false
	}
	h := w.Header()
	h.Add("Vary", "Origin")
	if len(c.Origins) == 1 && c.Origins[0] == "*" && !c.AllowCredentials {
		h.Set("Access-Control-Allow-Origin", "*")
	} else {
		h.Set("Access-Control-Allow-Origin", origin)
	}
	if c.AllowCredentials {
		h.Set("Access-Control-Allow-Credentials", "true")
	}
	if req.Method == http.MethodOptions && req.Header.Get("Access-Control-Request-Method") != "" {
		methods := c.Methods
		if len(methods) == 0 {
			methods = []string{req.Header.Get("Access-Control-Request-Method")}
		}
		h.Set("Access-Control-Allow-Methods", strings.Join(methods, ", "))
		if len(c.Headers) > 0 {
			h.Set("Access-Control-Allow-Headers", strings.Join(c.Headers, ", "))
		} else if rh := req.Header.Get("Access-Control-Request-Headers"); rh != "" {
			h.Set("Access-Control-Allow-Headers", rh)
		}
		h.Set("Access-Control-Max-Age", "600")
		w.WriteHeader(http.StatusNoContent)
		return true
	}
	return false
}

// limitsOf resolves a function's limits against defaults.
func limitsOf(f *control.Function) control.Limits {
	l := f.Limits
	if l.MemoryMB <= 0 {
		l.MemoryMB = defaultMemoryMB
	}
	if l.MaxConcurrency <= 0 {
		l.MaxConcurrency = defaultMaxConcurrency
	}
	if l.TimeoutMs <= 0 {
		l.TimeoutMs = defaultTimeout.Milliseconds()
	}
	if l.MaxBodyBytes <= 0 {
		l.MaxBodyBytes = defaultMaxBodyBytes
	}
	return l
}
