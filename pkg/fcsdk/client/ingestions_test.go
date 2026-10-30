package client_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/go-sdk/client"
)

func ingestionSource() client.IngestionSource {
	return client.HTTPSource{Auth: client.HTTPBearerAuth{Token: client.SecretRef{Name: "orders-token"}}}
}

func ingestionTarget() client.IngestionTarget {
	return client.IngestionTarget{
		Code: "orders.received", TargetURL: "https://app.example.com/hook", Pool: "default",
		Mode: client.TargetModeNextOnError, BatchSize: 10,
	}
}

func TestIngestionsListSendsQueryOnlyWhenGiven(t *testing.T) {
	srv, seen := newMockSrv(t, `{"ingestions":[],"hasMore":false}`)
	c := client.New(srv.URL)

	_, err := c.Ingestions().List(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, "/api/ingestions", seen.path)
	assert.Empty(t, seen.query)

	_, err = c.Ingestions().List(context.Background(), &client.ListIngestionsFilters{ClientID: "acme", Limit: new(uint32(5))})
	require.NoError(t, err)
	assert.Equal(t, "acme", seen.query.Get("clientId"))
	assert.Equal(t, "5", seen.query.Get("limit"))
	assert.Len(t, seen.query, 2)
}

func TestIngestionsListCursor(t *testing.T) {
	srv, seen := newMockSrv(t, `{"ingestions":[],"failures":[],"hasMore":true,"nextCursor":"c2"}`)
	c := client.New(srv.URL)

	page, err := c.Ingestions().List(context.Background(), nil)
	require.NoError(t, err)
	require.True(t, page.HasMore)
	assert.Equal(t, "c2", page.NextCursor)

	_, err = c.Ingestions().List(context.Background(), &client.ListIngestionsFilters{After: page.NextCursor})
	require.NoError(t, err)
	assert.Equal(t, "c2", seen.query.Get("after"))

	fails, err := c.Ingestions().Failures(context.Background(), "ing_1", client.IngestionFailureFilters{After: "f1"})
	require.NoError(t, err)
	assert.True(t, fails.HasMore)
	assert.Equal(t, "/api/ingestions/ing_1/failures", seen.path)
	assert.Equal(t, "f1", seen.query.Get("after"))
	assert.NotContains(t, seen.query, "limit")
}

func TestIngestionPathsAreEncoded(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	c := client.New(srv.URL)

	_, err := c.Ingestions().Get(context.Background(), "a/b c")
	require.NoError(t, err)
	assert.Equal(t, "/api/ingestions/a%2Fb%20c", gotPath)

	_, err = c.IngestionSets().GetApply(context.Background(), "my set/1", "ina_1")
	require.NoError(t, err)
	assert.Equal(t, "/api/ingestion-sets/my%20set%2F1/applies/ina_1", gotPath)
}

func TestIngestionSetDryRunOnlyWhenGiven(t *testing.T) {
	srv, seen := newMockSrv(t, `{"name":"repo","dryRun":false,"changes":[]}`)
	c := client.New(srv.URL)
	req := &client.ApplyIngestionSetRequest{Items: []client.IngestionSetItem{}}

	_, err := c.IngestionSets().Apply(context.Background(), "repo", req, nil)
	require.NoError(t, err)
	assert.Equal(t, http.MethodPut, seen.method)
	assert.Empty(t, seen.query)

	_, err = c.IngestionSets().Apply(context.Background(), "repo", req, new(false))
	require.NoError(t, err)
	assert.Equal(t, "false", seen.query.Get("dryRun"))

	_, err = c.IngestionSets().Reapply(context.Background(), "repo", "ina_1", nil)
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, seen.method)
	assert.Empty(t, seen.query)
}

func TestIngestionDefinitionIsYAML(t *testing.T) {
	var accept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "application/yaml")
		_, _ = w.Write([]byte("code: orders\nname: Orders\n"))
	}))
	t.Cleanup(srv.Close)

	got, err := client.New(srv.URL).Ingestions().Definition(context.Background(), "ing_1")
	require.NoError(t, err)
	assert.Equal(t, "code: orders\nname: Orders\n", got)
	assert.Equal(t, "application/yaml", accept)
}

func TestIngestionErrorStatuses(t *testing.T) {
	cases := []struct {
		status int
		body   string
		code   string
		check  func(*client.APIError) bool
	}{
		{http.StatusBadRequest, `{"error":"INGESTION_TARGET_INVALID","message":"pool not found"}`, "INGESTION_TARGET_INVALID", func(e *client.APIError) bool { return e.StatusCode == 400 }},
		{http.StatusForbidden, `{"error":"FORBIDDEN","message":"no"}`, "FORBIDDEN", (*client.APIError).IsForbidden},
		{http.StatusNotFound, `{"error":"INGESTION_NOT_FOUND","message":"gone"}`, "INGESTION_NOT_FOUND", (*client.APIError).IsNotFound},
		{http.StatusConflict, `{"error":"INGESTION_CODE_EXISTS","message":"taken"}`, "INGESTION_CODE_EXISTS", (*client.APIError).IsConflict},
	}
	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			t.Cleanup(srv.Close)

			_, err := client.New(srv.URL, client.WithRetry(1, 0)).Ingestions().Create(context.Background(), &client.CreateIngestionRequest{
				Application: "app", Client: "acme", Code: "orders", Name: "Orders",
				Source: ingestionSource(), Split: client.NoSplit{}, Target: ingestionTarget(),
			})
			var apiErr *client.APIError
			require.True(t, errors.As(err, &apiErr))
			assert.Equal(t, tc.code, apiErr.Code())
			assert.True(t, tc.check(apiErr))
		})
	}
}

