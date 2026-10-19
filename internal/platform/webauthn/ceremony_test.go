package webauthn

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	gowebauthn "github.com/go-webauthn/webauthn/webauthn"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/webauthn/webauthntest"
)

const (
	testRPID   = "localhost"
	testOrigin = "http://localhost:8080"
)

func newSvc(t *testing.T) *Service {
	t.Helper()
	s, err := NewService(Config{RPDisplayName: "FC", RPID: testRPID, RPOrigins: []string{testOrigin}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func user(id string, creds ...gowebauthn.Credential) *PrincipalUser {
	return &PrincipalUser{PrincipalID: id, DisplayName: id, Username: id + "@example.com", Credentials: creds}
}

func parseCreate(t *testing.T, body []byte) *protocol.ParsedCredentialCreationData {
	t.Helper()
	p, err := protocol.ParseCredentialCreationResponseBody(io.NopCloser(bytes.NewReader(body)))
	if err != nil {
		t.Fatalf("parse create: %v", err)
	}
	return p
}

func parseGet(t *testing.T, body []byte) *protocol.ParsedCredentialAssertionData {
	t.Helper()
	p, err := protocol.ParseCredentialRequestResponseBody(io.NopCloser(bytes.NewReader(body)))
	if err != nil {
		t.Fatalf("parse get: %v", err)
	}
	return p
}

// register runs a full registration and returns the verified credential.
func register(t *testing.T, s *Service, u *PrincipalUser, a *webauthntest.Authenticator) *gowebauthn.Credential {
	t.Helper()
	opts, sess, err := s.WebAuthn().BeginRegistration(u)
	if err != nil {
		t.Fatal(err)
	}
	cred, err := s.WebAuthn().CreateCredential(u, *sess, parseCreate(t, a.Register(opts.Response.Challenge.String(), testOrigin, testRPID)))
	if err != nil {
		t.Fatalf("registration should verify: %v", err)
	}
	return cred
}

func TestRegistration_HappyPath(t *testing.T) {
	s := newSvc(t)
	a := webauthntest.New(t)
	cred := register(t, s, user("prn_a"), a)
	if !bytes.Equal(cred.ID, a.CredID) || len(cred.PublicKey) == 0 {
		t.Fatalf("credential not captured: %+v", cred)
	}
}

func TestRegistration_WrongOriginRefused(t *testing.T) {
	s := newSvc(t)
	a := webauthntest.New(t)
	u := user("prn_a")
	opts, sess, _ := s.WebAuthn().BeginRegistration(u)
	_, err := s.WebAuthn().CreateCredential(u, *sess, parseCreate(t,
		a.Register(opts.Response.Challenge.String(), "https://evil.example", testRPID)))
	if err == nil {
		t.Fatal("a ceremony performed on another origin must be refused")
	}
}

func TestRegistration_WrongRPIDRefused(t *testing.T) {
	s := newSvc(t)
	a := webauthntest.New(t)
	u := user("prn_a")
	opts, sess, _ := s.WebAuthn().BeginRegistration(u)
	_, err := s.WebAuthn().CreateCredential(u, *sess, parseCreate(t,
		a.Register(opts.Response.Challenge.String(), testOrigin, "evil.example")))
	if err == nil {
		t.Fatal("a credential scoped to another RP ID must be refused")
	}
}

func TestRegistration_ChallengeBoundToItsCeremony(t *testing.T) {
	s := newSvc(t)
	a := webauthntest.New(t)
	u := user("prn_a")
	opts1, _, _ := s.WebAuthn().BeginRegistration(u)
	_, sess2, _ := s.WebAuthn().BeginRegistration(u)
	// A response to ceremony 1's challenge cannot finish ceremony 2.
	_, err := s.WebAuthn().CreateCredential(u, *sess2, parseCreate(t,
		a.Register(opts1.Response.Challenge.String(), testOrigin, testRPID)))
	if err == nil {
		t.Fatal("a response must not complete a different ceremony's challenge")
	}
}

func TestRegistration_ExpiredSessionRefused(t *testing.T) {
	s := newSvc(t)
	a := webauthntest.New(t)
	u := user("prn_a")
	opts, sess, _ := s.WebAuthn().BeginRegistration(u)
	sess.Expires = time.Now().Add(-time.Minute)
	if _, err := s.WebAuthn().CreateCredential(u, *sess, parseCreate(t,
		a.Register(opts.Response.Challenge.String(), testOrigin, testRPID))); err == nil {
		t.Fatal("an expired ceremony session must be refused")
	}
}

func assertOnce(t *testing.T, s *Service, u *PrincipalUser, a *webauthntest.Authenticator, mutate func(challenge string) []byte) (*gowebauthn.Credential, error) {
	t.Helper()
	_, sess, err := s.WebAuthn().BeginLogin(u)
	if err != nil {
		t.Fatal(err)
	}
	return s.WebAuthn().ValidateLogin(u, *sess, parseGet(t, mutate(sess.Challenge)))
}

func TestAuthentication_HappyPathAdvancesCounter(t *testing.T) {
	s := newSvc(t)
	a := webauthntest.New(t)
	reg := register(t, s, user("prn_a"), a)
	u := user("prn_a", *reg)

	cred, err := assertOnce(t, s, u, a, func(ch string) []byte { return a.Assert(ch, testOrigin, testRPID, []byte("prn_a")) })
	if err != nil {
		t.Fatalf("assertion should verify: %v", err)
	}
	if cred.Authenticator.SignCount != 1 || cred.Authenticator.CloneWarning {
		t.Fatalf("counter=%d clone=%v", cred.Authenticator.SignCount, cred.Authenticator.CloneWarning)
	}
}

func TestAuthentication_CounterRegressionRaisesCloneWarning(t *testing.T) {
	s := newSvc(t)
	a := webauthntest.New(t)
	reg := register(t, s, user("prn_a"), a)
	u := user("prn_a", *reg)

	c1, err := assertOnce(t, s, u, a, func(ch string) []byte { return a.AssertWithCounter(ch, testOrigin, testRPID, []byte("prn_a"), 10) })
	if err != nil || c1.Authenticator.CloneWarning {
		t.Fatalf("first assertion: err=%v clone=%v", err, c1.Authenticator.CloneWarning)
	}
	// The persisted credential now holds counter 10; a replayed lower value
	// is the clone signal. The library reports it as a flag, not an error,
	// which is why the API handler must check it.
	u2 := user("prn_a", *c1)
	c2, err := assertOnce(t, s, u2, a, func(ch string) []byte { return a.AssertWithCounter(ch, testOrigin, testRPID, []byte("prn_a"), 4) })
	if err != nil {
		t.Fatalf("library should not error on regression: %v", err)
	}
	if !c2.Authenticator.CloneWarning {
		t.Fatal("counter regression must set CloneWarning")
	}
}

func TestAuthentication_WrongOriginAndRPIDRefused(t *testing.T) {
	s := newSvc(t)
	a := webauthntest.New(t)
	reg := register(t, s, user("prn_a"), a)
	u := user("prn_a", *reg)
	for name, f := range map[string]func(string) []byte{
		"origin": func(ch string) []byte { return a.Assert(ch, "https://evil.example", testRPID, []byte("prn_a")) },
		"rpid":   func(ch string) []byte { return a.Assert(ch, testOrigin, "evil.example", []byte("prn_a")) },
	} {
		if _, err := assertOnce(t, s, u, a, f); err == nil {
			t.Errorf("%s: assertion for the wrong %s must be refused", name, name)
		}
	}
}

func TestAuthentication_ChallengeReuseAndForeignChallengeRefused(t *testing.T) {
	s := newSvc(t)
	a := webauthntest.New(t)
	reg := register(t, s, user("prn_a"), a)
	u := user("prn_a", *reg)

	_, sess1, _ := s.WebAuthn().BeginLogin(u)
	_, sess2, _ := s.WebAuthn().BeginLogin(u)
	body := a.Assert(sess1.Challenge, testOrigin, testRPID, []byte("prn_a"))
	if _, err := s.WebAuthn().ValidateLogin(u, *sess2, parseGet(t, body)); err == nil {
		t.Fatal("an assertion over ceremony 1's challenge must not satisfy ceremony 2")
	}
}

func TestAuthentication_CredentialOwnership(t *testing.T) {
	s := newSvc(t)
	alice, mallory := webauthntest.New(t), webauthntest.New(t)
	aliceReg := register(t, s, user("prn_alice"), alice)
	malloryReg := register(t, s, user("prn_mallory"), mallory)

	// Mallory's key signs, but the ceremony is for alice (whose credential
	// list does not contain it): refused.
	uAlice := user("prn_alice", *aliceReg)
	if _, err := assertOnce(t, s, uAlice, mallory, func(ch string) []byte {
		return mallory.Assert(ch, testOrigin, testRPID, []byte("prn_alice"))
	}); err == nil {
		t.Fatal("another principal's credential must not authenticate alice")
	}
	// A user handle that names someone else is refused too.
	uMallory := user("prn_mallory", *malloryReg)
	if _, err := assertOnce(t, s, uMallory, mallory, func(ch string) []byte {
		return mallory.Assert(ch, testOrigin, testRPID, []byte("prn_alice"))
	}); err == nil {
		t.Fatal("a mismatched user handle must be refused")
	}
}

func TestAuthentication_ExpiredSessionRefused(t *testing.T) {
	s := newSvc(t)
	a := webauthntest.New(t)
	reg := register(t, s, user("prn_a"), a)
	u := user("prn_a", *reg)
	_, sess, _ := s.WebAuthn().BeginLogin(u)
	sess.Expires = time.Now().Add(-time.Minute)
	if _, err := s.WebAuthn().ValidateLogin(u, *sess, parseGet(t, a.Assert(sess.Challenge, testOrigin, testRPID, []byte("prn_a")))); err == nil {
		t.Fatal("an expired ceremony session must be refused")
	}
}

// A client that volunteers an extension output nobody requested must not fail
// the ceremony (go-webauthn v0.18 rejects by default; we pin the v0.17
// behaviour so real browsers and password managers keep working).
func TestRegistration_UnsolicitedExtensionOutputTolerated(t *testing.T) {
	s := newSvc(t)
	a := webauthntest.New(t)
	u := user("prn_a")
	opts, sess, _ := s.WebAuthn().BeginRegistration(u)
	body := a.Register(opts.Response.Challenge.String(), testOrigin, testRPID)
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatal(err)
	}
	m["clientExtensionResults"] = map[string]any{"credProps": map[string]any{"rk": true}}
	body, _ = json.Marshal(m)
	if _, err := s.WebAuthn().CreateCredential(u, *sess, parseCreate(t, body)); err != nil {
		t.Fatalf("unsolicited extension output must be tolerated: %v", err)
	}
}
