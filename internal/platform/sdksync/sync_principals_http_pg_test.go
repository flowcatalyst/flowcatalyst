//go:build integration

package sdksync

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	appops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/passwordhash"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// TestSyncPrincipals_Endpoint_SetsPasswordHash drives the actual sync-principals
// HANDLER end-to-end: it decodes the raw wire JSON (proving the `passwordHash`
// field lands on syncPrincipalInputRequest), runs the handler with a real
// DB-backed State + an authorised caller, and asserts the hash is persisted
// verbatim to iam_principals.password_hash and verifies at login. This answers
// "if someone POSTs passwordHash to the endpoint, do we actually store it?".
func TestSyncPrincipals_Endpoint_SetsPasswordHash(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	uow := testpg.NewUoW(t)

	// Seed the application the endpoint scopes to (resolveApp → FindByCode).
	appCode := "syncpwhttp"
	_, err := usecaseop.Run(ctx, uow, appops.CreateApplication(application.NewRepository(pool)),
		appops.CreateCommand{Code: appCode, Name: "Sync PW HTTP"}, testpg.TestEC())
	require.NoError(t, err)

	s := &State{
		Apps:       application.NewRepository(pool),
		Principals: principal.NewRepository(pool),
		UoW:        uow,
	}

	// An anchor caller with all-applications access clears CanSyncPrincipals
	// (anchor short-circuit) + requireAppAccess.
	authCtx := auth.WithContext(ctx, &auth.AuthContext{
		PrincipalID:     "p_syncpw_http",
		Scope:           auth.ScopeAnchor,
		AllApplications: true,
	})

	hash, err := bcrypt.GenerateFromPassword([]byte("from-laravel-pw"), bcrypt.DefaultCost)
	require.NoError(t, err)
	hashStr := string(hash)

	// Build the request body from RAW WIRE JSON so the test exercises huma's
	// field decoding of `passwordHash`, not a hand-set struct field.
	rawBody := `{"principals":[{"email":"sync-http@example.com","name":"Sync HTTP","roles":["admin"],"passwordHash":` +
		mustJSONString(t, hashStr) + `}]}`
	var body syncPrincipalsRequest
	require.NoError(t, json.Unmarshal([]byte(rawBody), &body))
	require.Len(t, body.Principals, 1)
	require.NotNil(t, body.Principals[0].PasswordHash, "passwordHash must decode onto the wire struct")
	require.Equal(t, hashStr, *body.Principals[0].PasswordHash)

	out, err := s.syncPrincipals(authCtx, &syncPrincipalsInput{AppCode: appCode, Body: body})
	require.NoError(t, err)
	require.Equal(t, uint32(1), out.Body.Created)

	// The handler persisted the hash verbatim to the column the login flow reads.
	var stored *string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT password_hash FROM iam_principals WHERE email = $1`,
		"sync-http@example.com").Scan(&stored))
	require.NotNil(t, stored, "password_hash must be set from the endpoint payload")
	assert.Equal(t, hashStr, *stored, "stored verbatim — not re-hashed")
	assert.NoError(t, passwordhash.Verify("from-laravel-pw", *stored),
		"the imported password verifies at login")
}

func mustJSONString(t *testing.T, s string) string {
	t.Helper()
	b, err := json.Marshal(s)
	require.NoError(t, err)
	return string(b)
}
