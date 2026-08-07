package passwordpolicy

import (
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	email := "andrew@belac.io"
	name := "Andrew Graaff"

	cases := []struct {
		testName string
		pw       string
		wantCode string // "" = accepted
	}{
		{"good passphrase", "correct-horse-battery", ""},
		{"good with unicode", "sûrement-pas-devinable-9", ""},

		{"too short", "a1b2c3!", "PASSWORD_TOO_SHORT"},
		{"too long", strings.Repeat("x1", 65), "PASSWORD_TOO_LONG"},
		{"single repeated char", "aaaaaaaaaa", "PASSWORD_TOO_WEAK"},

		// The reported abuse: password == email, in any casing.
		{"exact email", "andrew@belac.io", "PASSWORD_CONTAINS_IDENTITY"},
		{"email different case", "Andrew@Belac.IO", "PASSWORD_CONTAINS_IDENTITY"},
		{"email embedded", "xx-andrew@belac.io-99", "PASSWORD_CONTAINS_IDENTITY"},
		{"local part embedded", "andrew2026!", "PASSWORD_CONTAINS_IDENTITY"},
		{"local part reversed", "!6202werdna", "PASSWORD_CONTAINS_IDENTITY"},
		{"name token embedded", "graaff-rules-1", "PASSWORD_CONTAINS_IDENTITY"},

		{"common password", "sunshine", "PASSWORD_TOO_COMMON"},
		{"common password cased", "Passw0rd", "PASSWORD_TOO_COMMON"},
		{"product name", "FlowCatalyst#2026", "PASSWORD_TOO_COMMON"},
	}
	for _, tc := range cases {
		t.Run(tc.testName, func(t *testing.T) {
			v := Validate(tc.pw, email, name)
			switch {
			case tc.wantCode == "" && v != nil:
				t.Fatalf("expected accept, got %s (%s)", v.Code, v.Message)
			case tc.wantCode != "" && v == nil:
				t.Fatalf("expected %s, got accept", tc.wantCode)
			case tc.wantCode != "" && v.Code != tc.wantCode:
				t.Fatalf("expected %s, got %s (%s)", tc.wantCode, v.Code, v.Message)
			}
		})
	}
}

// Short local parts must only reject on exact equality — containment of a
// 2-3 char token would forbid huge swaths of ordinary words.
func TestValidate_ShortLocalPart(t *testing.T) {
	if v := Validate("majority-vote-42", "jo@x.com", ""); v != nil {
		t.Fatalf("'jo' inside 'majority' must not reject: %s", v.Code)
	}
	if v := Validate("jo", "jo@x.com", ""); v == nil || v.Code != "PASSWORD_TOO_SHORT" {
		t.Fatalf("bare short password still fails length first, got %v", v)
	}
}

// Missing identity inputs must not panic or reject valid passwords.
func TestValidate_EmptyIdentity(t *testing.T) {
	if v := Validate("perfectly-fine-pass", "", ""); v != nil {
		t.Fatalf("unexpected reject: %s", v.Code)
	}
}

func TestCommonListLoaded(t *testing.T) {
	if len(commonPasswords()) < 9_000 {
		t.Fatalf("embedded common-password list looks truncated: %d entries", len(commonPasswords()))
	}
}
