package api

import (
	"strings"
	"testing"
)

func FuzzSafeRelativeReturnURL(f *testing.F) {
	for _, s := range []string{"/ok", "//evil.com", "/\\evil.com", "/\t/evil.com", "/\n/evil.com", "/\r/evil.com", "", "x", "/\x00/x"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		out := safeRelativeReturnURL(in)
		if out == "" {
			return
		}
		norm := strings.NewReplacer("\t", "", "\r", "", "\n", "", "\\", "/").Replace(out)
		if out != in || !strings.HasPrefix(norm, "/") || strings.HasPrefix(norm, "//") {
			t.Fatalf("accepted %q (browser reads %q)", in, norm)
		}
	})
}

func FuzzResetReturnURL(f *testing.F) {
	for _, s := range []string{"/oauth/authorize?client_id=a", "/oauth/authorize?x\t", "/other", "", "//oauth/authorize?"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		out := resetReturnURL(&in)
		if out != nil && (!strings.HasPrefix(*out, "/oauth/authorize?") || strings.ContainsAny(*out, "\t\r\n")) {
			t.Fatalf("accepted %q", in)
		}
	})
}
