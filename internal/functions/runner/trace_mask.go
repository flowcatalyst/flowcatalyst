package runner

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
)

// minMaskedSecretLen is the shortest secret value masked in a trace record;
// shorter values would mask unrelated text ("true", "0001").
const minMaskedSecretLen = 4

// maskValues returns the values of the secrets this version DECLARES and the
// runner has resolved for it (the same set the secret_get op can hand the
// guest), longest first so a value that contains another is masked whole.
// nil when there are none, which makes every mask method below a no-op that
// returns its input unchanged.
func (i *invTrace) maskValues() []string {
	if i == nil || i.fn == nil || i.ver == nil {
		return nil
	}
	snap := i.fn.snapshot()
	if snap == nil || len(snap.Secrets) == 0 {
		return nil
	}
	var out []string
	for _, name := range i.ver.describe.Secrets {
		if v, ok := snap.Secrets[name]; ok && len(v) >= minMaskedSecretLen {
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, func(a, b string) int { return len(b) - len(a) })
	return out
}

// maskString replaces every occurrence of a declared secret value in s with
// the redaction placeholder.
func maskString(s string, vals []string) string {
	for _, v := range vals {
		if strings.Contains(s, v) {
			s = strings.ReplaceAll(s, v, redacted)
		}
	}
	return s
}

// maskBytes is maskString for a raw body: it runs before the body is encoded
// (utf8 or base64) and truncated, so a value cannot hide in base64 or be cut
// in half at the limit.
func maskBytes(b []byte, vals []string) []byte {
	for _, v := range vals {
		if bytes.Contains(b, []byte(v)) {
			b = bytes.ReplaceAll(b, []byte(v), []byte(redacted))
		}
	}
	return b
}

// maskJSON masks the string keys and values of a JSON document. A document
// that holds no secret (in plain or JSON-escaped form) is returned untouched,
// byte for byte; otherwise it is walked, so a secret that happens to equal a
// piece of the JSON syntax cannot corrupt the document.
func maskJSON(data json.RawMessage, vals []string) json.RawMessage {
	if len(data) == 0 || len(vals) == 0 {
		return data
	}
	hit := false
	for _, v := range vals {
		esc, _ := json.Marshal(v)
		if bytes.Contains(data, []byte(v)) || bytes.Contains(data, esc[1:len(esc)-1]) {
			hit = true
			break
		}
	}
	if !hit {
		return data
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var doc any
	if err := dec.Decode(&doc); err != nil {
		// Not parseable (cannot happen for what the runner builds): drop it
		// rather than store a value that may carry a secret.
		return nil
	}
	out, err := json.Marshal(maskWalk(doc, vals))
	if err != nil {
		return nil
	}
	return out
}

func maskWalk(v any, vals []string) any {
	switch t := v.(type) {
	case string:
		return maskString(t, vals)
	case []any:
		for k := range t {
			t[k] = maskWalk(t[k], vals)
		}
		return t
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[maskString(k, vals)] = maskWalk(e, vals)
		}
		return out
	}
	return v
}
