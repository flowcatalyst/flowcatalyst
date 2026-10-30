package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/flowcatalyst/go-sdk/webhook"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
)

// devMaxShownBody is how much of a response body `fn dev` prints.
const devMaxShownBody = 8 << 10

// sendSpec is the request `fn dev --send` replays after every reload.
type sendSpec struct {
	file        string
	method      string
	path        string
	contentType string // "" = by file extension
	headers     []string
}

// contentTypeFor picks a Content-Type from the file's extension.
func contentTypeFor(file string) string {
	switch strings.ToLower(filepath.Ext(file)) {
	case ".xml":
		return "application/xml"
	case ".json":
		return "application/json"
	}
	return "application/octet-stream"
}

// parseHeaders splits "K: V" flags.
func parseHeaders(in []string) (http.Header, error) {
	h := http.Header{}
	for _, kv := range in {
		k, v, ok := strings.Cut(kv, ":")
		if !ok || strings.TrimSpace(k) == "" {
			return nil, fmt.Errorf("header %q is not 'Key: Value'", kv)
		}
		h.Add(strings.TrimSpace(k), strings.TrimSpace(v))
	}
	return h, nil
}

// buildRequest makes the HTTP request for body against base
// (http://127.0.0.1:port). Auth the caller did not supply is added from the
// endpoint the request lands on: a webhook endpoint gets a signature made
// with webhookSecret, a platform endpoint a dev bearer token.
func (s sendSpec) buildRequest(base, address string, desc *abi.Describe, webhookSecret string, body []byte) (*http.Request, error) {
	method := strings.ToUpper(s.method)
	if method == "" {
		method = http.MethodPost
	}
	path := s.path
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	hdr, err := parseHeaders(s.headers)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(method, base+"/fn/"+address+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header = hdr
	ct := s.contentType
	if ct == "" {
		ct = contentTypeFor(s.file)
	}
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", ct)
	}
	if desc != nil {
		route, _, _ := strings.Cut(path, "?")
		if rt, err := abi.NewRouter(desc.Endpoints); err == nil {
			if m, err := rt.Match(method, route); err == nil {
				switch m.Endpoint.Auth {
				case abi.AuthPlatform:
					if req.Header.Get("Authorization") == "" {
						req.Header.Set("Authorization", "Bearer dev")
					}
				case abi.AuthWebhook:
					if req.Header.Get(webhook.ValidatorSignatureHeader) == "" {
						ts := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
						req.Header.Set(webhook.ValidatorTimestampHeader, ts)
						req.Header.Set(webhook.ValidatorSignatureHeader, webhook.NewValidator(webhookSecret).ComputeSignature(ts, body))
					}
				}
			}
		}
	}
	return req, nil
}

// formatBody renders a response body for the terminal: JSON indented,
// binary summarised, anything over max truncated with a note.
func formatBody(body []byte, contentType string, max int) string {
	if len(body) == 0 {
		return "(empty body)"
	}
	if !utf8.Valid(body) {
		return fmt.Sprintf("(%d bytes of binary data)", len(body))
	}
	text := string(body)
	trimmed := bytes.TrimSpace(body)
	if (strings.Contains(contentType, "json") || (len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '['))) && json.Valid(trimmed) {
		var b bytes.Buffer
		if json.Indent(&b, trimmed, "", "  ") == nil {
			text = b.String()
		}
	}
	if len(text) > max {
		cut := text[:max]
		for !utf8.ValidString(cut) {
			cut = cut[:len(cut)-1]
		}
		return fmt.Sprintf("%s\n… truncated: showing %d of %d bytes", cut, len(cut), len(text))
	}
	return text
}

// shownHeader reports a response header worth printing: the content headers
// and anything a function adds itself (X-*).
func shownHeader(k string) bool {
	switch http.CanonicalHeaderKey(k) {
	case "Content-Type", "Content-Length", "Location", "Retry-After", "Allow", "Www-Authenticate", "Etag", "Cache-Control":
		return true
	}
	return strings.HasPrefix(http.CanonicalHeaderKey(k), "X-")
}

// formatResponse renders a replayed request's outcome: status line, selected
// headers, body, then the events and dispatches the call produced.
func formatResponse(w io.Writer, resp *http.Response, body []byte, took time.Duration, produced []string, st devStyle) {
	status := fmt.Sprintf("%d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	col := st.green
	if resp.StatusCode >= 400 {
		col = st.red
	}
	fmt.Fprintf(w, "%s %s  (%s)\n", st.dim("<-"), col(status), took.Round(time.Millisecond))
	keys := make([]string, 0, len(resp.Header))
	for k := range resp.Header {
		if shownHeader(k) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(w, "   %s: %s\n", k, strings.Join(resp.Header[k], ", "))
	}
	for line := range strings.SplitSeq(formatBody(body, resp.Header.Get("Content-Type"), devMaxShownBody), "\n") {
		fmt.Fprintf(w, "   %s\n", line)
	}
	if len(produced) == 0 {
		fmt.Fprintf(w, "   %s\n", st.dim("no events emitted, no jobs dispatched"))
		return
	}
	fmt.Fprintf(w, "   %s\n", st.dim("produced by this call:"))
	for _, l := range produced {
		fmt.Fprintf(w, "     %s\n", l)
	}
}
