package authservice

import (
	"strings"
	"testing"
)

// FuzzExtractBearerToken: only a "Bearer "-prefixed header yields a token, and
// the token is exactly the remainder.
func FuzzExtractBearerToken(f *testing.F) {
	for _, s := range []string{"", "Bearer ", "Bearer abc", "bearer abc", "Basic abc", "Bearer  two", "Bearer\tabc", "Bearer a.b.c"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, h string) {
		tok := ExtractBearerToken(h)
		if tok == "" {
			return
		}
		if !strings.HasPrefix(h, "Bearer ") || h != "Bearer "+tok {
			t.Fatalf("header %q yielded %q", h, tok)
		}
	})
}
