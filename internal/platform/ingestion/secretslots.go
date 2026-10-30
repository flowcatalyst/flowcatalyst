package ingestion

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// SecretSlot is one secret setting of an entry: where it sits in the entry and
// the name of the secret it holds there. The settings form, the API and fcdev
// all address a secret by its path, and the platform alone turns the path into
// a name (SecretNameFor), so a person who enters a value never sees or types a
// name (docs/ingestion-design.md section 13).
type SecretSlot struct {
	// Path is the dotted path of the setting below the entry's root, such as
	// "password", "credential.secretAccessKey" or "auth.clientSecret". For an
	// extension kind it is the setting's name.
	Path string
	// Name is the secret the entry names there, "" when it names none.
	Name string
}

// SecretSlots lists the secret settings a built-in source names, by walking
// the typed model. An extension kind's secret settings are known only from its
// schema (CustomSecretSlots).
func SecretSlots(src Source) []SecretSlot {
	var slots []SecretSlot
	add := func(path string, ref SecretRef) { slots = append(slots, SecretSlot{Path: path, Name: ref.Name}) }
	switch s := src.(type) {
	case Sftp:
		add("credential", s.Credential)
	case S3:
		if s.Credential != nil {
			add("credential.secretAccessKey", s.Credential.SecretAccessKey)
		}
	case Sqs:
		if s.Credential != nil {
			add("credential.secretAccessKey", s.Credential.SecretAccessKey)
		}
	case RabbitMQ:
		add("password", s.Password)
	case DatabasePoll:
		add("password", s.Password)
	case HTTP:
		switch a := s.Auth.(type) {
		case BasicAuth:
			add("auth.password", a.Password)
		case BearerAuth:
			add("auth.token", a.Token)
		}
	case APIPoll:
		switch a := s.Auth.(type) {
		case APIBasic:
			add("auth.password", a.Password)
		case APIBearer:
			add("auth.token", a.Token)
		case APIKey:
			add("auth.value", a.Value)
		case ClientCredentials:
			add("auth.clientSecret", a.ClientSecret)
		case Login:
			add("auth.password", a.Password)
		}
	}
	return slots
}

// CustomSecretSlots lists the secret settings of an extension kind: the
// settings its schema types SECRET, with the name the entry's settings hold
// for each ("" when it holds none).
func CustomSecretSlots(fields []SettingField, settings map[string]any) []SecretSlot {
	var slots []SecretSlot
	for _, f := range fields {
		if f.Type != SettingSecret {
			continue
		}
		name, _ := settings[f.Name].(string)
		slots = append(slots, SecretSlot{Path: f.Name, Name: name})
	}
	return slots
}

var secretPathSegment = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9]*$`)

// ValidSecretPath reports whether path addresses a setting: dotted names of
// letters and digits, as a setting name is.
func ValidSecretPath(path string) bool {
	for seg := range strings.SplitSeq(path, ".") {
		if !secretPathSegment.MatchString(seg) {
			return false
		}
	}
	return true
}

// DeriveSecretName is the name the platform gives the secret of a setting that
// has none yet: the entry's key and the setting's path in kebab case, joined by
// '-' (key mix-positions, setting clientSecret: mix-positions-client-secret; a
// nested setting joins its segments: auth.clientSecret gives
// mix-positions-auth-client-secret). It is a name a definition may carry.
func DeriveSecretName(key, path string) (string, error) {
	if key == "" || !ValidSecretPath(path) {
		return "", fmt.Errorf("no secret name can be made for %q of entry %q", path, key)
	}
	parts := []string{key}
	for seg := range strings.SplitSeq(path, ".") {
		parts = append(parts, kebab(seg))
	}
	name := strings.Join(parts, "-")
	if !validSecretName(name) {
		return "", fmt.Errorf("the derived secret name %q is not a valid secret name (lowercase letters, digits and '-', at most 100): name the secret in the value instead", name)
	}
	return name, nil
}

// kebab writes a camelCase word in lowercase words joined by '-'.
func kebab(s string) string {
	var b strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) && i > 0 {
			b.WriteByte('-')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// SecretNameAt is the name of the secret an entry's value holds at path: a
// built-in kind's reference ({"name": ...}) or an extension's plain text. ""
// when it holds none there.
func SecretNameAt(entry map[string]any, path string) string {
	var cur any = entry
	for seg := range strings.SplitSeq(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur = m[seg]
	}
	switch x := cur.(type) {
	case string:
		return x
	case map[string]any:
		name, _ := x["name"].(string)
		return name
	}
	return ""
}

// SecretNameFor is the name of the secret of the setting at path: the one
// the value being written already names there, else the one the entry
// holds now (existing entries keep the names they have), else the derived one.
func SecretNameFor(next, current map[string]any, key, path string) (string, error) {
	if name := SecretNameAt(next, path); name != "" {
		return name, nil
	}
	if name := SecretNameAt(current, path); name != "" {
		return name, nil
	}
	return DeriveSecretName(key, path)
}

// BindSecretName makes entry (changed in place) name the secret at path: a
// reference for a built-in kind, plain text for an extension's (plain).
func BindSecretName(entry map[string]any, path, name string, plain bool) {
	segs := strings.Split(path, ".")
	cur := entry
	for _, seg := range segs[:len(segs)-1] {
		next, ok := cur[seg].(map[string]any)
		if !ok {
			next = map[string]any{}
			cur[seg] = next
		}
		cur = next
	}
	last := segs[len(segs)-1]
	if plain {
		cur[last] = name
	} else {
		cur[last] = map[string]any{"name": name}
	}
}

// ValidSecretName reports whether s is a name a definition may give a secret.
func ValidSecretName(s string) bool { return validSecretName(s) }
