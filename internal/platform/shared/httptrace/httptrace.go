// Package httptrace holds what the ingestion and function traces share about
// a recorded outbound HTTP call: which of its parts a reader without the
// view-bodies permission may see.
package httptrace

import (
	"encoding/json"
	"net/url"
	"strings"
)

// OutboundSummary keeps the non-sensitive fields of an outbound call's
// record data (an ingestion's http record, a function's outbound record):
// the client, method, URL path (never scheme, host, user info or query),
// status, body sizes, duration, attempt and failure. It also returns the
// method, path and status, from which a reader without the detail
// permission is shown a one-line message. Data that does not parse gives
// nil and zero values.
func OutboundSummary(data []byte) (out []byte, method, path string, status int) {
	var in struct {
		Client  string `json:"client"`
		Request struct {
			Method        string `json:"method"`
			URL           string `json:"url"`
			BodyBytes     int64  `json:"bodyBytes"`
			BodyTruncated bool   `json:"bodyTruncated"`
		} `json:"request"`
		Response *struct {
			Status        int   `json:"status"`
			BodyBytes     int64 `json:"bodyBytes"`
			BodyTruncated bool  `json:"bodyTruncated"`
		} `json:"response"`
		DurationMs int64   `json:"durationMs"`
		Attempt    int     `json:"attempt"`
		Failure    *string `json:"failure"`
	}
	if len(data) == 0 || json.Unmarshal(data, &in) != nil {
		return nil, "", "", 0
	}
	path = urlPathOnly(in.Request.URL)
	type request struct {
		Method        string `json:"method"`
		URL           string `json:"url"`
		BodyBytes     int64  `json:"bodyBytes"`
		BodyTruncated bool   `json:"bodyTruncated"`
	}
	type response struct {
		Status        int   `json:"status"`
		BodyBytes     int64 `json:"bodyBytes"`
		BodyTruncated bool  `json:"bodyTruncated"`
	}
	kept := struct {
		Client     string    `json:"client,omitempty"`
		Request    request   `json:"request"`
		Response   *response `json:"response"`
		DurationMs int64     `json:"durationMs"`
		Attempt    int       `json:"attempt"`
		Failure    *string   `json:"failure,omitempty"`
	}{
		Client:     in.Client,
		Request:    request{Method: in.Request.Method, URL: path, BodyBytes: in.Request.BodyBytes, BodyTruncated: in.Request.BodyTruncated},
		DurationMs: in.DurationMs, Attempt: in.Attempt, Failure: in.Failure,
	}
	if in.Response != nil {
		kept.Response = &response{Status: in.Response.Status, BodyBytes: in.Response.BodyBytes, BodyTruncated: in.Response.BodyTruncated}
		status = in.Response.Status
	}
	out, err := json.Marshal(kept)
	if err != nil {
		return nil, "", "", 0
	}
	return out, in.Request.Method, path, status
}

// urlPathOnly is the path of a URL (scheme, host, user info and query
// dropped); a value that does not parse as a URL is cut at the first ? or #.
func urlPathOnly(raw string) string {
	if u, err := url.Parse(raw); err == nil && (u.Scheme != "" || u.Host != "" || strings.HasPrefix(raw, "/")) {
		return u.EscapedPath()
	}
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		raw = raw[:i]
	}
	return raw
}