// Every source and split member written and read back keeps its JSON, and a
// kind the SDK does not know survives untouched.
func TestIngestionDefinitionRoundTrip(t *testing.T) {
	sources := []string{
		`{"kind":"http","auth":{"kind":"flowcatalyst-token","application":"app"},"rateLimit":{"requests":10,"per":"PT1S"}}`,
		`{"kind":"http","auth":{"kind":"basic","username":"u","password":{"name":"p"}}}`,
		`{"kind":"sftp","host":"h","port":22,"directory":"/in","username":"u","credential":{"name":"k"},"include":"*.csv","every":"PT5M"}`,
		`{"kind":"s3","region":"eu-west-1","bucket":"b","prefix":"in/","every":"PT1M","credential":{"accessKeyId":"AK","secretAccessKey":{"name":"s"}}}`,
		`{"kind":"sqs","region":"eu-west-1","queue":"q"}`,
		`{"kind":"rabbitmq","uri":"amqps://h","queue":"q","username":"u","password":{"name":"p"}}`,
		`{"kind":"database-poll","jdbcUrl":"jdbc:x","username":"u","password":{"name":"p"},"table":"t","track":{"kind":"watermark","column":"id","type":"NUMBER"},"maxRows":100,"every":"PT30S"}`,
		`{"kind":"database-poll","jdbcUrl":"jdbc:x","username":"u","password":{"name":"p"},"table":"t","track":{"kind":"status","column":"s","keyColumn":"id","pending":"N","processed":"Y"},"maxRows":100,"every":"PT30S"}`,
		`{"kind":"api-poll","url":"https://x","method":"GET","headers":[{"name":"a","value":"b"}],"auth":{"kind":"oauth-client-credentials","tokenUrl":"https://t","clientId":"c","clientSecret":{"name":"s"},"credentialsIn":"BODY"},"results":{"kind":"json-path","path":"$.items"},"cursor":{"from":"$.next","param":"after"},"every":"PT1S","idleDelay":"PT1M"}`,
		`{"kind":"api-poll","url":"https://x","method":"POST","auth":{"kind":"none"},"results":{"kind":"records"},"every":"PT1S","idleDelay":"PT1M"}`,
		`{"kind":"custom","type":"mix","settings":{"yaml":"route","n":1}}`,
		`{"kind":"camel-route","yaml":"route"}`,
	}
	splits := []string{`{"kind":"none"}`, `{"kind":"csv","header":true}`, `{"kind":"json","arrayPath":"$.items"}`, `{"kind":"xml","element":"order"}`, `{"kind":"future","x":1}`}
	target := `{"code":"c","targetUrl":"https://x","pool":"p","connection":"conn","mode":"BLOCK_ON_ERROR","batchSize":5}`

	for _, src := range sources {
		for _, spl := range splits {
			body := `{"id":"ing_1","applicationId":"a","applicationCode":"app","clientId":"c","clientIdentifier":"acme","code":"orders","status":"ACTIVE","version":1,"latestVersion":1,"createdAt":"t","updatedAt":"t","name":"N","source":` + src + `,"split":` + spl + `,"target":` + target + `}`
			var got client.IngestionResponse
			require.NoError(t, json.Unmarshal([]byte(body), &got))
			out, err := json.Marshal(got)
			require.NoError(t, err)
			assert.JSONEq(t, body, string(out))
		}
	}
}

func TestIngestionDefinitionDecodesTypedMembers(t *testing.T) {
	var got client.IngestionVersionResponse
	require.NoError(t, json.Unmarshal([]byte(`{"id":"v","version":2,"live":true,"createdAt":"t","name":"N",
		"source":{"kind":"http","auth":{"kind":"bearer","token":{"name":"t"}}},
		"split":{"kind":"csv","header":true},
		"target":{"code":"c","targetUrl":"https://x","pool":"p","mode":"IMMEDIATE","batchSize":1}}`), &got))

	src, ok := got.Source.(client.HTTPSource)
	require.True(t, ok)
	assert.Equal(t, client.HTTPBearerAuth{Token: client.SecretRef{Name: "t"}}, src.Auth)
	assert.Equal(t, client.CSVSplit{Header: true}, got.Split)
	assert.Equal(t, client.TargetModeImmediate, got.Target.Mode)

	var odd client.IngestionVersionResponse
	require.NoError(t, json.Unmarshal([]byte(`{"source":{"kind":"mix","a":1},"split":{"kind":"none"}}`), &odd))
	assert.Equal(t, "mix", odd.Source.(client.Unrecognized).Kind())
}
