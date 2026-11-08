package fieldidentity

import (
	"strings"
	"testing"
)

func TestValidPhone(t *testing.T) {
	ok := []string{"", "  ", "+27 82 123 4567", "082-123-4567", "(021) 555 0100", "+1.415.555.2671", "1234567", "123456789012345"}
	for _, s := range ok {
		if !validPhone(s) {
			t.Errorf("validPhone(%q) = false, want true", s)
		}
	}
	bad := []string{"12345", "+1234567890123456", "082 123 4567 ext", "++27821234567", "27+821234567", strings.Repeat("1 ", 16) + "1", "123456"}
	for _, s := range bad {
		if validPhone(s) {
			t.Errorf("validPhone(%q) = true, want false", s)
		}
	}
}
