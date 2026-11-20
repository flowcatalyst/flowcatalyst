//go:build integration

package server

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/netguard"
	dispatchprocessing "github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob/processing"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduler"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/payloadstore"
	"github.com/flowcatalyst/go-sdk/tsid"
)

// A dispatch job of the new shape (FC_PAYLOAD_TABLE_WRITES on, a file payload
// store; docs/payload-offload-design.md, sections 3.2 and 7), created through
// POST /api/dispatch-jobs/batch with a connectionId, is delivered signed by
// that connection's service account, and the signature the subscriber
// verifies is computed over the exact bytes it receives, which are the
// producer's bytes: a payload held in the database and one read back from the
// payload store alike. Through the real platform router, the delivery
// callback and the server's delivery-credentials resolver.
func TestDispatchJobBatch_NewShapeIsSignedOverTheBytesSent(t *testing.T) {
	prev := netguard.Default.AllowLoopback
	netguard.Default.AllowLoopback = true
	t.Cleanup(func() { netguard.Default.AllowLoopback = prev })

	payloadCfg := payloadstore.DefaultConfig()
	payloadCfg.Store = "file://" + t.TempDir()
	payloadCfg.OffloadMinBytes = payloadstore.MinOffloadMinBytes
	payloadCfg.TableWrites = true
	srv, pool, bearer := goSDKPlatformWith(t, func(c *EnvCfg) { c.Payload = payloadCfg })
	ctx := context.Background()
	sfx := strings.ToLower(tsid.GenerateUntyped())
	app := "djpsig" + sfx[:6]
	clientID := goSDKApp(t, pool, app, "djpsig-"+sfx)

	const secret = "conn-payload-secret"
	connSA := "sac_" + sfx[:13]
	_, err := pool.Exec(ctx, `INSERT INTO iam_service_accounts (id, code, name, active, wh_auth_type, wh_signing_secret_ref)
		VALUES ($1, $2, $2, TRUE, 'BEARER_TOKEN', $3)`, connSA, "connsa-"+sfx, secret)
	require.NoError(t, err)
	connID := "con_" + sfx[:13]
	_, err = pool.Exec(ctx, `INSERT INTO msg_connections (id, code, name, status, service_account_id, application_code, client_id)
		VALUES ($1, $2, $2, 'ACTIVE', $3, $4, $5)`, connID, "hooks-"+sfx, connSA, app, clientID)
	require.NoError(t, err)

	// Bytes JSONB would change (spacing, key order, a repeated key), small
	// enough to stay in the database and large enough to be offloaded.
	inline := `{"z": 1,"a":  [3, 1] , "z":2}`
	offloaded := `{"z": 1, "pad":"` + strings.Repeat("s", 3*payloadstore.MinOffloadMinBytes) + `" , "z":2}`
	type item struct {
		Code         string `json:"code"`
		TargetURL    string `json:"targetUrl"`
		Payload      string `json:"payload"`
		ClientID     string `json:"clientId"`
		ConnectionID string `json:"connectionId"`
		DataOnly     bool   `json:"dataOnly"`
	}
	mk := func(p string) item {
		return item{
			Code: app + ":billing:invoice:sync", TargetURL: "https://integral.example.com/hook",
			Payload: p, ClientID: clientID, ConnectionID: connID, DataOnly: true,
		}
	}
	reqBody, err := json.Marshal(map[string]any{"items": []item{mk(inline), mk(offloaded)}})
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/dispatch-jobs/batch", bytes.NewReader(reqBody))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
	var br struct {
		Results []struct{ ID, Status string } `json:"results"`
	}
	require.NoError(t, json.Unmarshal(body, &br))
	require.Len(t, br.Results, 2)

	// The subscriber records what it receives.
	type received struct{ body, sig, ts string }
	var mu sync.Mutex
	var got []received
	sub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = append(got, received{body: string(b), sig: r.Header.Get("X-FlowCatalyst-Signature"), ts: r.Header.Get("X-FlowCatalyst-Timestamp")})
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(sub.Close)

	// The delivery callback as the server wires it: the job repository with
	// the payload service over the same store, and the real resolver.
	payloads, err := payloadstore.New(ctx, payloadCfg, pool)
	require.NoError(t, err)
	repos := buildRepos(pool)
	repos.dispatchJobRepo.UsePayloads(payloads)
	auth := scheduler.NewDispatchAuthService("payload-signing-dispatch-secret")
	r := chi.NewRouter()
	dispatchprocessing.New(repos.dispatchJobRepo, auth).
		WithDeliveryCredsResolver(dispatchDeliveryCredsResolver(repos)).
		Mount(r)
	proc := httptest.NewServer(r)
	t.Cleanup(proc.Close)

	for i, sent := range []string{inline, offloaded} {
		id := br.Results[i].ID
		var payloadCol, ref *string
		var gotConn *string
		require.NoError(t, pool.QueryRow(ctx,
			`SELECT j.payload, j.connection_id, p.ref FROM msg_dispatch_jobs j
			   JOIN msg_payloads p ON p.id = j.payload_id AND p.created_at = j.created_at
			  WHERE j.id = $1`, id).Scan(&payloadCol, &gotConn, &ref))
		assert.Nil(t, payloadCol, "a job of the new shape")
		require.NotNil(t, gotConn)
		assert.Equal(t, connID, *gotConn, "the connection is kept")
		assert.Equal(t, sent == offloaded, ref != nil, "offloaded only above the threshold")

		// Deliver to the loopback subscriber, connection and all.
		_, err := pool.Exec(ctx, `UPDATE msg_dispatch_jobs SET target_url = $2, status = 'QUEUED' WHERE id = $1`, id, sub.URL)
		require.NoError(t, err)
		msg, _ := json.Marshal(map[string]string{"messageId": id})
		preq, err := http.NewRequest(http.MethodPost, proc.URL+"/api/dispatch/process", bytes.NewReader(msg))
		require.NoError(t, err)
		preq.Header.Set("Content-Type", "application/json")
		preq.Header.Set("Authorization", "Bearer "+auth.Sign(id))
		presp, err := http.DefaultClient.Do(preq)
		require.NoError(t, err)
		praw, _ := io.ReadAll(presp.Body)
		presp.Body.Close()
		require.Equal(t, http.StatusOK, presp.StatusCode, string(praw))

		mu.Lock()
		require.Len(t, got, i+1, "one delivery per job")
		rec := got[i]
		mu.Unlock()
		assert.Equal(t, sent, rec.body, "the producer's bytes, byte for byte")
		require.NotEmpty(t, rec.sig, "signed: the connection's account has a signing secret")
		_, terr := time.Parse("2006-01-02T15:04:05.000Z", rec.ts)
		require.NoError(t, terr)
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(rec.ts))
		mac.Write([]byte(rec.body))
		assert.Equal(t, hex.EncodeToString(mac.Sum(nil)), rec.sig,
			"the signature verifies over the exact bytes received, with the connection's secret")
	}
}
