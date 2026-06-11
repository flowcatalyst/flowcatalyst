// Package validate holds the shared operations-layer input checks.
// Error codes and messages are caller-supplied verbatim — they are part
// of the 400 response body the SDKs see, so adopting these helpers must
// not change them.
package validate

import (
	"regexp"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// CodePattern is the strict resource-code rule (Rust code_pattern):
// a lowercase letter followed by lowercase alphanumerics and hyphens.
// Used by connection, service-account, subscription and scheduled-job
// codes.
var CodePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// CodeUnderscorePattern additionally allows underscores. It is the Rust
// pool_code_pattern and the application-code rule — real codes use
// underscores (logistics_portal, transport_order, master_data).
var CodeUnderscorePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

// Required returns usecase.Validation(code, message) when v is blank
// after trimming.
func Required(v, code, message string) error {
	if strings.TrimSpace(v) == "" {
		return usecase.Validation(code, message)
	}
	return nil
}

// Match returns usecase.Validation(code, message) when v does not
// match re.
func Match(re *regexp.Regexp, v, code, message string) error {
	if !re.MatchString(v) {
		return usecase.Validation(code, message)
	}
	return nil
}
