// Throwaway diagnostic: verify FLOWCATALYST_APP_KEY can decrypt a
// ciphertext (e.g. oauth_identity_providers.oidc_client_secret_ref).
//
// Usage:
//
//	FLOWCATALYST_APP_KEY='<key>' go run ./cmd/decrypt-check '<ciphertext>'
//	FLOWCATALYST_APP_KEY='<key>' go run ./cmd/decrypt-check -show '<ciphertext>'
//
// By default prints only the plaintext length. Pass -show to print the
// decrypted plaintext itself (it will land in your shell scrollback/history).
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
)

func main() {
	show := flag.Bool("show", false, "print the decrypted plaintext (not just its length)")
	flag.Parse()

	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: FLOWCATALYST_APP_KEY=... go run ./cmd/decrypt-check [-show] '<ciphertext>'")
		os.Exit(2)
	}
	ciphertext := flag.Arg(0)

	raw := os.Getenv("FLOWCATALYST_APP_KEY")
	fmt.Printf("key: %d chars (raw), %d chars (trimmed)\n", len(raw), len(trimmed(raw)))
	if raw != trimmed(raw) {
		fmt.Println("WARNING: key has leading/trailing whitespace — FromEnv does NOT trim it")
	}

	svc, err := encryption.FromEnv()
	if err != nil {
		fmt.Printf("FromEnv error (this is silently discarded in wire.go!): %v\n", err)
		os.Exit(1)
	}
	if svc == nil {
		fmt.Println("FLOWCATALYST_APP_KEY is not set")
		os.Exit(1)
	}

	pt, err := svc.Decrypt(ciphertext)
	if err != nil {
		fmt.Printf("DECRYPT FAILED: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("DECRYPT OK: plaintext is %d chars\n", len(pt))
	if *show {
		fmt.Printf("plaintext: %s\n", pt)
	}
}

func trimmed(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r' || s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	for len(s) > 0 && (s[0] == '\n' || s[0] == '\r' || s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	return s
}
