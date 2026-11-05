//go:build integration

package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/go-sdk/tsid"
)

// A dispatch job created through POST /api/dispatch-jobs/batch with a
// connectionId is signed by that connection's service account, end to end:
// through the real platform router and bearer auth, into msg_dispatch_jobs,
// and through the delivery-credentials resolver the dispatcher uses. Without a
// connectionId the same job falls back to the application's own account,
// which is what every such job got before the batch carried connectionId.
func TestDispatchJobBatch_ConnectionIDSignsWithTheConnectionsAccount(t *testing.T) {
	srv, pool, bearer := goSDKPlatform(t)
	ctx := context.Background()
	sfx := strings.ToLower(tsid.GenerateUntyped())
	app := "djsign" + sfx[:6]
	clientID := goSDKApp(t, pool, app, "djsign-"+sfx)

	// The application's own account signs with app-secret; the connection's
	// with conn-secret (plaintext refs: the legacy form the reader accepts).
	_, err := pool.Exec(ctx, `UPDATE iam_service_accounts SET active = TRUE, wh_auth_type = 'BEARER_TOKEN', wh_signing_secret_ref = 'app-secret',
		application_id = (SELECT id FROM app_applications WHERE code = $1)
		WHERE id = (SELECT p.service_account_id FROM app_applications a JOIN iam_principals p ON p.id = a.service_account_id WHERE a.code = $1)`, app)
	require.NoError(t, err)
	connSA := "sac_" + sfx[:13]
	_, err = pool.Exec(ctx, `INSERT INTO iam_service_accounts (id, code, name, active, wh_auth_type, wh_signing_secret_ref)
		VALUES ($1, $2, $2, TRUE, 'BEARER_TOKEN', 'conn-secret')`, connSA, "connsa-"+sfx)
	require.NoError(t, err)
	connID := "con_" + sfx[:13]
	_, err = pool.Exec(ctx, `INSERT INTO msg_connections (id, code, name, status, service_account_id, application_code, client_id)
		VALUES ($1, $2, $2, 'ACTIVE', $3, $4, $5)`, connID, "hooks-"+sfx, connSA, app, clientID)
	require.NoError(t, err)

	item := func(extra string) string {
		return `{"code":"` + app + `:billing:invoice:sync","targetUrl":"https://integral.example.com/hook","payload":"{}","clientId":"` + clientID + `"` + extra + `}`
	}
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/dispatch-jobs/batch",
		strings.NewReader(`{"items":[`+item(`,"connectionId":"`+connID+`"`)+`,`+item("")+`]}`))
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

	repos := buildRepos(pool)
	resolve := dispatchDeliveryCredsResolver(repos)
	signedBy := func(id string) (string, string) {
		t.Helper()
		job, err := repos.dispatchJobRepo.FindByID(ctx, id)
		require.NoError(t, err)
		require.NotNil(t, job)
		creds, err := resolve(ctx, job)
		require.NoError(t, err)
		require.Empty(t, creds.Reason)
		return creds.SignedBy, creds.SigningSecret
	}

	by, secret := signedBy(br.Results[0].ID)
	assert.Equal(t, "connsa-"+sfx, by, "the job's connection's account signs")
	assert.Equal(t, "conn-secret", secret)

	by, secret = signedBy(br.Results[1].ID)
	assert.NotEqual(t, "connsa-"+sfx, by, "no connectionId: the application's account")
	assert.Equal(t, "app-secret", secret)
}
