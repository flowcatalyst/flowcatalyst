package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
)

type runner struct {
	rustURL string
	goURL   string
	client  *http.Client
}

type caseResult struct {
	name   string
	status caseStatus
	// detail is the diff text on failure, or the reason on skip.
	detail string
}

type caseStatus int

const (
	statusPassed caseStatus = iota
	statusFailed
	statusSkipped
)

// run fires the case at both targets and returns the verdict. The two
// HTTP calls run in parallel so a slow target doesn't double the
// per-case latency; otherwise the cases stay sequential because some
// rely on side effects of prior cases (POST → GET round-trips).
func (r *runner) run(ctx context.Context, c parityCase) caseResult {
	res := caseResult{name: c.Name}

	// Substitute ${VAR}s in path/body/headers. Track missing names so
	// we can skip cleanly if a required token isn't set.
	path, missingPath := substituteVars(c.Request.Path)
	body, missingBody := substituteVars(c.Request.Body)
	hdrs := make(map[string]string, len(c.Request.Headers))
	var missingHdr []string
	for k, v := range c.Request.Headers {
		val, miss := substituteVars(v)
		missingHdr = append(missingHdr, miss...)
		hdrs[k] = val
	}
	missing := dedupe(append(append(missingPath, missingBody...), missingHdr...))
	if len(missing) > 0 {
		res.status = statusSkipped
		res.detail = "unset env vars: " + strings.Join(missing, ", ")
		return res
	}

	type out struct {
		resp *http.Response
		body []byte
		err  error
	}
	var rustOut, goOut out
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		rustOut.resp, rustOut.body, rustOut.err = r.fire(ctx, r.rustURL, c.Request.Method, path, hdrs, body) //nolint:bodyclose // fire closes the body; resp kept only for status/headers
	}()
	go func() {
		defer wg.Done()
		goOut.resp, goOut.body, goOut.err = r.fire(ctx, r.goURL, c.Request.Method, path, hdrs, body) //nolint:bodyclose // fire closes the body; resp kept only for status/headers
	}()
	wg.Wait()

	switch {
	case rustOut.err != nil && goOut.err != nil:
		res.status = statusFailed
		res.detail = fmt.Sprintf("both targets errored\n  rust: %v\n  go:   %v", rustOut.err, goOut.err)
		return res
	case rustOut.err != nil:
		res.status = statusFailed
		res.detail = fmt.Sprintf("rust errored: %v\n  go returned: %d", rustOut.err, goOut.resp.StatusCode)
		return res
	case goOut.err != nil:
		res.status = statusFailed
		res.detail = fmt.Sprintf("go errored: %v\n  rust returned: %d", goOut.err, rustOut.resp.StatusCode)
		return res
	}

	diff := compareResponses(c.Expect, rustOut.resp, rustOut.body, goOut.resp, goOut.body)
	if diff == "" {
		res.status = statusPassed
		return res
	}
	res.status = statusFailed
	res.detail = diff
	return res
}

// fire executes one HTTP request and returns the response + body. The
// caller is responsible for closing the response body — we always close
// here since we read the whole thing.
func (r *runner) fire(ctx context.Context, base, method, path string, headers map[string]string, body string) (*http.Response, []byte, error) {
	url := base + path
	var reader io.Reader
	if body != "" {
		reader = bytes.NewBufferString(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return nil, nil, fmt.Errorf("build request: %w", err)
	}
	// Apply headers in a stable order so the request looks identical
	// across runs — useful when stepping through tcpdump.
	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		req.Header.Set(k, headers[k])
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("do: %w", err)
	}
	defer resp.Body.Close()
	bb, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp, nil, fmt.Errorf("read body: %w", err)
	}
	return resp, bb, nil
}

func dedupe(xs []string) []string {
	if len(xs) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(xs))
	out := xs[:0]
	for _, x := range xs {
		if _, ok := seen[x]; ok {
			continue
		}
		seen[x] = struct{}{}
		out = append(out, x)
	}
	sort.Strings(out)
	return out
}
