package fn

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"time"
)

// requestMeta is the request frame's meta JSON (plan §5.2).
type requestMeta struct {
	ID             string              `json:"id"`
	Address        string              `json:"address"`
	Version        int                 `json:"version"`
	Method         string              `json:"method"`
	Path           string              `json:"path"`
	RawQuery       string              `json:"rawQuery"`
	Headers        map[string][]string `json:"headers"`
	Route          string              `json:"route"`
	PathParams     map[string]string   `json:"pathParams"`
	Caller         callerMeta          `json:"caller"`
	DeadlineUnixMs int64               `json:"deadlineUnixMs"`
}

type callerMeta struct {
	Kind            string   `json:"kind"`
	ID              string   `json:"id,omitempty"`
	Type            string   `json:"type,omitempty"`
	Tier            string   `json:"tier,omitempty"`
	Clients         []string `json:"clients,omitempty"`
	Roles           []string `json:"roles,omitempty"`
	Applications    []string `json:"applications,omitempty"`
	AllApplications bool     `json:"allApplications,omitempty"`
	Permissions     []string `json:"permissions,omitempty"`
}

// responseMeta is the response frame's meta JSON (plan §5.2).
type responseMeta struct {
	Status  int                 `json:"status"`
	Headers map[string][]string `json:"headers,omitempty"`
}

// panicResponseBody is written verbatim when a handler panics (plan §9:
// "500 with body {"error":"FUNCTION_PANIC"}").
const panicResponseBody = `{"error":"FUNCTION_PANIC"}`

// handleRequestFrame decodes a request frame, dispatches it through the
// registered mux, and encodes the response frame. It never panics: a
// handler panic is recovered and turned into a 500 FUNCTION_PANIC response,
// logged via host op 1.
func handleRequestFrame(frame []byte) []byte {
	meta, body, err := decodeFrame(frame)
	if err != nil {
		return panicFrame() // malformed input frame; nothing sane to run
	}
	var rm requestMeta
	if err := json.Unmarshal(meta, &rm); err != nil {
		return panicFrame()
	}

	target := rm.Path
	if rm.RawQuery != "" {
		target += "?" + rm.RawQuery
	}
	req, err := http.NewRequest(rm.Method, target, bytes.NewReader(body))
	if err != nil {
		return panicFrame()
	}
	if rm.Headers != nil {
		req.Header = http.Header(rm.Headers)
	}

	ctx := context.Background()
	if rm.DeadlineUnixMs > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, time.UnixMilli(rm.DeadlineUnixMs))
		defer cancel()
	}
	ctx = context.WithValue(ctx, ctxKeyCaller, callerFromMeta(rm.Caller))
	ctx = context.WithValue(ctx, ctxKeyInvocation, Info{
		ID:       rm.ID,
		Address:  rm.Address,
		Version:  rm.Version,
		Deadline: time.UnixMilli(rm.DeadlineUnixMs),
	})
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	panicked := runHandler(rec, req)
	if panicked {
		return panicFrame()
	}

	respMeta := responseMeta{Status: rec.Code, Headers: map[string][]string(rec.Header())}
	mb, merr := json.Marshal(respMeta)
	if merr != nil {
		return panicFrame()
	}
	return encodeFrame(mb, rec.Body.Bytes())
}

// runHandler serves req through the registered mux, recovering any panic.
// It returns true if a panic occurred.
func runHandler(rec *httptest.ResponseRecorder, req *http.Request) (panicked bool) {
	defer func() {
		if p := recover(); p != nil {
			logHandlerPanic(p)
			panicked = true
		}
	}()
	reg.mux.ServeHTTP(rec, req)
	return false
}

func logHandlerPanic(p any) {
	mb, err := json.Marshal(logMeta{Level: "ERROR", Msg: "handler panic", Attrs: map[string]any{"panic": toString(p)}})
	if err != nil {
		return
	}
	_, _, _, _ = currentHost.call(opLog, mb, nil)
}

func toString(v any) string {
	if err, ok := v.(error); ok {
		return err.Error()
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "unprintable panic value"
	}
	return string(b)
}

func panicFrame() []byte {
	mb, _ := json.Marshal(responseMeta{Status: http.StatusInternalServerError, Headers: map[string][]string{
		"Content-Type": {"application/json"},
	}})
	return encodeFrame(mb, []byte(panicResponseBody))
}

func callerFromMeta(cm callerMeta) Caller {
	c := Caller{kind: cm.Kind}
	if cm.Kind == "principal" {
		c.principal = &Principal{
			ID:              cm.ID,
			Type:            cm.Type,
			Tier:            cm.Tier,
			Clients:         cm.Clients,
			Roles:           cm.Roles,
			Applications:    cm.Applications,
			AllApplications: cm.AllApplications,
			Permissions:     cm.Permissions,
		}
	}
	return c
}
