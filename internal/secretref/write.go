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
//
// owner is the application and client the value belongs to (see Owner); a
// scoped allow-list entry admits the reference only at the owner's own path.
func CheckWrite(v string, owner Owner) error {
	return Default().CheckWrite(v, owner)
}

// PrecheckWrite is CheckWrite for a caller that does not yet know the owner
// (an operation's Validate, before the row is loaded): the unsupported-scheme
// refusal is final, and the allow-list refuses only a reference that no
// entry could admit for any owner. The owner-exact check is CheckWrite, which
// the caller still runs once it knows the row.
func PrecheckWrite(v string) error { return Default().PrecheckWrite(v) }

// CheckWrite is the package-level CheckWrite against s's allow-list.
func (s *Service) CheckWrite(v string, owner Owner) error {
	return s.checkWrite(v, func(ref string) *Error { return s.allowed(ref, owner) })
}

// PrecheckWrite is the package-level PrecheckWrite against s's allow-list.
func (s *Service) PrecheckWrite(v string) error {
	return s.checkWrite(v, func(ref string) *Error {
		return s.allowedBy(ref, func(entry string) bool { return matchEntry(entry, ref, Owner{}, true) })
	})
}

func (s *Service) checkWrite(v string, allowed func(ref string) *Error) error {
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
	if e := allowed(v); e != nil {
		return usecase.Validation(CodeNotAllowed, e.Msg)
	}
	return nil
}
