package secretref

import (
	"strings"

	"github.com/flowcatalyst/go-sdk/usecase"
)

// unresolvableSchemes are the external schemes encryption.EncryptSecretRef
// accepts on the write side that nothing resolves in this build. "env://"
// especially must never resolve: it would hand the platform's own environment
// to a caller.
var unresolvableSchemes = []string{"gcp-sm://", "vault://", "env://"}

// CheckWrite is the write-side check of a secret value on the paths that hand
// secrets out (function SECRET and DB settings, config SECRETs, ingestion
// secrets): a value that is not an external reference passes; a reference to a
// scheme nothing resolves is refused 400 SECRET_REFERENCE_UNSUPPORTED naming
// the two that work; an aws-sm:// or aws-ps:// reference must pass the
// allow-list, else 400 SECRET_REFERENCE_NOT_ALLOWED. The messages name the
// scheme or the reference, never a value. "literal:" and "encrypted:" values
// keep their meaning.
func CheckWrite(v string) error {
	return Default().CheckWrite(v)
}

// CheckWrite is the package-level CheckWrite against s's allow-list.
func (s *Service) CheckWrite(v string) error {
	v = strings.TrimSpace(v)
	for _, scheme := range unresolvableSchemes {
		if strings.HasPrefix(v, scheme) {
			return usecase.Validation(CodeUnsupported,
				"secret references of the scheme "+scheme+" are not supported; use "+SchemeSecretsManager+" or "+SchemeParameterStore)
		}
	}
	if !IsReference(v) {
		return nil
	}
	if e := s.allowed(v); e != nil {
		return usecase.Validation(CodeNotAllowed, e.Msg)
	}
	return nil
}
