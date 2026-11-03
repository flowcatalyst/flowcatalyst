//go:build integration

package fieldauth

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/authservice"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
	"github.com/flowcatalyst/go-sdk/auth"
)

// TestFieldJSE2EServer is not a test: it is the server clients/field-js's
// end-to-end suite (pnpm test:e2e) spawns. It is skipped unless
// FC_FIELDJS_E2E=1, so a normal `go test -tags=integration` ignores it.
//
// It boots the real fieldauth routes on the embedded Postgres (testpg) over a
// real TCP listener (so the proof htu is a real http://127.0.0.1:port URL),
// seeds a client, application, worker with a PIN and an enrolment code, and
// starts a tiny application backend that verifies requests with the SDK's
// FieldTokenVerifier. It prints one line, "FCFIELD_E2E {json}", and serves
// until its stdin closes (the JS suite closes it, then the test returns and
// testpg tears Postgres down cleanly). It reads no environment but the gate.
func TestFieldJSE2EServer(t *testing.T) {
	if os.Getenv("FC_FIELDJS_E2E") != "1" {
		t.Skip("set FC_FIELDJS_E2E=1 (clients/field-js `pnpm test:e2e` does)")
	}
	platLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	appLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	platURL := "http://" + platLn.Addr().String()
	appURL := "http://" + appLn.Addr().String()

	e := newEnv(t, envOpts{})
	// The tokens' issuer and the proof htu must be the real listener URL.
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
	svc, err := authservice.NewWithRSA(authservice.Config{Issuer: platURL, Audience: "flowcatalyst"}, string(privPEM), string(pubPEM))
	if err != nil {
		t.Fatal(err)
	}
	e.s.Auth, e.s.BaseURL = svc, platURL

	workerCode := fmt.Sprintf("w-e2e-%d", time.Now().UnixNano()%1000000)
	const workerPIN, newPIN = "482915", "739162"
	e.worker(workerCode, workerPIN, false, e.codeA)
	enrolCode := e.enrolCode(fieldidentity.ModePersonal, "", e.codeA)

	// Platform: the field routes plus the JWKS the application verifies with.
	pmux := http.NewServeMux()
	pmux.Handle("/field/", e.router)
	pmux.HandleFunc("/jwks.json", func(w http.ResponseWriter, _ *http.Request) {
		var keys []map[string]string
		for _, k := range svc.AllJWKSKeys() {
			keys = append(keys, map[string]string{"kty": "RSA", "use": "sig", "alg": "RS256", "kid": k.KeyID, "n": k.Components.N, "e": k.Components.E})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": keys})
	})

	// Application backend.
	verifier := auth.NewFieldTokenVerifier(auth.FieldTokenVerifierConfig{
		Issuer: platURL, AppCode: e.codeA, JWKSURL: platURL + "/jwks.json", BaseURL: appURL,
	})
	amux := http.NewServeMux()
	amux.Handle("/app/whoami", auth.FieldMiddleware(verifier)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, _ := auth.FieldPrincipalFrom(r.Context())
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"workerCode": p.WorkerCode, "appCode": p.AppCode, "deviceId": p.DeviceID, "jkt": p.JKT})
	})))

	psrv := &http.Server{Handler: pmux, ReadHeaderTimeout: 10 * time.Second}
	asrv := &http.Server{Handler: amux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = psrv.Serve(platLn) }()
	go func() { _ = asrv.Serve(appLn) }()
	t.Cleanup(func() { _ = psrv.Close(); _ = asrv.Close() })

	line, _ := json.Marshal(map[string]string{
		"baseUrl": platURL, "appUrl": appURL, "code": enrolCode, "workerCode": workerCode,
		"pin": workerPIN, "newPin": newPIN, "appCode": e.codeA,
	})
	fmt.Fprintf(os.Stdout, "FCFIELD_E2E %s\n", line)

	// Serve until the parent closes our stdin.
	_, _ = io.Copy(io.Discard, os.Stdin)
}
