package bridge

import (
	"strings"
	"testing"
)

// FuzzSafeRelativeReturnURL: whatever the input, an accepted value must stay
// on this site after a browser's URL normalisation (which drops ASCII tab,
// CR and LF anywhere in the string and treats "\" as "/").
func FuzzSafeRelativeReturnURL(f *testing.F) {
	for _, s := range []string{
		"/ok", "/oauth/authorize?client_id=a&state=b", "//evil.com", "/\\evil.com", "/\t/evil.com",
		"/\n/evil.com", "/\r/evil.com", "/%2f/evil.com", "/./", "", "x", "/ ", "/\x00/evil.com", "/\\\\x",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		out := safeRelativeReturnURL(in)
		if out == "" {
			return
		}
		if out != in {
			t.Fatalf("must return the input unchanged or empty: %q -> %q", in, out)
		}
		norm := strings.NewReplacer("\t", "", "\r", "", "\n", "", "\\", "/").Replace(out)
		if !strings.HasPrefix(norm, "/") || strings.HasPrefix(norm, "//") {
			t.Fatalf("accepted %q which a browser reads as %q (off-site)", in, norm)
		}
		for _, r := range out {
			if r < 0x20 || r == 0x7f {
				t.Fatalf("accepted %q with a control character", in)
			}
		}
	})
}

// FuzzExtractAudFromIDTokenHint must never panic on hostile hints.
func FuzzExtractAudFromIDTokenHint(f *testing.F) {
	for _, s := range []string{
		"", "a", "a.b", "a.b.c", "x.eyJhdWQiOiJjbGllbnQifQ.y", "x.eyJhdWQiOlsiYSIsImIiXX0.y",
		"x.eyJhdWQiOltdfQ.y", "x.bm90anNvbg.y", "x.e30.y", "x.!!!.y",
	} {
		f.Add(s)
	}
	f.Fuzz(func(_ *testing.T, in string) { _ = extractAudFromIDTokenHint(in) })
}
