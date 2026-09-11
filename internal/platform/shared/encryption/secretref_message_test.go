package encryption

import (
	"errors"
	"testing"
)

// TestUnsupportedSchemeMessage pins the rejection text: the scheme is quoted
// WITH its "://" (not `"ref"://`), and the supported list names only the
// secret-manager schemes, in their "aws-sm://" form — "literal:" is the dev
// bypass, not one of them.
func TestUnsupportedSchemeMessage(t *testing.T) {
	ref := "ref://some/secret"
	_, err := EncryptSecretRef(nil, &ref)
	if !errors.Is(err, ErrUnsupportedScheme) {
		t.Fatalf("want ErrUnsupportedScheme, got %v", err)
	}
	want := `unsupported secret-manager scheme "ref://"; supported: aws-sm://, aws-ps://, gcp-sm://, vault://, env:// ` +
		`(prefix the value with "encrypt:" to store it as an encrypted plaintext secret instead)`
	if err.Error() != want {
		t.Fatalf("message:\n got %s\nwant %s", err.Error(), want)
	}
}
