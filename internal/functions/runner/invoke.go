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

// MaxRequestBodyBytes is the runner-wide hard ceiling on a request body. The
// effective limit is min(the endpoint's or function's limit, this), so a
// manifest cannot ask for an unbounded read: the body is buffered in memory
// before it is framed for the guest, and a caller on the public entry is not
// trusted. 32 MiB is far above the 1 MiB default and any webhook payload while
// keeping the worst case per in-flight request small.
const MaxRequestBodyBytes = 32 << 20

// unknownAddress is the metric label for a request whose address is not a
// loaded function. The address comes from the URL or Host of an
// unauthenticated caller, so it must never become a label value.
const unknownAddress = "unknown"

// statusClientClosed is the (nginx) status recorded when the caller went away
// before the runner answered. Nobody reads the reply; the code only keeps
// access logs from claiming the function timed out.
const statusClientClosed = 499

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
		r.metrics.observe(r.metricAddress(t.address), 0, rerr.code, start)
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

	// A platform bearer token is checked before the body is touched, so an
	// unauthenticated caller cannot make the runner buffer anything. (Webhook
	// auth signs the body, so that path has to read first; it is bounded by
	// the same ceiling.)
	// Deliveries are always POST. A webhook endpoint that declares no method
	// matches every method, so refuse the rest here: a captured signed delivery
	// cannot be replayed as a GET/PUT/DELETE, and a webhook endpoint stays
	// unreachable by simple cross-site requests.
	if ep.Auth == abi.AuthWebhook && req.Method != http.MethodPost {
		outcome = "no_route"
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "the endpoint does not accept "+req.Method, 0)
		return
	}

	if caller == nil && ep.Auth == abi.AuthPlatform {
		c, err := r.tokens.Verify(req.Context(), req.Header.Get("Authorization"))
		if err != nil {
			outcome = "unauthenticated"
			writeError(w, http.StatusUnauthorized, "UNAUTHENTICATED", "a platform bearer token is required", 0)
			return
		}
		caller = c
	}

	maxBody := settings.Limits.MaxBodyBytes
	if maxBody <= 0 {
		maxBody = defaultMaxBodyBytes
	}
	if ep.MaxBodyBytes != nil {
		maxBody = *ep.MaxBodyBytes
	}
	maxBody = min(maxBody, MaxRequestBodyBytes)
	body, err := io.ReadAll(http.MaxBytesReader(w, req.Body, maxBody))
	if err != nil {
		var tooLarge *http.MaxBytesError
		switch {
		case errors.As(err, &tooLarge):
			outcome = "too_large"
			writeError(w, http.StatusRequestEntityTooLarge, "BODY_TOO_LARGE", fmt.Sprintf("the body is over %d bytes", maxBody), 0)
		case req.Context().Err() != nil:
			outcome = "client_closed"
			r.log.Debug("client went away while sending the body", "fn.address", fn.address, "err", err)
			w.WriteHeader(statusClientClosed)
		default:
			outcome = "bad_body"
			r.log.Debug("request body could not be read", "fn.address", fn.address, "err", err)
			writeError(w, http.StatusBadRequest, "BODY_UNREADABLE", "the request body could not be read", 0)
		}
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
			// Verified before the body was read; unreachable with caller nil.
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
	if !ver.enter() {
		// Retired by a promotion between resolve and here.
		outcome = "unavailable"
		writeError(w, http.StatusServiceUnavailable, "FUNCTION_UNAVAILABLE", "the function is not available right now", time.Second)
		return
	}
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
		// err can carry compiler or control-plane text; keep it in the log.
		r.log.Warn("function version unavailable", "fn.address", fn.address, "fn.version", ver.number, "err", err)
		writeError(w, http.StatusServiceUnavailable, "FUNCTION_UNAVAILABLE", "the function is not available right now", 5*time.Second)
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

	// From here until the instance is handed back, anything that leaves this
	// function (an early return or a panic, which net/http recovers) must not
	// keep the instance and its memory charge. A call that did not finish
	// cleanly is closed, never pooled: its state is unknown.
	returned := false
	defer func() {
		if !returned {
			_ = inst.Close(context.Background())
		}
	}()

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
		returned = true
		p.put(inst)
		outcome = "error"
		writeError(w, http.StatusInternalServerError, "FUNCTION_FAILED", "the request could not be encoded", 0)
		return
	}

	caps := &capabilities{fn: fn, ver: ver, invID: invID, log: r.guestLogger(fn, ver), emitter: r.cp, http: r.httpClient, database: r.db}
	out, herr := func() ([]byte, error) {
		defer caps.tx.rollbackAll()
		return inst.Handle(ctx, frame, caps)
	}()
	returned = true
	p.put(inst)
	if herr != nil {
		switch {
		case errors.Is(herr, engine.ErrDeadline) && req.Context().Err() != nil:
			// The caller hung up (the request context ended, not the call's
			// own deadline). Not a function fault, and there is nobody to tell.
			outcome = "client_closed"
			r.log.Debug("client went away during the call", "fn.address", fn.address, "fn.version", ver.number, "fn.invocation", invID)
			w.WriteHeader(statusClientClosed)
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
	switch {
	case err != nil:
		outcome = "malformed"
		r.log.Warn("function returned a malformed response", "fn.address", fn.address, "fn.version", ver.number, "fn.invocation", invID, "err", err)
		writeError(w, http.StatusInternalServerError, "FUNCTION_FAILED", "the function failed", 0)
	case !validGuestStatus(resp.Status):
		outcome = "malformed"
		r.log.Warn("function returned an out-of-range status", "fn.address", fn.address, "fn.version", ver.number, "fn.invocation", invID, "status", resp.Status)
		writeError(w, http.StatusBadGateway, "FUNCTION_BAD_RESPONSE", "the function returned an invalid response", 0)
	default:
		writeGuestResponse(w, resp, respBody, ep.CORS != nil, invID)
		outcome = statusClass(resp.Status)
	}
}

// validGuestStatus is the range a guest may answer with: informational (1xx)
// and out-of-range codes are not a final response.
func validGuestStatus(s int) bool { return s >= 200 && s <= 599 }

// writeGuestResponse sends the guest's answer. When the endpoint has a CORS
// policy the runner already set the Access-Control-* headers from it; the guest
// cannot add to or override them.
func writeGuestResponse(w http.ResponseWriter, resp abi.Response, body []byte, runnerCORS bool, invID string) {
	h := w.Header()
	for k, vs := range resp.Headers {
		if runnerCORS && strings.HasPrefix(http.CanonicalHeaderKey(k), "Access-Control-") {
			continue
		}
		for _, v := range vs {
			h.Add(k, v)
		}
	}
	for _, name := range hopHeaders {
		h.Del(name)
	}
	h.Del("Content-Length")
	h.Set("X-FlowCatalyst-Invocation", invID)
	w.WriteHeader(resp.Status)
	_, _ = w.Write(body)
}

func statusClass(s int) string {
	return strconv.Itoa(s/100) + "xx"
}

// metricAddress is the address label for a lookup miss: the function's own
// address when it is loaded (bounded by the pool), otherwise a fixed value.
func (r *Runner) metricAddress(address string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if _, ok := r.functions[address]; ok {
		return address
	}
	return unknownAddress
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
	if r.draining.Load() {
		// Shutting down: answer 503 (not a misleading 404) so callers retry elsewhere.
		return nil, nil, &resolveError{http.StatusServiceUnavailable, "FUNCTION_UNAVAILABLE", "the runner is shutting down", 5 * time.Second}
	}
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
	switch st, _ := ver.currentState(); st {
	case statePreparing:
		return nil, nil, &resolveError{http.StatusServiceUnavailable, "FUNCTION_PREPARING", "the version is still being prepared", 2 * time.Second}
	case stateFailed:
		return nil, nil, &resolveError{http.StatusServiceUnavailable, "FUNCTION_UNAVAILABLE", "the version could not be loaded", 30 * time.Second} // the reason is in the heartbeat and the log, not the caller's reply
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
		// A wildcard never applies with credentials (validation refuses the
		// combination; this covers documents published before it did).
		if (o == "*" && !c.AllowCredentials) || strings.EqualFold(o, origin) {
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
	h.Set("Vary", "Origin")
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
