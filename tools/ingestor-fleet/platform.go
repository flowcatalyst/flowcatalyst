package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"
)

// The names the test seeds on the platform.
const (
	appCode        = "fleet"
	clientIdent    = "fleet"
	poolCode       = "fleet-pool"
	runtimeSACode  = "fleet-runtime"
	sourceHost     = "fleet-pg.test"   // resolved by the runtimes' hosts file to 127.0.0.1
	feedHost       = "fleet-feed.test" // the same
	dbSecretName   = "env:db-password"
	dbSecretEnvVar = "INGEST_SECRET_V2_fleet__db_2Dpassword"
	jobSourcePfx   = "ingestion:" + appCode + ":"
)

// platform is the admin's session on the platform API.
type platform struct {
	base string
	http *http.Client
}

func newPlatform(base string) (*platform, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &platform{base: strings.TrimSuffix(base, "/"), http: &http.Client{Jar: jar, Timeout: 30 * time.Second}}, nil
}

// call sends body (marshalled when not nil) and decodes a 2xx answer into
// out (when not nil). The error names the call and status, never a body that
// may hold a secret.
func (p *platform) call(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, p.base+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Origin", p.base)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("%s %s answered %d: %s", method, path, resp.StatusCode, errorSummary(data))
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("%s %s: decoding the answer: %w", method, path, err)
		}
	}
	return nil
}

// errorSummary is the platform error envelope's code and message, never the
// raw body.
func errorSummary(data []byte) string {
	var e struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Detail  string `json:"detail"`
		Errors  []struct {
			Message  string `json:"message"`
			Location string `json:"location"`
		} `json:"errors"`
	}
	if json.Unmarshal(data, &e) != nil {
		return "(no error envelope)"
	}
	parts := []string{e.Code, e.Message, e.Detail}
	for _, x := range e.Errors {
		parts = append(parts, x.Location+": "+x.Message)
	}
	return strings.Join(nonEmpty(parts), "; ")
}

func nonEmpty(s []string) []string {
	var out []string
	for _, v := range s {
		if strings.TrimSpace(v) != "" {
			out = append(out, v)
		}
	}
	return out
}

func (p *platform) login(ctx context.Context, email, password string) error {
	return p.call(ctx, http.MethodPost, "/auth/login", map[string]string{"email": email, "password": password}, nil)
}

// ingestionSpec is one ingestion the test runs and the source that feeds it.
type ingestionSpec struct {
	code  string
	table string // the source table in the sources' database
	kind  string // database-poll or api-poll
	id    string // the platform's ing_ id, once created
}

// runtimeCredentials is the runtime service account's OAuth client. The
// secret goes only into the runtimes' environment.
type runtimeCredentials struct {
	clientID string
	secret   string
}

