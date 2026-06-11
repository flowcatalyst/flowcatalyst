package validate

import (
	"errors"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

func assertValidation(t *testing.T, err error, code, message string) {
	t.Helper()
	var ue *usecase.Error
	if !errors.As(err, &ue) {
		t.Fatalf("err = %v, want *usecase.Error", err)
	}
	if ue.Code != code || ue.Message != message {
		t.Errorf("err = (%q, %q), want (%q, %q)", ue.Code, ue.Message, code, message)
	}
}

func TestRequired(t *testing.T) {
	t.Parallel()

	if err := Required("value", "CODE_REQUIRED", "code is required"); err != nil {
		t.Errorf("Required(non-blank) = %v, want nil", err)
	}
	for _, blank := range []string{"", "   ", "\t\n"} {
		assertValidation(t, Required(blank, "CODE_REQUIRED", "code is required"),
			"CODE_REQUIRED", "code is required")
	}
}

func TestMatch(t *testing.T) {
	t.Parallel()

	if err := Match(CodePattern, "my-pool", "INVALID_CODE_FORMAT", "bad code"); err != nil {
		t.Errorf("Match(valid) = %v, want nil", err)
	}
	assertValidation(t, Match(CodePattern, "My-Pool", "INVALID_CODE_FORMAT", "bad code"),
		"INVALID_CODE_FORMAT", "bad code")
}

func TestCodePatterns(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in                 string
		strict, underscore bool
	}{
		{"a", true, true},
		{"my-pool", true, true},
		{"pool2", true, true},
		{"my_pool", false, true}, // the union case: sync/Rust accept, strict create used to reject
		{"logistics_portal", false, true},
		{"2pool", false, false},
		{"My-Pool", false, false},
		{"-pool", false, false},
		{"", false, false},
	}
	for _, tc := range cases {
		if got := CodePattern.MatchString(tc.in); got != tc.strict {
			t.Errorf("CodePattern(%q) = %v, want %v", tc.in, got, tc.strict)
		}
		if got := CodeUnderscorePattern.MatchString(tc.in); got != tc.underscore {
			t.Errorf("CodeUnderscorePattern(%q) = %v, want %v", tc.in, got, tc.underscore)
		}
	}
}
