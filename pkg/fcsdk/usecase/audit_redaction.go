package usecase

import (
	"bytes"
	"encoding/json"
	"strings"
)

// AuditMasked is implemented by a command whose audit document must hide a
// field the name rule would keep (a SECRET platform-config value, say). The
// fields are top-level JSON keys of the serialised command.
type AuditMasked interface {
	AuditMaskedFields() []string
}

const auditMask = "***"

var (
	auditSecretSuffixes = []string{"password", "passwordhash", "secret", "secretref", "passphrase", "token"}
	auditSecretExact    = map[string]bool{"apikey": true, "privatekey": true, "authorization": true, "cookie": true}
)

// RedactAuditJSON applies the audit redaction rule (the Java platform's
// docs/spec/audit-redaction.md; the shared cases are
// testdata/audit-redaction-vectors.json) to a serialised command: a key is
// secret when, lower-cased with '_' and '-' removed, it ends with password,
// passwordhash, secret, secretref, passphrase or token, or equals apikey,
// privatekey, authorization or cookie; a secret key's value becomes "***"
// whatever its type, except null and booleans, which are kept. Objects and
// arrays are walked; masked names top-level fields masked the same way.
// Input that is not a JSON object or array is returned unchanged.
func RedactAuditJSON(raw []byte, masked []string) ([]byte, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || (trimmed[0] != '{' && trimmed[0] != '[') {
		return raw, nil
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber() // numbers survive exactly, never through float64
	var doc any
	if err := dec.Decode(&doc); err != nil {
		return nil, err
	}
	maskSet := make(map[string]bool, len(masked))
	for _, m := range masked {
		maskSet[m] = true
	}
	return json.Marshal(redactAuditValue(doc, maskSet, true))
}

// RedactedAuditCommand marshals a command for an audit row with the rule
// applied, including the command's own AuditMaskedFields when it has them.
func RedactedAuditCommand(command any) ([]byte, error) {
	raw, err := json.Marshal(command)
	if err != nil {
		return nil, err
	}
	var masked []string
	if m, ok := command.(AuditMasked); ok {
		masked = m.AuditMaskedFields()
	}
	return RedactAuditJSON(raw, masked)
}

func redactAuditValue(v any, masked map[string]bool, topLevel bool) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			if isAuditSecretKey(k) || (topLevel && masked[k]) {
				out[k] = maskAuditValue(val)
			} else {
				out[k] = redactAuditValue(val, masked, false)
			}
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, el := range t {
			out[i] = redactAuditValue(el, masked, false)
		}
		return out
	default:
		return v
	}
}

func maskAuditValue(v any) any {
	switch v.(type) {
	case nil, bool:
		return v
	default:
		return auditMask
	}
}

func isAuditSecretKey(key string) bool {
	n := strings.NewReplacer("_", "", "-", "").Replace(strings.ToLower(key))
	if auditSecretExact[n] {
		return true
	}
	for _, s := range auditSecretSuffixes {
		if strings.HasSuffix(n, s) {
			return true
		}
	}
	return false
}