// seed creates the client, the integration application, the pool, the
// runtime's service account and the ingestions.
func (p *platform) seed(ctx context.Context, cfg config, specs []ingestionSpec) (runtimeCredentials, []ingestionSpec, error) {
	var creds runtimeCredentials
	var client struct {
		ID string `json:"id"`
	}
	if err := p.call(ctx, http.MethodPost, "/api/clients", map[string]string{"name": "Fleet", "identifier": clientIdent}, &client); err != nil {
		return creds, nil, err
	}
	var app struct {
		ID string `json:"id"`
	}
	if err := p.call(ctx, http.MethodPost, "/api/applications",
		map[string]string{"code": appCode, "name": "Fleet test", "type": "INTEGRATION"}, &app); err != nil {
		return creds, nil, err
	}
	if err := p.call(ctx, http.MethodPut, "/api/clients/"+client.ID+"/applications",
		map[string]any{"enabledApplicationIds": []string{app.ID}}, nil); err != nil {
		return creds, nil, err
	}
	if err := p.call(ctx, http.MethodPost, "/api/dispatch-pools",
		map[string]any{"code": poolCode, "name": "Fleet test", "description": "Jobs the fleet test's ingestions make", "concurrency": 20}, nil); err != nil {
		return creds, nil, err
	}

	// The runtime's account: anchor tier (no clients), every application (so
	// bound to none), holding platform:ingestion-runtime only.
	var sa struct {
		ServiceAccount struct {
			ID string `json:"id"`
		} `json:"serviceAccount"`
		OAuth struct {
			ClientID     string `json:"clientId"`
			ClientSecret string `json:"clientSecret"`
		} `json:"oauth"`
	}
	allApps := true
	if err := p.call(ctx, http.MethodPost, "/api/service-accounts",
		map[string]any{"code": runtimeSACode, "name": "Fleet test runtime", "allApplications": allApps}, &sa); err != nil {
		return creds, nil, err
	}
	if sa.OAuth.ClientID == "" || sa.OAuth.ClientSecret == "" {
		return creds, nil, fmt.Errorf("the runtime's service account was created without OAuth credentials in the answer")
	}
	if err := p.call(ctx, http.MethodPut, "/api/service-accounts/"+sa.ServiceAccount.ID+"/roles",
		map[string]any{"roles": []string{"platform:ingestion-runtime"}}, nil); err != nil {
		return creds, nil, err
	}
	creds = runtimeCredentials{clientID: sa.OAuth.ClientID, secret: sa.OAuth.ClientSecret}

	pgPort := pgPortOf(cfg.pgURL)
	out := make([]ingestionSpec, 0, len(specs))
	for _, s := range specs {
		var source, split map[string]any
		switch s.kind {
		case "database-poll":
			source = map[string]any{
				"kind":     "database-poll",
				"jdbcUrl":  fmt.Sprintf("jdbc:postgresql://%s:%s/%s", sourceHost, pgPort, cfg.sourceDB),
				"username": "postgres",
				"password": map[string]string{"name": dbSecretName},
				"table":    s.table,
				"track":    map[string]string{"kind": "watermark", "column": "id", "type": "NUMBER"},
				"maxRows":  500,
				"every":    isoSeconds(cfg.sourceEvery),
			}
			split = map[string]any{"kind": "none"}
		case "api-poll":
			initial := "0"
			source = map[string]any{
				"kind":      "api-poll",
				"url":       fmt.Sprintf("http://%s:%d/feed", feedHost, cfg.feedPort),
				"method":    "GET",
				"auth":      map[string]string{"kind": "none"},
				"results":   map[string]string{"kind": "json-path", "path": "$.items"},
				"cursor":    map[string]any{"from": "$.next", "param": "after", "initial": initial},
				"every":     isoSeconds(cfg.sourceEvery),
				"idleDelay": isoSeconds(cfg.sourceEvery),
			}
			split = map[string]any{"kind": "json", "arrayPath": "$.items"}
		}
		body := map[string]any{
			"application": appCode,
			"client":      clientIdent,
			"code":        s.code,
			"name":        "Fleet " + s.code,
			"live":        true,
			"source":      source,
			"split":       split,
			"target": map[string]any{
				"code":      appCode + ":fleet:record:ingested",
				"targetUrl": fmt.Sprintf("http://127.0.0.1:%d/hook", cfg.receiverPort),
				"pool":      poolCode,
				"mode":      "IMMEDIATE",
				"batchSize": 100,
				"dataOnly":  true,
			},
		}
		var created struct {
			ID   string `json:"id"`
			Save *struct {
				Outcome string `json:"outcome"`
			} `json:"save"`
		}
		if err := p.call(ctx, http.MethodPost, "/api/ingestions", body, &created); err != nil {
			return creds, nil, err
		}
		if created.ID == "" {
			return creds, nil, fmt.Errorf("ingestion %s created without an id in the answer", s.code)
		}
		s.id = created.ID
		out = append(out, s)
	}
	// The router reads which client queues to consume at start; the client
	// is new.
	if err := p.call(ctx, http.MethodPost, "/router/config/reload", nil, nil); err != nil {
		return creds, nil, err
	}
	return creds, out, nil
}

func isoSeconds(d time.Duration) string { return fmt.Sprintf("PT%dS", int(d/time.Second)) }

// pgPortOf reads the port of postgresql://user@host:port.
func pgPortOf(url string) string {
	_, port, found := strings.CutLast(url, ":")
	if !found {
		return "5432"
	}
	return strings.TrimSuffix(port, "/")
}
